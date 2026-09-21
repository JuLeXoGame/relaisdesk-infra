package database

import (
	"testing"
	"time"
)

func TestTOTPRFC6238Vectors(t *testing.T) {
	// RFC 6238 Appendix B test vector for SHA1:
	// Secret: "12345678901234567890" (ASCII 20 bytes) -> Base32: "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	secret := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"

	tests := []struct {
		unixTime int64
		expected string
	}{
		{59, "287082"},
		{1111111109, "081804"},
		{1111111111, "050471"},
		{1234567890, "005924"},
		{2000000000, "279037"},
	}

	for _, tc := range tests {
		got, err := CalculateTOTP(secret, time.Unix(tc.unixTime, 0))
		if err != nil {
			t.Fatalf("CalculateTOTP(%d) error: %v", tc.unixTime, err)
		}
		if got != tc.expected {
			t.Errorf("CalculateTOTP(%d) = %s; want %s", tc.unixTime, got, tc.expected)
		}
	}
}

func TestTOTPValidationAndWindow(t *testing.T) {
	secret, err := GenerateTOTPSecret()
	if err != nil {
		t.Fatalf("GenerateTOTPSecret() error: %v", err)
	}

	now := time.Now().UTC()
	currentCode, err := CalculateTOTP(secret, now)
	if err != nil {
		t.Fatalf("CalculateTOTP() error: %v", err)
	}

	// Current code should validate
	if !ValidateTOTPCode(secret, currentCode, now) {
		t.Errorf("ValidateTOTPCode() failed for current code %s", currentCode)
	}

	// Code from 25 seconds ago should validate (within ±1 step)
	pastCode, _ := CalculateTOTP(secret, now.Add(-25*time.Second))
	if !ValidateTOTPCode(secret, pastCode, now) {
		t.Errorf("ValidateTOTPCode() failed for past code %s", pastCode)
	}

	// Code from 25 seconds in future should validate (within ±1 step)
	futureCode, _ := CalculateTOTP(secret, now.Add(25*time.Second))
	if !ValidateTOTPCode(secret, futureCode, now) {
		t.Errorf("ValidateTOTPCode() failed for future code %s", futureCode)
	}

	// Code from 90 seconds ago should FAIL (outside ±1 step)
	oldCode, _ := CalculateTOTP(secret, now.Add(-90*time.Second))
	if ValidateTOTPCode(secret, oldCode, now) {
		t.Errorf("ValidateTOTPCode() unexpectedly accepted old code %s", oldCode)
	}

	// Invalid format code should fail
	if ValidateTOTPCode(secret, "12345", now) {
		t.Errorf("ValidateTOTPCode() unexpectedly accepted short code")
	}
	if ValidateTOTPCode(secret, "abcdef", now) {
		t.Errorf("ValidateTOTPCode() unexpectedly accepted alpha code")
	}
}

func TestRecoveryCodesGenerationAndConsumption(t *testing.T) {
	plainCodes, hashedCodes, err := GenerateRecoveryCodes(8)
	if err != nil {
		t.Fatalf("GenerateRecoveryCodes() error: %v", err)
	}
	if len(plainCodes) != 8 || len(hashedCodes) != 8 {
		t.Fatalf("Expected 8 codes, got %d plain, %d hashed", len(plainCodes), len(hashedCodes))
	}

	storedJSON := `["` + hashedCodes[0] + `","` + hashedCodes[1] + `","` + hashedCodes[2] + `"]`
	if CountRemainingRecoveryCodes(storedJSON) != 3 {
		t.Fatalf("CountRemainingRecoveryCodes() = %d; want 3", CountRemainingRecoveryCodes(storedJSON))
	}

	// Valid consumption of first code
	valid, updatedJSON, err := ValidateAndConsumeRecoveryCode(storedJSON, plainCodes[0])
	if err != nil || !valid {
		t.Fatalf("ValidateAndConsumeRecoveryCode() error: %v, valid: %v", err, valid)
	}
	if CountRemainingRecoveryCodes(updatedJSON) != 2 {
		t.Fatalf("CountRemainingRecoveryCodes() after consumption = %d; want 2", CountRemainingRecoveryCodes(updatedJSON))
	}

	// Reusing the same code should FAIL (single use)
	valid2, _, _ := ValidateAndConsumeRecoveryCode(updatedJSON, plainCodes[0])
	if valid2 {
		t.Fatalf("Reusing consumed recovery code should fail")
	}

	// Valid consumption of second code (case-insensitive and ignoring hyphens)
	unformatted := plainCodes[1]
	valid3, finalJSON, err := ValidateAndConsumeRecoveryCode(updatedJSON, unformatted)
	if err != nil || !valid3 {
		t.Fatalf("ValidateAndConsumeRecoveryCode() with unformatted code error: %v, valid: %v", err, valid3)
	}
	if CountRemainingRecoveryCodes(finalJSON) != 1 {
		t.Fatalf("CountRemainingRecoveryCodes() after second consumption = %d; want 1", CountRemainingRecoveryCodes(finalJSON))
	}
}
