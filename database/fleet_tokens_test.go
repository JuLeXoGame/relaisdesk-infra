package database

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestParkTokenLifecycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "park_lifecycle.db")
	db, err := InitDatabase(path)
	if err != nil {
		t.Fatalf("InitDatabase: %v", err)
	}
	defer db.Close()
	lic, err := CreateLicense(db, "parc@example.test", 365, 5, "")
	if err != nil {
		t.Fatalf("CreateLicense: %v", err)
	}

	tok, plaintext, err := CreateParkEnrollmentToken(db, 0, lic.LicenseID, "Vague GPO janvier", "", 3, 30)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !strings.HasPrefix(plaintext, "PARK-") || len(plaintext) != 44 { // 5 + 32 symboles + 7 tirets
		t.Fatalf("format token inattendu: %q", plaintext)
	}
	if tok.UseCount != 0 || !tok.IsActive {
		t.Fatalf("état initial inattendu: %+v", tok)
	}

	listed, err := ListParkEnrollmentTokens(db, 0, lic.LicenseID)
	if err != nil || len(listed) != 1 || listed[0].ID != tok.ID {
		t.Fatalf("list = %+v, err = %v", listed, err)
	}

	// Enroll two machines with the same token.
	for i, host := range []string{"PC-001", "PC-002"} {
		pub, key, _ := ed25519.GenerateKey(rand.Reader)
		public := base64.RawURLEncoding.EncodeToString(pub)
		proof := signedDeviceTestProof(key, "enroll", plaintext, "9000000"+string(rune('1'+i)), host, "windows", false)
		dev, err := EnrollDeviceWithFullInfo(db, plaintext, "9000000"+string(rune('1'+i)), host, "windows", public, "10.0.0.1", "", "", "test-agent", proof)
		if err != nil {
			t.Fatalf("enroll %s: %v", host, err)
		}
		if dev.EnrollmentVersion != 2 {
			t.Fatalf("version = %d, want 2", dev.EnrollmentVersion)
		}
		if dev.Alias != host {
			t.Fatalf("alias = %q, want hostname %q", dev.Alias, host)
		}
		if dev.DeviceID == "" {
			t.Fatal("device_id vide")
		}
	}
	listed, _ = ListParkEnrollmentTokens(db, 0, lic.LicenseID)
	if listed[0].UseCount != 2 {
		t.Fatalf("use_count = %d, want 2", listed[0].UseCount)
	}

	// Third enroll OK (cap 3), fourth refused.
	pub3, key3, _ := ed25519.GenerateKey(rand.Reader)
	proof3 := signedDeviceTestProof(key3, "enroll", plaintext, "90000003", "PC-003", "windows", false)
	if _, err := EnrollDeviceWithFullInfo(db, plaintext, "90000003", "PC-003", "windows", base64.RawURLEncoding.EncodeToString(pub3), "10.0.0.1", "", "", "test-agent", proof3); err != nil {
		t.Fatalf("enroll 3: %v", err)
	}
	pub4, key4, _ := ed25519.GenerateKey(rand.Reader)
	proof4 := signedDeviceTestProof(key4, "enroll", plaintext, "90000004", "PC-004", "windows", false)
	if _, err := EnrollDeviceWithFullInfo(db, plaintext, "90000004", "PC-004", "windows", base64.RawURLEncoding.EncodeToString(pub4), "10.0.0.1", "", "", "test-agent", proof4); !errors.Is(err, ErrDeviceLimit) {
		t.Fatalf("enroll 4 = %v, want ErrDeviceLimit", err)
	}

	// Revoke: further enrolls refused, but already enrolled devices keep
	// their identity (heartbeat still works).
	if err := RevokeParkEnrollmentToken(db, 0, lic.LicenseID, tok.ID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	pub5, key5, _ := ed25519.GenerateKey(rand.Reader)
	proof5 := signedDeviceTestProof(key5, "enroll", plaintext, "90000005", "PC-005", "windows", false)
	if _, err := EnrollDeviceWithFullInfo(db, plaintext, "90000005", "PC-005", "windows", base64.RawURLEncoding.EncodeToString(pub5), "10.0.0.1", "", "", "test-agent", proof5); !errors.Is(err, ErrDeviceAuthorization) {
		t.Fatalf("enroll révoqué = %v, want ErrDeviceAuthorization", err)
	}
}

// A lost enroll response replayed by the same install (same proof key, same
// RustDesk ID) must return the existing device without burning another token
// use; a fresh key still enrolls a new device.
func TestParkTokenEnrollIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "park_idem.db")
	db, err := InitDatabase(path)
	if err != nil {
		t.Fatalf("InitDatabase: %v", err)
	}
	defer db.Close()
	lic, err := CreateLicense(db, "parc-idem@example.test", 365, 5, "")
	if err != nil {
		t.Fatalf("CreateLicense: %v", err)
	}
	_, plaintext, err := CreateParkEnrollmentToken(db, 0, lic.LicenseID, "rejouabilité", "", 10, 30)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	pub, key, _ := ed25519.GenerateKey(rand.Reader)
	public := base64.RawURLEncoding.EncodeToString(pub)
	first, err := EnrollDeviceWithFullInfo(db, plaintext, "90000101", "PC-REJOUÉ", "windows", public, "10.0.0.1", "", "", "test-agent",
		signedDeviceTestProof(key, "enroll", plaintext, "90000101", "PC-REJOUÉ", "windows", false))
	if err != nil {
		t.Fatalf("enroll 1: %v", err)
	}
	replay, err := EnrollDeviceWithFullInfo(db, plaintext, "90000101", "PC-REJOUÉ", "windows", public, "10.0.0.1", "", "", "test-agent",
		signedDeviceTestProof(key, "enroll", plaintext, "90000101", "PC-REJOUÉ", "windows", false))
	if err != nil {
		t.Fatalf("rejouement: %v", err)
	}
	if replay.DeviceID != first.DeviceID {
		t.Fatalf("rejouement a créé %q, want %q (doublon)", replay.DeviceID, first.DeviceID)
	}
	listed, _ := ListParkEnrollmentTokens(db, 0, lic.LicenseID)
	if listed[0].UseCount != 1 {
		t.Fatalf("use_count = %d, want 1 après rejouement", listed[0].UseCount)
	}
	// A reinstall (fresh proof key) still enrolls a new device.
	pub2, key2, _ := ed25519.GenerateKey(rand.Reader)
	second, err := EnrollDeviceWithFullInfo(db, plaintext, "90000101", "PC-REJOUÉ", "windows", base64.RawURLEncoding.EncodeToString(pub2), "10.0.0.1", "", "", "test-agent",
		signedDeviceTestProof(key2, "enroll", plaintext, "90000101", "PC-REJOUÉ", "windows", false))
	if err != nil {
		t.Fatalf("réinstallation: %v", err)
	}
	if second.DeviceID == first.DeviceID {
		t.Fatal("réinstallation fusionnée à tort avec l'ancien poste")
	}
}

func TestParkTokenRejectsBadInput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "park_bad.db")
	db, err := InitDatabase(path)
	if err != nil {
		t.Fatalf("InitDatabase: %v", err)
	}
	defer db.Close()
	lic, err := CreateLicense(db, "parc2@example.test", 365, 5, "")
	if err != nil {
		t.Fatalf("CreateLicense: %v", err)
	}
	if _, _, err := CreateParkEnrollmentToken(db, 0, lic.LicenseID, "x", "", 0, 400); err == nil {
		t.Fatal("TTL 400 jours accepté")
	}
	if _, _, err := CreateParkEnrollmentToken(db, 0, lic.LicenseID, "x", "", 100001, 30); err == nil {
		t.Fatal("max_uses démesuré accepté")
	}
	if _, _, err := CreateParkEnrollmentToken(db, 0, lic.LicenseID, "x", "dossier-inexistant", 10, 30); err == nil {
		t.Fatal("dossier inconnu accepté")
	}
	if _, _, err := CreateParkEnrollmentToken(db, 0, "LIC-INCONNUE", "x", "", 10, 30); !errors.Is(err, ErrDeviceAuthorization) {
		t.Fatalf("licence inconnue = %v, want ErrDeviceAuthorization", err)
	}
}
