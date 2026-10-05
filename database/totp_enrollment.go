package database

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

func verifyAccountPasswordTx(tx *sql.Tx, email, password string) error {
	var hash sql.NullString
	if err := tx.QueryRow(`SELECT password_hash FROM customer_accounts WHERE email=? COLLATE NOCASE`, email).Scan(&hash); err != nil || !hash.Valid || !checkPassword(hash.String, password) {
		return errors.New("mot de passe incorrect")
	}
	return nil
}

func EnableCustomerTOTPWithPassword(db *sql.DB, email, password, secret, code string, recovery []string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = verifyAccountPasswordTx(tx, email, password); err != nil {
		return err
	}
	if err = enableCustomerTOTPTx(tx, email, secret, code, recovery, time.Now().UTC()); err != nil {
		return err
	}
	if err = revokeAccountSessionsTx(tx, email); err != nil {
		return err
	}
	return tx.Commit()
}

func enableCustomerTOTPTx(tx *sql.Tx, email, secret, code string, recovery []string, now time.Time) error {
	email = strings.ToLower(strings.TrimSpace(email))
	enabled, _, _, _, err := getMFAConfigTx(tx, "", email)
	if err != nil {
		return err
	}
	if enabled {
		return errors.New("la double authentification existante ne peut pas être remplacée par cette opération")
	}
	var expectedSecretStored, expectedRecoveryStored string
	var expiryRaw any
	if err = tx.QueryRow(`SELECT secret,recovery_codes,expires_at FROM totp_enrollments WHERE email=? COLLATE NOCASE`, email).Scan(&expectedSecretStored, &expectedRecoveryStored, &expiryRaw); err != nil {
		return errors.New("préparation 2FA introuvable ou expirée")
	}
	expectedSecret, _, err := openTOTPSecret(expectedSecretStored)
	if err != nil {
		return err
	}
	expectedRecovery, _, err := openTOTPSecret(expectedRecoveryStored)
	if err != nil {
		return err
	}
	expiry, err := ParseSQLiteTime(expiryRaw)
	if err != nil || !expiry.After(now) {
		return errors.New("préparation 2FA expirée")
	}
	hashes := make([]string, len(recovery))
	for i, c := range recovery {
		hashes[i] = HashRecoveryCode(c)
	}
	encoded, err := json.Marshal(hashes)
	if err != nil {
		return err
	}
	if len(recovery) != recoveryCount || secret != expectedSecret || string(encoded) != expectedRecovery || !ValidateTOTPCode(secret, strings.TrimSpace(code), now) {
		return errors.New("paramètres ou code de confirmation 2FA incorrects")
	}
	sealedSecret, err := protectTOTPSecret(secret)
	if err != nil {
		return err
	}
	sealedRecovery, err := protectTOTPSecret(expectedRecovery)
	if err != nil {
		return err
	}
	result, err := tx.Exec(`UPDATE customer_accounts SET totp_enabled=1,totp_secret=?,totp_recovery_codes=?,totp_confirmed_at=? WHERE email=? COLLATE NOCASE AND COALESCE(totp_enabled,0)=0`, sealedSecret, sealedRecovery, now.Format(time.RFC3339), email)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil || n != 1 {
		return errors.New("compte introuvable ou 2FA déjà activée")
	}
	_, err = tx.Exec(`DELETE FROM totp_enrollments WHERE email=? COLLATE NOCASE`, email)
	return err
}
