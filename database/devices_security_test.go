package database

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func signedDeviceTestProof(key ed25519.PrivateKey, action, id, rd, host, osName string, ready bool) DeviceProof {
	nonce := make([]byte, 24)
	_, _ = rand.Read(nonce)
	p := DeviceProof{Timestamp: time.Now().Unix(), Nonce: base64.RawURLEncoding.EncodeToString(nonce), Ready: ready}
	pub := base64.RawURLEncoding.EncodeToString(key.Public().(ed25519.PublicKey))
	p.Signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(key, DeviceProofMessage(action, id, rd, host, osName, pub, p)))
	return p
}
func TestFleetCryptographicEnrollmentAndRevocation(t *testing.T) {
	db, err := InitDatabase(filepath.Join(t.TempDir(), "fleet.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	lic, err := CreateLicense(db, "fleet@example.invalid", 365, 3, "")
	if err != nil {
		t.Fatal(err)
	}
	dev, err := CreatePermanentEnrollment(db, 0, lic.LicenseID, "PC", "")
	if err != nil {
		t.Fatal(err)
	}
	var stored string
	if err = db.QueryRow("SELECT permanent_code FROM devices WHERE device_id=?", dev.DeviceID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored == dev.PermanentCode {
		t.Fatal("raw installation secret stored")
	}
	pub, key, _ := ed25519.GenerateKey(rand.Reader)
	public := base64.RawURLEncoding.EncodeToString(pub)
	enroll := func(key ed25519.PrivateKey, id string, p DeviceProof) error {
		_, err := EnrollDevice(db, dev.PermanentCode, id, "PC", "windows", base64.RawURLEncoding.EncodeToString(key.Public().(ed25519.PublicKey)), "192.0.2.1", p)
		return err
	}
	if _, err = EnrollDevice(db, dev.PermanentCode, "123456789", "PC", "windows", public, "192.0.2.1"); err == nil {
		t.Fatal("missing proof accepted")
	}
	proof := signedDeviceTestProof(key, "enroll", dev.PermanentCode, "123456789", "PC", "windows", false)
	if err = enroll(key, "123456789", proof); err != nil {
		t.Fatal(err)
	}
	if err = enroll(key, "123456789", proof); err == nil {
		t.Fatal("replayed request accepted")
	}
	_, other, _ := ed25519.GenerateKey(rand.Reader)
	if err = enroll(other, "987654321", signedDeviceTestProof(other, "enroll", dev.PermanentCode, "987654321", "PC", "windows", false)); err == nil {
		t.Fatal("identity substitution accepted")
	}
	if err = enroll(key, "123456789", signedDeviceTestProof(key, "enroll", dev.PermanentCode, "123456789", "PC", "windows", false)); err != nil {
		t.Fatal("same-device lost-response retry rejected", err)
	}
	if err = DeviceHeartbeat(db, dev.DeviceID, "192.0.2.1"); err == nil {
		t.Fatal("anonymous heartbeat accepted")
	}
	heartbeat := signedDeviceTestProof(key, "heartbeat", dev.DeviceID, "", "", "", true)
	if err = DeviceHeartbeat(db, dev.DeviceID, "192.0.2.1", heartbeat); err != nil {
		t.Fatal(err)
	}
	if err = DeviceHeartbeat(db, dev.DeviceID, "192.0.2.1", heartbeat); err == nil {
		t.Fatal("heartbeat replay accepted")
	}
	heartbeat = signedDeviceTestProof(key, "heartbeat", dev.DeviceID, "", "", "", true)
	heartbeat.Ready = false
	if err = DeviceHeartbeat(db, dev.DeviceID, "192.0.2.1", heartbeat); err == nil {
		t.Fatal("tampered presence accepted")
	}
	if _, err = db.Exec("UPDATE licences SET expires_at=? WHERE license_id=?", time.Now().Add(-time.Hour), lic.LicenseID); err != nil {
		t.Fatal(err)
	}
	if err = DeviceHeartbeat(db, dev.DeviceID, "192.0.2.1", signedDeviceTestProof(key, "heartbeat", dev.DeviceID, "", "", "", true)); err == nil {
		t.Fatal("expired license accepted")
	}
	if err = TechnicianDeleteDevice(db, lic.LicenseID, dev.DeviceID); err != nil {
		t.Fatal(err)
	}
	if err = DeviceHeartbeat(db, dev.DeviceID, "192.0.2.1", signedDeviceTestProof(key, "heartbeat", dev.DeviceID, "", "", "", true)); err == nil {
		t.Fatal("deleted device accepted")
	}
}
func TestFleetEnrollmentExpiryAndConcurrentIdentity(t *testing.T) {
	db, err := InitDatabase(filepath.Join(t.TempDir(), "fleet.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	lic, _ := CreateLicense(db, "fleet-race@example.invalid", 365, 3, "")
	if _, err := CreatePermanentEnrollment(db, 0, lic.LicenseID, strings.Repeat("A", 101), ""); err == nil {
		t.Fatal("unbounded alias")
	}
	dev, _ := CreatePermanentEnrollment(db, 0, lic.LicenseID, "PC", "")
	var wait sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			pub, key, _ := ed25519.GenerateKey(rand.Reader)
			_, err := EnrollDevice(db, dev.PermanentCode, "123456789", "PC", "windows", base64.RawURLEncoding.EncodeToString(pub), "192.0.2.1", signedDeviceTestProof(key, "enroll", dev.PermanentCode, "123456789", "PC", "windows", false))
			results <- err
		}()
	}
	wait.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("expected one binding, got %d", successes)
	}
	for _, version := range []int{0, 1} {
		d, _ := CreatePermanentEnrollment(db, 0, lic.LicenseID, "Expired", "")
		_, err = db.Exec("UPDATE devices SET enrollment_version=?,created_at=? WHERE device_id=?", version, time.Now().Add(-time.Hour), d.DeviceID)
		if err != nil {
			t.Fatal(err)
		}
		pub, key, _ := ed25519.GenerateKey(rand.Reader)
		_, err = EnrollDevice(db, d.PermanentCode, "123456789", "PC", "windows", base64.RawURLEncoding.EncodeToString(pub), "192.0.2.1", signedDeviceTestProof(key, "enroll", d.PermanentCode, "123456789", "PC", "windows", false))
		if err == nil {
			t.Fatal("legacy/expired enrollment accepted")
		}
	}
}
