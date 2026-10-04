package database

import (
	"path/filepath"
	"testing"
	"time"
)

func TestCustomer2FAFlow(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "customer_2fa_test.db")
	db, err := InitDatabase(dbPath)
	if err != nil {
		t.Fatalf("InitDatabase failed: %v", err)
	}
	defer db.Close()

	email := "customer.2fa@relaisdesk.fr"
	password := "CorrectHorseBatteryStaple123!"

	// Create customer account
	ident, err := EnsureCustomer(db, email, "business", "Client Test 2FA")
	if err != nil {
		t.Fatalf("EnsureCustomer failed: %v", err)
	}
	if ident.ID == 0 {
		t.Fatalf("expected non-zero ID")
	}

	// Insert active licence so customer is eligible
	now := time.Now().UTC()
	_, err = db.Exec(`
		INSERT INTO licences (customer_id, license_id, email, license_key, status, created_at, expires_at, max_connections)
		VALUES (?, 'LIC-CUST-2FA', ?, ?, 'active', ?, ?, 1)
	`, ident.ID, email, HashLicenseKey("KEY-CUST-2FA"), now.Format(time.RFC3339), now.Add(30*24*time.Hour).Format(time.RFC3339))
	if err != nil {
		t.Fatalf("failed to insert licence: %v", err)
	}

	// Set password
	err = SetCustomerPassword(db, email, password)
	if err != nil {
		t.Fatalf("SetCustomerPassword failed: %v", err)
	}

	// 1. Password login without 2FA
	authRes, err := ValidateCustomerPasswordWith2FA(db, email, password)
	if err != nil {
		t.Fatalf("ValidateCustomerPasswordWith2FA failed: %v", err)
	}
	if authRes.Requires2FA {
		t.Fatalf("expected requires_2fa=false initially")
	}
	if authRes.SessionToken == "" {
		t.Fatalf("expected session token")
	}

	// 2. Setup 2FA
	secret, otpauthURL, recoveryCodes, err := SetupCustomerTOTP(db, email)
	if err != nil {
		t.Fatalf("SetupCustomerTOTP failed: %v", err)
	}
	if secret == "" || otpauthURL == "" || len(recoveryCodes) != 8 {
		t.Fatalf("invalid setup return values")
	}

	// Wrong initial code fails activation
	err = EnableCustomerTOTP(db, email, secret, "123456", recoveryCodes)
	if err == nil {
		t.Fatalf("expected failure with wrong initial code")
	}

	// Correct initial code enables 2FA
	validCode, err := CalculateTOTP(secret, time.Now().UTC())
	if err != nil {
		t.Fatalf("CalculateTOTP failed: %v", err)
	}
	err = EnableCustomerTOTP(db, email, secret, validCode, recoveryCodes)
	if err != nil {
		t.Fatalf("EnableCustomerTOTP failed: %v", err)
	}

	// Check status
	enabled, confirmedAt, remaining, err := GetCustomerTOTPStatus(db, email)
	if err != nil || !enabled || confirmedAt == "" || remaining != 8 {
		t.Fatalf("GetCustomerTOTPStatus unexpected: enabled=%v, confirmedAt=%s, rem=%d, err=%v", enabled, confirmedAt, remaining, err)
	}

	// 3. Password login with 2FA enabled -> Challenge issued
	authRes2, err := ValidateCustomerPasswordWith2FA(db, email, password)
	if err != nil {
		t.Fatalf("ValidateCustomerPasswordWith2FA failed: %v", err)
	}
	if !authRes2.Requires2FA {
		t.Fatalf("expected requires_2fa=true")
	}
	if authRes2.ChallengeToken == "" {
		t.Fatalf("expected challenge token")
	}
	if authRes2.SessionToken != "" {
		t.Fatalf("expected empty session token")
	}

	// Verify challenge with invalid code
	_, _, _, err = VerifyCustomer2FAChallenge(db, authRes2.ChallengeToken, "999999")
	if err == nil {
		t.Fatalf("expected error on invalid code")
	}

	// Verify challenge with valid TOTP code
	totpCode, err := CalculateTOTP(secret, time.Now().UTC())
	if err != nil {
		t.Fatalf("CalculateTOTP failed: %v", err)
	}
	sessToken, custID, verifiedEmail, err := VerifyCustomer2FAChallenge(db, authRes2.ChallengeToken, totpCode)
	if err != nil {
		t.Fatalf("VerifyCustomer2FAChallenge failed: %v", err)
	}
	if sessToken == "" || custID == "" || verifiedEmail != email {
		t.Fatalf("unexpected challenge verification result: %s, %s, %s", sessToken, custID, verifiedEmail)
	}

	// 4. Recovery codes regeneration
	newCodes, err := RegenerateCustomerRecoveryCodes(db, email, password, recoveryCodes[1])
	if err != nil {
		t.Fatalf("RegenerateCustomerRecoveryCodes failed: %v", err)
	}
	if len(newCodes) != 8 {
		t.Fatalf("expected 8 regenerated codes")
	}

	// 5. Login using a recovery code
	authRes3, err := ValidateCustomerPasswordWith2FA(db, email, password)
	if err != nil || !authRes3.Requires2FA {
		t.Fatalf("failed to get challenge for recovery code login: %v", err)
	}
	sessTokenRec, _, _, err := VerifyCustomer2FAChallenge(db, authRes3.ChallengeToken, newCodes[0])
	if err != nil {
		t.Fatalf("VerifyCustomer2FAChallenge with recovery code failed: %v", err)
	}
	if sessTokenRec == "" {
		t.Fatalf("expected valid session with recovery code")
	}

	// Check 7 codes remain
	_, _, remAfterRec, err := GetCustomerTOTPStatus(db, email)
	if err != nil || remAfterRec != 7 {
		t.Fatalf("expected 7 remaining codes, got %d", remAfterRec)
	}

	// 6. Disable 2FA
	err = DisableCustomerTOTP(db, email, password, newCodes[1])
	if err != nil {
		t.Fatalf("DisableCustomerTOTP failed: %v", err)
	}

	enabledAfter, _, _, err := GetCustomerTOTPStatus(db, email)
	if err != nil || enabledAfter {
		t.Fatalf("expected 2FA disabled, got enabled=%v", enabledAfter)
	}
}

func TestCustomer2FAEmailCodeAndTrustedDevice(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "customer_2fa_device_test.db")
	db, err := InitDatabase(dbPath)
	if err != nil {
		t.Fatalf("InitDatabase failed: %v", err)
	}
	defer db.Close()

	email := "user-device@example.com"
	password := "SecurePassword123!"

	ident, err := EnsureCustomer(db, email, "business", "Device User")
	if err != nil {
		t.Fatalf("EnsureCustomer failed: %v", err)
	}

	now := time.Now().UTC()
	_, err = db.Exec(`
		INSERT INTO licences (customer_id, license_id, email, license_key, status, created_at, expires_at, max_connections)
		VALUES (?, 'LIC-DEVICE-2FA', ?, ?, 'active', ?, ?, 1)
	`, ident.ID, email, HashLicenseKey("KEY-DEVICE-2FA"), now.Format(time.RFC3339), now.Add(30*24*time.Hour).Format(time.RFC3339))
	if err != nil {
		t.Fatalf("failed to insert licence: %v", err)
	}

	if err := SetCustomerPassword(db, email, password); err != nil {
		t.Fatalf("SetCustomerPassword failed: %v", err)
	}

	// Enable 2FA
	secret, _, recoveryCodes, err := SetupCustomerTOTP(db, email)
	if err != nil {
		t.Fatalf("SetupCustomerTOTP failed: %v", err)
	}
	totpCode, _ := CalculateTOTP(secret, time.Now().UTC())
	if err := EnableCustomerTOTPWithPassword(db, email, password, secret, totpCode, recoveryCodes); err != nil {
		t.Fatalf("EnableCustomerTOTPWithPassword failed: %v", err)
	}

	// 1. Password login returns 2FA challenge
	authRes, err := ValidateCustomerPasswordWith2FA(db, email, password)
	if err != nil || !authRes.Requires2FA || authRes.ChallengeToken == "" {
		t.Fatalf("expected 2FA challenge: %v, %+v", err, authRes)
	}

	// 2. Request email 2FA code
	code, codeEmail, err := SendCustomer2FAEmailCode(db, authRes.ChallengeToken)
	if err != nil || len(code) != 6 || codeEmail != email {
		t.Fatalf("SendCustomer2FAEmailCode failed: %v, code=%s, email=%s", err, code, codeEmail)
	}

	// Immediate re-request should be throttled (< 30s)
	_, _, err = SendCustomer2FAEmailCode(db, authRes.ChallengeToken)
	if err == nil {
		t.Fatalf("expected rate limit error on immediate re-request")
	}

	// 3. Verify challenge with the email code and rememberDevice=true
	sessToken, custID, verifiedEmail, deviceToken, err := VerifyCustomer2FAChallengeWithDevice(db, authRes.ChallengeToken, code, true, "Firefox on Windows")
	if err != nil {
		t.Fatalf("VerifyCustomer2FAChallengeWithDevice failed: %v", err)
	}
	if sessToken == "" || custID == "" || verifiedEmail != email || deviceToken == "" {
		t.Fatalf("unexpected verification result: sess=%s, dev=%s", sessToken, deviceToken)
	}

	// 4. Subsequent login with deviceToken skips 2FA
	authResTrusted, err := ValidateCustomerPasswordWith2FA(db, email, password, deviceToken)
	if err != nil || authResTrusted.Requires2FA || authResTrusted.SessionToken == "" {
		t.Fatalf("expected trusted device to bypass 2FA: %v, %+v", err, authResTrusted)
	}

	// 5. Revoking trusted devices forces 2FA again
	if err := RevokeCustomerTrustedDevices(db, email); err != nil {
		t.Fatalf("RevokeCustomerTrustedDevices failed: %v", err)
	}
	authResAfterRevoke, err := ValidateCustomerPasswordWith2FA(db, email, password, deviceToken)
	if err != nil || !authResAfterRevoke.Requires2FA {
		t.Fatalf("expected 2FA required after trusted devices revocation: %v, %+v", err, authResAfterRevoke)
	}
}

func TestCustomerPasswordLockout(t *testing.T) {
	db, err := InitDatabase(filepath.Join(t.TempDir(), "customer_lockout_test.db"))
	if err != nil {
		t.Fatalf("InitDatabase failed: %v", err)
	}
	defer db.Close()

	email := "lockout@relaisdesk.fr"
	password := "CorrectHorseBatteryStaple123!"
	ident, err := EnsureCustomer(db, email, "business", "Lockout User")
	if err != nil {
		t.Fatalf("EnsureCustomer failed: %v", err)
	}
	now := time.Now().UTC()
	if _, err = db.Exec(`
		INSERT INTO licences (customer_id, license_id, email, license_key, status, created_at, expires_at, max_connections)
		VALUES (?, 'LIC-LOCKOUT', ?, ?, 'active', ?, ?, 1)
	`, ident.ID, email, HashLicenseKey("KEY-LOCKOUT"), now.Format(time.RFC3339), now.Add(30*24*time.Hour).Format(time.RFC3339)); err != nil {
		t.Fatalf("failed to insert licence: %v", err)
	}
	if err := SetCustomerPassword(db, email, password); err != nil {
		t.Fatalf("SetCustomerPassword failed: %v", err)
	}

	// A few failures followed by a success reset the counter.
	for i := 0; i < 3; i++ {
		if _, err := ValidateCustomerPasswordWith2FA(db, email, "wrong-password"); err == nil {
			t.Fatal("wrong password accepted")
		}
	}
	if _, err := ValidateCustomerPasswordWith2FA(db, email, password); err != nil {
		t.Fatalf("correct password rejected: %v", err)
	}
	var attempts int
	if err := db.QueryRow("SELECT failed_password_attempts FROM customer_accounts WHERE email=?", email).Scan(&attempts); err != nil || attempts != 0 {
		t.Fatalf("counter after success = %d, %v", attempts, err)
	}

	// Ten consecutive failures lock the account, even for the right password.
	for i := 0; i < 10; i++ {
		if _, err := ValidateCustomerPasswordWith2FA(db, email, "wrong-password"); err == nil {
			t.Fatal("wrong password accepted")
		}
	}
	if _, err := ValidateCustomerPasswordWith2FA(db, email, password); err == nil {
		t.Fatal("locked account accepted the correct password")
	}

	// An expired lockout releases the account with a clean slate.
	if _, err := db.Exec("UPDATE customer_accounts SET password_locked_until = ? WHERE email = ?",
		time.Now().UTC().Add(-time.Minute).Format(time.RFC3339), email); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateCustomerPasswordWith2FA(db, email, password); err != nil {
		t.Fatalf("released account rejected: %v", err)
	}
	if err := db.QueryRow("SELECT failed_password_attempts FROM customer_accounts WHERE email=?", email).Scan(&attempts); err != nil || attempts != 0 {
		t.Fatalf("counter after release = %d, %v", attempts, err)
	}
}
