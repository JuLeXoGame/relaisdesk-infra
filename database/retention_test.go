package database

import (
	"path/filepath"
	"testing"
	"time"
)

func TestPurgeExpiredOperationalDataHonorsRetentionBoundaries(t *testing.T) {
	db, err := InitDatabase(filepath.Join(t.TempDir(), "retention.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	license, err := CreateLicense(db, "tech@example.com", 365, 1, "retention test")
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		code    string
		expires time.Time
	}{
		{"OLD-CODE", now.AddDate(0, 0, -31)},
		{"NEW-CODE", now.AddDate(0, 0, -29)},
	} {
		if _, err := db.Exec(`INSERT INTO viewer_codes (code, technician_license_id, expires_at) VALUES (?, ?, ?)`, item.code, license.LicenseID, item.expires.Format(time.RFC3339)); err != nil {
			t.Fatal(err)
		}
	}
	for _, connected := range []time.Time{now.AddDate(-1, 0, -1), now.AddDate(0, -11, -29)} {
		if _, err := db.Exec(`INSERT INTO connections_log (license_id, client_ip, connected_at) VALUES (?, '192.0.2.1', ?)`, license.LicenseID, connected.Format(time.RFC3339)); err != nil {
			t.Fatal(err)
		}
	}
	for _, resolved := range []time.Time{now.AddDate(-1, 0, -1), now.AddDate(0, -11, -29)} {
		if _, err := db.Exec(`INSERT INTO security_alerts (type, message, resolved_at) VALUES ('test', 'test', ?)`, resolved.Format(time.RFC3339)); err != nil {
			t.Fatal(err)
		}
	}

	oldOrder, err := CreateOrder(db, "old@example.com", "starter", 1, "stripe", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE orders SET expires_at = ? WHERE order_id = ?`, now.AddDate(0, -4, 0).Format(time.RFC3339), oldOrder.OrderID); err != nil {
		t.Fatal(err)
	}
	oldWithdrawal, err := CreateWithdrawalRequest(db, oldOrder.OrderID, oldOrder.Email, "Client")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE withdrawal_requests SET requested_at = ? WHERE request_id = ?`, now.AddDate(-5, 0, -1).Format(time.RFC3339), oldWithdrawal.RequestID); err != nil {
		t.Fatal(err)
	}
	for index, status := range []string{"done", "dead", "pending"} {
		completedAt := any(nil)
		if status != "pending" {
			completedAt = now.AddDate(0, 0, -31).Format(time.RFC3339)
		}
		if _, err := db.Exec(`
			INSERT INTO jobs (job_type, payload, unique_key, status, available_at, completed_at)
			VALUES ('test', '{}', ?, ?, ?, ?)
		`, index, status, now.AddDate(0, 0, -31).Format(time.RFC3339), completedAt); err != nil {
			t.Fatal(err)
		}
	}

	result, err := PurgeExpiredOperationalData(db, now)
	if err != nil {
		t.Fatal(err)
	}
	if result.ViewerCodes != 1 || result.ConnectionLogs != 1 || result.SecurityAlerts != 1 || result.WithdrawalRecords != 1 || result.UnpaidOrders != 1 || result.CompletedJobs != 2 {
		t.Fatalf("résultat purge inattendu: %+v", result)
	}

	for table, want := range map[string]int{"viewer_codes": 1, "connections_log": 1, "security_alerts": 1, "withdrawal_requests": 0, "orders": 0, "jobs": 1} {
		var count int
		if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != want {
			t.Fatalf("%s: count = %d, want %d", table, count, want)
		}
	}
}
