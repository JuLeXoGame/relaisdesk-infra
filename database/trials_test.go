package database

import (
	"bytes"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func trialTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := InitDatabase(filepath.Join(t.TempDir(), "trial.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}
func verifiedTrial(t *testing.T, db *sql.DB, email, plan, cycle string, techs int, now time.Time) *Trial {
	t.Helper()
	a, err := CreateTrialApplication(db, email, plan, techs, BillingDetails{Name: "Client", Address: "1 rue Test", PostalCode: "75001", City: "Paris", Country: "France", CustomerType: "business", TermsVersion: TrialTermsVersion, TermsAccepted: true, BillingCycle: cycle}, now)
	if err != nil {
		t.Fatal(err)
	}
	token, err := CreateTrialEmailToken(db, a.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	var stored string
	if err = db.QueryRow(`SELECT token_hash FROM trial_applications WHERE id=?`, a.ID).Scan(&stored); err != nil || stored == token {
		t.Fatal("token not hashed", err)
	}
	a, err = VerifyTrialEmail(db, token, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = VerifyTrialEmail(db, token, now); !errors.Is(err, ErrTrialUnavailable) {
		t.Fatal("replayed email token accepted")
	}
	return a
}

func TestTrialAllCapacitiesOneOfferAndNoRenewalReset(t *testing.T) {
	for _, tc := range []struct {
		plan  string
		techs int
		price int64
	}{{"starter", 1, 2490}, {"pro", 5, 11000}, {"ultra", 10, 19900}, {"ultra", 500, 405900}} {
		t.Run(tc.plan+time.Duration(tc.techs).String(), func(t *testing.T) {
			db := trialTestDB(t)
			now := time.Now().UTC().Truncate(time.Second)
			secret := bytes.Repeat([]byte{7}, 32)
			a := verifiedTrial(t, db, "user@example.com", tc.plan, "monthly", tc.techs, now)
			a, err := ReserveTrial(db, a.ID, secret, "card-test", now)
			if err != nil {
				t.Fatal(err)
			}
			if a.TrialEnd-a.TrialStart != 30*86400 || a.PriceCents != tc.price || a.Technicians != tc.techs {
				t.Fatalf("unexpected trial %+v", a)
			}
			a, err = ActivateTrial(db, a.ID, "sub_test", a.TrialEnd)
			if err != nil {
				t.Fatal(err)
			}
			again, err := ActivateTrial(db, a.ID, "sub_test", a.TrialEnd)
			if err != nil || again.LicenseID != a.LicenseID {
				t.Fatal("activation not idempotent", err)
			}
			quotas, err := ListFleetQuotas(db, 0, a.LicenseID)
			if err != nil || len(quotas) != 1 {
				t.Fatalf("trial quotas: %+v %v", quotas, err)
			}
			if !strings.EqualFold(quotas[0].Plan, tc.plan) || quotas[0].Limit != ManagedDeviceLimit(tc.plan, tc.techs) || !quotas[0].CanEnroll {
				t.Fatalf("trial quota differs from chosen offer: %+v", quotas)
			}
			// Cancellation and expiration never clear claims or create a new offer.
			db.Exec(`UPDATE licences SET expires_at='2000-01-01' WHERE license_id=?`, a.LicenseID)
			db.Exec(`UPDATE trial_applications SET stripe_status='canceled',ended_at=? WHERE id=?`, now.Unix(), a.ID)
			next := verifiedTrial(t, db, "user@example.com", "pro", "annual", 5, now.Add(time.Hour))
			if _, err = ReserveTrial(db, next.ID, secret, "different-card", now.Add(time.Hour)); !errors.Is(err, ErrTrialUsed) {
				t.Fatalf("existing customer got another trial: %v", err)
			}
			other := verifiedTrial(t, db, "new-email@example.com", "starter", "monthly", 1, now.Add(time.Hour))
			if _, err = ReserveTrial(db, other.ID, secret, "card-test", now.Add(time.Hour)); !errors.Is(err, ErrTrialUsed) {
				t.Fatalf("reused card accepted: %v", err)
			}
			var rawCount int
			db.QueryRow(`SELECT COUNT(*) FROM trial_claims WHERE key_hash IN ('card-test','user@example.com')`).Scan(&rawCount)
			if rawCount != 0 {
				t.Fatal("raw marker persisted")
			}
		})
	}
}

func TestTrialConcurrentCardClaimsAtomic(t *testing.T) {
	db := trialTestDB(t)
	now := time.Now().UTC()
	a := verifiedTrial(t, db, "a@example.com", "starter", "monthly", 1, now)
	b := verifiedTrial(t, db, "b@example.com", "pro", "monthly", 5, now)
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, id := range []string{a.ID, b.ID} {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			_, err := ReserveTrial(db, id, bytes.Repeat([]byte{2}, 32), "shared-card", now)
			results <- err
		}(id)
	}
	wg.Wait()
	close(results)
	success, denied := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, ErrTrialUsed) {
			denied++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || denied != 1 {
		t.Fatalf("success=%d denied=%d", success, denied)
	}
	var claims int
	db.QueryRow(`SELECT COUNT(*) FROM trial_claims`).Scan(&claims)
	if claims != 3 {
		t.Fatalf("partial transaction leaked markers: %d", claims)
	}
}

func TestTrialPaymentsUsePaidPeriodsAndAreIdempotent(t *testing.T) {
	db := trialTestDB(t)
	now := time.Now().UTC().Truncate(time.Second)
	a := verifiedTrial(t, db, "paid@example.com", "pro", "monthly", 5, now)
	a, err := ReserveTrial(db, a.ID, bytes.Repeat([]byte{3}, 32), "payment-card", now)
	if err != nil {
		t.Fatal(err)
	}
	a, err = ActivateTrial(db, a.ID, "sub_paid", a.TrialEnd)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreateRenewalOrder(db, a.Email, a.LicenseID, "stripe", TrialTermsVersion, true, false); err == nil {
		t.Fatal("manual renewal of recurring subscription accepted")
	}
	if _, err = RecordSubscriptionPayment(db, a.ID, "in_zero", 0, a.TrialEnd, a.TrialEnd+30*86400); err == nil {
		t.Fatal("zero invoice granted paid access")
	}
	if _, err = RecordSubscriptionPayment(db, a.ID, "in_wrong", 10900, a.TrialEnd, a.TrialEnd+30*86400); err == nil {
		t.Fatal("wrong amount accepted")
	}
	for _, p := range []struct {
		id     string
		offset int64
	}{{"in_later", 31}, {"in_first", 0}, {"in_later", 31}} {
		if _, err = RecordSubscriptionPayment(db, a.ID, p.id, a.PriceCents, a.TrialEnd+p.offset*86400, a.TrialEnd+(p.offset+30)*86400); err != nil {
			t.Fatal(err)
		}
	}
	lic, err := GetLicense(db, a.LicenseID)
	if err != nil {
		t.Fatal(err)
	}
	if lic.ExpiresAt.Unix() != a.TrialEnd+61*86400 {
		t.Fatalf("replay/out-of-order changed expiry: %v", lic.ExpiresAt)
	}
	var orders int
	db.QueryRow(`SELECT COUNT(*) FROM orders`).Scan(&orders)
	if orders != 2 {
		t.Fatalf("duplicate orders: %d", orders)
	}
	db.Exec(`UPDATE licences SET status='revoked' WHERE license_id=?`, a.LicenseID)
	if _, err = RecordSubscriptionPayment(db, a.ID, "in_revoked", a.PriceCents, a.TrialEnd+61*86400, a.TrialEnd+91*86400); err != nil {
		t.Fatal(err)
	}
	lic, _ = GetLicense(db, a.LicenseID)
	if lic.Status != "revoked" {
		t.Fatal("payment unrevoked license")
	}
}

func TestTrialRetentionAndUnverifiedAccess(t *testing.T) {
	db := trialTestDB(t)
	now := time.Now().UTC()
	old := now.AddDate(-4, 0, 0)
	a := verifiedTrial(t, db, "old@example.com", "starter", "monthly", 1, old)
	a, err := ReserveTrial(db, a.ID, bytes.Repeat([]byte{1}, 32), "old-card", old)
	if err != nil {
		t.Fatal(err)
	}
	a, err = ActivateTrial(db, a.ID, "sub_old", a.TrialEnd)
	if err != nil {
		t.Fatal(err)
	}
	db.Exec(`UPDATE trial_applications SET stripe_status='canceled',ended_at=? WHERE id=?`, old.Unix(), a.ID)
	result, err := PurgeExpiredOperationalData(db, now)
	if err != nil || result.TrialClaims != 3 {
		t.Fatalf("retention=%+v err=%v", result, err)
	}
	// The account's retained licence history still prevents repeat trials.
	b := verifiedTrial(t, db, a.Email, "starter", "monthly", 1, now)
	if _, err = ReserveTrial(db, b.ID, bytes.Repeat([]byte{1}, 32), "new-card", now); !errors.Is(err, ErrTrialUsed) {
		t.Fatal(err)
	}
	if _, err = ReserveTrial(db, b.ID, nil, "new-card", now); !errors.Is(err, ErrTrialUnavailable) {
		t.Fatal("missing key accepted")
	}
	c := verifiedTrial(t, db, "new-card-owner@example.com", "starter", "monthly", 1, now)
	if _, err = ReserveTrial(db, c.ID, bytes.Repeat([]byte{9}, 32), "old-card", now); err == nil {
		t.Fatal("rotating the secret bypassed card history")
	}
}
