package database

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"path/filepath"
	"testing"
)

func TestQueueAndPopDeviceUpdate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test_device_update.db")
	db, err := InitDatabase(path)
	if err != nil {
		t.Fatalf("InitDatabase failed: %v", err)
	}
	defer db.Close()

	cust, err := EnsureCustomer(db, "update-owner@example.com", "business", "Update Corp")
	if err != nil {
		t.Fatalf("EnsureCustomer: %v", err)
	}

	lic, err := CreateLicense(db, "update-owner@example.com", 365, 5, "Update Pro")
	if err != nil {
		t.Fatalf("CreateLicense: %v", err)
	}
	if _, err := db.Exec("UPDATE licences SET customer_id = ? WHERE license_id = ?", cust.ID, lic.LicenseID); err != nil {
		t.Fatalf("link license: %v", err)
	}

	// Create and enroll device
	dev, err := CreatePermanentEnrollment(db, cust.ID, lic.LicenseID, "Serveur Comptable", "Bureau 101")
	if err != nil {
		t.Fatalf("CreatePermanentEnrollment: %v", err)
	}

	pub, key, _ := ed25519.GenerateKey(rand.Reader)
	public := base64.RawURLEncoding.EncodeToString(pub)
	enrollProof := signedDeviceTestProof(key, "enroll", dev.PermanentCode, "112233445", "COMPTA-PC", "windows", false)

	enrolled, err := EnrollDeviceWithFullInfo(db, dev.PermanentCode, "112233445", "COMPTA-PC", "windows", public, "192.168.1.50", "00:11:22:33:44:55", "192.168.1.255", "1.0.1", enrollProof)
	if err != nil {
		t.Fatalf("EnrollDeviceWithFullInfo: %v", err)
	}

	if enrolled.AgentVersion != "1.0.1" {
		t.Fatalf("expected AgentVersion '1.0.1', got '%s'", enrolled.AgentVersion)
	}

	// 1. Queue an update request
	err = QueueDeviceUpdate(db, enrolled.DeviceID, cust.ID, lic.LicenseID, "1.0.2")
	if err != nil {
		t.Fatalf("QueueDeviceUpdate: %v", err)
	}

	// 2. Immediate duplicate must be throttled
	err = QueueDeviceUpdate(db, enrolled.DeviceID, cust.ID, lic.LicenseID, "1.0.2")
	if err != ErrUpdateAlreadyPending {
		t.Fatalf("expected ErrUpdateAlreadyPending, got: %v", err)
	}

	// 3. Pop pending update
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("Begin tx: %v", err)
	}
	targetVer, err := PopPendingDeviceUpdate(tx, enrolled.DeviceID)
	if err != nil {
		tx.Rollback()
		t.Fatalf("PopPendingDeviceUpdate: %v", err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatalf("Commit tx: %v", err)
	}

	if targetVer != "1.0.2" {
		t.Fatalf("expected targetVersion '1.0.2', got '%s'", targetVer)
	}

	// 4. Second pop should return empty
	tx2, err := db.Begin()
	if err != nil {
		t.Fatalf("Begin tx2: %v", err)
	}
	targetVer2, err := PopPendingDeviceUpdate(tx2, enrolled.DeviceID)
	if err != nil {
		tx2.Rollback()
		t.Fatalf("PopPendingDeviceUpdate 2: %v", err)
	}
	_ = tx2.Commit()
	if targetVer2 != "" {
		t.Fatalf("expected empty targetVer on second pop, got '%s'", targetVer2)
	}

	// 5. Update reported version
	err = UpdateDeviceAgentVersion(db, enrolled.DeviceID, "1.0.2")
	if err != nil {
		t.Fatalf("UpdateDeviceAgentVersion: %v", err)
	}
	devCheck, err := GetDeviceByID(db, enrolled.DeviceID)
	if err != nil {
		t.Fatalf("GetDeviceByID: %v", err)
	}
	if devCheck.AgentVersion != "1.0.2" {
		t.Fatalf("expected updated AgentVersion '1.0.2', got '%s'", devCheck.AgentVersion)
	}
}
