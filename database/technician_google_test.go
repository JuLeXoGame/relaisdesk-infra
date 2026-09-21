package database

import (
	"path/filepath"
	"testing"
	"time"
)

func TestTechnicianGoogleLogin(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "tech_google_test.db")
	db, err := InitDatabase(dbPath)
	if err != nil {
		t.Fatalf("InitDatabase failed: %v", err)
	}
	defer db.Close()

	testEmail := "google-tech@example.com"

	// 1. Try login before customer or license exists -> should fail
	_, _, err = ValidateTechnicianGoogle(db, testEmail, "")
	if err == nil {
		t.Fatal("Expected failure when no customer/license exists, got nil")
	}

	// 2. Create customer identity and active license
	_, err = EnsureCustomer(db, testEmail, "business", "Google Tech Corp")
	if err != nil {
		t.Fatalf("EnsureCustomer failed: %v", err)
	}

	lic, err := CreateLicense(db, testEmail, 30, 3, "Google Tech License")
	if err != nil {
		t.Fatalf("CreateLicense failed: %v", err)
	}

	// 3. Login with Google without 2FA -> should succeed directly
	authRes, returnedLic, err := ValidateTechnicianGoogle(db, testEmail, "")
	if err != nil {
		t.Fatalf("ValidateTechnicianGoogle failed: %v", err)
	}
	if authRes.Requires2FA {
		t.Fatal("Did not expect 2FA to be required for fresh account")
	}
	if authRes.SessionToken == "" {
		t.Fatal("Expected valid session token")
	}
	if returnedLic.LicenseID != lic.LicenseID {
		t.Fatalf("Expected license ID %s, got %s", lic.LicenseID, returnedLic.LicenseID)
	}

	// Verify session token validity
	sessLicID, err := ValidateTechnicianSession(db, authRes.SessionToken)
	if err != nil || sessLicID != lic.LicenseID {
		t.Fatalf("ValidateTechnicianSession failed: sessLicID=%s, err=%v", sessLicID, err)
	}

	// 4. Enable 2FA on account -> verify 2FA challenge is returned
	totpSecret := "JBSWY3DPEHPK3PXP"
	_, err = db.Exec(`UPDATE customer_accounts SET totp_enabled = 1, totp_secret = ? WHERE email = ?`, totpSecret, testEmail)
	if err != nil {
		t.Fatalf("Failed to enable TOTP: %v", err)
	}

	authRes2FA, _, err := ValidateTechnicianGoogle(db, testEmail, "")
	if err != nil {
		t.Fatalf("ValidateTechnicianGoogle with 2FA failed: %v", err)
	}
	if !authRes2FA.Requires2FA || authRes2FA.ChallengeToken == "" {
		t.Fatalf("Expected 2FA challenge, got requires2FA=%v, token=%s", authRes2FA.Requires2FA, authRes2FA.ChallengeToken)
	}

	// 5. Provide valid TOTP code in call -> direct session
	validCode, err := CalculateTOTP(totpSecret, time.Now().UTC())
	if err != nil {
		t.Fatalf("CalculateTOTP failed: %v", err)
	}
	authResWithCode, _, err := ValidateTechnicianGoogle(db, testEmail, validCode)
	if err != nil {
		t.Fatalf("ValidateTechnicianGoogle with direct TOTP code failed: %v", err)
	}
	if authResWithCode.Requires2FA || authResWithCode.SessionToken == "" {
		t.Fatalf("Expected immediate session with TOTP code, got requires2FA=%v", authResWithCode.Requires2FA)
	}
}
