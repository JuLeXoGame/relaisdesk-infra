package handlers

import (
	"api/config"
	"api/mailer"
	dbpkg "database"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestQueueCartReminders(t *testing.T) {
	db, err := dbpkg.InitDatabase(t.TempDir() + "/cart-reminders.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	now := time.Now().UTC().Truncate(time.Second)
	o, err := dbpkg.CreateOrder(db, "cart@example.com", "starter", 1, "stripe", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE orders SET created_at = ?, expires_at = ? WHERE order_id = ?`,
		now.Add(-30*time.Hour).Format(time.RFC3339), now.Add(18*time.Hour).Format(time.RFC3339), o.OrderID); err != nil {
		t.Fatal(err)
	}

	queued, err := QueueCartReminders(db, now)
	if err != nil {
		t.Fatal(err)
	}
	if queued != 1 {
		t.Fatalf("first pass queued = %d, want 1", queued)
	}
	queued, err = QueueCartReminders(db, now)
	if err != nil {
		t.Fatal(err)
	}
	if queued != 0 {
		t.Fatalf("second pass queued = %d, want 0 (dedup)", queued)
	}

	var payload string
	if err := db.QueryRow(`SELECT payload FROM jobs WHERE unique_key = ?`, "cart-reminder-r1:"+o.OrderID).Scan(&payload); err != nil {
		t.Fatalf("job missing: %v", err)
	}
	var decoded struct {
		OrderID string `json:"order_id"`
		Round   int    `json:"round"`
	}
	if err := json.Unmarshal([]byte(payload), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.OrderID != o.OrderID || decoded.Round != 1 {
		t.Errorf("payload = %+v", decoded)
	}
}

func TestProcessCartReminderEmail(t *testing.T) {
	db, err := dbpkg.InitDatabase(t.TempDir() + "/cart-process.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	o, err := dbpkg.CreateOrder(db, "cart2@example.com", "starter", 1, "stripe", "", "")
	if err != nil {
		t.Fatal(err)
	}
	job := &dbpkg.Job{Payload: `{"order_id":"` + o.OrderID + `","round":1}`}

	// Unconfigured SMTP surfaces the mailer error (job retried later).
	cfg := &config.Config{PublicWebsiteURL: "https://relaisdesk.fr"}
	err = processCartReminderEmail(db, cfg, &mailer.Mailer{}, job)
	if err == nil || !strings.Contains(err.Error(), "SMTP non configuré") {
		t.Fatalf("err = %v, want SMTP failure", err)
	}

	// Paid since enqueue: silently skipped.
	if _, err := db.Exec(`UPDATE orders SET status = 'paid' WHERE order_id = ?`, o.OrderID); err != nil {
		t.Fatal(err)
	}
	if err := processCartReminderEmail(db, cfg, &mailer.Mailer{}, job); err != nil {
		t.Fatalf("paid order err = %v, want nil", err)
	}
}
