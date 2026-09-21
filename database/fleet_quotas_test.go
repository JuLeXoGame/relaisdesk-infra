package database

import (
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
)

func seedFleet(t *testing.T, db *sql.DB, license string, count int) {
	t.Helper()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for i := 0; i < count; i++ {
		_, err = tx.Exec(`INSERT INTO devices(device_id,license_id,permanent_code,enrollment_version,status) VALUES(?,?,?,2,'offline')`, fmt.Sprintf("fixture-%s-%d", license, i), license, fmt.Sprintf("hash-%s-%d", license, i))
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func TestManagedDeviceCatalog(t *testing.T) {
	for _, tc := range []struct {
		plan        string
		techs, want int
	}{
		{"starter", 1, 500}, {"starter", 50, 500}, {"pro", 1, 1000}, {"pro", 5, 1000},
		{"ultra", 10, 2000}, {"ultra", 11, 2005}, {"Ultra", 20, 2050}, {"personnalisé", 50, 2200},
		{"ultra", 100, 2450}, {"ultra", 200, 2950}, {"ultra", 490, 4400}, {"ultra", 499, 4445},
		{"ultra", 500, 4500}, {"ultra", 10000, 4500},
		{"", 1, 500}, {"", 5, 1000}, {"", 10, 2000},
	} {
		if got := ManagedDeviceLimit(tc.plan, tc.techs); got != tc.want {
			t.Fatalf("%+v: %d", tc, got)
		}
	}
}

func TestFleetQuotaReservationDeletionExpiryAndOwnership(t *testing.T) {
	db, err := InitDatabase(filepath.Join(t.TempDir(), "quota.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	lic, err := CreateLicense(db, "quota@example.com", 30, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	seedFleet(t, db, lic.LicenseID, 499)
	dev, err := CreatePermanentEnrollment(db, 0, lic.LicenseID, "last", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePermanentEnrollment(db, 0, lic.LicenseID, "excess", ""); !errors.Is(err, ErrDeviceLimit) {
		t.Fatalf("quota bypass: %v", err)
	}
	quotas, err := ListFleetQuotas(db, 0, lic.LicenseID)
	if err != nil {
		t.Fatal(err)
	}
	if len(quotas) != 1 || quotas[0].Registered != 499 || quotas[0].Reserved != 1 || quotas[0].CanEnroll {
		t.Fatalf("quota %+v", quotas)
	}
	if _, err = CreatePermanentEnrollment(db, quotas[0].CustomerID+1, lic.LicenseID, "intruder", ""); !errors.Is(err, ErrDeviceAuthorization) {
		t.Fatalf("ownership: %v", err)
	}
	if _, err = db.Exec("UPDATE devices SET created_at=datetime('now','-16 minutes') WHERE device_id=?", dev.DeviceID); err != nil {
		t.Fatal(err)
	}
	dev, err = CreatePermanentEnrollment(db, 0, lic.LicenseID, "expired reservation replaced", "")
	if err != nil {
		t.Fatal(err)
	}
	if err = TechnicianDeleteDevice(db, lic.LicenseID, dev.DeviceID); err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePermanentEnrollment(db, 0, lic.LicenseID, "deleted reservation replaced", ""); err != nil {
		t.Fatal(err)
	}
	other, err := CreateLicense(db, "other@example.com", 30, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePermanentEnrollment(db, 0, other.LicenseID, "separate quota", ""); err != nil {
		t.Fatal(err)
	}
	if err = RevokeLicense(db, lic.LicenseID, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePermanentEnrollment(db, 0, lic.LicenseID, "revoked", ""); !errors.Is(err, ErrDeviceAuthorization) {
		t.Fatalf("revoked: %v", err)
	}
}

func TestFleetQuotaConcurrentRequestsAcrossConnections(t *testing.T) {
	path := filepath.Join(t.TempDir(), "concurrent.db")
	db, err := InitDatabase(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	second, err := InitDatabase(path)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	lic, err := CreateLicense(db, "parallel@example.com", 30, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	seedFleet(t, db, lic.LicenseID, 499)
	var wg sync.WaitGroup
	results := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			handle := db
			if i%2 == 0 {
				handle = second
			}
			_, e := CreatePermanentEnrollment(handle, 0, lic.LicenseID, "parallel", "")
			results <- e
		}(i)
	}
	wg.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		} else if !errors.Is(err, ErrDeviceLimit) {
			t.Errorf("unexpected concurrent error: %v", err)
		}
	}
	if successes != 1 {
		t.Fatalf("expected exactly one reservation, got %d", successes)
	}
	quotas, err := ListFleetQuotas(db, 0, lic.LicenseID)
	if err != nil || quotas[0].Used != 500 {
		t.Fatalf("quota=%+v err=%v", quotas, err)
	}
}

func TestFleetQuotaUsesPurchasedPlanNotConnectionCount(t *testing.T) {
	db, err := InitDatabase(filepath.Join(t.TempDir(), "plan.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	lic, err := CreateLicense(db, "pro@example.com", 30, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO orders(order_id,email,plan,technicians,price,payment_method,status,license_id) VALUES('order-pro','pro@example.com','Pro',1,110,'card','paid',?)`, lic.LicenseID)
	if err != nil {
		t.Fatal(err)
	}
	quotas, err := ListFleetQuotas(db, 0, lic.LicenseID)
	if err != nil {
		t.Fatal(err)
	}
	if quotas[0].Limit != 1000 {
		t.Fatalf("Pro purchase lost: %+v", quotas)
	}
}

func TestFleetQuotaProAndUltraBoundaries(t *testing.T) {
	for _, tc := range []struct{ techs, limit int }{{5, 1000}, {10, 2000}, {20, 2050}, {499, 4445}, {500, 4500}} {
		t.Run(fmt.Sprint(tc.techs), func(t *testing.T) {
			db := trialTestDB(t)
			lic, err := CreateLicense(db, "boundary@example.com", 30, tc.techs, "")
			if err != nil {
				t.Fatal(err)
			}
			seedFleet(t, db, lic.LicenseID, tc.limit-1)
			if _, err = CreatePermanentEnrollment(db, 0, lic.LicenseID, "last", ""); err != nil {
				t.Fatal(err)
			}
			if _, err = CreatePermanentEnrollment(db, 0, lic.LicenseID, "excess", ""); !errors.Is(err, ErrDeviceLimit) {
				t.Fatalf("quota bypass: %v", err)
			}
		})
	}
}

func TestFleetQuotaDowngradePreservesDevicesAndBlocksPendingEnrollments(t *testing.T) {
	db := trialTestDB(t)
	lic, err := CreateLicense(db, "downgrade@example.com", 30, 5, "")
	if err != nil {
		t.Fatal(err)
	}
	dev, err := CreatePermanentEnrollment(db, 0, lic.LicenseID, "existing", "")
	if err != nil {
		t.Fatal(err)
	}
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	public := base64.RawURLEncoding.EncodeToString(pub)
	proof := signedDeviceTestProof(key, "enroll", dev.PermanentCode, "123456789", "PC", "linux", false)
	if _, err = EnrollDevice(db, dev.PermanentCode, "123456789", "PC", "linux", public, "192.0.2.1", proof); err != nil {
		t.Fatal(err)
	}
	seedFleet(t, db, lic.LicenseID, 500)
	pending, err := CreatePermanentEnrollment(db, 0, lic.LicenseID, "pending", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("UPDATE licences SET max_connections=1 WHERE license_id=?", lic.LicenseID); err != nil {
		t.Fatal(err)
	}
	proof = signedDeviceTestProof(key, "enroll", pending.PermanentCode, "987654321", "PC2", "linux", false)
	if _, err = EnrollDevice(db, pending.PermanentCode, "987654321", "PC2", "linux", public, "192.0.2.1", proof); !errors.Is(err, ErrDeviceLimit) {
		t.Fatalf("downgrade bypass: %v", err)
	}
	proof = signedDeviceTestProof(key, "heartbeat", dev.DeviceID, "", "", "", true)
	if err = DeviceHeartbeat(db, dev.DeviceID, "192.0.2.1", proof); err != nil {
		t.Fatalf("existing device blocked: %v", err)
	}
	q, err := ListFleetQuotas(db, 0, lic.LicenseID)
	if err != nil {
		t.Fatal(err)
	}
	if q[0].Registered != 501 || q[0].Remaining != 0 || q[0].CanEnroll {
		t.Fatalf("existing devices lost: %+v", q)
	}
}
