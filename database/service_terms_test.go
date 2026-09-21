package database

import (
	"testing"
	"time"
)

func TestServiceTermsImmutableAndIsolated(t *testing.T) {
	db, w := serviceFixture(t, "hourly")
	if err := AcceptServiceTerms(db, w.CustomerID, "v1", "hash-one"); err != nil {
		t.Fatal(err)
	}
	at, e := ServiceTermsAccepted(db, w.CustomerID, "v1", "hash-one")
	if e != nil || at == 0 {
		t.Fatal(at, e)
	}
	if e = AcceptServiceTerms(db, w.CustomerID, "v1", "hash-two"); e == nil {
		t.Fatal("accepted contract overwritten")
	}
	again, e := ServiceTermsAccepted(db, w.CustomerID, "v1", "hash-one")
	if e != nil || again != at {
		t.Fatal("original proof changed")
	}
	other, e := ServiceTermsAccepted(db, w.CustomerID+1, "v1", "hash-one")
	if e != nil || other != 0 {
		t.Fatal("foreign proof accepted")
	}
}

func TestServiceRetention(t *testing.T) {
	db, w := serviceFixture(t, "hourly")
	now := time.Now()
	if _, e := FinishServiceWork(db, w.ID, true, now); e != nil {
		t.Fatal(e)
	}
	var closed int64
	if e := db.QueryRow(`SELECT closed_at FROM service_work_closures WHERE work_id=?`, w.ID).Scan(&closed); e != nil || closed == 0 {
		t.Fatal(closed, e)
	}
	if _, e := db.Exec(`UPDATE service_work_closures SET closed_at=?`, now.AddDate(-5, 0, -1).Unix()); e != nil {
		t.Fatal(e)
	}
	result, e := PurgeExpiredOperationalData(db, now)
	if e != nil || result.ServiceWork != 1 {
		t.Fatal(result, e)
	}
	var count int
	if e = db.QueryRow(`SELECT COUNT(*) FROM service_work_closures`).Scan(&count); e != nil || count != 0 {
		t.Fatal("closure orphan", e)
	}

	for i := 0; i < 3; i++ {
		var rate string
		if e = db.QueryRow(`SELECT id FROM service_rates LIMIT 1`).Scan(&rate); e != nil {
			t.Fatal(e)
		}
		draft, e := CreateServiceWork(db, w.CustomerID, w.LicenseID, "", "device", "DEV-TEST-1234", "123456789", rate)
		if e != nil {
			t.Fatal(e)
		}
		created := now.AddDate(0, 0, -91).Unix()
		if i == 1 {
			created = now.Unix()
		}
		started := int64(0)
		if i == 2 {
			started = created
		}
		if _, e = db.Exec(`UPDATE service_work SET created_at=?,checkout_started=? WHERE id=?`, created, started, draft.ID); e != nil {
			t.Fatal(e)
		}
	}
	result, e = PurgeExpiredOperationalData(db, now)
	if e != nil || result.ServiceDrafts != 1 {
		t.Fatal(result, e)
	}
	if e = db.QueryRow(`SELECT COUNT(*) FROM service_work`).Scan(&count); e != nil || count != 2 {
		t.Fatal("recent or uncertain checkout deleted", count, e)
	}
}
