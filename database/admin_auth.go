package database

import (
	"crypto/subtle"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// AdminAuthResult represents the result of an admin authentication attempt.
type AdminAuthResult struct {
	SessionToken   string `json:"session_token,omitempty"`
	LicenseID      string `json:"license_id"`
	Email          string `json:"email"`
	Role           string `json:"role"`
	Requires2FA    bool   `json:"requires_2fa"`
	ChallengeToken string `json:"challenge_token,omitempty"`
}

// FindActiveAdminLicense locates an active, non-expired license with 'ADMIN' in notes
// belonging to the given email directly or via its customer relation.
func FindActiveAdminLicense(db *sql.DB, email string) (*License, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return nil, errors.New("adresse e-mail requise")
	}

	query := `
		SELECT id, license_id, email, license_key, status, created_at, expires_at,
		       max_connections, current_connections, last_connection_at, notes,
		       revoked_at, revoke_reason,
		       key_hint
		FROM licences
		WHERE (LOWER(email) = ? OR customer_id IN (
			SELECT id FROM customers WHERE LOWER(billing_email) = ?
			UNION
			SELECT customer_id FROM customer_accounts WHERE LOWER(email) = ?
		))
		  AND status = 'active'
		  AND expires_at > CURRENT_TIMESTAMP
		  AND UPPER(TRIM(COALESCE(notes, ''))) = 'ADMIN'
		ORDER BY expires_at DESC
		LIMIT 1
	`
	row := db.QueryRow(query, email, email, email)
	lic, err := scanLicense(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("aucune licence administrateur active associée à cet e-mail")
		}
		return nil, err
	}
	return lic, nil
}

// ValidateAdminEmailPassword checks email and account password, verifies that an active
// ADMIN license is held, and returns a session or a 2FA challenge.
func ValidateAdminEmailPassword(db *sql.DB, email, plainPassword string) (*AdminAuthResult, *License, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	lic, err := FindActiveAdminLicense(db, email)
	if err != nil {
		return nil, nil, err
	}
	if plainPassword == "" {
		return nil, nil, errors.New("mot de passe requis")
	}
	auth, lic, err := completeTechnicianLogin(db, lic, email, "", plainPassword)
	if err != nil {
		return nil, nil, err
	}
	return &AdminAuthResult{SessionToken: auth.SessionToken, LicenseID: lic.LicenseID, Email: email, Role: "admin", Requires2FA: auth.Requires2FA, ChallengeToken: auth.ChallengeToken}, lic, nil
}

// VerifyAdmin2FAChallenge validates the 6-digit TOTP code or recovery code against a challenge,
// and on success issues the final admin session token.
func VerifyAdmin2FAChallenge(db *sql.DB, challengeToken, code string) (*AdminAuthResult, *License, error) {
	token, lic, err := VerifyTechnician2FAChallenge(db, challengeToken, code)
	if err != nil {
		return nil, nil, err
	}
	member, memberErr := TeamMemberForSession(db, token)
	if memberErr != nil || member != nil || !strings.EqualFold(strings.TrimSpace(lic.Notes), "ADMIN") {
		_ = DeleteTechnicianSession(db, token)
		return nil, nil, errors.New("cette licence ne dispose pas des droits administrateur")
	}
	return &AdminAuthResult{SessionToken: token, LicenseID: lic.LicenseID, Email: lic.Email, Role: "admin"}, lic, nil
}

// SetAdminPasswordWithLicense sets an initial password only. Existing accounts
// must use password recovery and cannot bypass MFA using a licence key.
func SetAdminPasswordWithLicense(db *sql.DB, licenseID, licenseKey, newPassword string) (string, error) {
	licenseID = strings.ToUpper(strings.TrimSpace(licenseID))
	licenseKey = strings.TrimSpace(licenseKey)
	newPassword = strings.TrimSpace(newPassword)

	if licenseID == "" || licenseKey == "" {
		return "", errors.New("identifiant et clé de licence requis")
	}
	if len(newPassword) < 8 {
		return "", errors.New("le mot de passe doit comporter au moins 8 caractères")
	}

	lic, err := ValidateLicenseCredentials(db, licenseID, licenseKey)
	if err != nil {
		return "", errors.New("identifiants de licence invalides")
	}

	if !strings.EqualFold(strings.TrimSpace(lic.Notes), "ADMIN") {
		return "", errors.New("cette licence ne dispose pas des droits administrateur")
	}

	email := strings.ToLower(strings.TrimSpace(lic.Email))
	if email == "" {
		return "", errors.New("aucun e-mail associé à cette licence")
	}

	pwHash, err := hashPassword(newPassword)
	if err != nil {
		return "", fmt.Errorf("hachage du mot de passe: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	tx, err := db.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	fresh, err := activeLicenseTx(tx, licenseID)
	if err != nil || !strings.EqualFold(strings.TrimSpace(fresh.Notes), "ADMIN") ||
		!strings.EqualFold(fresh.Email, email) || subtle.ConstantTimeCompare([]byte(fresh.LicenseKey), []byte(HashLicenseKey(licenseKey))) != 1 {
		return "", errors.New("identifiants administrateur invalides")
	}
	enabled, _, _, _, err := getMFAConfigTx(tx, licenseID, email)
	if err != nil {
		return "", err
	}
	if enabled {
		return "", errors.New("double authentification active ; utilisez la récupération sécurisée du compte")
	}

	var cID sql.NullInt64
	if err = tx.QueryRow(`SELECT id FROM customers WHERE LOWER(billing_email) = ?`, email).Scan(&cID); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}

	result, err := tx.Exec(`
		INSERT INTO customer_accounts (email, customer_id, password_hash, created_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(email) DO UPDATE SET password_hash = excluded.password_hash
        WHERE COALESCE(customer_accounts.password_hash, '') = '' AND COALESCE(customer_accounts.totp_enabled,0) = 0
	`, email, cID, pwHash, now)
	if err != nil {
		return "", fmt.Errorf("enregistrement mot de passe: %w", err)
	}

	n, err := result.RowsAffected()
	if err != nil || n != 1 {
		return "", errors.New("mot de passe déjà configuré ; utilisez la récupération sécurisée du compte")
	}
	if err = tx.Commit(); err != nil {
		return "", err
	}
	return email, nil
}
