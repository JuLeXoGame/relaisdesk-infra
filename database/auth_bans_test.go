package database

import (
	"path/filepath"
	"testing"
	"time"
)

func TestAuthBansLifecycle(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test_auth_bans.db")
	db, err := InitDatabase(dbPath)
	if err != nil {
		t.Fatalf("InitDatabase() error: %v", err)
	}
	defer db.Close()

	testIP := "198.51.100.42"

	// 1. Initial state: not banned
	banned, _, err := IsIPBanned(db, testIP, time.Now().UTC())
	if err != nil {
		t.Fatalf("IsIPBanned() error: %v", err)
	}
	if banned {
		t.Fatalf("expected IP not to be banned initially")
	}

	// 2. Register 4 failures with threshold of 5 -> should NOT be banned
	for i := 1; i <= 4; i++ {
		isBanned, _, err := RecordAuthFailure(db, testIP, 5, 15*time.Minute, "test_failure")
		if err != nil {
			t.Fatalf("RecordAuthFailure() error at attempt %d: %v", i, err)
		}
		if isBanned {
			t.Fatalf("IP should not be banned after %d failures", i)
		}
	}

	// 3. 5th failure -> should trigger ban
	isBanned, bannedUntil, err := RecordAuthFailure(db, testIP, 5, 15*time.Minute, "admin_login_failure")
	if err != nil {
		t.Fatalf("RecordAuthFailure() 5th error: %v", err)
	}
	if !isBanned {
		t.Fatalf("expected IP to be banned after 5 failures")
	}
	if !bannedUntil.After(time.Now().UTC()) {
		t.Fatalf("expected bannedUntil to be in the future, got: %v", bannedUntil)
	}

	// 4. Verify IsIPBanned
	banned, gotUntil, err := IsIPBanned(db, testIP, time.Now().UTC())
	if err != nil {
		t.Fatalf("IsIPBanned() error: %v", err)
	}
	if !banned {
		t.Fatalf("IsIPBanned() should return true for banned IP")
	}
	if gotUntil.Unix() != bannedUntil.Unix() {
		t.Fatalf("expected bannedUntil %v, got %v", bannedUntil, gotUntil)
	}

	// 5. Verify security_alerts entry was created
	alerts, err := ListSecurityAlerts(db, 10)
	if err != nil {
		t.Fatalf("ListSecurityAlerts() error: %v", err)
	}
	if len(alerts) == 0 {
		t.Fatalf("expected at least 1 security alert to be recorded for ban")
	}
	found := false
	for _, a := range alerts {
		if a.Type == "auth_brute_force" && a.FirstIP == testIP {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected security alert of type auth_brute_force for IP %s", testIP)
	}

	// 6. Test LoadActiveBans (startup reload simulation)
	activeBans, err := LoadActiveBans(db, time.Now().UTC())
	if err != nil {
		t.Fatalf("LoadActiveBans() error: %v", err)
	}
	if _, ok := activeBans[testIP]; !ok {
		t.Fatalf("expected activeBans to contain %s", testIP)
	}

	// 7. Verify unban / success
	// If ban has expired, RecordAuthSuccess clears it
	if err := RecordAuthSuccess(db, testIP); err != nil {
		t.Fatalf("RecordAuthSuccess() error: %v", err)
	}

	// 8. Test PurgeExpiredBans
	deleted, err := PurgeExpiredBans(db, time.Now().UTC().Add(1*time.Hour))
	if err != nil {
		t.Fatalf("PurgeExpiredBans() error: %v", err)
	}
	if deleted == 0 {
		t.Fatalf("expected PurgeExpiredBans to delete expired ban")
	}
}
