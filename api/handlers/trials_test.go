package handlers

import (
	"api/config"
	"api/middleware"
	"bytes"
	dbpkg "database"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type billingTransport func(*http.Request) (*http.Response, error)

func fakeStripeID(prefix, id string) string { return prefix + strings.ReplaceAll(id, "-", "X") }

func (f billingTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func mockBilling(t *testing.T, fn func(*http.Request) any) {
	t.Helper()
	original := subscriptionHTTPClient
	t.Cleanup(func() { subscriptionHTTPClient = original })
	subscriptionHTTPClient = &http.Client{Transport: billingTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "api.stripe.com" || r.URL.Scheme != "https" || r.Header.Get("Stripe-Version") != subscriptionAPIVersion {
			t.Fatal("unexpected Stripe target/version")
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		body, err := json.Marshal(fn(r))
		if err != nil {
			t.Fatal(err)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(body))}, nil
	})}
}
func billingFixture(t *testing.T) (*sql.DB, *config.Config, *dbpkg.Trial) {
	t.Helper()
	db, err := dbpkg.InitDatabase(filepath.Join(t.TempDir(), "trial.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	cfg := &config.Config{TrialsEnabled: true, TrialFingerprintKey: base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{4}, 32)), StripeSecretKey: "sk_test_fixture", StripeWebhookSecret: "whsec_fixture", SMTPHost: "smtp.example.test", SMTPPort: 587, SMTPFrom: "RelaisDesk <test@example.com>", PublicWebsiteURL: "https://relaisdesk.fr", InvoicesDir: t.TempDir()}
	a := newBillingTrial(t, db, "trial@example.com")
	return db, cfg, a
}
func newBillingTrial(t *testing.T, db *sql.DB, email string) *dbpkg.Trial {
	t.Helper()
	now := time.Now().UTC()
	a, err := dbpkg.CreateTrialApplication(db, email, "pro", 5, dbpkg.BillingDetails{Name: "Entreprise", Address: "1 rue Test", PostalCode: "75001", City: "Paris", Country: "France", CustomerType: "business", TermsVersion: dbpkg.TrialTermsVersion, TermsAccepted: true, BillingCycle: "monthly"}, now)
	if err != nil {
		t.Fatal(err)
	}
	token, err := dbpkg.CreateTrialEmailToken(db, a.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	a, err = dbpkg.VerifyTrialEmail(db, token, now)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`UPDATE trial_applications SET stripe_customer_id=?,checkout_session_id=? WHERE id=?`, fakeStripeID("cus_", a.ID), fakeStripeID("cs_", a.ID), a.ID)
	if err != nil {
		t.Fatal(err)
	}
	a, err = dbpkg.GetTrial(db, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	return a
}
func setupResponse(a *dbpkg.Trial, fingerprint string) any {
	return map[string]any{
		"id": a.CheckoutSessionID, "mode": "setup", "status": "complete", "customer": a.StripeCustomerID, "client_reference_id": a.ID, "metadata": map[string]string{"relaisdesk_trial": a.ID},
		"setup_intent": map[string]any{"id": "seti_ok", "status": "succeeded", "customer": a.StripeCustomerID, "metadata": map[string]string{"relaisdesk_trial": a.ID}, "payment_method": map[string]any{"id": "pm_ok", "customer": a.StripeCustomerID, "type": "card", "card": map[string]string{"fingerprint": fingerprint}}}}
}
func subscriptionResponse(a *dbpkg.Trial) map[string]any {
	interval := "month"
	if a.BillingCycle == "annual" {
		interval = "year"
	}
	return map[string]any{
		"id": fakeStripeID("sub_", a.ID), "customer": a.StripeCustomerID, "status": "trialing", "trial_start": a.TrialStart, "trial_end": a.TrialEnd, "current_period_end": a.TrialEnd, "cancel_at_period_end": false, "metadata": map[string]string{"relaisdesk_trial": a.ID},
		"items": map[string]any{"data": []any{map[string]any{"quantity": 1, "price": map[string]any{"id": "price_test", "currency": "eur", "unit_amount": a.PriceCents, "recurring": map[string]any{"interval": interval, "interval_count": 1}}}}}}
}

func TestTrialSetupChecksCardBeforeSubscriptionAndRetry(t *testing.T) {
	db, cfg, a := billingFixture(t)
	creates := 0
	var sub map[string]any
	mockBilling(t, func(r *http.Request) any {
		switch {
		case strings.HasPrefix(r.URL.Path, "/v1/checkout/sessions/"):
			return setupResponse(a, "fingerprint-one")
		case r.URL.Path == "/v1/subscriptions" && r.Method == "GET":
			if sub == nil {
				return map[string]any{"data": []any{}}
			}
			return map[string]any{"data": []any{sub}}
		case r.URL.Path == "/v1/products/"+trialProductID:
			return map[string]string{"id": trialProductID}
		case r.URL.Path == "/v1/subscriptions" && r.Method == "POST":
			creates++
			a, _ = dbpkg.GetTrial(db, a.ID)
			if a.State != "provisioning" {
				t.Fatal("subscription before atomic eligibility check")
			}
			if r.Form.Get("trial_end") != fmt.Sprint(a.TrialEnd) || a.TrialEnd-a.TrialStart != 30*86400 || r.Form.Get("items[0][price_data][unit_amount]") != "11000" {
				t.Fatal("wrong trial contract", r.Form)
			}
			if r.Header.Get("Idempotency-Key") == "" {
				t.Fatal("missing idempotency")
			}
			sub = subscriptionResponse(a)
			return sub
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
			return nil
		}
	})
	if err := completeTrialSetup(db, cfg, a.CheckoutSessionID); err != nil {
		t.Fatal(err)
	}
	if err := completeTrialSetup(db, cfg, a.CheckoutSessionID); err != nil {
		t.Fatal(err)
	}
	if creates != 1 {
		t.Fatalf("subscriptions created=%d", creates)
	}
	savedKey := cfg.TrialFingerprintKey
	cfg.TrialsEnabled = false
	cfg.TrialFingerprintKey = ""
	if err := completeTrialSetup(db, cfg, a.CheckoutSessionID); err != nil {
		t.Fatal("existing subscription recovery depends on new-trial settings", err)
	}
	cfg.TrialsEnabled = true
	cfg.TrialFingerprintKey = savedKey
	a, _ = dbpkg.GetTrial(db, a.ID)
	if a.LicenseID == "" {
		t.Fatal("missing trial licence")
	}
	// A second email with the same card must never reach POST /subscriptions.
	a = newBillingTrial(t, db, "repeat@example.com")
	if err := completeTrialSetup(db, cfg, a.CheckoutSessionID); err != nil {
		t.Fatal(err)
	}
	a, _ = dbpkg.GetTrial(db, a.ID)
	if a.State != "rejected" || a.SubscriptionID != "" || creates != 1 {
		t.Fatal("repeat trial created a billable subscription")
	}
}

func TestTrialRefusesUnverifiedCardAndCrossCustomer(t *testing.T) {
	for _, mismatch := range []string{"status", "customer", "fingerprint", "session"} {
		t.Run(mismatch, func(t *testing.T) {
			db, cfg, a := billingFixture(t)
			mockBilling(t, func(r *http.Request) any {
				out := setupResponse(a, "card").(map[string]any)
				si := out["setup_intent"].(map[string]any)
				switch mismatch {
				case "status":
					si["status"] = "requires_action"
				case "customer":
					si["customer"] = "cus_else"
				case "fingerprint":
					si["payment_method"].(map[string]any)["card"] = map[string]string{}
				case "session":
					out["id"] = "cs_else"
				}
				return out
			})
			if err := completeTrialSetup(db, cfg, a.CheckoutSessionID); err == nil {
				t.Fatal("unverified card accepted")
			}
			a, _ = dbpkg.GetTrial(db, a.ID)
			if a.State != "verified" || a.LicenseID != "" {
				t.Fatal("access granted on failed check")
			}
		})
	}
}

func TestTrialCheckoutUsesSetupNoImmediatePayment(t *testing.T) {
	db, cfg, a := billingFixture(t)
	mockBilling(t, func(r *http.Request) any {
		if r.URL.Path != "/v1/checkout/sessions" || r.Form.Get("mode") != "setup" || r.Form.Get("customer") != a.StripeCustomerID {
			t.Fatal("not a setup checkout")
		}
		summary := r.Form.Get("custom_text[submit][message]")
		if !strings.Contains(summary, "110.00 EUR/mois") || !strings.Contains(summary, "30 jours") || !strings.Contains(summary, "automatiquement") {
			t.Fatal("missing disclosure")
		}
		if r.Form.Get("line_items[0][price]") != "" {
			t.Fatal("setup contains a charge")
		}
		return map[string]string{"id": a.CheckoutSessionID, "url": "https://checkout.stripe.com/c/pay/test"}
	})
	if _, err := trialCheckout(cfg, db, a); err != nil {
		t.Fatal(err)
	}
}

func TestTrialConsentFeatureGateAndB2C(t *testing.T) {
	db, cfg, _ := billingFixture(t)
	request := func(cfg *config.Config, body string) int {
		rec := httptest.NewRecorder()
		PublicTrialRequestHandler(db, cfg).ServeHTTP(rec, httptest.NewRequest("POST", "/", strings.NewReader(body)))
		return rec.Code
	}
	if code := request(&config.Config{}, `{}`); code != 503 {
		t.Fatal(code)
	}
	base := map[string]any{"email": "new@example.com", "plan": "starter", "technicians": 1, "billing_cycle": "monthly", "name": "Client", "address": "1 rue", "postal_code": "75001", "city": "Paris", "customer_type": "business", "terms_accepted": true, "terms_version": publicTermsVersion, "recurring_accepted": true, "trial_terms_version": dbpkg.TrialTermsVersion}
	for _, field := range []string{"terms_accepted", "recurring_accepted", "trial_terms_version"} {
		saved := base[field]
		delete(base, field)
		raw, _ := json.Marshal(base)
		if code := request(cfg, string(raw)); code != 400 {
			t.Fatalf("missing %s = %d", field, code)
		}
		base[field] = saved
	}
	for field, stale := range map[string]string{"terms_version": "2026-09-11", "trial_terms_version": "2026-09-11-fleet-v2"} {
		saved := base[field]
		base[field] = stale
		raw, _ := json.Marshal(base)
		if code := request(cfg, string(raw)); code != 400 {
			t.Fatalf("outdated %s accepted: %d", field, code)
		}
		base[field] = saved
	}
	base["customer_type"] = "consumer"
	raw, _ := json.Marshal(base)
	if code := request(cfg, string(raw)); code != 503 {
		t.Fatal("B2C opened implicitly", code)
	}
	base["customer_type"] = "business"
	base["expected_price_cents"] = 1
	raw, _ = json.Marshal(base)
	if code := request(cfg, string(raw)); code != 409 {
		t.Fatal("stale price accepted", code)
	}
	base["expected_price_cents"] = 2490
	raw, _ = json.Marshal(base)
	if code := request(cfg, string(raw)); code != 202 {
		t.Fatal("valid request rejected", code)
	}
}

func TestTrialCancellationIsScopedAndPreservesTrialExpiry(t *testing.T) {
	db, cfg, a := billingFixture(t)
	a, err := dbpkg.ReserveTrial(db, a.ID, cfg.TrialKey(), "cancel-card", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	a, _, err = dbpkg.ActivateTrial(db, a.ID, fakeStripeID("sub_", a.ID), a.TrialEnd)
	if err != nil {
		t.Fatal(err)
	}
	sub := subscriptionResponse(a)
	cancels := 0
	mockBilling(t, func(r *http.Request) any {
		if r.Method == "POST" {
			cancels++
			if r.Form.Get("cancel_at_period_end") != "true" {
				t.Fatal("must cancel at period end")
			}
			sub["cancel_at_period_end"] = true
		}
		return sub
	})
	handler := middleware.CustomerAuth(db)(CustomerCancelSubscriptionHandler(db, cfg))
	session := func(email string) string {
		token, ok, err := dbpkg.CreateCustomerLoginToken(db, email)
		if err != nil || !ok {
			t.Fatal(err)
		}
		s, _, err := dbpkg.ConsumeCustomerLoginToken(db, token)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	call := func(token string) int {
		rec := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/", strings.NewReader(`{"id":"`+a.ID+`"}`))
		r.Header.Set("Authorization", "Bearer "+token)
		handler.ServeHTTP(rec, r)
		return rec.Code
	}
	other := newBillingTrial(t, db, "other@example.com")
	if code := call(session(other.Email)); code != 404 || cancels != 0 {
		t.Fatal("cross-account cancellation", code)
	}
	if code := call(session(a.Email)); code != 200 || cancels != 1 {
		t.Fatal("cancellation failed", code)
	}
	lic, _ := dbpkg.GetLicense(db, a.LicenseID)
	if lic.ExpiresAt.Unix() != a.TrialEnd {
		t.Fatal("cancellation changed entitled trial")
	}
}

func TestSubscriptionInvoiceRecheckRejectsZeroAndWrongAmounts(t *testing.T) {
	db, cfg, a := billingFixture(t)
	a, err := dbpkg.ReserveTrial(db, a.ID, cfg.TrialKey(), "invoice-card", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	a, _, err = dbpkg.ActivateTrial(db, a.ID, fakeStripeID("sub_", a.ID), a.TrialEnd)
	if err != nil {
		t.Fatal(err)
	}
	amount := int64(0)
	mockBilling(t, func(r *http.Request) any {
		if strings.HasPrefix(r.URL.Path, "/v1/subscriptions/") {
			return subscriptionResponse(a)
		}
		return map[string]any{"id": "in_test", "subscription": a.SubscriptionID, "customer": a.StripeCustomerID, "status": "paid", "paid": true, "currency": "eur", "total": amount, "amount_paid": amount, "billing_reason": "subscription_cycle", "lines": map[string]any{"has_more": false, "data": []any{map[string]any{"type": "subscription", "subscription": a.SubscriptionID, "quantity": 1, "price": map[string]string{"id": "price_test"}, "currency": "eur", "amount": amount, "period": map[string]int64{"start": a.TrialEnd, "end": a.TrialEnd + 30*86400}}}}}
	})
	if err = processSubscriptionInvoice(db, cfg, "in_test"); err != nil {
		t.Fatal(err)
	}
	lic, _ := dbpkg.GetLicense(db, a.LicenseID)
	if lic.ExpiresAt.Unix() != a.TrialEnd {
		t.Fatal("zero invoice prolonged trial")
	}
	amount = 1
	if err = processSubscriptionInvoice(db, cfg, "in_test"); err == nil {
		t.Fatal("wrong amount accepted")
	}
	amount = a.PriceCents
	if err = processSubscriptionInvoice(db, cfg, "in_test"); err != nil {
		t.Fatal(err)
	}
	if err = processSubscriptionInvoice(db, cfg, "in_test"); err != nil {
		t.Fatal(err)
	}
	lic, _ = dbpkg.GetLicense(db, a.LicenseID)
	if lic.ExpiresAt.Unix() != a.TrialEnd+30*86400 {
		t.Fatal("paid period incorrect")
	}
}
