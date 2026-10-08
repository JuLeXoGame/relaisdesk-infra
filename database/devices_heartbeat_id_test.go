package database

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"path/filepath"
	"testing"
)

// Regression: the engine id can rotate under a running agent (config
// rebuilt, keys regenerated) while the fleet row kept the stale id forever,
// pointing technicians at a ghost peer. Heartbeats carry the live id and
// the row follows it; invalid values never wipe.
func TestHeartbeatAdoptsRotatedRustDeskID(t *testing.T) {
	db, err := InitDatabase(filepath.Join(t.TempDir(), "heartbeat-id.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	cust, err := EnsureCustomer(db, "heartbeat-id@example.com", "business", "Heartbeat ID Corp")
	if err != nil {
		t.Fatal(err)
	}
	lic, err := CreateLicense(db, "heartbeat-id@example.com", 365, 5, "Heartbeat ID Pro")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("UPDATE licences SET customer_id = ? WHERE license_id = ?", cust.ID, lic.LicenseID); err != nil {
		t.Fatal(err)
	}
	dev, err := CreatePermanentEnrollment(db, cust.ID, lic.LicenseID, "PC Atelier", "")
	if err != nil {
		t.Fatal(err)
	}
	pub, key, _ := ed25519.GenerateKey(rand.Reader)
	public := base64.RawURLEncoding.EncodeToString(pub)
	if _, err := EnrollDevice(db, dev.PermanentCode, "1261345246", "PC-ATELIER", "linux", public, "192.168.1.50", signedDeviceTestProof(key, "enroll", dev.PermanentCode, "1261345246", "PC-ATELIER", "linux", false)); err != nil {
		t.Fatalf("enroll: %v", err)
	}
	beat := func(rustdeskID string) {
		t.Helper()
		proof := signedDeviceTestProof(key, "heartbeat", dev.DeviceID, "", "", "", true)
		if err := DeviceHeartbeatWithFullInfo(db, dev.DeviceID, "192.168.1.50", "", "", "", rustdeskID, proof); err != nil {
			t.Fatalf("heartbeat id=%q: %v", rustdeskID, err)
		}
	}
	current := func() string {
		t.Helper()
		row, err := GetDeviceByID(db, dev.DeviceID)
		if err != nil {
			t.Fatal(err)
		}
		return row.RustDeskID
	}

	beat("1027287547")
	if got := current(); got != "1027287547" {
		t.Fatalf("rotated id not adopted: %q", got)
	}
	beat("bogus")
	beat("")
	beat("12")
	if got := current(); got != "1027287547" {
		t.Fatalf("invalid heartbeat wiped id: %q", got)
	}
	beat("1027287547")
	if got := current(); got != "1027287547" {
		t.Fatalf("same id broke: %q", got)
	}
}
