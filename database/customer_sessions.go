package database

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	customerLoginTokenLifetime = 15 * time.Minute
	customerSessionLifetime    = 8 * time.Hour
	customerSessionIdleTimeout = 30 * time.Minute
)

// CreateCustomerLoginToken creates a single-use magic-link token only when the
// address already owns a paid order or a licence. The boolean is deliberately
// separate so HTTP handlers can return the same public response in both cases.
func CreateCustomerLoginToken(db *sql.DB, email string) (token string, eligible bool, err error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return "", false, errors.New("email requis")
	}

	eligible, err = CustomerLoginEligible(db, email)
	if err != nil {
		return "", false, fmt.Errorf("vérification compte client: %w", err)
	}
	if !eligible {
		return "", false, nil
	}
	identity, err := GetCustomerIdentityByEmail(db, email)
	if err != nil {
		return "", false, fmt.Errorf("identité client introuvable: %w", err)
	}

	token, err = generateSecureToken()
	if err != nil {
		return "", false, err
	}
	now := time.Now().UTC()
	tx, err := db.Begin()
	if err != nil {
		return "", false, err
	}
	defer tx.Rollback()

	if _, err = tx.Exec(`
		INSERT INTO customer_accounts (email, customer_id) VALUES (?, ?)
		ON CONFLICT(email) DO UPDATE SET customer_id = excluded.customer_id
	`, email, identity.ID); err != nil {
		return "", false, fmt.Errorf("création compte client: %w", err)
	}
	if _, err = tx.Exec(`
		UPDATE customer_login_tokens SET consumed_at = ?
		WHERE email = ? COLLATE NOCASE AND consumed_at IS NULL
	`, now.Format(time.RFC3339), email); err != nil {
		return "", false, fmt.Errorf("invalidation des anciens liens: %w", err)
	}
	if _, err = tx.Exec(`
		INSERT INTO customer_login_tokens (token_hash, email, customer_id, expires_at)
		VALUES (?, ?, ?, ?)
	`, hashSessionToken(token), email, identity.ID, now.Add(customerLoginTokenLifetime).Format(time.RFC3339)); err != nil {
		return "", false, fmt.Errorf("création lien de connexion: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return "", false, err
	}
	return token, true, nil
}

func CustomerLoginEligible(db *sql.DB, email string) (bool, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	var exists int
	err := db.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM customer_users u
			WHERE LOWER(u.email) = ? AND (
				EXISTS(SELECT 1 FROM licences l WHERE l.customer_id = u.customer_id)
				OR EXISTS(SELECT 1 FROM orders o WHERE o.customer_id = u.customer_id AND o.status IN ('paid', 'processing'))
				OR EXISTS(SELECT 1 FROM trial_applications t WHERE t.customer_id = u.customer_id AND t.verified_at IS NOT NULL)
				OR EXISTS(SELECT 1 FROM team_members m WHERE m.member_customer_id=u.customer_id AND m.email=u.email COLLATE NOCASE AND m.status='active')
			)
		)
	`, email).Scan(&exists)
	return exists == 1, err
}

// ConsumeCustomerLoginToken atomically burns a magic link and returns a new
// opaque session token. Only hashes are persisted.
func ConsumeCustomerLoginToken(db *sql.DB, token string, mfaCode ...string) (sessionToken, email string, err error) {
	if strings.TrimSpace(token) == "" {
		return "", "", errors.New("lien invalide ou expiré")
	}
	sessionToken, err = generateSecureToken()
	if err != nil {
		return "", "", err
	}
	now := time.Now().UTC()
	tx, err := db.Begin()
	if err != nil {
		return "", "", err
	}
	defer tx.Rollback()

	var expiresRaw any
	var customerID int64
	if err = tx.QueryRow(`
		SELECT email, customer_id, expires_at
		FROM customer_login_tokens
		WHERE token_hash = ? AND consumed_at IS NULL
	`, hashSessionToken(token)).Scan(&email, &customerID, &expiresRaw); err != nil {
		return "", "", errors.New("lien invalide ou expiré")
	}
	expiresAt, parseErr := ParseSQLiteTime(expiresRaw)
	if parseErr != nil || !expiresAt.After(now) {
		return "", "", errors.New("lien invalide ou expiré")
	}
	code := ""
	if len(mfaCode) > 0 {
		code = mfaCode[0]
	}
	if err = verifyEmailTokenMFATx(tx, token, email, code, now); err != nil {
		return "", "", err
	}
	result, err := tx.Exec(`
		UPDATE customer_login_tokens SET consumed_at = ?
		WHERE token_hash = ? AND consumed_at IS NULL
	`, now.Format(time.RFC3339), hashSessionToken(token))
	if err != nil {
		return "", "", err
	}
	rows, err := result.RowsAffected()
	if err != nil || rows != 1 {
		return "", "", errors.New("lien invalide ou expiré")
	}
	if _, err = tx.Exec(`
		INSERT INTO customer_sessions (token_hash, email, customer_id, expires_at, last_used_at)
		VALUES (?, ?, ?, ?, ?)
	`, hashSessionToken(sessionToken), email, customerID, now.Add(customerSessionLifetime).Format(time.RFC3339), now.Format(time.RFC3339)); err != nil {
		return "", "", fmt.Errorf("création session client: %w", err)
	}
	if _, err = tx.Exec(`UPDATE customer_accounts SET last_login_at = ? WHERE email = ?`, now.Format(time.RFC3339), email); err != nil {
		return "", "", err
	}
	if err = tx.Commit(); err != nil {
		return "", "", err
	}
	return sessionToken, email, nil
}

// ValidateCustomerSession validates absolute and idle expiration and returns
// the verified billing e-mail used to scope every customer query.
func ValidateCustomerSession(db *sql.DB, token string) (string, error) {
	identity, err := ValidateCustomerSessionIdentity(db, token)
	if err != nil {
		return "", err
	}
	return identity.Email, nil
}

func ValidateCustomerSessionIdentity(db *sql.DB, token string) (*CustomerIdentity, error) {
	if strings.TrimSpace(token) == "" {
		return nil, errors.New("session client invalide")
	}
	tokenHash := hashSessionToken(token)
	var email string
	var customerID int64
	var expiresRaw, lastUsedRaw any
	if err := db.QueryRow(`
		SELECT email, customer_id, expires_at, last_used_at
		FROM customer_sessions WHERE token_hash = ?
	`, tokenHash).Scan(&email, &customerID, &expiresRaw, &lastUsedRaw); err != nil {
		return nil, errors.New("session client invalide")
	}
	expiresAt, expiresErr := ParseSQLiteTime(expiresRaw)
	lastUsedAt, usedErr := ParseSQLiteTime(lastUsedRaw)
	now := time.Now().UTC()
	if expiresErr != nil || usedErr != nil || !expiresAt.After(now) || now.Sub(lastUsedAt) > customerSessionIdleTimeout {
		_, _ = db.Exec(`DELETE FROM customer_sessions WHERE token_hash = ?`, tokenHash)
		return nil, errors.New("session client expirée")
	}
	if _, err := db.Exec(`UPDATE customer_sessions SET last_used_at = ? WHERE token_hash = ?`, now.Format(time.RFC3339), tokenHash); err != nil {
		return nil, err
	}
	identity, err := GetCustomerIdentityByID(db, customerID, email)
	if err != nil {
		return nil, errors.New("identité de session client invalide")
	}
	return identity, nil
}

func DeleteCustomerSession(db *sql.DB, token string) error {
	_, err := db.Exec(`DELETE FROM customer_sessions WHERE token_hash = ?`, hashSessionToken(token))
	return err
}

func SetCustomerRenewalReminders(db *sql.DB, email string, enabled bool) error {
	identity, err := GetCustomerIdentityByEmail(db, email)
	if err != nil {
		return errors.New("compte client introuvable")
	}
	return SetCustomerRenewalRemindersByID(db, identity.ID, enabled)
}

func SetCustomerRenewalRemindersByID(db *sql.DB, customerID int64, enabled bool) error {
	value := 0
	if enabled {
		value = 1
	}
	result, err := db.Exec(`UPDATE customers SET renewal_reminders_enabled = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, value, customerID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return errors.New("compte client introuvable")
	}
	return nil
}

func CustomerRenewalRemindersEnabled(db *sql.DB, email string) (bool, error) {
	identity, err := GetCustomerIdentityByEmail(db, email)
	if err != nil {
		return false, err
	}
	return CustomerRenewalRemindersEnabledByID(db, identity.ID)
}

func CustomerRenewalRemindersEnabledByID(db *sql.DB, customerID int64) (bool, error) {
	var enabled bool
	err := db.QueryRow(`SELECT renewal_reminders_enabled FROM customers WHERE id = ?`, customerID).Scan(&enabled)
	return enabled, err
}

func hashPassword(password string) (string, error) {
	password = strings.TrimSpace(password)
	if len(password) < 8 {
		return "", errors.New("le mot de passe doit contenir au moins 8 caractères")
	}
	if len(password) > 72 {
		return "", errors.New("le mot de passe ne peut pas dépasser 72 caractères")
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("erreur de hachage du mot de passe: %w", err)
	}
	return string(hashed), nil
}

func checkPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// SetCustomerPassword hashes and sets a customer's password directly.
func SetCustomerPassword(db *sql.DB, email, plainPassword string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return errors.New("adresse e-mail requise")
	}
	identity, err := GetCustomerIdentityByEmail(db, email)
	if err != nil {
		identity, err = EnsureCustomer(db, email, "business", "")
		if err != nil {
			return fmt.Errorf("compte client introuvable: %w", err)
		}
	}
	pwHash, err := hashPassword(plainPassword)
	if err != nil {
		return err
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.Exec(`
		INSERT INTO customer_accounts (email, customer_id, password_hash)
		VALUES (?, ?, ?)
		ON CONFLICT(email) DO UPDATE SET password_hash = excluded.password_hash, customer_id = excluded.customer_id
	`, email, identity.ID, pwHash)
	if err != nil {
		return err
	}
	if err = revokeAccountSessionsTx(tx, email); err != nil {
		return err
	}
	return tx.Commit()
}

// ValidateCustomerPassword verifies email and password, creating a customer session on success.
// If the account has no password yet, hasPassword will be false with an informative error.
func ValidateCustomerPassword(db *sql.DB, email, plainPassword string) (sessionToken string, customerID string, hasPassword bool, err error) {
	res, err := ValidateCustomerPasswordWith2FA(db, email, plainPassword)
	if res == nil {
		return "", "", false, err
	}
	if res.Requires2FA {
		return "", res.CustomerID, res.HasPassword, ErrMFARequired
	}
	return res.SessionToken, res.CustomerID, res.HasPassword, err
}

// SetCustomerPasswordWithToken verifies a reset token, saves the new password, burns the token, and creates a session.
func SetCustomerPasswordWithToken(db *sql.DB, token, plainPassword string) (sessionToken string, email string, customerID string, err error) {
	return ResetCustomerPassword(db, token, plainPassword, "", "", "", nil)
}

// ResetCustomerPassword keeps email verification, the existing second factor,
// optional initial enrollment, password replacement and session revocation atomic.
func ResetCustomerPassword(db *sql.DB, token, plainPassword, currentCode, secret, confirmationCode string, recoveryCodes []string) (sessionToken string, email string, customerID string, err error) {
	if strings.TrimSpace(token) == "" {
		return "", "", "", errors.New("lien invalide ou expiré")
	}
	pwHash, err := hashPassword(plainPassword)
	if err != nil {
		return "", "", "", err
	}

	now := time.Now().UTC()
	tx, err := db.Begin()
	if err != nil {
		return "", "", "", err
	}
	defer tx.Rollback()

	var expiresRaw any
	var custID int64
	if err = tx.QueryRow(`
		SELECT email, customer_id, expires_at
		FROM customer_login_tokens
		WHERE token_hash = ? AND consumed_at IS NULL
	`, hashSessionToken(token)).Scan(&email, &custID, &expiresRaw); err != nil {
		return "", "", "", errors.New("lien invalide ou expiré")
	}
	expiresAt, parseErr := ParseSQLiteTime(expiresRaw)
	if parseErr != nil || !expiresAt.After(now) {
		return "", "", "", errors.New("lien invalide ou expiré")
	}

	if err = verifyEmailTokenMFATx(tx, token, email, currentCode, now); err != nil {
		return "", "", "", err
	}
	if secret != "" || confirmationCode != "" || len(recoveryCodes) > 0 {
		if err = enableCustomerTOTPTx(tx, email, secret, confirmationCode, recoveryCodes, now); err != nil {
			return "", "", "", err
		}
	}
	result, err := tx.Exec(`
		UPDATE customer_login_tokens SET consumed_at = ?
		WHERE token_hash = ? AND consumed_at IS NULL
	`, now.Format(time.RFC3339), hashSessionToken(token))
	if err != nil {
		return "", "", "", err
	}
	rows, err := result.RowsAffected()
	if err != nil || rows != 1 {
		return "", "", "", errors.New("lien invalide ou expiré")
	}

	// Update password in customer_accounts
	if _, err = tx.Exec(`
		INSERT INTO customer_accounts (email, customer_id, password_hash, last_login_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(email) DO UPDATE SET password_hash = excluded.password_hash, customer_id = excluded.customer_id, last_login_at = excluded.last_login_at
	`, email, custID, pwHash, now.Format(time.RFC3339)); err != nil {
		return "", "", "", fmt.Errorf("enregistrement mot de passe: %w", err)
	}

	if err = revokeAccountSessionsTx(tx, email); err != nil {
		return "", "", "", err
	}
	// Create session
	sessionToken, err = generateSecureToken()
	if err != nil {
		return "", "", "", err
	}
	if _, err = tx.Exec(`
		INSERT INTO customer_sessions (token_hash, email, customer_id, expires_at, last_used_at)
		VALUES (?, ?, ?, ?, ?)
	`, hashSessionToken(sessionToken), email, custID, now.Add(customerSessionLifetime).Format(time.RFC3339), now.Format(time.RFC3339)); err != nil {
		return "", "", "", fmt.Errorf("création session client: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return "", "", "", err
	}

	identity, err := GetCustomerIdentityByID(db, custID, email)
	if err != nil {
		return sessionToken, email, "", nil
	}
	return sessionToken, email, identity.PublicID, nil
}

// ChangeCustomerPassword verifies the current password and updates to a new password.
func ChangeCustomerPassword(db *sql.DB, email, oldPassword, newPassword string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	newHash, err := hashPassword(newPassword)
	if err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var oldHash sql.NullString
	if err = tx.QueryRow("SELECT password_hash FROM customer_accounts WHERE email=? COLLATE NOCASE", email).Scan(&oldHash); err != nil || !oldHash.Valid || !checkPassword(oldHash.String, oldPassword) {
		return errors.New("mot de passe actuel incorrect")
	}
	if _, err = tx.Exec("UPDATE customer_accounts SET password_hash=? WHERE email=? COLLATE NOCASE", newHash, email); err != nil {
		return err
	}
	if err = revokeAccountSessionsTx(tx, email); err != nil {
		return err
	}
	return tx.Commit()
}

// CreateCustomerGoogleSession is the legacy session-only interface. It refuses
// MFA accounts; callers that support challenges use AuthenticateCustomerGoogle.
func CreateCustomerGoogleSession(db *sql.DB, email string) (sessionToken, customerEmail, customerPublicID string, err error) {
	result, err := AuthenticateCustomerGoogle(db, email)
	if err != nil {
		return "", "", "", err
	}
	if result.Requires2FA {
		return "", "", "", ErrMFARequired
	}
	return result.SessionToken, result.Email, result.CustomerID, nil
}

// AuthenticateCustomerGoogle consumes an already verified Google identity. Google
// authenticates the first factor only; local MFA is still required when enabled.
func AuthenticateCustomerGoogle(db *sql.DB, email string) (*CustomerPasswordAuthResult, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return nil, errors.New("adresse e-mail requise")
	}

	eligible, err := CustomerLoginEligible(db, email)
	if err != nil {
		return nil, fmt.Errorf("vérification éligibilité: %w", err)
	}
	if !eligible {
		return nil, errors.New("aucun compte client associé à cette adresse e-mail")
	}

	identity, err := GetCustomerIdentityByEmail(db, email)
	if err != nil {
		return nil, fmt.Errorf("identité client introuvable: %w", err)
	}

	now := time.Now().UTC()
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	result := &CustomerPasswordAuthResult{Email: email, CustomerID: identity.PublicID}
	enabled, _, _, _, err := getMFAConfigTx(tx, "", email)
	if err != nil {
		return nil, err
	}
	if enabled {
		token, err := generateSecureToken()
		if err != nil {
			return nil, err
		}
		if _, err = tx.Exec(`DELETE FROM customer_2fa_challenges WHERE email=? COLLATE NOCASE`, email); err != nil {
			return nil, err
		}
		// An email to the same Google account is not an independent second factor.
		if _, err = tx.Exec(`INSERT INTO customer_2fa_challenges(challenge_token,email,customer_id,created_at,expires_at,attempts,email_code_allowed) VALUES (?,?,?,?,?,0,0)`, hashSessionToken(token), email, identity.ID, now.Format(time.RFC3339), now.Add(customer2FAChallengeLifetime).Format(time.RFC3339)); err != nil {
			return nil, err
		}
		result.Requires2FA, result.ChallengeToken = true, token
	} else {
		result.SessionToken, err = createCustomerSessionTx(tx, email, identity.ID, now)
		if err != nil {
			return nil, err
		}
	}

	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return result, nil
}
