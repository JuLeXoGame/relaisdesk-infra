package database

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTOTPProtectRoundtrip(t *testing.T) {
	a, err := protectTOTPSecret("JBSWY3DPEHPK3PXP")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(a, totpProtectedPrefix) || strings.Contains(a, "JBSWY3DPEHPK3PXP") {
		t.Fatalf("ciphertext suspect: %q", a)
	}
	b, err := protectTOTPSecret("JBSWY3DPEHPK3PXP")
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("deux chiffrements identiques : nonce non aléatoire ?")
	}
	plain, legacy, err := openTOTPSecret(a)
	if err != nil || legacy || plain != "JBSWY3DPEHPK3PXP" {
		t.Fatalf("open = %q legacy=%v err=%v", plain, legacy, err)
	}
}

func TestTOTPProtectRejectsTamperAndWrongKey(t *testing.T) {
	sealed, err := protectTOTPSecret("JBSWY3DPEHPK3PXP")
	if err != nil {
		t.Fatal(err)
	}
	tampered := sealed[:len(sealed)-2] + "AA"
	if _, _, err := openTOTPSecret(tampered); err == nil {
		t.Fatal("ciphertext altéré accepté")
	}
	if _, _, err := openTOTPSecret(totpProtectedPrefix + "!!!"); err == nil {
		t.Fatal("base64 invalide accepté")
	}
	SetTOTPDataKey([]byte("fedcba9876543210fedcba9876543210"))
	defer SetTOTPDataKey([]byte("0123456789abcdef0123456789abcdef"))
	if _, _, err := openTOTPSecret(sealed); err == nil {
		t.Fatal("mauvaise clé acceptée")
	}
}

func TestTOTPProtectValidatesInput(t *testing.T) {
	if _, err := protectTOTPSecret(""); err == nil {
		t.Fatal("secret vide accepté")
	}
	if _, err := protectTOTPSecret(strings.Repeat("x", 4097)); err == nil {
		t.Fatal("secret surdimensionné accepté")
	}
	SetTOTPDataKey(nil)
	defer SetTOTPDataKey([]byte("0123456789abcdef0123456789abcdef"))
	if _, err := protectTOTPSecret("JBSWY3DPEHPK3PXP"); err == nil {
		t.Fatal("écriture sans clé acceptée (fail-open)")
	}
}

func TestTOTPLegacyPlaintextStillVerifies(t *testing.T) {
	plain, legacy, err := openTOTPSecret("JBSWY3DPEHPK3PXP")
	if err != nil || !legacy || plain != "JBSWY3DPEHPK3PXP" {
		t.Fatalf("legacy = %q %v %v", plain, legacy, err)
	}
}

// Stored TOTP material must be ciphertext: the transient enrollment row, the
// account secret and the recovery blob. Legacy plaintext rows migrate to
// ciphertext on next read.
func TestTOTPStoredEncryptedEndToEnd(t *testing.T) {
	db, err := InitDatabase(filepath.Join(t.TempDir(), "totp_crypto.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	email := "crypto.2fa@relaisdesk.fr"
	ident, err := EnsureCustomer(db, email, "business", "Crypto")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if _, err := db.Exec(`INSERT INTO licences (customer_id, license_id, email, license_key, status, created_at, expires_at, max_connections) VALUES (?, 'LIC-CRYPTO-2FA', ?, ?, 'active', ?, ?, 1)`, ident.ID, email, HashLicenseKey("KEY-CRYPTO-2FA"), now.Format(time.RFC3339), now.Add(30*24*time.Hour).Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	if err := SetCustomerPassword(db, email, "CorrectHorseBatteryStaple123!"); err != nil {
		t.Fatal(err)
	}
	secret, _, recoveryCodes, err := SetupCustomerTOTP(db, email)
	if err != nil {
		t.Fatal(err)
	}
	var transient, transientRec string
	if err := db.QueryRow(`SELECT secret,recovery_codes FROM totp_enrollments WHERE email=?`, email).Scan(&transient, &transientRec); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(transient, totpProtectedPrefix) || !strings.HasPrefix(transientRec, totpProtectedPrefix) {
		t.Fatal("préparation 2FA stockée en clair")
	}
	code, err := CalculateTOTP(secret, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := EnableCustomerTOTP(db, email, secret, code, recoveryCodes); err != nil {
		t.Fatal(err)
	}
	var stored, storedRec string
	if err := db.QueryRow(`SELECT totp_secret,totp_recovery_codes FROM customer_accounts WHERE email=?`, email).Scan(&stored, &storedRec); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(stored, totpProtectedPrefix) || !strings.HasPrefix(storedRec, totpProtectedPrefix) {
		t.Fatal("secret ou secours persistants stockés en clair")
	}
	// The factor still verifies end to end.
	auth, err := ValidateCustomerPasswordWith2FA(db, email, "CorrectHorseBatteryStaple123!")
	if err != nil || !auth.Requires2FA {
		t.Fatalf("challenge attendu: %+v %v", auth, err)
	}
	code2, _ := CalculateTOTP(secret, time.Now().UTC())
	if _, _, _, err := VerifyCustomer2FAChallenge(db, auth.ChallengeToken, code2); err != nil {
		t.Fatalf("vérification: %v", err)
	}
	// Legacy plaintext rows migrate on read.
	legacyEmail := "legacy.2fa@relaisdesk.fr"
	if _, err := EnsureCustomer(db, legacyEmail, "business", "Legacy"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE customer_accounts SET totp_enabled=1,totp_secret=? WHERE email=?`, secret, legacyEmail); err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	enabled, opened, _, _, err := getMFAConfigTx(tx, "", legacyEmail)
	if err != nil || !enabled || opened != secret {
		tx.Rollback()
		t.Fatalf("legacy illisible: %v %v", opened, err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	var migrated string
	if err := db.QueryRow(`SELECT totp_secret FROM customer_accounts WHERE email=?`, legacyEmail).Scan(&migrated); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(migrated, totpProtectedPrefix) {
		t.Fatal("ligne legacy non migrée à la lecture")
	}
	if reopened, _, err := openTOTPSecret(migrated); err != nil || reopened != secret {
		t.Fatal("ligne migrée illisible")
	}
}
