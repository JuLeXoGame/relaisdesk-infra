package database

import (
	"database/sql"
	"fmt"
	"time"
)

func ListLicensesByCustomer(db *sql.DB, email string) ([]License, error) {
	identity, err := GetCustomerIdentityByEmail(db, email)
	if err != nil {
		return nil, err
	}
	return ListLicensesByCustomerID(db, identity.ID)
}

func ListLicensesByCustomerID(db *sql.DB, customerID int64) ([]License, error) {
	rows, err := db.Query(`
		SELECT id, license_id, email, license_key, status, created_at, expires_at,
		       max_connections, current_connections, last_connection_at, notes,
		       revoked_at, revoke_reason
		FROM licences WHERE customer_id = ? ORDER BY created_at DESC
	`, customerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]License, 0)
	for rows.Next() {
		item, err := scanLicense(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	return items, rows.Err()
}

func HasPendingRenewal(db *sql.DB, licenseID string) (bool, error) {
	var exists bool
	err := db.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM orders
			WHERE renewal_license_id = ? AND status IN ('pending', 'processing')
			  AND datetime(expires_at) > CURRENT_TIMESTAMP
		)
	`, licenseID).Scan(&exists)
	return exists, err
}

type RenewalReminderCandidate struct {
	LicenseID    string
	Email        string
	ExpiresAt    time.Time
	ReminderType string
}

// ListRenewalReminderCandidates returns at most one due reminder stage per
// licence. Stages are revisited as expiry approaches and deduplicated by the
// renewal_reminders table.
func ListRenewalReminderCandidates(db *sql.DB, now time.Time) ([]RenewalReminderCandidate, error) {
	rows, err := db.Query(`
		SELECT l.license_id, l.email, l.expires_at,
		       CASE
		         WHEN datetime(l.expires_at) <= datetime(?) THEN 'expired'
		         WHEN datetime(l.expires_at) <= datetime(?) THEN 'd1'
		         WHEN datetime(l.expires_at) <= datetime(?) THEN 'd3'
		         WHEN datetime(l.expires_at) <= datetime(?) THEN 'd7'
		         ELSE 'd14'
		       END
		FROM licences l
		LEFT JOIN customers c ON c.id = l.customer_id
		WHERE COALESCE(c.renewal_reminders_enabled, 1) = 1
		  AND l.status != 'revoked'
		  AND NOT EXISTS (SELECT 1 FROM trial_applications t WHERE t.license_id = l.license_id AND t.stripe_status NOT IN ('canceled', 'incomplete_expired'))
		  AND datetime(l.expires_at) > datetime(?)
		  AND datetime(l.expires_at) <= datetime(?)
		  AND NOT EXISTS (
			SELECT 1 FROM orders o
			WHERE o.renewal_license_id = l.license_id AND o.status IN ('pending', 'processing')
			  AND datetime(o.expires_at) > datetime(?)
		  )
		LIMIT 500
	`,
		now.UTC().Format(time.RFC3339), now.Add(24*time.Hour).UTC().Format(time.RFC3339),
		now.Add(3*24*time.Hour).UTC().Format(time.RFC3339), now.Add(7*24*time.Hour).UTC().Format(time.RFC3339),
		now.Add(-7*24*time.Hour).UTC().Format(time.RFC3339), now.Add(14*24*time.Hour).UTC().Format(time.RFC3339),
		now.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]RenewalReminderCandidate, 0)
	for rows.Next() {
		var item RenewalReminderCandidate
		var expiresRaw any
		if err := rows.Scan(&item.LicenseID, &item.Email, &expiresRaw, &item.ReminderType); err != nil {
			return nil, err
		}
		item.ExpiresAt, err = ParseSQLiteTime(expiresRaw)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// ClaimRenewalReminder prevents duplicate messages between API replicas and
// permits a retry after one hour when SMTP failed.
func ClaimRenewalReminder(db *sql.DB, candidate RenewalReminderCandidate, now time.Time) (bool, error) {
	cycle := candidate.ExpiresAt.UTC().Format(time.RFC3339)
	result, err := db.Exec(`
		INSERT INTO renewal_reminders (license_id, reminder_type, cycle_expires_at, claimed_at, attempts)
		VALUES (?, ?, ?, ?, 1)
		ON CONFLICT(license_id, reminder_type, cycle_expires_at) DO UPDATE SET
			claimed_at = excluded.claimed_at,
			attempts = renewal_reminders.attempts + 1
		WHERE renewal_reminders.sent_at IS NULL
		  AND datetime(renewal_reminders.claimed_at) <= datetime(?)
	`, candidate.LicenseID, candidate.ReminderType, cycle, now.UTC().Format(time.RFC3339), now.Add(-time.Hour).UTC().Format(time.RFC3339))
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows == 1, err
}

func FinishRenewalReminder(db *sql.DB, candidate RenewalReminderCandidate, sendErr error) error {
	cycle := candidate.ExpiresAt.UTC().Format(time.RFC3339)
	if sendErr == nil {
		_, err := db.Exec(`
			UPDATE renewal_reminders SET sent_at = CURRENT_TIMESTAMP, last_error = NULL
			WHERE license_id = ? AND reminder_type = ? AND cycle_expires_at = ?
		`, candidate.LicenseID, candidate.ReminderType, cycle)
		return err
	}
	message := sendErr.Error()
	if len(message) > 500 {
		message = message[:500]
	}
	_, err := db.Exec(`
		UPDATE renewal_reminders SET last_error = ?
		WHERE license_id = ? AND reminder_type = ? AND cycle_expires_at = ?
	`, message, candidate.LicenseID, candidate.ReminderType, cycle)
	if err != nil {
		return fmt.Errorf("enregistrement échec relance: %w", err)
	}
	return nil
}
