package database

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAdminAuthWithEmailAndPassword(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "admin_auth_test.db")
	db, err := InitDatabase(dbPath)
	if err != nil {
		t.Fatalf("InitDatabase failed: %v", err)
	}
	defer db.Close()

	adminEmail := "operator@relaisdesk.fr"
	adminLicenseID := "MP-ADMN-0001-TEST"
	adminLicenseKey := "mpsk_admin0000000000000000000000001"
	password := "SecretAdmin2026!"

	// 1. Insert admin lifetime license
	expiresFuture := time.Now().UTC().Add(36500 * 24 * time.Hour).Format("2006-01-02 15:04:05")
	_, err = db.Exec(`
		INSERT INTO licences (license_id, email, license_key, status, created_at, expires_at, max_connections, notes)
		VALUES (?, ?, ?, 'active', CURRENT_TIMESTAMP, ?, 50, 'ADMIN')
	`, adminLicenseID, adminEmail, HashLicenseKey(adminLicenseKey), expiresFuture)
	if err != nil {
		t.Fatalf("Failed to insert admin license: %v", err)
	}

	// 2. Test login BEFORE setting password -> should fail gracefully
	_, _, err = ValidateAdminEmailPassword(db, adminEmail, password)
	if err == nil || !strings.Contains(err.Error(), "aucun mot de passe") {
		t.Fatalf("Expected error about no password, got: %v", err)
	}

	// 3. Set password using license credentials
	setMail, err := SetAdminPasswordWithLicense(db, adminLicenseID, adminLicenseKey, password)
	if err != nil {
		t.Fatalf("SetAdminPasswordWithLicense failed: %v", err)
	}
	if setMail != adminEmail {
		t.Errorf("Expected email %s, got %s", adminEmail, setMail)
	}

	// 4. Test login with WRONG password -> should fail with incorrect password
	_, _, err = ValidateAdminEmailPassword(db, adminEmail, "WrongPassword123!")
	if err == nil || !strings.Contains(err.Error(), "mot de passe incorrect") {
		t.Fatalf("Expected error about incorrect password, got: %v", err)
	}

	// 5. Test login with VALID email & password -> should succeed
	authRes, lic, err := ValidateAdminEmailPassword(db, adminEmail, password)
	if err != nil {
		t.Fatalf("ValidateAdminEmailPassword failed: %v", err)
	}
	if authRes == nil || authRes.SessionToken == "" {
		t.Fatalf("Expected valid session token, got %+v", authRes)
	}
	if authRes.Role != "admin" {
		t.Errorf("Expected role admin, got %s", authRes.Role)
	}
	if authRes.Requires2FA {
		t.Errorf("Expected Requires2FA=false, got true")
	}
	if lic.LicenseID != adminLicenseID {
		t.Errorf("Expected licenseID %s, got %s", adminLicenseID, lic.LicenseID)
	}

	// Verify session in database
	licID, err := ValidateTechnicianSession(db, authRes.SessionToken)
	if err != nil {
		t.Fatalf("Session token was not valid in technician_sessions: %v", err)
	}
	if licID != adminLicenseID {
		t.Errorf("Expected technician session license %s, got %s", adminLicenseID, licID)
	}
	if err := ValidateAdminSession(db, authRes.SessionToken); err != nil {
		t.Errorf("Session token was not valid in admin_sessions: %v", err)
	}

	// 6. Test with ordinary user email (having license without ADMIN notes)
	userEmail := "user@client.fr"
	userLicID := "MP-USER-0002-TEST"
	userLicKey := "mpsk_user00000000000000000000000002"
	_, err = db.Exec(`
		INSERT INTO licences (license_id, email, license_key, status, created_at, expires_at, max_connections, notes)
		VALUES (?, ?, ?, 'active', CURRENT_TIMESTAMP, ?, 2, 'starter')
	`, userLicID, userEmail, HashLicenseKey(userLicKey), expiresFuture)
	if err != nil {
		t.Fatalf("Failed to insert user license: %v", err)
	}
	// Setup user password
	pwHash, _ := hashPassword(password)
	_, _ = db.Exec(`INSERT INTO customer_accounts (email, password_hash) VALUES (?, ?)`, userEmail, pwHash)

	_, _, err = ValidateAdminEmailPassword(db, userEmail, password)
	if err == nil || !strings.Contains(err.Error(), "aucune licence administrateur") {
		t.Fatalf("Expected error about no admin license for ordinary user, got: %v", err)
	}
}

func TestAdminAuthWith2FA(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "admin_auth_2fa_test.db")
	db, err := InitDatabase(dbPath)
	if err != nil {
		t.Fatalf("InitDatabase failed: %v", err)
	}
	defer db.Close()

	adminEmail := "secadmin@relaisdesk.fr"
	adminLicenseID := "MP-ADMN-2FA1-TEST"
	adminLicenseKey := "mpsk_admin2fa000000000000000000001"
	password := "StrongPassword2026!"

	expiresFuture := time.Now().UTC().Add(36500 * 24 * time.Hour).Format("2006-01-02 15:04:05")
	_, err = db.Exec(`
		INSERT INTO licences (license_id, email, license_key, status, created_at, expires_at, max_connections, notes)
		VALUES (?, ?, ?, 'active', CURRENT_TIMESTAMP, ?, 10, 'ADMIN')
	`, adminLicenseID, adminEmail, HashLicenseKey(adminLicenseKey), expiresFuture)
	if err != nil {
		t.Fatalf("Failed to insert admin license: %v", err)
	}

	_, err = SetAdminPasswordWithLicense(db, adminLicenseID, adminLicenseKey, password)
	if err != nil {
		t.Fatalf("SetAdminPasswordWithLicense failed: %v", err)
	}

	// Setup and enable 2FA on the admin account
	secret, _, recoveryCodes, err := SetupCustomerTOTP(db, adminEmail)
	if err != nil {
		t.Fatalf("SetupCustomerTOTP failed: %v", err)
	}
	validCode, err := CalculateTOTP(secret, time.Now().UTC())
	if err != nil {
		t.Fatalf("CalculateTOTP failed: %v", err)
	}
	if err := EnableCustomerTOTP(db, adminEmail, secret, validCode, recoveryCodes); err != nil {
		t.Fatalf("EnableCustomerTOTP failed: %v", err)
	}

	// 1. First step: login with email & password -> must return ChallengeToken and Requires2FA=true
	authRes, _, err := ValidateAdminEmailPassword(db, adminEmail, password)
	if err != nil {
		t.Fatalf("ValidateAdminEmailPassword with 2FA failed: %v", err)
	}
	if !authRes.Requires2FA || authRes.ChallengeToken == "" {
		t.Fatalf("Expected Requires2FA=true and challenge token, got %+v", authRes)
	}

	// 2. Second step: verify with TOTP code
	code2, _ := CalculateTOTP(secret, time.Now().UTC())
	finalRes, lic, err := VerifyAdmin2FAChallenge(db, authRes.ChallengeToken, code2)
	if err != nil {
		t.Fatalf("VerifyAdmin2FAChallenge failed: %v", err)
	}
	if finalRes.SessionToken == "" || finalRes.Role != "admin" {
		t.Fatalf("Expected valid admin session, got %+v", finalRes)
	}
	if lic.LicenseID != adminLicenseID {
		t.Errorf("Expected licenseID %s, got %s", adminLicenseID, lic.LicenseID)
	}

	// Verify session
	if err := ValidateAdminSession(db, finalRes.SessionToken); err != nil {
		t.Errorf("Admin session validation failed: %v", err)
	}
}
