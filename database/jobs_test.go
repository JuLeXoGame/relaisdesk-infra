package database

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestPersistentJobQueueDeduplicatesRetriesAndCompletes(t *testing.T) {
	db, err := InitDatabase(filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().UTC()
	inserted, err := EnqueueJob(db, "email", `{}`, "email:1", 3, now)
	if err != nil || !inserted {
		t.Fatalf("enqueue=%v err=%v", inserted, err)
	}
	inserted, err = EnqueueJob(db, "email", `{}`, "email:1", 3, now)
	if err != nil || inserted {
		t.Fatalf("duplicate enqueue=%v err=%v", inserted, err)
	}
	job, err := ClaimNextJob(db, now)
	if err != nil || job == nil || job.Attempts != 1 {
		t.Fatalf("claim=%+v err=%v", job, err)
	}
	if err := FailJob(db, job, errors.New("smtp down"), now); err != nil {
		t.Fatal(err)
	}
	if claimed, err := ClaimNextJob(db, now); err != nil || claimed != nil {
		t.Fatalf("backoff ignored: %+v %v", claimed, err)
	}
	if _, err := db.Exec(`UPDATE jobs SET available_at = ? WHERE id = ?`, now.Add(-time.Minute).Format(time.RFC3339), job.ID); err != nil {
		t.Fatal(err)
	}
	job, err = ClaimNextJob(db, now)
	if err != nil || job == nil || job.Attempts != 2 {
		t.Fatalf("retry=%+v err=%v", job, err)
	}
	if err := CompleteJob(db, job.ID); err != nil {
		t.Fatal(err)
	}
	if claimed, err := ClaimNextJob(db, now.Add(time.Hour)); err != nil || claimed != nil {
		t.Fatalf("completed job reclaimed: %+v %v", claimed, err)
	}
}

func TestDeadJobGetsCompletionDateForRetention(t *testing.T) {
	db, err := InitDatabase(filepath.Join(t.TempDir(), "dead-job.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().UTC()
	if _, err := EnqueueJob(db, "email", `{}`, "email:dead", 1, now); err != nil {
		t.Fatal(err)
	}
	job, err := ClaimNextJob(db, now)
	if err != nil || job == nil {
		t.Fatalf("claim=%+v err=%v", job, err)
	}
	if err := FailJob(db, job, errors.New("permanent failure"), now); err != nil {
		t.Fatal(err)
	}
	var status string
	var completed bool
	if err := db.QueryRow(`SELECT status, completed_at IS NOT NULL FROM jobs WHERE id = ?`, job.ID).Scan(&status, &completed); err != nil {
		t.Fatal(err)
	}
	if status != "dead" || !completed {
		t.Fatalf("status=%q completed=%v", status, completed)
	}
}
