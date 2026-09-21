package database

import (
	"path/filepath"
	"testing"
	"time"
)

func TestGoogleLoginCannotBypassMFA(t *testing.T) {
	db, err := InitDatabase(filepath.Join(t.TempDir(), "google-security.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	const email = "google-mfa@gmail.com"
	if _, err = CreateLicense(db, email, 30, 5, "Pro"); err != nil {
		t.Fatal(err)
	}
	secret, _, recovery, err := SetupCustomerTOTP(db, email)
	if err != nil {
		t.Fatal(err)
	}
	code, err := CalculateTOTP(secret, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err = EnableCustomerTOTP(db, email, secret, code, recovery); err != nil {
		t.Fatal(err)
	}
	if token, _, _, err := CreateCustomerGoogleSession(db, email); err == nil || token != "" {
		t.Fatal("Google login issued a session without the enabled second factor")
	}
	var sessions int
	if err = db.QueryRow("SELECT COUNT(*) FROM customer_sessions WHERE email=?", email).Scan(&sessions); err != nil {
		t.Fatal(err)
	}
	if sessions != 0 {
		t.Fatal("Google login persisted a session before MFA")
	}
	result, err := AuthenticateCustomerGoogle(db, email)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Requires2FA || result.ChallengeToken == "" || result.SessionToken != "" {
		t.Fatal("missing second-factor challenge")
	}
	if _, _, err = SendCustomer2FAEmailCode(db, result.ChallengeToken); err == nil {
		t.Fatal("Google mailbox accepted as its own second factor")
	}
	if _, _, _, err = VerifyCustomer2FAChallenge(db, result.ChallengeToken, "invalid"); err == nil {
		t.Fatal("invalid factor accepted")
	}
	token, _, _, err := VerifyCustomer2FAChallenge(db, result.ChallengeToken, recovery[0])
	if err != nil || token == "" {
		t.Fatalf("recovery factor rejected: %v", err)
	}
	if _, _, _, err = VerifyCustomer2FAChallenge(db, result.ChallengeToken, recovery[0]); err == nil {
		t.Fatal("challenge reused")
	}
}

func TestGoogleSecurityMigrationExpiresOldMFASessionsOnlyOnce(t *testing.T) {
	db, err := InitDatabase(filepath.Join(t.TempDir(), "migration.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	const email = "old-google@gmail.com"
	lic, err := CreateLicense(db, email, 30, 5, "Pro")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE customer_accounts SET totp_enabled=1 WHERE email=?`, email); err != nil {
		t.Fatal(err)
	}
	insert := func(token, address string) {
		t.Helper()
		if _, err := db.Exec(`INSERT INTO customer_sessions(token_hash,email,expires_at,last_used_at) VALUES (?,?,?,?)`, hashSessionToken(token), address, time.Now().Add(time.Hour), time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	insert("legacy-mfa", email)
	if _, err = CreateLicense(db, "other@example.com", 30, 1, "Starter"); err != nil {
		t.Fatal(err)
	}
	insert("unaffected", "other@example.com")
	if _, err = CreateTechnicianSession(db, lic.LicenseID); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`DELETE FROM security_migrations WHERE name='20260919-google-mfa-v1'`); err != nil {
		t.Fatal(err)
	}
	if err = applyAuthSecurityMigration(db); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = db.QueryRow(`SELECT COUNT(*) FROM customer_sessions WHERE email=?`, email).Scan(&count); err != nil || count != 0 {
		t.Fatalf("legacy MFA session survived: %d %v", count, err)
	}
	if err = db.QueryRow(`SELECT COUNT(*) FROM technician_sessions WHERE license_id=?`, lic.LicenseID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("legacy technician exchange survived: %d %v", count, err)
	}
	if err = db.QueryRow(`SELECT COUNT(*) FROM customer_sessions`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("unrelated session removed: %d %v", count, err)
	}
	insert("new-mfa", email)
	if err = applyAuthSecurityMigration(db); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT COUNT(*) FROM customer_sessions WHERE email=?`, email).Scan(&count); err != nil || count != 1 {
		t.Fatalf("migration not idempotent: %d %v", count, err)
	}
}
