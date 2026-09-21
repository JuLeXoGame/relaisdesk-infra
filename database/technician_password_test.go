package database

import (
	"path/filepath"
	"testing"
	"time"
)

func TestTechnicianEmailPasswordLogin(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "tech_pwd_test.db")
	db, err := InitDatabase(dbPath)
	if err != nil {
		t.Fatalf("InitDatabase failed: %v", err)
	}
	defer db.Close()

	testEmail := "tech@example.com"
	testPassword := "SuperSecr3tPassword!"

	// 1. Create customer and account with password
	_, err = EnsureCustomer(db, testEmail, "business", "Test Company")
	if err != nil {
		t.Fatalf("EnsureCustomer failed: %v", err)
	}

	if err := SetCustomerPassword(db, testEmail, testPassword); err != nil {
		t.Fatalf("SetCustomerPassword failed: %v", err)
	}

	// 2. Try login BEFORE any active license is issued -> must fail with no active license
	_, _, err = ValidateTechnicianEmailPassword(db, testEmail, testPassword, "")
	if err == nil {
		t.Fatal("Expected error because no license is active, got nil")
	}

	// 3. Create active license linked to customer
	lic, err := CreateLicense(db, testEmail, 30, 2, "Test license")
	if err != nil {
		t.Fatalf("CreateLicense failed: %v", err)
	}

	// 4. Test with WRONG password -> must fail
	_, _, err = ValidateTechnicianEmailPassword(db, testEmail, "WrongPassword123!", "")
	if err == nil {
		t.Fatal("Expected error with wrong password, got nil")
	}

	// 5. Test with VALID password -> must succeed
	authRes, returnedLic, err := ValidateTechnicianEmailPassword(db, testEmail, testPassword, "")
	if err != nil {
		t.Fatalf("ValidateTechnicianEmailPassword failed: %v", err)
	}
	if authRes.Requires2FA {
		t.Fatal("Did not expect 2FA to be required")
	}
	if authRes.SessionToken == "" {
		t.Fatal("Expected non-empty session token")
	}
	if returnedLic.LicenseID != lic.LicenseID {
		t.Fatalf("Expected license ID %s, got %s", lic.LicenseID, returnedLic.LicenseID)
	}

	// Verify session was created
	sessLicID, err := ValidateTechnicianSession(db, authRes.SessionToken)
	if err != nil || sessLicID != lic.LicenseID {
		t.Fatalf("Session validation failed: lic=%s, err=%v", sessLicID, err)
	}

	// 6. Test with 2FA enabled on the account
	totpSecret := "JBSWY3DPEHPK3PXP" // standard base32 secret
	_, err = db.Exec(`UPDATE customer_accounts SET totp_enabled = 1, totp_secret = ? WHERE email = ?`, totpSecret, testEmail)
	if err != nil {
		t.Fatalf("Failed to enable TOTP: %v", err)
	}

	// Attempt without TOTP code -> should return 2FA challenge
	authRes2FA, _, err := ValidateTechnicianEmailPassword(db, testEmail, testPassword, "")
	if err != nil {
		t.Fatalf("ValidateTechnicianEmailPassword with 2FA failed: %v", err)
	}
	if !authRes2FA.Requires2FA || authRes2FA.ChallengeToken == "" {
		t.Fatalf("Expected 2FA challenge, got requires2FA=%v, token=%s", authRes2FA.Requires2FA, authRes2FA.ChallengeToken)
	}

	// Attempt with valid TOTP code in initial request
	validCode, err := CalculateTOTP(totpSecret, time.Now().UTC())
	if err != nil {
		t.Fatalf("CalculateTOTP failed: %v", err)
	}
	authResWithCode, _, err := ValidateTechnicianEmailPassword(db, testEmail, testPassword, validCode)
	if err != nil {
		t.Fatalf("ValidateTechnicianEmailPassword with direct TOTP code failed: %v", err)
	}
	if authResWithCode.Requires2FA || authResWithCode.SessionToken == "" {
		t.Fatalf("Expected immediate session with direct TOTP code, got requires2FA=%v", authResWithCode.Requires2FA)
	}
}
