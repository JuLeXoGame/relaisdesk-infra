package handlers

import (
	"api/config"
	"api/mailer"
	dbpkg "database"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestConsumerTrialRequiresExplicitOpeningAndConsent(t *testing.T) {
	db, cfg, _ := billingFixture(t)
	cfg.B2CSalesEnabled = true
	cfg.LegalPhone = "06 62 85 59 30"
	req := trialRequest{PublicOrderRequest: PublicOrderRequest{Email: "consumer@example.com", Plan: "pro", Technicians: 5, BillingCycle: "monthly", Name: "Client", Address: "1 rue", PostalCode: "75001", City: "Paris", CustomerType: "consumer", TermsAccepted: true, TermsVersion: publicTermsVersion}, RecurringAccepted: true, TrialTermsVersion: dbpkg.TrialTermsVersion, ExpectedPriceCents: 11000}
	call := func() int {
		raw, _ := json.Marshal(req)
		rr := httptest.NewRecorder()
		PublicTrialRequestHandler(db, cfg)(rr, httptest.NewRequest("POST", "/", strings.NewReader(string(raw))))
		return rr.Code
	}
	if got := call(); got != 503 {
		t.Fatal("opened without explicit decision", got)
	}
	cfg.B2CMediationPendingAcknowledged = true
	if got := call(); got != 400 {
		t.Fatal("opened without express performance request", got)
	}
	req.ImmediatePerformanceRequested = true
	if got := call(); got != 202 {
		t.Fatal("explicit consumer request rejected", got)
	}
	var count int
	db.QueryRow(`SELECT COUNT(*) FROM trial_applications WHERE email='consumer@example.com'`).Scan(&count)
	if count != 1 {
		t.Fatal("wrong applications count", count)
	}
}

func TestConsumerWithdrawalBeforePaymentIsDurableAndCancelsImmediately(t *testing.T) {
	db, cfg, a := billingFixture(t)
	a.Billing.CustomerType = "consumer"
	raw, _ := json.Marshal(a.Billing)
	db.Exec(`UPDATE trial_applications SET billing_json=? WHERE id=?`, string(raw), a.ID)
	a, err := dbpkg.ReserveTrial(db, a.ID, cfg.TrialKey(), "withdraw-card", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	a, err = dbpkg.ActivateTrial(db, a.ID, fakeStripeID("sub_", a.ID), a.TrialEnd)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = dbpkg.RequestTrialWithdrawal(db, a.ID, "other@example.com", "Client", time.Now()); err == nil {
		t.Fatal("cross-customer withdrawal")
	}
	now := time.Now().UTC().Truncate(time.Second)
	w, err := dbpkg.RequestTrialWithdrawal(db, a.ID, a.Email, "Client", now)
	if err != nil {
		t.Fatal(err)
	}
	if !w.Immediate {
		t.Fatal("free trial withdrawal not immediate")
	}
	again, err := dbpkg.RequestTrialWithdrawal(db, a.ID, a.Email, "Client", now.Add(time.Hour))
	if err != nil || again.RequestID != w.RequestID || again.RequestedAt != w.RequestedAt {
		t.Fatal("not idempotent", err)
	}
	lic, _ := dbpkg.GetLicense(db, a.LicenseID)
	if lic.Status != "revoked" {
		t.Fatal("access not stopped")
	}
	var jobs, claims int
	db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE job_type LIKE 'trial_withdrawal_%'`).Scan(&jobs)
	db.QueryRow(`SELECT COUNT(*) FROM trial_claims`).Scan(&claims)
	if jobs != 2 || claims != 3 {
		t.Fatal("evidence/retry/anti-repeat lost", jobs, claims)
	}
	sub := subscriptionResponse(a)
	deletes := 0
	mockBilling(t, func(r *http.Request) any {
		if r.Method == "DELETE" {
			deletes++
			if r.Form.Get("invoice_now") != "false" || r.Form.Get("prorate") != "false" {
				t.Fatal("withdrawal must not charge")
			}
			sub["status"] = "canceled"
		}
		return sub
	})
	if err = processTrialWithdrawal(db, cfg, a.ID); err != nil {
		t.Fatal(err)
	}
	if err = processTrialWithdrawal(db, cfg, a.ID); err != nil {
		t.Fatal(err)
	}
	if deletes != 1 {
		t.Fatal("duplicate deletion", deletes)
	}
	// A late invoice must not grant access after withdrawal.
	if _, err = dbpkg.RecordSubscriptionPayment(db, a.ID, "in_after_withdrawal", a.PriceCents, a.TrialEnd, a.TrialEnd+30*86400); err != nil {
		t.Fatal(err)
	}
	lic, _ = dbpkg.GetLicense(db, a.LicenseID)
	if lic.Status != "revoked" {
		t.Fatal("invoice revived withdrawn licence")
	}
	w, _ = dbpkg.GetTrialWithdrawal(db, a.ID)
	if w.Status != "refund_review" {
		t.Fatal("refund not flagged")
	}
}

func TestLateWithdrawalIsRecordedForHumanReview(t *testing.T) {
	db, cfg, a := billingFixture(t)
	a, err := dbpkg.ReserveTrial(db, a.ID, cfg.TrialKey(), "late-card", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	a, err = dbpkg.ActivateTrial(db, a.ID, fakeStripeID("sub_", a.ID), a.TrialEnd)
	if err != nil {
		t.Fatal(err)
	}
	w, err := dbpkg.RequestTrialWithdrawal(db, a.ID, a.Email, "Client", time.Unix(a.TrialEnd+1, 0))
	if err != nil {
		t.Fatal(err)
	}
	if w.Immediate {
		t.Fatal("late notice automatically adjudicated")
	}
	lic, _ := dbpkg.GetLicense(db, a.LicenseID)
	if lic.Status == "revoked" {
		t.Fatal("paid access improperly revoked")
	}
}

func TestAnnualNoticeQueueWindowDedupAndFailSafe(t *testing.T) {
	db, cfg, a := billingFixture(t)
	a, err := dbpkg.ReserveTrial(db, a.ID, cfg.TrialKey(), "annual-card", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	a, err = dbpkg.ActivateTrial(db, a.ID, fakeStripeID("sub_", a.ID), a.TrialEnd)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	end := now.Add(60 * 24 * time.Hour).Unix()
	if _, err = db.Exec(`UPDATE trial_applications SET billing_cycle='annual',paid_through=?,stripe_status='active' WHERE id=?`, end, a.ID); err != nil {
		t.Fatal(err)
	}
	a, err = dbpkg.GetTrial(db, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err = QueueSubscriptionRenewalNotices(db, now); err != nil {
			t.Fatal(err)
		}
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE job_type=?`, jobSubscriptionRenewalNotice).Scan(&n)
	if n != 1 {
		t.Fatal("duplicate notice", n)
	}
	// SMTP failure must not be recorded as delivery evidence.
	p, _ := json.Marshal(subscriptionRenewalNotice{ID: a.ID, End: end})
	mockBilling(t, func(r *http.Request) any {
		s := subscriptionResponse(a)
		s["status"] = "active"
		s["current_period_end"] = end
		return s
	})
	if err = processSubscriptionRenewalNotice(db, cfg, &mailer.Mailer{}, &dbpkg.Job{Payload: string(p)}); err == nil || !strings.Contains(err.Error(), "SMTP non configuré") {
		t.Fatalf("expected SMTP failure after valid Stripe contract, got %v", err)
	}
	db.QueryRow(`SELECT COUNT(*) FROM subscription_renewal_notices`).Scan(&n)
	if n != 0 {
		t.Fatal("false delivery evidence")
	}
	if err = QueueSubscriptionRenewalNotices(db, now.Add(26*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	a, _ = dbpkg.GetTrial(db, a.ID)
	if a.CancelRequestedAt == 0 {
		t.Fatal("missing notice did not stop renewal")
	}
	db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE job_type=?`, jobTrialCancel).Scan(&n)
	if n != 1 {
		t.Fatal("missing durable cancellation")
	}
}

func TestStripeModesCannotBeMixed(t *testing.T) {
	db, _, _ := billingFixture(t)
	if err := dbpkg.BindBillingEnvironment(db, "live"); err != nil {
		t.Fatal(err)
	}
	if err := dbpkg.BindBillingEnvironment(db, "test"); err == nil {
		t.Fatal("test mode mixed into live database")
	}
	err := processStripeEvent(db, &config.Config{StripeSecretKey: "sk_live_fixture"}, []byte(`{"id":"evt_fixture","type":"checkout.session.completed","livemode":false}`))
	if err == nil {
		t.Fatal("test event accepted as real payment")
	}
}

func TestCancellationRollsBackWhenDurableJobCannotBeSaved(t *testing.T) {
	db, _, a := billingFixture(t)
	if _, err := db.Exec(`CREATE TRIGGER deny_cancel_job BEFORE INSERT ON jobs WHEN NEW.job_type='trial_cancel' BEGIN SELECT RAISE(ABORT,'simulated storage failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err := dbpkg.RequestSubscriptionCancellation(db, a.ID, time.Now()); err == nil {
		t.Fatal("storage failure ignored")
	}
	a, _ = dbpkg.GetTrial(db, a.ID)
	if a.CancelRequestedAt != 0 {
		t.Fatal("intent without persistent retry survived transaction")
	}
	db.Exec(`DROP TRIGGER deny_cancel_job`)
	if err := dbpkg.RequestSubscriptionCancellation(db, a.ID, time.Now()); err != nil {
		t.Fatal(err)
	}
}
