package database

import (
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	customer2FAChallengeLifetime  = 5 * time.Minute
	customer2FAEmailCodeLifetime  = 15 * time.Minute
	customerTrustedDeviceLifetime = 30 * 24 * time.Hour
	max2FAAttempts                = 5
)

type CustomerPasswordAuthResult struct {
	SessionToken   string `json:"session_token,omitempty"`
	CustomerID     string `json:"customer_id,omitempty"`
	Email          string `json:"email,omitempty"`
	Requires2FA    bool   `json:"requires_2fa"`
	ChallengeToken string `json:"challenge_token,omitempty"`
	HasPassword    bool   `json:"has_password"`
}

// ValidateCustomerPasswordWith2FA checks credentials. If 2FA is active on the account,
// it issues a short-lived challenge token instead of a session token unless a valid trusted device token is provided.
func ValidateCustomerPasswordWith2FA(db *sql.DB, email, plainPassword string, deviceToken ...string) (*CustomerPasswordAuthResult, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	eligible, err := CustomerLoginEligible(db, email)
	if err != nil || !eligible {
		return nil, errors.New("aucun compte client actif pour cette adresse")
	}
	identity, err := GetCustomerIdentityByEmail(db, email)
	if err != nil {
		return nil, err
	}
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var pwHash sql.NullString
	if err = tx.QueryRow("SELECT password_hash FROM customer_accounts WHERE email=? COLLATE NOCASE", email).Scan(&pwHash); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	res := &CustomerPasswordAuthResult{Email: email, CustomerID: identity.PublicID, HasPassword: pwHash.Valid && pwHash.String != ""}
	if !res.HasPassword {
		return res, errors.New("aucun mot de passe n'a été défini pour ce compte")
	}
	if !checkPassword(pwHash.String, plainPassword) {
		return res, errors.New("mot de passe incorrect")
	}
	enabled, _, _, _, err := getMFAConfigTx(tx, "", email)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if enabled {
		// Check if a valid trusted device token was provided
		if len(deviceToken) > 0 && strings.TrimSpace(deviceToken[0]) != "" {
			dT := strings.TrimSpace(deviceToken[0])
			var exists int
			err = tx.QueryRow(`
				SELECT 1 FROM customer_trusted_devices
				WHERE device_token_hash = ? AND email = ? COLLATE NOCASE AND expires_at > ?
			`, hashSessionToken(dT), email, now.Format(time.RFC3339)).Scan(&exists)
			if err == nil && exists == 1 {
				// Trusted device verified! Update last_used_at and issue session token directly without 2FA
				_, _ = tx.Exec(`UPDATE customer_trusted_devices SET last_used_at = ? WHERE device_token_hash = ?`, now.Format(time.RFC3339), hashSessionToken(dT))
				res.Requires2FA = false
				res.SessionToken, err = createCustomerSessionTx(tx, email, identity.ID, now)
				if err != nil {
					return nil, err
				}
				if err = tx.Commit(); err != nil {
					return nil, err
				}
				return res, nil
			}
		}

		token, e := generateSecureToken()
		if e != nil {
			return nil, e
		}
		if _, err = tx.Exec("DELETE FROM customer_2fa_challenges WHERE email=? COLLATE NOCASE", email); err != nil {
			return nil, err
		}
		if _, err = tx.Exec("INSERT INTO customer_2fa_challenges(challenge_token,email,customer_id,created_at,expires_at,attempts) VALUES (?,?,?,?,?,0)", hashSessionToken(token), email, identity.ID, now.Format(time.RFC3339), now.Add(customer2FAChallengeLifetime).Format(time.RFC3339)); err != nil {
			return nil, err
		}
		res.Requires2FA = true
		res.ChallengeToken = token
	} else {
		res.SessionToken, err = createCustomerSessionTx(tx, email, identity.ID, now)
		if err != nil {
			return nil, err
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return res, nil
}

// GenerateRandom6DigitCode generates a cryptographically secure 6-digit numeric code.
func GenerateRandom6DigitCode() (string, error) {
	var b [3]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	num := (int(b[0])<<16|int(b[1])<<8|int(b[2]))%900000 + 100000
	return fmt.Sprintf("%06d", num), nil
}

// SendCustomer2FAEmailCode generates a single-use 6-digit numeric code and attaches it to the challenge.
func SendCustomer2FAEmailCode(db *sql.DB, challengeToken string) (code string, email string, err error) {
	challengeToken = strings.TrimSpace(challengeToken)
	if challengeToken == "" {
		return "", "", errors.New("jeton de challenge requis")
	}
	now := time.Now().UTC()
	var emailVal string
	var expiresRaw any
	var attempts int
	var lastSentRaw sql.NullString
	var emailCodeAllowed int

	err = db.QueryRow(`
		SELECT email, expires_at, attempts, email_code_sent_at, email_code_allowed
		FROM customer_2fa_challenges
		WHERE challenge_token = ?
	`, hashSessionToken(challengeToken)).Scan(&emailVal, &expiresRaw, &attempts, &lastSentRaw, &emailCodeAllowed)
	if err != nil {
		return "", "", errors.New("session de vérification invalide ou expirée")
	}
	if emailCodeAllowed != 1 {
		return "", "", errors.New("utilisez votre application d'authentification ou un code de secours après une connexion Google")
	}
	expiry, e := ParseSQLiteTime(expiresRaw)
	if e != nil || !expiry.After(now) || attempts >= max2FAAttempts {
		return "", "", errors.New("session de vérification expirée ou nombre maximal de tentatives dépassé")
	}

	// Rate limit: 30 seconds cooldown between code requests
	if lastSentRaw.Valid && lastSentRaw.String != "" {
		if lastSent, parseErr := ParseSQLiteTime(lastSentRaw.String); parseErr == nil {
			if now.Sub(lastSent) < 30*time.Second {
				return "", "", errors.New("veuillez patienter 30 secondes avant de demander un nouveau code")
			}
		}
	}

	code, err = GenerateRandom6DigitCode()
	if err != nil {
		return "", "", err
	}

	codeHash := hashSessionToken(code)
	codeExpires := now.Add(customer2FAEmailCodeLifetime)
	if expiry.Before(codeExpires) {
		expiry = codeExpires
	}

	_, err = db.Exec(`
		UPDATE customer_2fa_challenges
		SET email_code_hash = ?, email_code_expires_at = ?, email_code_sent_at = ?, expires_at = ?
		WHERE challenge_token = ?
	`, codeHash, codeExpires.Format(time.RFC3339), now.Format(time.RFC3339), expiry.Format(time.RFC3339), hashSessionToken(challengeToken))
	if err != nil {
		return "", "", err
	}

	return code, emailVal, nil
}

// CreateCustomer2FAChallenge creates a single-use, 5-minute challenge token for 2FA validation.
func CreateCustomer2FAChallenge(db *sql.DB, email string, customerID int64) (string, error) {
	token, err := generateSecureToken()
	if err != nil {
		return "", err
	}
	now := time.Now().UTC()
	expiresAt := now.Add(customer2FAChallengeLifetime)

	// Clean up any stale challenges for this email
	_, _ = db.Exec(`DELETE FROM customer_2fa_challenges WHERE email = ? COLLATE NOCASE`, email)

	_, err = db.Exec(`
		INSERT INTO customer_2fa_challenges (challenge_token, email, customer_id, created_at, expires_at, attempts)
		VALUES (?, ?, ?, ?, ?, 0)
	`, hashSessionToken(token), email, customerID, now.Format(time.RFC3339), expiresAt.Format(time.RFC3339))
	if err != nil {
		return "", err
	}
	return token, nil
}

// VerifyCustomer2FAChallengeWithDevice validates the code (TOTP, recovery or email code) against the challenge.
// If rememberDevice is true, it issues a 30-day device token.
func VerifyCustomer2FAChallengeWithDevice(db *sql.DB, challengeToken, code string, rememberDevice bool, deviceName ...string) (sessionToken string, customerID string, email string, deviceToken string, err error) {
	tx, err := db.Begin()
	if err != nil {
		return "", "", "", "", err
	}
	defer tx.Rollback()
	now := time.Now().UTC()
	email, _, cID, err := verifyChallengeTx(tx, "customer_2fa_challenges", strings.TrimSpace(challengeToken), code, now)
	if err != nil {
		return "", "", "", "", err
	}
	sessionToken, err = createCustomerSessionTx(tx, email, cID, now)
	if err != nil {
		return "", "", "", "", err
	}

	if rememberDevice {
		rawDeviceToken, genErr := generateSecureToken()
		if genErr == nil {
			devName := "Navigateur"
			if len(deviceName) > 0 && strings.TrimSpace(deviceName[0]) != "" {
				devName = strings.TrimSpace(deviceName[0])
			}
			deviceToken = rawDeviceToken
			deviceExpires := now.Add(customerTrustedDeviceLifetime)
			_, _ = tx.Exec(`
				INSERT INTO customer_trusted_devices(device_token_hash, email, customer_id, device_name, created_at, expires_at, last_used_at)
				VALUES (?, ?, ?, ?, ?, ?, ?)
			`, hashSessionToken(rawDeviceToken), email, cID, devName, now.Format(time.RFC3339), deviceExpires.Format(time.RFC3339), now.Format(time.RFC3339))
		}
	}

	if err = tx.Commit(); err != nil {
		return "", "", "", "", err
	}
	identity, err := GetCustomerIdentityByID(db, cID, email)
	if err != nil {
		return "", "", "", "", err
	}
	return sessionToken, identity.PublicID, email, deviceToken, nil
}

// VerifyCustomer2FAChallenge validates the code and issues a customer session.
func VerifyCustomer2FAChallenge(db *sql.DB, challengeToken, code string) (sessionToken string, customerID string, email string, err error) {
	sessionToken, customerID, email, _, err = VerifyCustomer2FAChallengeWithDevice(db, challengeToken, code, false)
	return sessionToken, customerID, email, err
}

// RevokeCustomerTrustedDevices removes all trusted devices for the given email account.
func RevokeCustomerTrustedDevices(db *sql.DB, email string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	if db == nil || email == "" {
		return nil
	}
	_, err := db.Exec(`DELETE FROM customer_trusted_devices WHERE email = ? COLLATE NOCASE`, email)
	return err
}

// SetupCustomerTOTP initiates the 2FA enrollment process by generating a new secret and recovery codes.
func SetupCustomerTOTP(db *sql.DB, email string) (secret string, otpauthURL string, recoveryCodes []string, err error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return "", "", nil, errors.New("adresse e-mail requise")
	}
	secret, err = GenerateTOTPSecret()
	if err != nil {
		return "", "", nil, err
	}
	recoveryCodes, hashes, err := GenerateRecoveryCodes(recoveryCount)
	if err != nil {
		return "", "", nil, err
	}
	encoded, err := json.Marshal(hashes)
	if err != nil {
		return "", "", nil, err
	}
	tx, err := db.Begin()
	if err != nil {
		return "", "", nil, err
	}
	defer tx.Rollback()
	enabled, _, _, _, err := getMFAConfigTx(tx, "", email)
	if err != nil {
		return "", "", nil, err
	}
	if enabled {
		return "", "", nil, errors.New("la double authentification est déjà activée ; désactivez-la avec votre facteur actuel avant de la remplacer")
	}
	_, err = tx.Exec("INSERT INTO totp_enrollments(email,secret,recovery_codes,expires_at) VALUES (?,?,?,?) ON CONFLICT(email) DO UPDATE SET secret=excluded.secret,recovery_codes=excluded.recovery_codes,expires_at=excluded.expires_at", email, secret, string(encoded), time.Now().UTC().Add(10*time.Minute).Format(time.RFC3339))
	if err != nil {
		return "", "", nil, err
	}
	if err = tx.Commit(); err != nil {
		return "", "", nil, err
	}
	return secret, GenerateTOTPURL(email, secret), recoveryCodes, nil
}

// EnableCustomerTOTP verifies the user's initial confirmation code and activates 2FA.
func EnableCustomerTOTP(db *sql.DB, email, secret, confirmationCode string, recoveryCodes []string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = enableCustomerTOTPTx(tx, email, secret, confirmationCode, recoveryCodes, time.Now().UTC()); err != nil {
		return err
	}
	if err = revokeAccountSessionsTx(tx, email); err != nil {
		return err
	}
	return tx.Commit()
}

// DisableCustomerTOTP requires both the account password and an existing factor.
func DisableCustomerTOTP(db *sql.DB, email, password string, mfaCode ...string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	code := ""
	if len(mfaCode) > 0 {
		code = mfaCode[0]
	}
	if err = verifyAccountPasswordTx(tx, email, password); err != nil {
		return err
	}
	if err = consumeMFACodeTx(tx, "", email, code, time.Now().UTC()); err != nil {
		return err
	}
	if _, err = tx.Exec("UPDATE customer_accounts SET totp_enabled=0,totp_secret=NULL,totp_recovery_codes=NULL,totp_confirmed_at=NULL WHERE email=? COLLATE NOCASE", email); err != nil {
		return err
	}
	if _, err = tx.Exec("DELETE FROM totp_enrollments WHERE email=? COLLATE NOCASE", email); err != nil {
		return err
	}
	if err = revokeAccountSessionsTx(tx, email); err != nil {
		return err
	}
	return tx.Commit()
}

// GetCustomerTOTPStatus checks if 2FA is active and how many recovery codes remain.
func GetCustomerTOTPStatus(db *sql.DB, email string) (enabled bool, confirmedAt string, remainingCodes int, err error) {
	email = strings.ToLower(strings.TrimSpace(email))
	var totpEnabled int
	var confAt, recCodes sql.NullString

	err = db.QueryRow(`
		SELECT COALESCE(totp_enabled, 0), totp_confirmed_at, totp_recovery_codes
		FROM customer_accounts WHERE email = ? COLLATE NOCASE
	`, email).Scan(&totpEnabled, &confAt, &recCodes)
	if err != nil {
		return false, "", 0, err
	}

	if totpEnabled != 1 {
		return false, "", 0, nil
	}

	remaining := 0
	if recCodes.Valid {
		remaining = CountRemainingRecoveryCodes(recCodes.String)
	}

	cAt := ""
	if confAt.Valid {
		cAt = confAt.String
	}

	return true, cAt, remaining, nil
}

// RegenerateCustomerRecoveryCodes requires the password and an existing factor.
func RegenerateCustomerRecoveryCodes(db *sql.DB, email, password string, mfaCode ...string) ([]string, error) {
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	code := ""
	if len(mfaCode) > 0 {
		code = mfaCode[0]
	}
	if err = verifyAccountPasswordTx(tx, email, password); err != nil {
		return nil, err
	}
	if err = consumeMFACodeTx(tx, "", email, code, time.Now().UTC()); err != nil {
		return nil, err
	}
	plain, hashes, err := GenerateRecoveryCodes(recoveryCount)
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(hashes)
	if err != nil {
		return nil, err
	}
	if _, err = tx.Exec("UPDATE customer_accounts SET totp_recovery_codes=? WHERE email=? COLLATE NOCASE", string(encoded), email); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return plain, nil
}
