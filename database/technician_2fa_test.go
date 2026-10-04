package database

import (
	"path/filepath"
	"testing"
	"time"
)

func TestTechnician2FAFlow(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "tech_2fa_test.db")
	db, err := InitDatabase(dbPath)
	if err != nil {
		t.Fatalf("InitDatabase failed: %v", err)
	}
	defer db.Close()

	email := "tech.test@relaisdesk.fr"
	licID := "LIC-TECH-2FA-01"
	licKey := "KEY-TECH-SECRET-99"

	// Create customer account
	ident, err := EnsureCustomer(db, email, "business", "Acme Tech")
	if err != nil {
		t.Fatalf("EnsureCustomer failed: %v", err)
	}

	// Create license
	now := time.Now().UTC()
	_, err = db.Exec(`
		INSERT INTO licences (customer_id, license_id, email, license_key, status, created_at, expires_at, max_connections, current_connections)
		VALUES (?, ?, ?, ?, 'active', ?, ?, 3, 0)
	`, ident.ID, licID, email, HashLicenseKey(licKey), now.Format(time.RFC3339), now.Add(30*24*time.Hour).Format(time.RFC3339))
	if err != nil {
		t.Fatalf("failed to insert license: %v", err)
	}

	// Case 1: 2FA disabled -> Direct login
	authRes, lic, err := ValidateTechnicianCredentialsWith2FA(db, licID, licKey, "")
	if err != nil {
		t.Fatalf("login without 2FA failed: %v", err)
	}
	if authRes.Requires2FA {
		t.Fatalf("expected requires_2fa=false, got true")
	}
	if authRes.SessionToken == "" {
		t.Fatalf("expected valid session token")
	}
	if lic.LicenseID != licID {
		t.Fatalf("expected license %s, got %s", licID, lic.LicenseID)
	}

	// Case 2: Enable 2FA on customer account
	secret, _, recoveryCodes, err := SetupCustomerTOTP(db, email)
	if err != nil {
		t.Fatalf("SetupCustomerTOTP failed: %v", err)
	}
	initialCode, err := CalculateTOTP(secret, time.Now().UTC())
	if err != nil {
		t.Fatalf("CalculateTOTP failed: %v", err)
	}
	err = EnableCustomerTOTP(db, email, secret, initialCode, recoveryCodes)
	if err != nil {
		t.Fatalf("EnableCustomerTOTP failed: %v", err)
	}

	// Now try login without code -> Should require 2FA
	authRes, _, err = ValidateTechnicianCredentialsWith2FA(db, licID, licKey, "")
	if err != nil {
		t.Fatalf("ValidateTechnicianCredentialsWith2FA failed: %v", err)
	}
	if !authRes.Requires2FA {
		t.Fatalf("expected requires_2fa=true")
	}
	if authRes.ChallengeToken == "" {
		t.Fatalf("expected challenge token")
	}
	if authRes.SessionToken != "" {
		t.Fatalf("expected no session token yet")
	}

	// Try verifying challenge with wrong code
	_, _, err = VerifyTechnician2FAChallenge(db, authRes.ChallengeToken, "000000")
	if err == nil {
		t.Fatalf("expected error for invalid code, got nil")
	}

	// Verify challenge with valid TOTP code
	totpCode, err := CalculateTOTP(secret, time.Now().UTC())
	if err != nil {
		t.Fatalf("CalculateTOTP failed: %v", err)
	}
	sessToken, lic2, err := VerifyTechnician2FAChallenge(db, authRes.ChallengeToken, totpCode)
	if err != nil {
		t.Fatalf("VerifyTechnician2FAChallenge with valid TOTP failed: %v", err)
	}
	if sessToken == "" {
		t.Fatalf("expected non-empty session token")
	}
	if lic2.LicenseID != licID {
		t.Fatalf("expected license %s, got %s", licID, lic2.LicenseID)
	}

	// Re-using the same challenge token must fail (burned)
	_, _, err = VerifyTechnician2FAChallenge(db, authRes.ChallengeToken, totpCode)
	if err == nil {
		t.Fatalf("expected error reusing burned challenge token")
	}

	// Case 3: Verify with Recovery Code
	authRes2, _, err := ValidateTechnicianCredentialsWith2FA(db, licID, licKey, "")
	if err != nil || !authRes2.Requires2FA {
		t.Fatalf("expected 2FA challenge, err=%v", err)
	}
	usedRecCode := recoveryCodes[0]
	sessTokenRec, _, err := VerifyTechnician2FAChallenge(db, authRes2.ChallengeToken, usedRecCode)
	if err != nil {
		t.Fatalf("VerifyTechnician2FAChallenge with recovery code failed: %v", err)
	}
	if sessTokenRec == "" {
		t.Fatalf("expected non-empty session token with recovery code")
	}

	// Verify recovery code was consumed
	_, _, rem, err := GetCustomerTOTPStatus(db, email)
	if err != nil {
		t.Fatalf("GetCustomerTOTPStatus failed: %v", err)
	}
	if rem != len(recoveryCodes)-1 {
		t.Fatalf("expected %d remaining recovery codes, got %d", len(recoveryCodes)-1, rem)
	}

	// Case 4: One-shot login with optionalCode supplied
	oneShotCode := recoveryCodes[1]
	authRes3, _, err := ValidateTechnicianCredentialsWith2FA(db, licID, licKey, oneShotCode)
	if err != nil {
		t.Fatalf("one-shot login with TOTP failed: %v", err)
	}
	if authRes3.Requires2FA {
		t.Fatalf("expected requires_2fa=false for one-shot, got true")
	}
	if authRes3.SessionToken == "" {
		t.Fatalf("expected non-empty session token for one-shot")
	}

	// Case 5: Email Code generation and verification
	authResEmail, _, err := ValidateTechnicianCredentialsWith2FA(db, licID, licKey, "")
	if err != nil || !authResEmail.Requires2FA {
		t.Fatalf("expected 2FA challenge for email test, err=%v", err)
	}
	emailCode, codeEmail, err := SendTechnician2FAEmailCode(db, authResEmail.ChallengeToken)
	if err != nil {
		t.Fatalf("SendTechnician2FAEmailCode failed: %v", err)
	}
	if codeEmail != email {
		t.Fatalf("expected email %s, got %s", email, codeEmail)
	}
	if len(emailCode) != 6 {
		t.Fatalf("expected 6-digit email code, got %s", emailCode)
	}

	// Rate limit check: requesting immediately again should fail
	_, _, err = SendTechnician2FAEmailCode(db, authResEmail.ChallengeToken)
	if err == nil {
		t.Fatalf("expected rate-limit error, got nil")
	}

	// Verify challenge with email code and rememberDevice=true
	sessTokenEmail, _, devToken, err := VerifyTechnician2FAChallengeWithDevice(db, authResEmail.ChallengeToken, emailCode, true, "Mon PC Portable")
	if err != nil {
		t.Fatalf("VerifyTechnician2FAChallengeWithDevice failed with email code: %v", err)
	}
	if sessTokenEmail == "" {
		t.Fatalf("expected session token with email code")
	}
	if devToken == "" {
		t.Fatalf("expected non-empty device token when rememberDevice=true")
	}

	// Case 6: Subsequent login with trusted device token should bypass 2FA
	authResTrusted, _, err := ValidateTechnicianCredentialsWith2FA(db, licID, licKey, "", devToken)
	if err != nil {
		t.Fatalf("login with trusted device token failed: %v", err)
	}
	if authResTrusted.Requires2FA {
		t.Fatalf("expected 2FA bypass with valid trusted device, but requires_2fa=true")
	}
	if authResTrusted.SessionToken == "" {
		t.Fatalf("expected session token with trusted device")
	}
}
