package handlers

import (
	"api/middleware"
	"bytes"
	dbpkg "database"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCustomerBillingPortal(t *testing.T) {
	db, cfg, trial := billingFixture(t)

	withIdentity := func(customerID int64, body string) *http.Request {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/customer/billing-portal", bytes.NewReader([]byte(body)))
		return req.WithContext(context.WithValue(req.Context(), middleware.CustomerIdentityContextKey, &dbpkg.CustomerIdentity{ID: customerID}))
	}
	call := func(req *http.Request) (int, map[string]any) {
		rec := httptest.NewRecorder()
		CustomerBillingPortalHandler(db, cfg)(rec, req)
		var payload map[string]any
		if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return rec.Code, payload
	}

	// 401 without customer identity.
	code, _ := call(httptest.NewRequest(http.MethodPost, "/api/v1/customer/billing-portal", bytes.NewReader([]byte(`{"id":"x"}`))))
	if code != http.StatusUnauthorized {
		t.Fatalf("no identity status = %d, want 401", code)
	}

	owner, err := dbpkg.EnsureCustomer(db, "trial@example.com", "business", "Owner")
	if err != nil {
		t.Fatal(err)
	}
	stranger, err := dbpkg.EnsureCustomer(db, "stranger@example.com", "business", "Stranger")
	if err != nil {
		t.Fatal(err)
	}

	// 404 unknown trial or wrong owner.
	code, _ = call(withIdentity(owner.ID, `{"id":"does-not-exist"}`))
	if code != http.StatusNotFound {
		t.Fatalf("unknown trial status = %d, want 404", code)
	}
	if _, err := db.Exec(`UPDATE trial_applications SET customer_id = ? WHERE id = ?`, owner.ID, trial.ID); err != nil {
		t.Fatal(err)
	}
	code, _ = call(withIdentity(stranger.ID, `{"id":"`+trial.ID+`"}`))
	if code != http.StatusNotFound {
		t.Fatalf("wrong owner status = %d, want 404", code)
	}
	code, _ = call(withIdentity(owner.ID, `{"id":""}`))
	if code != http.StatusBadRequest {
		t.Fatalf("empty id status = %d, want 400", code)
	}

	// 409 when no Stripe customer is attached yet.
	if _, err := db.Exec(`UPDATE trial_applications SET stripe_customer_id = '' WHERE id = ?`, trial.ID); err != nil {
		t.Fatal(err)
	}
	code, _ = call(withIdentity(owner.ID, `{"id":"`+trial.ID+`"}`))
	if code != http.StatusConflict {
		t.Fatalf("no stripe customer status = %d, want 409", code)
	}

	// 503 when Stripe is not configured.
	broken := *cfg
	broken.StripeSecretKey = ""
	rec := httptest.NewRecorder()
	CustomerBillingPortalHandler(db, &broken)(rec, withIdentity(owner.ID, `{"id":"`+trial.ID+`"}`))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("no stripe key status = %d, want 503", rec.Code)
	}

	// 200 with a mocked Stripe portal session.
	if _, err := db.Exec(`UPDATE trial_applications SET stripe_customer_id = 'cus_test_123' WHERE id = ?`, trial.ID); err != nil {
		t.Fatal(err)
	}
	mockBilling(t, func(r *http.Request) any {
		if r.URL.Path != "/v1/billing_portal/sessions" {
			t.Fatalf("stripe path = %s", r.URL.Path)
		}
		if got := r.PostForm.Get("customer"); got != "cus_test_123" {
			t.Fatalf("customer = %q", got)
		}
		if got := r.PostForm.Get("return_url"); got != "https://relaisdesk.fr/client/" {
			t.Fatalf("return_url = %q", got)
		}
		return map[string]any{"url": "https://billing.stripe.com/session/test_123"}
	})
	code, payload := call(withIdentity(owner.ID, `{"id":"`+trial.ID+`"}`))
	if code != http.StatusOK {
		t.Fatalf("status = %d, payload = %v", code, payload)
	}
	if payload["url"] != "https://billing.stripe.com/session/test_123" {
		t.Errorf("url = %v", payload["url"])
	}
}
