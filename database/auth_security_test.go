package database

import (
	"path/filepath"
	"testing"
	"time"
)

func TestSecurityMigrationInvalidatesLegacySessionsOnlyOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "migration.db")
	db, err := InitDatabase(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	lic, err := CreateLicense(db, "migration@example.invalid", 30, 1, "ADMIN")
	if err != nil {
		t.Fatal(err)
	}
	old, err := CreateAdminSession(db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`DELETE FROM security_migrations WHERE name='20260912-auth-v1'`); err != nil {
		t.Fatal(err)
	}
	if err = applyAuthSecurityMigration(db); err != nil {
		t.Fatal(err)
	}
	if ValidateAdminSession(db, old) == nil {
		t.Fatal("legacy session survived migration")
	}
	if _, err = GetLicense(db, lic.LicenseID); err != nil {
		t.Fatal("migration removed licence")
	}
	fresh, err := CreateAdminSession(db)
	if err != nil {
		t.Fatal(err)
	}
	if err = applyAuthSecurityMigration(db); err != nil {
		t.Fatal(err)
	}
	if err = ValidateAdminSession(db, fresh); err != nil {
		t.Fatal("idempotent migration removed fresh session")
	}
}

func TestTOTPReplayRejectedAcrossChallenges(t *testing.T) {
	db, err := InitDatabase(filepath.Join(t.TempDir(), "totp.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	lic, err := CreateLicense(db, "totp@example.invalid", 30, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	if err = SetCustomerPassword(db, lic.Email, "SyntheticPassword2026!"); err != nil {
		t.Fatal(err)
	}
	secret, _, recovery, err := SetupCustomerTOTP(db, lic.Email)
	if err != nil {
		t.Fatal(err)
	}
	code, err := CalculateTOTP(secret, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err = EnableCustomerTOTP(db, lic.Email, secret, code, recovery); err != nil {
		t.Fatal(err)
	}
	if _, _, err = ValidateTechnicianCredentialsWith2FA(db, lic.LicenseID, lic.LicenseKey, code); err != nil {
		t.Fatal(err)
	}
	if _, _, err = ValidateTechnicianCredentialsWith2FA(db, lic.LicenseID, lic.LicenseKey, code); err == nil {
		t.Fatal("same TOTP accepted twice")
	}
}

func TestAdminLogoutInvalidatesLicenseBoundSession(t *testing.T) {
	db, err := InitDatabase(filepath.Join(t.TempDir(), "logout.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	lic, err := CreateLicense(db, "logout@example.invalid", 30, 1, "ADMIN")
	if err != nil {
		t.Fatal(err)
	}
	token, err := CreateTechnicianSession(db, lic.LicenseID)
	if err != nil {
		t.Fatal(err)
	}
	if err = ValidateAdminSession(db, token); err != nil {
		t.Fatal(err)
	}
	if err = DeleteAdminSession(db, token); err != nil {
		t.Fatal(err)
	}
	if ValidateAdminSession(db, token) == nil {
		t.Fatal("admin logout left session valid")
	}
	if _, err = ValidateTechnicianSession(db, token); err == nil {
		t.Fatal("admin logout left technician session valid")
	}
}

func TestTechnicianLoginRejectsStaleValidatedLicense(t *testing.T) {
	db, err := InitDatabase(filepath.Join(t.TempDir(), "rotation.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	lic, err := CreateLicense(db, "rotation@example.invalid", 30, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE licences SET license_key=? WHERE license_id=?`, "synthetic-rotated-key", lic.LicenseID); err != nil {
		t.Fatal(err)
	}
	if _, _, err = completeTechnicianLogin(db, lic, lic.Email, "", ""); err == nil {
		t.Fatal("stale validated key still granted session")
	}
}
