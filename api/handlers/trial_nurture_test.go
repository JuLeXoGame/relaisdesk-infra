package handlers

import (
	"api/mailer"
	dbpkg "database"
	"strings"
	"testing"
	"time"
)

func TestQueueTrialNurture(t *testing.T) {
	db, _, _ := billingFixture(t)
	now := time.Now().UTC().Truncate(time.Second)

	mk := func(email string, start, end time.Time, state, stripeStatus string) string {
		t.Helper()
		a := newBillingTrial(t, db, email)
		if _, err := db.Exec(`UPDATE trial_applications SET state = ?, stripe_status = ?, trial_start = ?, trial_end = ? WHERE id = ?`,
			state, stripeStatus, start.Unix(), end.Unix(), a.ID); err != nil {
			t.Fatal(err)
		}
		return a.ID
	}
	nurtureID := mk("nurture@example.com", now.Add(-4*24*time.Hour), now.Add(26*24*time.Hour), "active", "trialing")
	finalID := mk("final@example.com", now.Add(-27*24*time.Hour), now.Add(2*24*time.Hour), "active", "trialing")
	mk("fresh@example.com", now.Add(-24*time.Hour), now.Add(29*24*time.Hour), "active", "trialing")
	mk("ended@example.com", now.Add(-31*24*time.Hour), now.Add(-time.Hour), "active", "trialing")

	if err := QueueTrialNurture(db, now); err != nil {
		t.Fatal(err)
	}
	assertJob := func(key, kind string) {
		t.Helper()
		var payload string
		if err := db.QueryRow(`SELECT payload FROM jobs WHERE unique_key = ?`, key).Scan(&payload); err != nil {
			t.Fatalf("job %s missing: %v", key, err)
		}
		if !strings.Contains(payload, `"kind":"`+kind+`"`) {
			t.Errorf("job %s payload = %s", key, payload)
		}
	}
	assertJob("trial-nurture:"+nurtureID, "nurture")
	assertJob("trial-reminder-final:"+finalID, "reminder_final")

	// Idempotent second pass.
	if err := QueueTrialNurture(db, now); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE unique_key LIKE 'trial-nurture:%' OR unique_key LIKE 'trial-reminder-final:%'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("jobs = %d, want 2 (no duplicates)", count)
	}

	// Cancelled trials are skipped by the sender guard.
	if _, err := db.Exec(`UPDATE trial_applications SET cancel_requested_at = ? WHERE id = ?`, now.Unix(), nurtureID); err != nil {
		t.Fatal(err)
	}
}

func TestTrialNurtureSendGuards(t *testing.T) {
	db, cfg, _ := billingFixture(t)
	now := time.Now().UTC().Truncate(time.Second)
	a := newBillingTrial(t, db, "guard@example.com")
	if _, err := db.Exec(`UPDATE trial_applications SET state = 'active', stripe_status = 'trialing', trial_start = ?, trial_end = ? WHERE id = ?`,
		now.Add(-4*24*time.Hour).Unix(), now.Add(26*24*time.Hour).Unix(), a.ID); err != nil {
		t.Fatal(err)
	}

	// Reaches the mailer (which fails unconfigured) → dispatch works.
	err := processTrialEmail(db, cfg, &mailer.Mailer{}, &dbpkg.Job{Payload: `{"id":"` + a.ID + `","kind":"nurture"}`})
	if err == nil || !strings.Contains(err.Error(), "SMTP non configuré") {
		t.Fatalf("nurture err = %v, want SMTP failure", err)
	}

	// Cancelled → silently skipped.
	if _, err := db.Exec(`UPDATE trial_applications SET cancel_requested_at = ? WHERE id = ?`, now.Unix(), a.ID); err != nil {
		t.Fatal(err)
	}
	if err := processTrialEmail(db, cfg, &mailer.Mailer{}, &dbpkg.Job{Payload: `{"id":"` + a.ID + `","kind":"nurture"}`}); err != nil {
		t.Fatalf("cancelled nurture err = %v, want nil", err)
	}
	if err := processTrialEmail(db, cfg, &mailer.Mailer{}, &dbpkg.Job{Payload: `{"id":"` + a.ID + `","kind":"reminder_final"}`}); err != nil {
		t.Fatalf("cancelled final err = %v, want nil", err)
	}
}
