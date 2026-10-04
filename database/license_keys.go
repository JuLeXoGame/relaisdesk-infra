package database

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"strings"
)

// License keys are bearer credentials: they are stored hashed (SHA-256, like
// session and challenge tokens) and shown in plaintext exactly once, at
// creation. A short key_hint (first/last characters) is stored alongside so
// support can still identify a key without ever recovering it.
const licenseKeyHintPrefixLen = 9 // "mpsk_" + 4 chars

// HashLicenseKey returns the lowercase hex SHA-256 of a plaintext key.
func HashLicenseKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

// LicenseKeyHint returns the support-safe identifier of a plaintext key
// ("mpsk_ab12...wxyz" shape). It reveals too little to authenticate.
func LicenseKeyHint(key string) string {
	key = strings.TrimSpace(key)
	if len(key) < licenseKeyHintPrefixLen+4 {
		return "****"
	}
	return key[:licenseKeyHintPrefixLen] + "..." + key[len(key)-4:]
}

// IsHashedLicenseKey reports whether a stored value already is a SHA-256 hex
// digest (as opposed to a legacy plaintext "mpsk_..." key).
func IsHashedLicenseKey(stored string) bool {
	if len(stored) != sha256.Size*2 {
		return false
	}
	for _, c := range stored {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return false
		}
	}
	return true
}

// MigrateLicenseKeysToHash one-way migrates legacy plaintext licence keys to
// SHA-256 hashes and backfills their hints. Idempotent: already-hashed rows
// are left untouched.
func MigrateLicenseKeysToHash(db *sql.DB) error {
	rows, err := db.Query(`SELECT id, license_key FROM licences WHERE license_key LIKE 'mpsk\_%' ESCAPE '\'`)
	if err != nil {
		return err
	}
	defer rows.Close()
	type legacyRow struct {
		id  int64
		key string
	}
	var pending []legacyRow
	for rows.Next() {
		var row legacyRow
		if err := rows.Scan(&row.id, &row.key); err != nil {
			return err
		}
		pending = append(pending, row)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, row := range pending {
		if _, err := db.Exec(
			`UPDATE licences SET license_key = ?, key_hint = ? WHERE id = ? AND license_key = ?`,
			HashLicenseKey(row.key), LicenseKeyHint(row.key), row.id, row.key,
		); err != nil {
			return err
		}
	}
	return nil
}
