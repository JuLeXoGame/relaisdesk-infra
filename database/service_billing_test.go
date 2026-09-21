package database

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func serviceFixture(t *testing.T, mode string) (*sql.DB, *ServiceWork) {
	t.Helper()
	db, e := InitDatabase(filepath.Join(t.TempDir(), "billing.db"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { db.Close() })
	lic, e := CreateLicense(db, "provider@example.test", 30, 5, "Pro")
	if e != nil {
		t.Fatal(e)
	}
	if e = db.QueryRow(`SELECT customer_id FROM licences WHERE license_id=?`, lic.LicenseID).Scan(&lic.CustomerID); e != nil {
		t.Fatal(e)
	}
	if _, e = EnsureServiceMerchant(db, lic.CustomerID); e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(`UPDATE service_merchants SET enabled=1,account_id='acct_test' WHERE customer_id=?`, lic.CustomerID); e != nil {
		t.Fatal(e)
	}
	rate, e := CreateServiceRate(db, lic.CustomerID, ServiceRate{Label: "Assistance", Mode: mode, Cents: 6000})
	if e != nil {
		t.Fatal(e)
	}
	work, e := CreateServiceWork(db, lic.CustomerID, lic.LicenseID, "", "device", "DEV-TEST-1234", "123456789", rate.ID)
	if e != nil {
		t.Fatal(e)
	}
	return db, work
}
func TestServiceMeterConfirmedIntervals(t *testing.T) {
	db, w := serviceFixture(t, "hourly")
	start := time.Now()
	conn := strings.Repeat("a", 32)
	pulse := func(seq, ms int64, closed bool, offset time.Duration) *ServiceWork {
		t.Helper()
		v, e := RecordServicePulse(db, w.ID, conn, seq, ms, closed, start.Add(offset))
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	pulse(1, 0, false, 0)
	v := pulse(2, 2000, false, 2*time.Second)
	if v.ConnectedMS != 2000 {
		t.Fatal(v.ConnectedMS)
	}
	v = pulse(2, 2000, false, 3*time.Second)
	if v.ConnectedMS != 2000 {
		t.Fatal("duplicate billed")
	}
	if _, e := RecordServicePulse(db, w.ID, strings.Repeat("b", 32), 1, 0, false, start.Add(4*time.Second)); e == nil {
		t.Fatal("parallel session accepted")
	}
	if _, e := FinishServiceWork(db, w.ID, false, start.Add(4*time.Second)); e == nil {
		t.Fatal("active connection finalized")
	}
	v = pulse(3, 4000, false, 30*time.Second)
	if v.ConnectedMS != 2000 {
		t.Fatal("gap billed")
	}
	v = pulse(4, 6000, true, 32*time.Second)
	if v.ConnectedMS != 4000 {
		t.Fatal(v.ConnectedMS)
	}
	// A closed session cannot be revived by a delayed pulse.
	if _, e := RecordServicePulse(db, w.ID, conn, 5, 6001, false, start.Add(33*time.Second)); e == nil {
		t.Fatal("closed session resumed")
	}
	conn = strings.Repeat("c", 32)
	pulse(1, 0, false, 34*time.Second)
	pulse(2, 1000, true, 35*time.Second)
	v, e := FinishServiceWork(db, w.ID, false, start.Add(36*time.Second))
	if e != nil {
		t.Fatal(e)
	}
	if v.ConnectedMS != 5000 || v.AmountCents != 8 {
		t.Fatalf("unexpected duration/price: %+v", v)
	}
	again, e := FinishServiceWork(db, w.ID, false, start.Add(time.Hour))
	if e != nil || again.AmountCents != 8 {
		t.Fatal("finish not idempotent", e)
	}
	if _, e := RecordServicePulse(db, w.ID, strings.Repeat("d", 32), 1, 0, false, start.Add(time.Hour)); e == nil {
		t.Fatal("finalized meter reopened")
	}
}
func TestServicePrepaidGateAndImmutableRate(t *testing.T) {
	db, w := serviceFixture(t, "prepaid")
	conn := strings.Repeat("a", 32)
	now := time.Now()
	if _, e := RecordServicePulse(db, w.ID, conn, 1, 0, false, now); e == nil {
		t.Fatal("unpaid prepaid session accepted")
	}
	db.Exec(`UPDATE service_rates SET cents=500,active=0`)
	w, e := GetServiceWork(db, w.ID)
	if e != nil || w.AmountCents != 6000 {
		t.Fatal("price snapshot changed", e)
	}
	db.Exec(`UPDATE service_work SET paid=1 WHERE id=?`, w.ID)
	if _, e = RecordServicePulse(db, w.ID, conn, 1, 0, false, now); e != nil {
		t.Fatal(e)
	}
	if _, e = FinishServiceWork(db, w.ID, true, now.Add(time.Minute)); e == nil {
		t.Fatal("paid charge silently cancelled")
	}
}
func TestServiceClockAndSequenceAbuse(t *testing.T) {
	db, w := serviceFixture(t, "hourly")
	now := time.Now()
	conn := strings.Repeat("a", 32)
	RecordServicePulse(db, w.ID, conn, 1, 0, false, now)
	if _, e := RecordServicePulse(db, w.ID, conn, 2, 100000, false, now.Add(time.Second)); e == nil {
		t.Fatal("fabricated duration accepted")
	}
	if _, e := RecordServicePulse(db, w.ID, conn, 2, 1, false, now.Add(-time.Second)); e == nil {
		t.Fatal("clock reversal accepted")
	}
	v, e := RecordServicePulse(db, w.ID, conn, 3, 2000, true, now.Add(2*time.Second))
	if e != nil || v.ConnectedMS != 0 {
		t.Fatal("missing sequence billed", e)
	}
}
func TestServiceTariffsAndRounding(t *testing.T) {
	db, w := serviceFixture(t, "hourly")
	for _, r := range []ServiceRate{{Label: "x", Mode: "hourly", Cents: -1}, {Label: "x", Mode: "monthly", Cents: 6000}, {Label: "", Mode: "hourly", Cents: 6000}, {Label: "x\n", Mode: "hourly", Cents: 1000001}} {
		if _, e := CreateServiceRate(db, w.CustomerID, r); e == nil {
			t.Fatal("invalid rate accepted", r)
		}
	}
	for _, v := range []struct{ ms, want int64 }{{0, 0}, {300000, 500}, {900000, 1500}, {1250, 2}, {3600000, 6000}} {
		got, e := ServiceAmount(6000, v.ms)
		if e != nil || got != v.want {
			t.Fatal(v, got, e)
		}
	}
	if _, e := ServiceAmount(1000001, 3600000); e == nil {
		t.Fatal("amount overflow bound not applied")
	}
}
