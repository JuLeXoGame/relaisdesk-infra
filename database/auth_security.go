package database

import (
	"database/sql"
	"errors"
	"strings"
	"time"
)

var ErrMFARequired = errors.New("code de double authentification ou code de secours requis")

func verifyEmailTokenMFATx(tx *sql.Tx, token, email, code string, now time.Time) error {
	enabled, _, _, _, err := getMFAConfigTx(tx, "", email)
	if err != nil || !enabled {
		return err
	}
	if strings.TrimSpace(code) == "" {
		return ErrMFARequired
	}
	var attempts int
	if err = tx.QueryRow(`SELECT mfa_attempts FROM customer_login_tokens WHERE token_hash=?`, hashSessionToken(token)).Scan(&attempts); err != nil {
		return err
	}
	if attempts >= max2FAAttempts {
		return errors.New("trop de tentatives ; demandez un nouveau lien")
	}
	if _, err = tx.Exec(`UPDATE customer_login_tokens SET mfa_attempts=mfa_attempts+1 WHERE token_hash=?`, hashSessionToken(token)); err != nil {
		return err
	}
	if err = consumeMFACodeTx(tx, "", email, code, now); err != nil {
		if e := tx.Commit(); e != nil {
			return e
		}
		return err
	}
	return nil
}

// Technician sessions are licence-scoped; revoke the customer's licence sessions
// as well as their personal sessions when an account credential changes.
func revokeAccountSessionsTx(tx *sql.Tx, email string) error {
	for _, table := range []string{"technician_sessions", "technician_2fa_challenges"} {
		if _, err := tx.Exec(`DELETE FROM `+table+` WHERE team_member_id IN (SELECT member_id FROM team_members WHERE email=? COLLATE NOCASE)`, email); err != nil {
			return err
		}
	}
	const licences = `SELECT license_id FROM licences WHERE email = ? COLLATE NOCASE OR customer_id IN (SELECT customer_id FROM customer_accounts WHERE email = ? COLLATE NOCASE)`
	for _, stmt := range []string{
		`DELETE FROM admin_sessions WHERE token_hash IN (SELECT token FROM technician_sessions WHERE license_id IN (` + licences + `))`,
		`DELETE FROM technician_sessions WHERE license_id IN (` + licences + `)`,
		`DELETE FROM technician_2fa_challenges WHERE license_id IN (` + licences + `)`,
	} {
		if _, err := tx.Exec(stmt, email, email); err != nil {
			return err
		}
	}
	for _, table := range []string{"customer_sessions", "customer_2fa_challenges", "customer_login_tokens", "customer_trusted_devices"} {
		if _, err := tx.Exec(`DELETE FROM `+table+` WHERE email = ? COLLATE NOCASE`, email); err != nil {
			return err
		}
	}
	return nil
}

func applyAuthSecurityMigration(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`CREATE TABLE IF NOT EXISTS security_migrations (name TEXT PRIMARY KEY)`); err != nil {
		return err
	}
	result, err := tx.Exec(`INSERT OR IGNORE INTO security_migrations(name) VALUES ('20260912-auth-v1')`)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 1 {
		// Legacy sessions do not reliably attest which authentication factors passed.
		for _, table := range []string{"admin_sessions", "technician_sessions", "customer_sessions", "customer_2fa_challenges", "technician_2fa_challenges", "customer_login_tokens"} {
			if _, err = tx.Exec(`DELETE FROM ` + table); err != nil {
				return err
			}
		}
	}
	if _, err = tx.Exec(`CREATE TABLE IF NOT EXISTS totp_enrollments (email TEXT PRIMARY KEY COLLATE NOCASE, secret TEXT NOT NULL, recovery_codes TEXT NOT NULL, expires_at DATETIME NOT NULL)`); err != nil {
		return err
	}
	if _, err = tx.Exec(`CREATE TABLE IF NOT EXISTS used_totp_codes (scope TEXT NOT NULL, code_hash TEXT NOT NULL, expires_at DATETIME NOT NULL, PRIMARY KEY(scope,code_hash))`); err != nil {
		return err
	}
	result, err = tx.Exec(`INSERT OR IGNORE INTO security_migrations(name) VALUES ('20260919-google-mfa-v1')`)
	if err != nil {
		return err
	}
	n, err = result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 1 {
		// Existing sessions do not identify the Google login path. Expire the
		// affected MFA accounts' sessions once, including technician exchanges.
		rows, err := tx.Query(`SELECT email FROM customer_accounts WHERE totp_enabled=1`)
		if err != nil {
			return err
		}
		var emails []string
		for rows.Next() {
			var email string
			if err := rows.Scan(&email); err != nil {
				rows.Close()
				return err
			}
			emails = append(emails, email)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		if err := rows.Close(); err != nil {
			return err
		}
		for _, email := range emails {
			if err := revokeAccountSessionsTx(tx, email); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

func getMFAConfigTx(tx *sql.Tx, licenseID, email string) (enabled bool, secret, recovery, source string, err error) {
	if licenseID != "" && email != "" {
		memberID, e := technicianMemberTx(tx, licenseID, email)
		if e != nil {
			return false, "", "", "", e
		}
		if memberID != "" {
			licenseID = ""
		} // An invited user's second factor is never the owner's.
	}
	for _, spec := range []struct{ table, field, value, source string }{{"customer_accounts", "email", email, "customer"}, {"licences", "license_id", licenseID, "license"}} {
		if strings.TrimSpace(spec.value) == "" {
			continue
		}
		var active int
		var s, r sql.NullString
		e := tx.QueryRow(`SELECT COALESCE(totp_enabled,0),totp_secret,totp_recovery_codes FROM `+spec.table+` WHERE `+spec.field+` = ? COLLATE NOCASE`, spec.value).Scan(&active, &s, &r)
		if errors.Is(e, sql.ErrNoRows) {
			continue
		}
		if e != nil {
			return false, "", "", "", e
		}
		if active != 0 {
			if !s.Valid || s.String == "" {
				return false, "", "", "", errors.New("configuration 2FA invalide")
			}
			secret, secretLegacy, err := openTOTPSecret(s.String)
			if err != nil {
				return false, "", "", "", err
			}
			recovery := r.String
			recoveryLegacy := false
			if r.Valid && r.String != "" {
				opened, legacy, err := openTOTPSecret(r.String)
				if err != nil {
					return false, "", "", "", err
				}
				recovery, recoveryLegacy = opened, legacy
			}
			// Opportunistic migration: re-protect legacy plaintext rows on
			// read (skipped silently when no key is provisioned).
			if secretLegacy && secret != "" {
				if sealed, err := protectTOTPSecret(secret); err == nil {
					_, _ = tx.Exec(`UPDATE `+spec.table+` SET totp_secret=? WHERE `+spec.field+` = ? COLLATE NOCASE`, sealed, spec.value)
				}
			}
			if recoveryLegacy && recovery != "" {
				if sealed, err := protectTOTPSecret(recovery); err == nil {
					_, _ = tx.Exec(`UPDATE `+spec.table+` SET totp_recovery_codes=? WHERE `+spec.field+` = ? COLLATE NOCASE`, sealed, spec.value)
				}
			}
			return true, secret, recovery, spec.source, nil
		}
	}
	return false, "", "", "", nil
}

// Verification, one-time consumption and session creation must share a transaction.
func consumeMFACodeTx(tx *sql.Tx, licenseID, email, code string, now time.Time) error {
	enabled, secret, recovery, source, err := getMFAConfigTx(tx, licenseID, email)
	if err != nil {
		return err
	}
	if !enabled {
		return errors.New("configuration 2FA introuvable")
	}
	code = strings.TrimSpace(code)
	if code == "" {
		return ErrMFARequired
	}
	table, field, value := "customer_accounts", "email", email
	if source == "license" {
		table, field, value = "licences", "license_id", licenseID
	}
	if len(code) == 6 && ValidateTOTPCode(secret, code, now) {
		if _, err = tx.Exec(`DELETE FROM used_totp_codes WHERE expires_at <= ?`, now.Format(time.RFC3339)); err != nil {
			return err
		}
		scope := source + ":" + strings.ToLower(value) + ":" + hashSessionToken(secret)
		res, e := tx.Exec(`INSERT OR IGNORE INTO used_totp_codes(scope,code_hash,expires_at) VALUES (?,?,?)`, scope, hashSessionToken(code), now.Add(2*time.Minute).Format(time.RFC3339))
		if e != nil {
			return e
		}
		n, e := res.RowsAffected()
		if e != nil {
			return e
		}
		if n != 1 {
			return errors.New("code déjà utilisé ; attendez le prochain code ou utilisez un code de secours")
		}
		return nil
	}
	valid, updated, err := ValidateAndConsumeRecoveryCode(recovery, code)
	if err != nil || !valid {
		return errors.New("code d'authentification ou code de secours incorrect")
	}
	// Ciphertext is randomized: re-read the stored blob for compare-and-swap
	// (keeps the double-spend guard), and persist the updated blob protected
	// whenever a key is available.
	var stored string
	if err := tx.QueryRow(`SELECT totp_recovery_codes FROM `+table+` WHERE `+field+` = ? COLLATE NOCASE`, value).Scan(&stored); err != nil {
		return err
	}
	updatedStored := updated
	if strings.HasPrefix(stored, totpProtectedPrefix) || totpKeyAvailable() {
		sealed, err := protectTOTPSecret(updated)
		if err != nil {
			return err
		}
		updatedStored = sealed
	}
	res, err := tx.Exec(`UPDATE `+table+` SET totp_recovery_codes=? WHERE `+field+` = ? COLLATE NOCASE AND totp_recovery_codes=?`, updatedStored, value, stored)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return errors.New("code de secours déjà utilisé")
	}
	return nil
}

func createCustomerSessionTx(tx *sql.Tx, email string, customerID int64, now time.Time) (string, error) {
	token, err := generateSecureToken()
	if err != nil {
		return "", err
	}
	if _, err = tx.Exec(`INSERT INTO customer_sessions(token_hash,email,customer_id,expires_at,last_used_at) VALUES (?,?,?,?,?)`, hashSessionToken(token), email, customerID, now.Add(customerSessionLifetime).Format(time.RFC3339), now.Format(time.RFC3339)); err != nil {
		return "", err
	}
	if _, err = tx.Exec(`UPDATE customer_accounts SET last_login_at=? WHERE email=? COLLATE NOCASE`, now.Format(time.RFC3339), email); err != nil {
		return "", err
	}
	return token, nil
}

func activeLicenseTx(tx *sql.Tx, licenseID string) (*License, error) {
	lic, err := scanLicense(tx.QueryRow(`SELECT id,license_id,email,license_key,status,created_at,expires_at,max_connections,current_connections,last_connection_at,notes,revoked_at,revoke_reason,key_hint FROM licences WHERE license_id=?`, licenseID))
	if err != nil {
		return nil, err
	}
	if lic.Status != "active" || !lic.ExpiresAt.After(time.Now().UTC()) {
		return nil, errors.New("licence inactive ou expirée")
	}
	return lic, nil
}

func createTechnicianSessionTx(tx *sql.Tx, licenseID string, now time.Time) (string, error) {
	lic, err := activeLicenseTx(tx, licenseID)
	if err != nil {
		return "", err
	}
	token, err := generateSecureToken()
	if err != nil {
		return "", err
	}
	lifetime := technicianSessionLifetime
	if strings.EqualFold(strings.TrimSpace(lic.Notes), "ADMIN") {
		lifetime = adminSessionLifetime
	}
	_, err = tx.Exec(`INSERT INTO technician_sessions(token,license_id,expires_at,last_used_at) VALUES (?,?,?,?)`, hashSessionToken(token), licenseID, now.Add(lifetime).Format(time.RFC3339), now.Format(time.RFC3339))
	if err != nil {
		return "", err
	}
	return token, nil
}

// On invalid codes commit only the failed-attempt counter. On success leave the
// transaction open so the caller commits consumption together with the session.
func verifyChallengeTx(tx *sql.Tx, table, token, code string, now time.Time) (email, licenseID string, customerID int64, err error) {
	var expiresRaw any
	var attempts int
	var emailCodeHash sql.NullString
	var emailCodeExpiresRaw sql.NullString
	emailCodeAllowed := 1
	if table == "customer_2fa_challenges" {
		err = tx.QueryRow(`SELECT email,customer_id,expires_at,attempts,email_code_hash,email_code_expires_at,email_code_allowed FROM customer_2fa_challenges WHERE challenge_token=?`, hashSessionToken(token)).Scan(&email, &customerID, &expiresRaw, &attempts, &emailCodeHash, &emailCodeExpiresRaw, &emailCodeAllowed)
	} else {
		err = tx.QueryRow(`SELECT email,license_id,expires_at,attempts,email_code_hash,email_code_expires_at FROM technician_2fa_challenges WHERE challenge_token=?`, hashSessionToken(token)).Scan(&email, &licenseID, &expiresRaw, &attempts, &emailCodeHash, &emailCodeExpiresRaw)
	}
	if err != nil {
		return "", "", 0, errors.New("session de vérification invalide ou expirée")
	}
	expiry, e := ParseSQLiteTime(expiresRaw)
	if e != nil || !expiry.After(now) || attempts >= max2FAAttempts {
		return "", "", 0, errors.New("session de vérification expirée ou nombre maximal de tentatives dépassé")
	}
	if licenseID != "" {
		if _, err = activeLicenseTx(tx, licenseID); err != nil {
			return "", "", 0, err
		}
	}
	if _, err = tx.Exec(`UPDATE `+table+` SET attempts=attempts+1 WHERE challenge_token=?`, hashSessionToken(token)); err != nil {
		return "", "", 0, err
	}

	codeMatches := false
	if emailCodeAllowed == 1 && emailCodeHash.Valid && emailCodeHash.String != "" && emailCodeHash.String == hashSessionToken(code) {
		if codeExp, parseErr := ParseSQLiteTime(emailCodeExpiresRaw.String); parseErr == nil && codeExp.After(now) {
			codeMatches = true
		}
	}

	if !codeMatches {
		if err = consumeMFACodeTx(tx, licenseID, email, code, now); err != nil {
			if commitErr := tx.Commit(); commitErr != nil {
				return "", "", 0, commitErr
			}
			return "", "", 0, err
		}
	}
	result, e := tx.Exec(`DELETE FROM `+table+` WHERE challenge_token=?`, hashSessionToken(token))
	if e != nil {
		return "", "", 0, e
	}
	n, e := result.RowsAffected()
	if e != nil || n != 1 {
		return "", "", 0, errors.New("session de vérification déjà utilisée")
	}
	return email, licenseID, customerID, nil
}
