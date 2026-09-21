package database

import (
	"database/sql"
	"encoding/json"
	"time"
)

// Persist intent and its Stripe retry in one transaction: recording a request
// without a job would otherwise allow billing to continue after a disk error.
func RequestSubscriptionCancellation(db *sql.DB, id string, now time.Time) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.Exec(`UPDATE trial_applications SET cancel_requested_at=COALESCE(cancel_requested_at,?) WHERE id=?`, now.Unix(), id)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return sql.ErrNoRows
	}
	payload, err := json.Marshal(map[string]string{"value": id})
	if err != nil {
		return err
	}
	_, err = tx.Exec(`INSERT INTO jobs(job_type,payload,unique_key,max_attempts) VALUES('trial_cancel',?,?,30)
        ON CONFLICT(unique_key) DO UPDATE SET status='pending',attempts=0,available_at=CURRENT_TIMESTAMP,
        locked_at=NULL,completed_at=NULL,last_error=NULL WHERE jobs.status='dead'`, string(payload), "trial-cancel:"+id)
	if err != nil {
		return err
	}
	return tx.Commit()
}
