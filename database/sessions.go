package database

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log"
	"strings"
	"time"
)

const (
	technicianSessionLifetime = 24 * time.Hour
	adminSessionLifetime      = 12 * time.Hour
	adminSessionIdleTimeout   = 2 * time.Hour
)

func generateSecureToken() (string, error) {
	b := make([]byte, 48) // 64 base64 chars
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate secure token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// CreateTechnicianSession generates a secure session token and saves it in the database.
func CreateTechnicianSession(db *sql.DB, licenseID string) (string, error) {
	tx, err := db.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	token, err := createTechnicianSessionTx(tx, licenseID, time.Now().UTC())
	if err != nil {
		return "", err
	}
	if err = tx.Commit(); err != nil {
		return "", err
	}
	return token, nil
}

// ValidateTechnicianSession returns the licenseID if the token is valid and not expired.
func ValidateTechnicianSession(db *sql.DB, token string) (string, error) {
	if token == "" {
		return "", fmt.Errorf("session invalide ou inexistante")
	}

	var licenseID string
	var expiresAtRaw any
	tokenHash := hashSessionToken(token)

	query := `SELECT s.license_id, s.expires_at FROM technician_sessions s JOIN licences l ON l.license_id=s.license_id WHERE s.token = ? AND l.status='active' AND julianday(l.expires_at)>julianday('now') AND (UPPER(TRIM(COALESCE(l.notes,'')))!='ADMIN' OR julianday(s.last_used_at)>julianday('now','-2 hours'))`
	err := db.QueryRow(query, tokenHash).Scan(&licenseID, &expiresAtRaw)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("session invalide ou inexistante")
		}
		return "", err
	}

	expiresAt, err := ParseSQLiteTime(expiresAtRaw)
	if err != nil {
		return "", fmt.Errorf("erreur parsing date expiration session: %w", err)
	}

	if time.Now().UTC().After(expiresAt) {
		DeleteTechnicianSession(db, token)
		return "", fmt.Errorf("session expirée")
	}
	if _, err = TeamMemberForSession(db, token); err != nil {
		return "", fmt.Errorf("session d'équipe révoquée ou licence non éligible")
	}

	touchSessionLastUsed(db, "technician_sessions", "token", tokenHash, time.Now().UTC().Format("2006-01-02 15:04:05"))

	return licenseID, nil
}

// DeleteTechnicianSession removes a session.
func DeleteTechnicianSession(db *sql.DB, token string) error {
	_, err := db.Exec(`DELETE FROM technician_sessions WHERE token = ?`, hashSessionToken(token))
	return err
}

// CreateAdminSession exchanges the long-lived master secret for a short-lived opaque token.
func CreateAdminSession(db *sql.DB) (string, error) {
	token, err := generateSecureToken()
	if err != nil {
		return "", err
	}
	now := time.Now().UTC()
	_, err = db.Exec(
		`INSERT INTO admin_sessions (token_hash, expires_at, last_used_at) VALUES (?, ?, ?)`,
		hashSessionToken(token),
		now.Add(adminSessionLifetime).Format("2006-01-02 15:04:05"),
		now.Format("2006-01-02 15:04:05"),
	)
	if err != nil {
		return "", fmt.Errorf("failed to create admin session: %w", err)
	}
	return token, nil
}

// ValidateAdminSession checks an opaque admin token without storing the token itself.
func ValidateAdminSession(db *sql.DB, token string) error {
	if token == "" {
		return fmt.Errorf("session admin invalide")
	}
	var expiresAtRaw any
	var lastUsedAtRaw any
	tokenHash := hashSessionToken(token)
	if err := db.QueryRow(
		`SELECT expires_at, COALESCE(last_used_at, created_at) FROM admin_sessions WHERE token_hash = ?`,
		tokenHash,
	).Scan(&expiresAtRaw, &lastUsedAtRaw); err != nil {
		// Licence-backed administrators never get an independent master session.
		licenseID, e := ValidateTechnicianSession(db, token)
		if e == nil {
			member, memberErr := TeamMemberForSession(db, token)
			if memberErr != nil || member != nil {
				return fmt.Errorf("session admin invalide")
			}
			lic, e := GetLicense(db, licenseID)
			if e == nil && lic.Status == "active" && lic.ExpiresAt.After(time.Now().UTC()) && strings.EqualFold(strings.TrimSpace(lic.Notes), "ADMIN") {
				return nil
			}
		}
		return fmt.Errorf("session admin invalide")
	}
	expiresAt, err := ParseSQLiteTime(expiresAtRaw)
	lastUsedAt, lastUsedErr := ParseSQLiteTime(lastUsedAtRaw)
	now := time.Now().UTC()
	if err != nil || lastUsedErr != nil || !expiresAt.After(now) || now.Sub(lastUsedAt) > adminSessionIdleTimeout {
		_, _ = db.Exec(`DELETE FROM admin_sessions WHERE token_hash = ?`, tokenHash)
		return fmt.Errorf("session admin expirée")
	}
	touchSessionLastUsed(db, "admin_sessions", "token_hash", tokenHash, now.Format("2006-01-02 15:04:05"))
	return nil
}

func DeleteAdminSession(db *sql.DB, token string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`DELETE FROM admin_sessions WHERE token_hash = ?`, hashSessionToken(token)); err != nil {
		return err
	}
	if _, err = tx.Exec(`DELETE FROM technician_sessions WHERE token = ?`, hashSessionToken(token)); err != nil {
		return err
	}
	return tx.Commit()
}

// PurgeExpiredSessions removes all sessions that have passed their expiration date.
func PurgeExpiredSessions(db *sql.DB) error {
	if _, err := db.Exec(`DELETE FROM technician_sessions WHERE expires_at < CURRENT_TIMESTAMP`); err != nil {
		return err
	}
	if _, err := db.Exec(`DELETE FROM admin_sessions WHERE expires_at < CURRENT_TIMESTAMP`); err != nil {
		return err
	}
	if _, err := db.Exec(`DELETE FROM customer_sessions WHERE expires_at < CURRENT_TIMESTAMP`); err != nil {
		return err
	}
	_, err := db.Exec(`DELETE FROM customer_login_tokens WHERE expires_at < CURRENT_TIMESTAMP OR consumed_at IS NOT NULL`)
	return err
}

func hashSessionToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// isSQLiteBusyError reports lock contention (SQLITE_BUSY / SQLITE_LOCKED)
// from the driver error text, without depending on driver internals.
func isSQLiteBusyError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "database is locked") ||
		strings.Contains(msg, "database table is locked") ||
		strings.Contains(msg, "database is busy") ||
		strings.Contains(msg, "sqlite_busy")
}

// touchSessionLastUsed refreshes last_used_at on a best-effort basis. The
// session row was already authenticated by the caller, so a transient write
// failure (SQLITE_BUSY when the dashboard fires parallel requests, slow disk,
// read-only replica...) must never turn a valid session into a 401. It
// retries lock contention briefly, then logs and lets validation succeed.
func touchSessionLastUsed(db *sql.DB, table, column, tokenHash, timestamp string) {
	const attempts = 5
	var err error
	for i := 0; i < attempts; i++ {
		if i > 0 {
			time.Sleep(time.Duration(5*i) * time.Millisecond)
		}
		_, err = db.Exec(`UPDATE `+table+` SET last_used_at = ? WHERE `+column+` = ?`, timestamp, tokenHash)
		if err == nil {
			return
		}
		if !isSQLiteBusyError(err) {
			break
		}
	}
	log.Printf("[sessions] mise à jour last_used_at impossible pour %s (session acceptée) : %v", table, err)
}

// HashSessionToken computes SHA-256 hex string for tokens.
func HashSessionToken(token string) string {
	return hashSessionToken(token)
}
