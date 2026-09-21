package database

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// AuthBan represents an IP ban entry.
type AuthBan struct {
	IP            string    `json:"ip"`
	FailedCount   int       `json:"failed_count"`
	BannedUntil   time.Time `json:"banned_until"`
	LastAttemptAt time.Time `json:"last_attempt_at"`
	Reason        string    `json:"reason"`
}

// RecordAuthFailure tracks failed authentication attempts for an IP.
// If the number of failures reaches maxFailures within a 15-minute rolling window,
// it bans the IP until now + banDuration, logs a security alert, and returns isBanned = true.
func RecordAuthFailure(db *sql.DB, ip string, maxFailures int, banDuration time.Duration, reason string) (bool, time.Time, error) {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return false, time.Time{}, errors.New("ip address is required")
	}
	if maxFailures <= 0 {
		maxFailures = 5
	}
	if banDuration <= 0 {
		banDuration = 15 * time.Minute
	}
	if reason == "" {
		reason = "brute_force"
	}

	now := time.Now().UTC()

	tx, err := db.Begin()
	if err != nil {
		return false, time.Time{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var (
		failedCount      int
		bannedUntilRaw   any
		lastAttemptAtRaw any
	)

	err = tx.QueryRow(`
		SELECT failed_count, banned_until, last_attempt_at
		FROM auth_bans
		WHERE ip = ?`, ip).Scan(&failedCount, &bannedUntilRaw, &lastAttemptAtRaw)

	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, time.Time{}, fmt.Errorf("query auth_bans: %w", err)
	}

	if err == nil {
		// Existing record found
		if bannedUntilRaw != nil {
			if bannedUntil, errParse := ParseSQLiteTime(bannedUntilRaw); errParse == nil && bannedUntil.After(now) {
				// Already currently banned
				_, _ = tx.Exec(`UPDATE auth_bans SET last_attempt_at = ? WHERE ip = ?`,
					now.Format(time.RFC3339), ip)
				_ = tx.Commit()
				return true, bannedUntil, nil
			}
		}

		// Check if last attempt is older than 15 minutes
		if lastAttemptAtRaw != nil {
			if lastAttempt, errParse := ParseSQLiteTime(lastAttemptAtRaw); errParse == nil {
				if now.Sub(lastAttempt) > 15*time.Minute {
					failedCount = 0
				}
			}
		}
	}

	failedCount++

	if failedCount >= maxFailures {
		bannedUntil := now.Add(banDuration)
		_, err = tx.Exec(`
			INSERT INTO auth_bans (ip, failed_count, banned_until, last_attempt_at, reason)
			VALUES (?, ?, ?, ?, ?)
			ON CONFLICT(ip) DO UPDATE SET
				failed_count = excluded.failed_count,
				banned_until = excluded.banned_until,
				last_attempt_at = excluded.last_attempt_at,
				reason = excluded.reason
		`, ip, failedCount, bannedUntil.Format(time.RFC3339), now.Format(time.RFC3339), reason)
		if err != nil {
			return false, time.Time{}, fmt.Errorf("upsert auth_bans (ban): %w", err)
		}

		alertMsg := fmt.Sprintf("IP %s bannie jusqu'à %s suite à %d échecs d'authentification consécutifs (%s)",
			ip, bannedUntil.Format(time.RFC3339), failedCount, reason)
		_, _ = tx.Exec(`
			INSERT INTO security_alerts (type, first_ip, message, created_at)
			VALUES ('auth_brute_force', ?, ?, ?)
		`, ip, alertMsg, now.Format(time.RFC3339))

		if err := tx.Commit(); err != nil {
			return false, time.Time{}, fmt.Errorf("commit tx: %w", err)
		}
		return true, bannedUntil, nil
	}

	// Not yet banned
	_, err = tx.Exec(`
		INSERT INTO auth_bans (ip, failed_count, banned_until, last_attempt_at, reason)
		VALUES (?, ?, '1970-01-01 00:00:00', ?, ?)
		ON CONFLICT(ip) DO UPDATE SET
			failed_count = excluded.failed_count,
			last_attempt_at = excluded.last_attempt_at,
			reason = excluded.reason
	`, ip, failedCount, now.Format(time.RFC3339), reason)
	if err != nil {
		return false, time.Time{}, fmt.Errorf("upsert auth_bans (count): %w", err)
	}

	if err := tx.Commit(); err != nil {
		return false, time.Time{}, fmt.Errorf("commit tx: %w", err)
	}
	return false, time.Time{}, nil
}

// RecordAuthSuccess clears failure counts for an IP if not actively banned.
func RecordAuthSuccess(db *sql.DB, ip string) error {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return nil
	}
	now := time.Now().UTC()
	_, err := db.Exec(`
		DELETE FROM auth_bans
		WHERE ip = ? AND datetime(banned_until) <= datetime(?)
	`, ip, now.Format(time.RFC3339))
	return err
}

// IsIPBanned checks whether an IP is currently banned.
func IsIPBanned(db *sql.DB, ip string, now time.Time) (bool, time.Time, error) {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return false, time.Time{}, nil
	}
	var bannedUntilRaw any
	err := db.QueryRow(`
		SELECT banned_until
		FROM auth_bans
		WHERE ip = ? AND datetime(banned_until) > datetime(?)
	`, ip, now.UTC().Format(time.RFC3339)).Scan(&bannedUntilRaw)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, time.Time{}, nil
		}
		return false, time.Time{}, err
	}
	bannedUntil, err := ParseSQLiteTime(bannedUntilRaw)
	if err != nil {
		return false, time.Time{}, err
	}
	return true, bannedUntil, nil
}

// LoadActiveBans retrieves all currently active bans from the database.
func LoadActiveBans(db *sql.DB, now time.Time) (map[string]time.Time, error) {
	bans := make(map[string]time.Time)
	rows, err := db.Query(`
		SELECT ip, banned_until
		FROM auth_bans
		WHERE datetime(banned_until) > datetime(?)
	`, now.UTC().Format(time.RFC3339))
	if err != nil {
		return bans, err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			ip             string
			bannedUntilRaw any
		)
		if err := rows.Scan(&ip, &bannedUntilRaw); err != nil {
			return bans, err
		}
		if t, err := ParseSQLiteTime(bannedUntilRaw); err == nil && t.After(now) {
			bans[ip] = t
		}
	}
	return bans, rows.Err()
}

// PurgeExpiredBans deletes bans that expired before the cutoff.
func PurgeExpiredBans(db *sql.DB, cutoff time.Time) (int64, error) {
	res, err := db.Exec(`
		DELETE FROM auth_bans
		WHERE datetime(banned_until) < datetime(?)
	`, cutoff.UTC().Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
