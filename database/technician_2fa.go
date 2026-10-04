package database

import (
	"crypto/subtle"
	"database/sql"
	"errors"
	"strings"
	"time"
)

const (
	technician2FAChallengeLifetime = 5 * time.Minute
)

// TechnicianAuthResult contains the outcome of a technician login attempt.
type TechnicianAuthResult struct {
	SessionToken   string    `json:"session_token,omitempty"`
	LicenseID      string    `json:"license_id"`
	Email          string    `json:"email"`
	Requires2FA    bool      `json:"requires_2fa"`
	ChallengeToken string    `json:"challenge_token,omitempty"`
	ExpiresAt      time.Time `json:"expires_at"`
}

// GetTechnician2FAConfig retrieves the effective TOTP secret and recovery codes
// for a license (checking the associated customer account first, then the license).
func GetTechnician2FAConfig(db *sql.DB, licenseID, email string) (enabled bool, secret string, recoveryCodesJSON string, source string, err error) {
	tx, err := db.Begin()
	if err != nil {
		return false, "", "", "", err
	}
	defer tx.Rollback()
	return getMFAConfigTx(tx, licenseID, email)
}

// CreateTechnician2FAChallenge creates a short-lived challenge token for 2FA validation.
func CreateTechnician2FAChallenge(db *sql.DB, licenseID, email string) (string, error) {
	token, err := generateSecureToken()
	if err != nil {
		return "", err
	}
	now := time.Now().UTC()
	expiresAt := now.Add(technician2FAChallengeLifetime)

	// Clean up stale challenges for this license
	_, _ = db.Exec(`DELETE FROM technician_2fa_challenges WHERE license_id = ?`, licenseID)

	_, err = db.Exec(`
		INSERT INTO technician_2fa_challenges (challenge_token, license_id, email, created_at, expires_at, attempts)
		VALUES (?, ?, ?, ?, ?, 0)
	`, hashSessionToken(token), licenseID, email, now.Format(time.RFC3339), expiresAt.Format(time.RFC3339))
	if err != nil {
		return "", err
	}
	return token, nil
}

// SendTechnician2FAEmailCode generates a single-use 6-digit numeric code and attaches it to the challenge.
func SendTechnician2FAEmailCode(db *sql.DB, challengeToken string) (code string, email string, err error) {
	challengeToken = strings.TrimSpace(challengeToken)
	if challengeToken == "" {
		return "", "", errors.New("jeton de challenge requis")
	}
	now := time.Now().UTC()
	var emailVal string
	var expiresRaw any
	var attempts int
	var lastSentRaw sql.NullString

	err = db.QueryRow(`
		SELECT email, expires_at, attempts, email_code_sent_at
		FROM technician_2fa_challenges
		WHERE challenge_token = ?
	`, hashSessionToken(challengeToken)).Scan(&emailVal, &expiresRaw, &attempts, &lastSentRaw)
	if err != nil {
		return "", "", errors.New("session de vérification invalide ou expirée")
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
		UPDATE technician_2fa_challenges
		SET email_code_hash = ?, email_code_expires_at = ?, email_code_sent_at = ?, expires_at = ?
		WHERE challenge_token = ?
	`, codeHash, codeExpires.Format(time.RFC3339), now.Format(time.RFC3339), expiry.Format(time.RFC3339), hashSessionToken(challengeToken))
	if err != nil {
		return "", "", err
	}

	return code, emailVal, nil
}

// VerifyTechnician2FAChallengeWithDevice validates the TOTP or email code against the challenge,
// and on success consumes the challenge, issues a technician session, and optionally issues a 30-day device token.
func VerifyTechnician2FAChallengeWithDevice(db *sql.DB, challengeToken, code string, rememberDevice bool, deviceName ...string) (sessionToken string, lic *License, deviceToken string, err error) {
	tx, err := db.Begin()
	if err != nil {
		return "", nil, "", err
	}
	defer tx.Rollback()
	now := time.Now().UTC()
	var challengeMember sql.NullString
	if err = tx.QueryRow(`SELECT team_member_id FROM technician_2fa_challenges WHERE challenge_token=?`, hashSessionToken(strings.TrimSpace(challengeToken))).Scan(&challengeMember); err != nil {
		return "", nil, "", errors.New("session de vérification invalide ou expirée")
	}
	email, licenseID, _, err := verifyChallengeTx(tx, "technician_2fa_challenges", strings.TrimSpace(challengeToken), code, now)
	if err != nil {
		return "", nil, "", err
	}
	lic, err = activeLicenseTx(tx, licenseID)
	if err != nil {
		return "", nil, "", err
	}
	memberID, err := technicianMemberTx(tx, licenseID, email)
	if err != nil || memberID != challengeMember.String {
		return "", nil, "", ErrTeamAccess
	}
	sessionToken, err = createMemberTechnicianSessionTx(tx, licenseID, memberID, now)
	if err != nil {
		return "", nil, "", err
	}

	if rememberDevice {
		rawDeviceToken, genErr := generateSecureToken()
		if genErr == nil {
			devName := "Appareil Technicien"
			if len(deviceName) > 0 && strings.TrimSpace(deviceName[0]) != "" {
				devName = strings.TrimSpace(deviceName[0])
			}
			deviceToken = rawDeviceToken
			deviceExpires := now.Add(customerTrustedDeviceLifetime)
			var cID sql.NullInt64
			_ = tx.QueryRow(`SELECT customer_id FROM licences WHERE license_id = ?`, licenseID).Scan(&cID)
			_, _ = tx.Exec(`
				INSERT INTO customer_trusted_devices(device_token_hash, email, customer_id, device_name, created_at, expires_at, last_used_at)
				VALUES (?, ?, ?, ?, ?, ?, ?)
			`, hashSessionToken(rawDeviceToken), email, cID.Int64, devName, now.Format(time.RFC3339), deviceExpires.Format(time.RFC3339), now.Format(time.RFC3339))
		}
	}

	if err = tx.Commit(); err != nil {
		return "", nil, "", err
	}
	return sessionToken, lic, deviceToken, nil
}

// VerifyTechnician2FAChallenge validates the TOTP or recovery code against the challenge,
// and on success consumes the challenge and issues a technician session.
func VerifyTechnician2FAChallenge(db *sql.DB, challengeToken, code string) (sessionToken string, lic *License, err error) {
	sessionToken, lic, _, err = VerifyTechnician2FAChallengeWithDevice(db, challengeToken, code, false)
	return sessionToken, lic, err
}

// ValidateTechnicianCredentialsWith2FA checks license credentials and checks if 2FA is required.
// If optionalCode is supplied and matches, the session is created immediately.
func ValidateTechnicianCredentialsWith2FA(db *sql.DB, licenseID, licenseKey, optionalCode string, deviceToken ...string) (*TechnicianAuthResult, *License, error) {
	lic, err := ValidateLicenseCredentials(db, licenseID, licenseKey)
	if err != nil {
		return nil, nil, err
	}
	dT := ""
	if len(deviceToken) > 0 {
		dT = deviceToken[0]
	}
	return completeTechnicianLogin(db, lic, lic.Email, optionalCode, "", dT)
}

// FindActiveTechnicianLicense locates an active, non-expired license belonging to the given email
// directly or via its customer relation.
func FindActiveTechnicianLicense(db *sql.DB, email string) (*License, error) {
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
		ORDER BY expires_at DESC
		LIMIT 1
	`
	row := db.QueryRow(query, email, email, email)
	lic, err := scanLicense(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("aucun abonnement ou licence active associée à cet e-mail")
		}
		return nil, err
	}
	return lic, nil
}

// ValidateTechnicianEmailPassword verifies email and password against customer_accounts,
// locates their active subscription/license, and returns a session or a 2FA challenge.
func ValidateTechnicianEmailPassword(db *sql.DB, email, plainPassword, optionalCode string, extra ...string) (*TechnicianAuthResult, *License, error) {
	requestedLicense := ""
	deviceToken := ""
	if len(extra) > 0 {
		requestedLicense = extra[0]
	}
	if len(extra) > 1 {
		deviceToken = extra[1]
	}
	email = strings.ToLower(strings.TrimSpace(email))
	lic, err := FindActiveTechnicianLicense(db, email)
	if requestedLicense != "" {
		lic, err = GetLicenseByID(db, requestedLicense)
	} else if err != nil {
		members, e := ListMyTeamMemberships(db, email)
		if e != nil {
			return nil, nil, e
		}
		if len(members) > 0 {
			lic, err = GetLicenseByID(db, members[0].LicenseID)
		}
	}
	if err != nil {
		return nil, nil, err
	}
	if plainPassword == "" {
		return nil, nil, errors.New("mot de passe requis")
	}
	return completeTechnicianLogin(db, lic, email, optionalCode, plainPassword, deviceToken)
}

// The password is checked under the same transaction as factor validation and
// session/challenge issuance, preventing password-reset/login races.
func completeTechnicianLogin(db *sql.DB, lic *License, email, optionalCode, password string, deviceToken ...string) (*TechnicianAuthResult, *License, error) {
	validatedKey := lic.LicenseKey
	wasAdmin := strings.EqualFold(strings.TrimSpace(lic.Notes), "ADMIN")
	tx, err := db.Begin()
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback()
	lic, err = activeLicenseTx(tx, lic.LicenseID)
	if err != nil {
		return nil, nil, err
	}
	if wasAdmin && !strings.EqualFold(strings.TrimSpace(lic.Notes), "ADMIN") {
		return nil, nil, errors.New("droits administrateur retirés")
	}
	if password == "" && (subtle.ConstantTimeCompare([]byte(validatedKey), []byte(lic.LicenseKey)) != 1 || !strings.EqualFold(lic.Email, email)) {
		return nil, nil, errors.New("clé de licence remplacée ; reconnectez-vous")
	}
	memberID := ""
	if password != "" {
		memberID, err = technicianMemberTx(tx, lic.LicenseID, email)
		if err != nil {
			return nil, nil, errors.New("licence non associée à ce compte")
		}
		var hash sql.NullString
		if err = tx.QueryRow("SELECT password_hash FROM customer_accounts WHERE email=? COLLATE NOCASE", email).Scan(&hash); err != nil || !hash.Valid || hash.String == "" {
			return nil, nil, errors.New("aucun mot de passe configuré pour ce compte")
		}
		// Per-account password throttle (mirrors the customer login path):
		// the per-IP StrictAuthLimiter alone cannot stop distributed
		// guessing against one technician/admin e-mail.
		if locked, err := customerPasswordLocked(tx, email); err != nil {
			return nil, nil, err
		} else if locked {
			return nil, nil, errors.New("compte temporairement verrouillé, réessayez plus tard")
		}
		if !checkPassword(hash.String, password) {
			if err := recordCustomerPasswordFailure(tx, email); err != nil {
				return nil, nil, err
			}
			// Persist the throttle counter: the deferred rollback
			// below would otherwise silently drop it.
			if err := tx.Commit(); err != nil {
				return nil, nil, err
			}
			return nil, nil, errors.New("mot de passe incorrect")
		}
		if err := resetCustomerPasswordFailures(tx, email); err != nil {
			return nil, nil, err
		}
	}
	enabled, _, _, _, err := getMFAConfigTx(tx, lic.LicenseID, email)
	if err != nil {
		return nil, nil, err
	}
	now := time.Now().UTC()
	res := &TechnicianAuthResult{LicenseID: lic.LicenseID, Email: email, ExpiresAt: lic.ExpiresAt}
	if enabled && strings.TrimSpace(optionalCode) == "" {
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
				res.SessionToken, err = createMemberTechnicianSessionTx(tx, lic.LicenseID, memberID, now)
				if err != nil {
					return nil, nil, err
				}
				if err = tx.Commit(); err != nil {
					return nil, nil, err
				}
				return res, lic, nil
			}
		}

		token, e := generateSecureToken()
		if e != nil {
			return nil, nil, e
		}
		if _, err = tx.Exec("DELETE FROM technician_2fa_challenges WHERE license_id=? AND email=? COLLATE NOCASE", lic.LicenseID, email); err != nil {
			return nil, nil, err
		}
		if _, err = tx.Exec("INSERT INTO technician_2fa_challenges(challenge_token,license_id,email,created_at,expires_at,attempts,team_member_id) VALUES (?,?,?,?,?,0,?)", hashSessionToken(token), lic.LicenseID, email, now.Format(time.RFC3339), now.Add(technician2FAChallengeLifetime).Format(time.RFC3339), memberID); err != nil {
			return nil, nil, err
		}
		res.Requires2FA = true
		res.ChallengeToken = token
	} else {
		if enabled {
			if err = consumeMFACodeTx(tx, lic.LicenseID, email, optionalCode, now); err != nil {
				return nil, nil, err
			}
		}
		res.SessionToken, err = createMemberTechnicianSessionTx(tx, lic.LicenseID, memberID, now)
		if err != nil {
			return nil, nil, err
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, nil, err
	}
	return res, lic, nil
}

// ValidateTechnicianGoogle verifies an eligible technician using their verified Google account,
// locates their active subscription/license (or team membership), and returns a session or a 2FA challenge.
func ValidateTechnicianGoogle(db *sql.DB, email, optionalCode string, extra ...string) (*TechnicianAuthResult, *License, error) {
	requestedLicense := ""
	deviceToken := ""
	if len(extra) > 0 {
		requestedLicense = extra[0]
	}
	if len(extra) > 1 {
		deviceToken = extra[1]
	}
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return nil, nil, errors.New("adresse e-mail requise")
	}

	lic, err := FindActiveTechnicianLicense(db, email)
	if requestedLicense != "" {
		lic, err = GetLicenseByID(db, requestedLicense)
	} else if err != nil {
		members, e := ListMyTeamMemberships(db, email)
		if e != nil {
			return nil, nil, e
		}
		if len(members) > 0 {
			lic, err = GetLicenseByID(db, members[0].LicenseID)
		}
	}
	if err != nil || lic == nil {
		return nil, nil, errors.New("aucun abonnement ou licence active associée à ce compte Google")
	}

	return completeTechnicianGoogleLogin(db, lic, email, optionalCode, deviceToken)
}

func completeTechnicianGoogleLogin(db *sql.DB, lic *License, email, optionalCode string, deviceToken ...string) (*TechnicianAuthResult, *License, error) {
	wasAdmin := strings.EqualFold(strings.TrimSpace(lic.Notes), "ADMIN")
	tx, err := db.Begin()
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback()

	lic, err = activeLicenseTx(tx, lic.LicenseID)
	if err != nil {
		return nil, nil, err
	}
	if wasAdmin && !strings.EqualFold(strings.TrimSpace(lic.Notes), "ADMIN") {
		return nil, nil, errors.New("droits administrateur retirés")
	}

	memberID, err := technicianMemberTx(tx, lic.LicenseID, email)
	if err != nil {
		return nil, nil, errors.New("licence non associée à ce compte")
	}

	enabled, _, _, _, err := getMFAConfigTx(tx, lic.LicenseID, email)
	if err != nil {
		return nil, nil, err
	}

	now := time.Now().UTC()
	res := &TechnicianAuthResult{LicenseID: lic.LicenseID, Email: email, ExpiresAt: lic.ExpiresAt}

	if enabled && strings.TrimSpace(optionalCode) == "" {
		// Check if a valid trusted device token was provided
		if len(deviceToken) > 0 && strings.TrimSpace(deviceToken[0]) != "" {
			dT := strings.TrimSpace(deviceToken[0])
			var exists int
			err = tx.QueryRow(`
				SELECT 1 FROM customer_trusted_devices
				WHERE device_token_hash = ? AND email = ? COLLATE NOCASE AND expires_at > ?
			`, hashSessionToken(dT), email, now.Format(time.RFC3339)).Scan(&exists)
			if err == nil && exists == 1 {
				_, _ = tx.Exec(`UPDATE customer_trusted_devices SET last_used_at = ? WHERE device_token_hash = ?`, now.Format(time.RFC3339), hashSessionToken(dT))
				res.Requires2FA = false
				res.SessionToken, err = createMemberTechnicianSessionTx(tx, lic.LicenseID, memberID, now)
				if err != nil {
					return nil, nil, err
				}
				if err = tx.Commit(); err != nil {
					return nil, nil, err
				}
				return res, lic, nil
			}
		}

		token, e := generateSecureToken()
		if e != nil {
			return nil, nil, e
		}
		if _, err = tx.Exec("DELETE FROM technician_2fa_challenges WHERE license_id=? AND email=? COLLATE NOCASE", lic.LicenseID, email); err != nil {
			return nil, nil, err
		}
		if _, err = tx.Exec("INSERT INTO technician_2fa_challenges(challenge_token,license_id,email,created_at,expires_at,attempts,team_member_id) VALUES (?,?,?,?,?,0,?)", hashSessionToken(token), lic.LicenseID, email, now.Format(time.RFC3339), now.Add(technician2FAChallengeLifetime).Format(time.RFC3339), memberID); err != nil {
			return nil, nil, err
		}
		res.Requires2FA = true
		res.ChallengeToken = token
	} else {
		if enabled {
			if err = consumeMFACodeTx(tx, lic.LicenseID, email, optionalCode, now); err != nil {
				return nil, nil, err
			}
		}
		res.SessionToken, err = createMemberTechnicianSessionTx(tx, lic.LicenseID, memberID, now)
		if err != nil {
			return nil, nil, err
		}
	}

	if err = tx.Commit(); err != nil {
		return nil, nil, err
	}
	return res, lic, nil
}
