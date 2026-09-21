package database

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"path/filepath"
	"testing"
)

func TestDeviceLifecycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test_devices_lifecycle.db")
	db, err := InitDatabase(path)
	if err != nil {
		t.Fatalf("InitDatabase failed: %v", err)
	}
	defer db.Close()

	cust, err := EnsureCustomer(db, "fleet-owner@example.com", "business", "Fleet Corp")
	if err != nil {
		t.Fatalf("EnsureCustomer: %v", err)
	}

	lic, err := CreateLicense(db, "fleet-owner@example.com", 365, 5, "Fleet Pro")
	if err != nil {
		t.Fatalf("CreateLicense: %v", err)
	}
	if _, err := db.Exec("UPDATE licences SET customer_id = ? WHERE license_id = ?", cust.ID, lic.LicenseID); err != nil {
		t.Fatalf("link license to customer: %v", err)
	}

	// 1. Create permanent enrollment
	dev, err := CreatePermanentEnrollment(db, cust.ID, lic.LicenseID, "Serveur Compta", "Salle serveur RDC")
	if err != nil {
		t.Fatalf("CreatePermanentEnrollment failed: %v", err)
	}
	if dev.DeviceID == "" || dev.PermanentCode == "" {
		t.Fatalf("DeviceID or PermanentCode empty: %+v", dev)
	}
	if dev.Status != DeviceStatusOffline {
		t.Fatalf("expected offline initially, got %s", dev.Status)
	}

	// 2. Enroll device from remote agent
	pub, key, _ := ed25519.GenerateKey(rand.Reader)
	public := base64.RawURLEncoding.EncodeToString(pub)
	proof := signedDeviceTestProof(key, "enroll", dev.PermanentCode, "987654321", "SRV-COMPTA", "windows", false)
	enrolled, err := EnrollDevice(db, dev.PermanentCode, "987654321", "SRV-COMPTA", "windows", public, "192.168.1.50", proof)
	if err != nil {
		t.Fatalf("EnrollDevice failed: %v", err)
	}
	if enrolled.Status != DeviceStatusOffline {
		t.Fatalf("expected online status after enrollment, got %s", enrolled.Status)
	}
	if enrolled.RustDeskID != "987654321" || enrolled.Hostname != "SRV-COMPTA" {
		t.Fatalf("unexpected enrolled data: %+v", enrolled)
	}

	// 3. Heartbeat
	if err := DeviceHeartbeat(db, dev.DeviceID, "192.168.1.50", signedDeviceTestProof(key, "heartbeat", dev.DeviceID, "", "", "", true)); err != nil {
		t.Fatalf("DeviceHeartbeat failed: %v", err)
	}

	// 4. List by customer
	customerDevices, err := ListDevicesByCustomer(db, cust.ID)
	if err != nil {
		t.Fatalf("ListDevicesByCustomer failed: %v", err)
	}
	if len(customerDevices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(customerDevices))
	}
	if customerDevices[0].DeviceID != dev.DeviceID {
		t.Fatalf("expected device %s, got %s", dev.DeviceID, customerDevices[0].DeviceID)
	}
	if customerDevices[0].Status != DeviceStatusOnline {
		t.Fatalf("expected status online, got %s", customerDevices[0].Status)
	}

	// 5. List by license
	licenseDevices, err := ListDevicesByLicense(db, lic.LicenseID)
	if err != nil {
		t.Fatalf("ListDevicesByLicense failed: %v", err)
	}
	if len(licenseDevices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(licenseDevices))
	}

	// 6. Update alias
	if err := UpdateDeviceAlias(db, cust.ID, dev.DeviceID, "Serveur Comptabilité Principal", "Mis à jour"); err != nil {
		t.Fatalf("UpdateDeviceAlias failed: %v", err)
	}
	fetched, err := GetDeviceByID(db, dev.DeviceID)
	if err != nil {
		t.Fatalf("GetDeviceByID failed: %v", err)
	}
	if fetched.Alias != "Serveur Comptabilité Principal" || fetched.Notes != "Mis à jour" {
		t.Fatalf("UpdateDeviceAlias did not persist: %+v", fetched)
	}

	// 7. Delete device
	if err := DeleteDevice(db, cust.ID, dev.DeviceID); err != nil {
		t.Fatalf("DeleteDevice failed: %v", err)
	}
	customerDevicesAfter, err := ListDevicesByCustomer(db, cust.ID)
	if err != nil {
		t.Fatalf("ListDevicesByCustomer failed: %v", err)
	}
	if len(customerDevicesAfter) != 0 {
		t.Fatalf("expected 0 devices after delete, got %d", len(customerDevicesAfter))
	}
}

func TestTechnicianDeviceOperations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test_tech_devices.db")
	db, err := InitDatabase(path)
	if err != nil {
		t.Fatalf("InitDatabase failed: %v", err)
	}
	defer db.Close()

	lic, err := CreateLicense(db, "tech@example.com", 30, 2, "Tech License")
	if err != nil {
		t.Fatalf("CreateLicense: %v", err)
	}

	dev, err := CreatePermanentEnrollment(db, 0, lic.LicenseID, "PC Atelier", "")
	if err != nil {
		t.Fatalf("CreatePermanentEnrollment failed: %v", err)
	}

	if err := TechnicianUpdateDevice(db, lic.LicenseID, dev.DeviceID, "PC Atelier Modifié", "Notes atelier"); err != nil {
		t.Fatalf("TechnicianUpdateDevice failed: %v", err)
	}

	fetched, err := GetDeviceByID(db, dev.DeviceID)
	if err != nil {
		t.Fatalf("GetDeviceByID: %v", err)
	}
	if fetched.Alias != "PC Atelier Modifié" {
		t.Fatalf("unexpected alias: %s", fetched.Alias)
	}

	if err := TechnicianDeleteDevice(db, lic.LicenseID, dev.DeviceID); err != nil {
		t.Fatalf("TechnicianDeleteDevice failed: %v", err)
	}

	list, err := ListDevicesByLicense(db, lic.LicenseID)
	if err != nil {
		t.Fatalf("ListDevicesByLicense failed: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected 0 devices, got %d", len(list))
	}
}
