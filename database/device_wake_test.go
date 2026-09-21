package database

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"path/filepath"
	"testing"
)

func TestDeviceWakeAndNetworkLifecycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test_device_wake.db")
	db, err := InitDatabase(path)
	if err != nil {
		t.Fatalf("InitDatabase failed: %v", err)
	}
	defer db.Close()

	cust, err := EnsureCustomer(db, "wake-owner@example.com", "business", "Wake Corp")
	if err != nil {
		t.Fatalf("EnsureCustomer: %v", err)
	}

	lic, err := CreateLicense(db, "wake-owner@example.com", 365, 5, "Wake Pro")
	if err != nil {
		t.Fatalf("CreateLicense: %v", err)
	}
	if _, err := db.Exec("UPDATE licences SET customer_id = ? WHERE license_id = ?", cust.ID, lic.LicenseID); err != nil {
		t.Fatalf("link license: %v", err)
	}

	// 1. Create permanent enrollment for Device 1 (sleeping target)
	dev1, err := CreatePermanentEnrollment(db, cust.ID, lic.LicenseID, "PC-Cible", "Bureau 1")
	if err != nil {
		t.Fatalf("CreatePermanentEnrollment dev1: %v", err)
	}

	pub1, key1, _ := ed25519.GenerateKey(rand.Reader)
	public1 := base64.RawURLEncoding.EncodeToString(pub1)
	proof1 := signedDeviceTestProof(key1, "enroll", dev1.PermanentCode, "111111111", "PC-CIBLE", "windows", false)

	testMAC := "00:11:22:33:44:55"
	testBroadcast := "192.168.1.255"
	enrolled1, err := EnrollDeviceWithNetwork(db, dev1.PermanentCode, "111111111", "PC-CIBLE", "windows", public1, "192.168.1.50", testMAC, testBroadcast, proof1)
	if err != nil {
		t.Fatalf("EnrollDeviceWithNetwork dev1: %v", err)
	}
	if enrolled1.MACAddress != testMAC || enrolled1.SubnetBroadcast != testBroadcast {
		t.Fatalf("expected MAC %s, got %s; broadcast %s, got %s", testMAC, enrolled1.MACAddress, testBroadcast, enrolled1.SubnetBroadcast)
	}

	// 2. Create permanent enrollment for Device 2 (online relay peer)
	dev2, err := CreatePermanentEnrollment(db, cust.ID, lic.LicenseID, "PC-Relais", "Accueil")
	if err != nil {
		t.Fatalf("CreatePermanentEnrollment dev2: %v", err)
	}

	pub2, key2, _ := ed25519.GenerateKey(rand.Reader)
	public2 := base64.RawURLEncoding.EncodeToString(pub2)
	proof2 := signedDeviceTestProof(key2, "enroll", dev2.PermanentCode, "222222222", "PC-RELAIS", "windows", false)

	_, err = EnrollDeviceWithNetwork(db, dev2.PermanentCode, "222222222", "PC-RELAIS", "windows", public2, "192.168.1.60", "AA:BB:CC:DD:EE:FF", testBroadcast, proof2)
	if err != nil {
		t.Fatalf("EnrollDeviceWithNetwork dev2: %v", err)
	}

	// Heartbeat dev2 to make it online
	heartbeatProof2 := signedDeviceTestProof(key2, "heartbeat", dev2.DeviceID, "", "", "", true)
	if err := DeviceHeartbeatWithNetwork(db, dev2.DeviceID, "192.168.1.60", "aa:bb:cc:dd:ee:ff", testBroadcast, heartbeatProof2); err != nil {
		t.Fatalf("DeviceHeartbeatWithNetwork dev2: %v", err)
	}

	d2Check, err := GetDeviceByID(db, dev2.DeviceID)
	if err != nil || d2Check.Status != DeviceStatusOnline {
		t.Fatalf("expected dev2 online, got %v (err: %v)", d2Check.Status, err)
	}

	// 3. Queue WoL for dev1
	target, onlinePeers, err := QueueDeviceWake(db, dev1.DeviceID, cust.ID, "")
	if err != nil {
		t.Fatalf("QueueDeviceWake failed: %v", err)
	}
	if target.DeviceID != dev1.DeviceID {
		t.Fatalf("expected target %s, got %s", dev1.DeviceID, target.DeviceID)
	}
	if onlinePeers != 1 {
		t.Fatalf("expected 1 online peer (dev2), got %d", onlinePeers)
	}

	// Duplicate immediate request should be throttled
	_, _, err = QueueDeviceWake(db, dev1.DeviceID, cust.ID, "")
	if err != ErrDeviceWakePending {
		t.Fatalf("expected ErrDeviceWakePending, got %v", err)
	}

	// 4. Test PopPendingDeviceWakes in transaction (as during dev2 heartbeat)
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("Begin tx: %v", err)
	}
	wakes, err := PopPendingDeviceWakes(tx, lic.LicenseID)
	if err != nil {
		tx.Rollback()
		t.Fatalf("PopPendingDeviceWakes: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit tx: %v", err)
	}

	if len(wakes) != 1 || wakes[0] != testMAC {
		t.Fatalf("expected wakes [%s], got %v", testMAC, wakes)
	}

	// Second pop should be empty since already dispatched
	tx2, _ := db.Begin()
	wakes2, _ := PopPendingDeviceWakes(tx2, lic.LicenseID)
	tx2.Commit()
	if len(wakes2) != 0 {
		t.Fatalf("expected 0 pending wakes on second pop, got %v", wakes2)
	}

	// 5. Test UpdateDeviceMAC
	newMAC := "11:22:33:44:55:66"
	if err := UpdateDeviceMAC(db, dev1.DeviceID, newMAC); err != nil {
		t.Fatalf("UpdateDeviceMAC failed: %v", err)
	}
	updatedDev, err := GetDeviceByID(db, dev1.DeviceID)
	if err != nil || updatedDev.MACAddress != newMAC {
		t.Fatalf("expected updated MAC %s, got %s (err: %v)", newMAC, updatedDev.MACAddress, err)
	}
}
