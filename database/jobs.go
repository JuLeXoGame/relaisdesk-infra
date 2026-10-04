package database

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

type Job struct {
	ID          int64
	Type        string
	Payload     string
	UniqueKey   string
	Status      string
	Attempts    int
	MaxAttempts int
}

// MaxJobPayloadBytes caps queued payloads (legitimate ones are IDs and small
// JSON; even raw Stripe events stay far below). Unknown job types are
// rejected at processing time by the worker's default branch.
const MaxJobPayloadBytes = 256 * 1024

func EnqueueJob(db *sql.DB, jobType, payload, uniqueKey string, maxAttempts int, availableAt time.Time) (bool, error) {
	jobType = strings.TrimSpace(jobType)
	uniqueKey = strings.TrimSpace(uniqueKey)
	if jobType == "" || uniqueKey == "" || len(jobType) > 80 || len(uniqueKey) > 255 {
		return false, errors.New("type ou clé de travail invalide")
	}
	if len(payload) > MaxJobPayloadBytes {
		return false, errors.New("charge de travail trop volumineuse")
	}
	if maxAttempts <= 0 || maxAttempts > 100 {
		maxAttempts = 12
	}
	if availableAt.IsZero() {
		availableAt = time.Now().UTC()
	}
	result, err := db.Exec(`
		INSERT INTO jobs (job_type, payload, unique_key, max_attempts, available_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(unique_key) DO UPDATE SET
			status = 'pending', attempts = 0, available_at = excluded.available_at,
			locked_at = NULL, completed_at = NULL, last_error = NULL
		WHERE jobs.status = 'dead'
	`, jobType, payload, uniqueKey, maxAttempts, availableAt.UTC().Format(time.RFC3339))
	if err != nil {
		return false, fmt.Errorf("mise en file du travail: %w", err)
	}
	rows, err := result.RowsAffected()
	return rows == 1, err
}

// ClaimNextJob atomically leases one ready job. A crashed worker's lease is
// recovered after fifteen minutes without requiring a separate supervisor.
func ClaimNextJob(db *sql.DB, now time.Time) (*Job, error) {
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	now = now.UTC()
	var job Job
	err = tx.QueryRow(`
		SELECT id, job_type, payload, unique_key, status, attempts, max_attempts
		FROM jobs
		WHERE (status = 'pending' AND datetime(available_at) <= datetime(?))
		   OR (status = 'processing' AND datetime(locked_at) <= datetime(?))
		ORDER BY id ASC LIMIT 1
	`, now.Format(time.RFC3339), now.Add(-15*time.Minute).Format(time.RFC3339)).Scan(
		&job.ID, &job.Type, &job.Payload, &job.UniqueKey, &job.Status, &job.Attempts, &job.MaxAttempts,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	result, err := tx.Exec(`
		UPDATE jobs SET status = 'processing', locked_at = ?, attempts = attempts + 1
		WHERE id = ? AND ((status = 'pending' AND datetime(available_at) <= datetime(?))
		   OR (status = 'processing' AND datetime(locked_at) <= datetime(?)))
	`, now.Format(time.RFC3339), job.ID, now.Format(time.RFC3339), now.Add(-15*time.Minute).Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	rows, err := result.RowsAffected()
	if err != nil || rows != 1 {
		return nil, err
	}
	job.Status = "processing"
	job.Attempts++
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &job, nil
}

// ScrubJobPayload replaces a job's stored payload, used to purge one-time
// secrets (license keys) after successful delivery while keeping the row
// for audit. Only the claimed worker calls it.
func ScrubJobPayload(db *sql.DB, id int64, redacted string) error {
	_, err := db.Exec(`UPDATE jobs SET payload = ? WHERE id = ?`, redacted, id)
	return err
}

func CompleteJob(db *sql.DB, id int64) error {
	result, err := db.Exec(`
		UPDATE jobs SET status = 'done', completed_at = CURRENT_TIMESTAMP,
		locked_at = NULL, last_error = NULL WHERE id = ? AND status = 'processing'
	`, id)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil || rows != 1 {
		return errors.New("travail absent ou non verrouillé")
	}
	return nil
}

func FailJob(db *sql.DB, job *Job, processingErr error, now time.Time) error {
	if job == nil || processingErr == nil {
		return errors.New("travail et erreur requis")
	}
	message := processingErr.Error()
	if len(message) > 1000 {
		message = message[:1000]
	}
	status := "pending"
	if job.Attempts >= job.MaxAttempts {
		status = "dead"
	}
	delay := time.Minute * time.Duration(1<<min(max(job.Attempts-1, 0), 8))
	_, err := db.Exec(`
		UPDATE jobs SET status = ?, available_at = ?, locked_at = NULL, last_error = ?,
		completed_at = CASE WHEN ? = 'dead' THEN ? ELSE NULL END
		WHERE id = ? AND status = 'processing'
	`, status, now.UTC().Add(delay).Format(time.RFC3339), message, status,
		now.UTC().Format(time.RFC3339), job.ID)
	return err
}
