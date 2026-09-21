package database

import (
	"path/filepath"
	"testing"
)

func TestCustomerPasswordAuthentication(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "customer_password_test.db")
	db, err := InitDatabase(dbPath)
	if err != nil {
		t.Fatalf("InitDatabase() error: %v", err)
	}
	defer db.Close()

	email := "client@entreprise.fr"

	// 1. Create a customer with an active license
	lic, err := CreateLicense(db, email, 30, 1, "Notes")
	if err != nil {
		t.Fatalf("CreateLicense() error: %v", err)
	}
	if lic == nil {
		t.Fatalf("licence nil")
	}

	// 2. Initial state: customer eligible, but has NO password set
	_, _, hasPassword, err := ValidateCustomerPassword(db, email, "anyPassword123")
	if err == nil {
		t.Fatalf("expected error when validating against account without password")
	}
	if hasPassword {
		t.Fatalf("expected hasPassword to be false initially")
	}

	// 3. Set password directly
	if err := SetCustomerPassword(db, email, "Short"); err == nil {
		t.Fatalf("expected error for password shorter than 8 chars")
	}
	if err := SetCustomerPassword(db, email, "SuperSecret2026!"); err != nil {
		t.Fatalf("SetCustomerPassword() error: %v", err)
	}

	// 4. Validate with wrong password
	_, _, hasPassword, err = ValidateCustomerPassword(db, email, "WrongPassword2026!")
	if err == nil {
		t.Fatalf("expected error with wrong password")
	}
	if !hasPassword {
		t.Fatalf("expected hasPassword to be true after password was set")
	}

	// 5. Validate with correct password
	sessionToken, customerID, hasPassword, err := ValidateCustomerPassword(db, email, "SuperSecret2026!")
	if err != nil {
		t.Fatalf("ValidateCustomerPassword() error: %v", err)
	}
	if !hasPassword || sessionToken == "" || customerID == "" {
		t.Fatalf("unexpected result: hasPassword=%v, token=%q, id=%q", hasPassword, sessionToken, customerID)
	}

	// 6. Verify session is recognized as valid
	gotEmail, err := ValidateCustomerSession(db, sessionToken)
	if err != nil || gotEmail != email {
		t.Fatalf("ValidateCustomerSession() got %q, err: %v", gotEmail, err)
	}

	// 7. Test password change
	if err := ChangeCustomerPassword(db, email, "WrongOldPassword", "NewPassword2026!"); err == nil {
		t.Fatalf("expected error when old password does not match")
	}
	if err := ChangeCustomerPassword(db, email, "SuperSecret2026!", "NewPassword2026!"); err != nil {
		t.Fatalf("ChangeCustomerPassword() error: %v", err)
	}
	// Verify new password works and old one fails
	_, _, _, err = ValidateCustomerPassword(db, email, "SuperSecret2026!")
	if err == nil {
		t.Fatalf("old password should no longer work")
	}
	_, _, _, err = ValidateCustomerPassword(db, email, "NewPassword2026!")
	if err != nil {
		t.Fatalf("new password should work: %v", err)
	}

	// 8. Test set password with reset token
	token, eligible, err := CreateCustomerLoginToken(db, email)
	if err != nil || !eligible || token == "" {
		t.Fatalf("CreateCustomerLoginToken() error: %v, eligible=%v", err, eligible)
	}
	tokenSession, tokenEmail, tokenCustID, err := SetCustomerPasswordWithToken(db, token, "ResetTokenPassword2026!")
	if err != nil {
		t.Fatalf("SetCustomerPasswordWithToken() error: %v", err)
	}
	if tokenEmail != email || tokenSession == "" || tokenCustID == "" {
		t.Fatalf("unexpected token reset result: email=%q, session=%q, custID=%q", tokenEmail, tokenSession, tokenCustID)
	}

	// Token must now be consumed
	_, _, _, err = SetCustomerPasswordWithToken(db, token, "ShouldFailTokenConsumed!")
	if err == nil {
		t.Fatalf("consumed token should be rejected")
	}

	// The reset password must now be the active password
	_, _, _, err = ValidateCustomerPassword(db, email, "ResetTokenPassword2026!")
	if err != nil {
		t.Fatalf("password set via token should work: %v", err)
	}

	// 9. Test Google Session creation for eligible customer
	googleSession, googleEmail, googleCustID, err := CreateCustomerGoogleSession(db, email)
	if err != nil {
		t.Fatalf("CreateCustomerGoogleSession() error: %v", err)
	}
	if googleEmail != email || googleSession == "" || googleCustID == "" {
		t.Fatalf("unexpected Google session result: email=%q, session=%q, custID=%q", googleEmail, googleSession, googleCustID)
	}

	// 10. Test Google Session creation for unknown/ineligible customer
	_, _, _, err = CreateCustomerGoogleSession(db, "unknown@randomdomain.fr")
	if err == nil {
		t.Fatalf("expected error for ineligible customer Google login")
	}
}

