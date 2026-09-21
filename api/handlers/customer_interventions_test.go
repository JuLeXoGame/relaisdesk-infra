package handlers

import (
	"bytes"
	dbpkg "database"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"api/middleware"
)

func TestCustomerInterventionActions(t *testing.T) {
	db, err := dbpkg.InitDatabase(filepath.Join(t.TempDir(), "customer-interventions.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	setupCustomer := func(email string) (string, *dbpkg.License) {
		order, err := dbpkg.CreateOrderWithBilling(db, email, "starter", 1, "stripe", "", "", &dbpkg.BillingDetails{
			Name: email, Address: "1 rue Test", PostalCode: "75001", City: "Paris", CustomerType: "business",
			TermsVersion: publicTermsVersion, TermsAccepted: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		lic, err := dbpkg.FulfillPendingOrder(db, order.OrderID, "test")
		if err != nil {
			t.Fatal(err)
		}
		token, _, err := dbpkg.CreateCustomerLoginToken(db, email)
		if err != nil {
			t.Fatal(err)
		}
		session, _, err := dbpkg.ConsumeCustomerLoginToken(db, token)
		if err != nil {
			t.Fatal(err)
		}
		return session, lic
	}
	sessionA, licA := setupCustomer("clientA@example.com")
	sessionB, _ := setupCustomer("clientB@example.com")

	handler := middleware.CustomerAuth(db)(CustomerInterventionActionHandler(db))
	call := func(url, token, body string) *httptest.ResponseRecorder {
		var reader *bytes.Buffer
		if body == "" {
			reader = bytes.NewBuffer(nil)
		} else {
			reader = bytes.NewBufferString(body)
		}
		req := httptest.NewRequest(http.MethodPost, url, reader)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}
	decode := func(rec *httptest.ResponseRecorder) map[string]any {
		t.Helper()
		var resp map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("invalid JSON response: %v body=%s", err, rec.Body.String())
		}
		return resp
	}

	item, err := dbpkg.CreateIntervention(db, licA.LicenseID, nil, "ref-a", "Assistance")
	if err != nil {
		t.Fatal(err)
	}

	// 1. Start a planned intervention.
	rec := call("/api/v1/customer/interventions/"+item.InterventionID+"/start", sessionA, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("start status=%d body=%s", rec.Code, rec.Body.String())
	}
	if got := decode(rec)["status"]; got != "in_progress" {
		t.Fatalf("start status=%v want=in_progress", got)
	}

	// 2. Complete it with a summary.
	rec = call("/api/v1/customer/interventions/"+item.InterventionID+"/complete", sessionA, `{"summary":"Tout est rentre dans l'ordre"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("complete status=%d body=%s", rec.Code, rec.Body.String())
	}
	completed := decode(rec)
	if completed["status"] != "completed" {
		t.Fatalf("complete status=%v want=completed", completed["status"])
	}
	if completed["summary"] != "Tout est rentre dans l'ordre" {
		t.Fatalf("complete summary=%v not persisted", completed["summary"])
	}
	duration, _ := completed["duration_minutes"].(float64)
	if duration < 1 {
		t.Fatalf("complete duration_minutes=%v want>=1", completed["duration_minutes"])
	}

	// 3. Completing again stays 200 (idempotent), cancelling a completed fiche fails.
	rec = call("/api/v1/customer/interventions/"+item.InterventionID+"/complete", sessionA, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("re-complete status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = call("/api/v1/customer/interventions/"+item.InterventionID+"/cancel", sessionA, "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("cancel completed status=%d want=400 body=%s", rec.Code, rec.Body.String())
	}

	// 4. Cancel a planned intervention.
	planned, err := dbpkg.CreateIntervention(db, licA.LicenseID, nil, "ref-b", "Assistance")
	if err != nil {
		t.Fatal(err)
	}
	rec = call("/api/v1/customer/interventions/"+planned.InterventionID+"/cancel", sessionA, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("cancel status=%d body=%s", rec.Code, rec.Body.String())
	}
	if got := decode(rec)["status"]; got != "cancelled" {
		t.Fatalf("cancel status=%v want=cancelled", got)
	}

	// 5. Unknown action, malformed path, unknown reference.
	rec = call("/api/v1/customer/interventions/"+planned.InterventionID+"/archive", sessionA, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown action status=%d want=404 body=%s", rec.Code, rec.Body.String())
	}
	rec = call("/api/v1/customer/interventions/"+planned.InterventionID, sessionA, "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("malformed path status=%d want=400 body=%s", rec.Code, rec.Body.String())
	}
	rec = call("/api/v1/customer/interventions/INT-DOESNOTEXIST/start", sessionA, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown reference status=%d want=404 body=%s", rec.Code, rec.Body.String())
	}

	// 6. Another customer cannot act on the fiche, anonymous calls are rejected.
	rec = call("/api/v1/customer/interventions/"+planned.InterventionID+"/start", sessionB, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross-customer status=%d want=404 body=%s", rec.Code, rec.Body.String())
	}
	rec = call("/api/v1/customer/interventions/"+planned.InterventionID+"/start", "", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous status=%d want=401 body=%s", rec.Code, rec.Body.String())
	}
}
