package handlers

import (
	"api/config"
	"api/middleware"
	"api/servicelegal"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	dbpkg "database"
	"database/sql"
	"encoding/hex"
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

type serviceTransport func(*http.Request) (*http.Response, error)

func (f serviceTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func serviceTestFixture(t *testing.T) (*sql.DB, *dbpkg.License, string, *config.Config) {
	t.Helper()
	db, e := dbpkg.InitDatabase(filepath.Join(t.TempDir(), "billing.db"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { db.Close() })
	lic, e := dbpkg.CreateLicense(db, "merchant@example.test", 365, 5, "Pro")
	if e != nil {
		t.Fatal(e)
	}
	token, e := dbpkg.CreateTechnicianSession(db, lic.LicenseID)
	if e != nil {
		t.Fatal(e)
	}
	if e = db.QueryRow(`SELECT customer_id FROM licences WHERE license_id=?`, lic.LicenseID).Scan(&lic.CustomerID); e != nil {
		t.Fatal(e)
	}
	if _, e = dbpkg.EnsureServiceMerchant(db, lic.CustomerID); e != nil {
		t.Fatal(e)
	}
	db.Exec(`UPDATE service_merchants SET enabled=1,account_id='acct_merchant'`)
	if e = dbpkg.AcceptServiceTerms(db, lic.CustomerID, servicelegal.Version, servicelegal.Hash()); e != nil {
		t.Fatal(e)
	}
	return db, lic, token, &config.Config{ServicePaymentsEnabled: true, StripeSecretKey: "sk_test_fixture", StripeConnectWebhookSecret: "whsec_fixture", PublicWebsiteURL: "https://relaisdesk.fr"}
}
func serviceTestCall(h http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func TestServiceAPIIsolationAndDisabled(t *testing.T) {
	db, lic, token, c := serviceTestFixture(t)
	rate, e := dbpkg.CreateServiceRate(db, lic.CustomerID, dbpkg.ServiceRate{Label: "Help", Mode: "hourly", Cents: 6000})
	if e != nil {
		t.Fatal(e)
	}
	code, e := dbpkg.CreateViewerCode(db, lic.LicenseID, "")
	if e != nil {
		t.Fatal(e)
	}
	db.Exec(`UPDATE viewer_codes SET client_rustdesk_id='123456789' WHERE code=?`, code.Code)
	h := middleware.TechnicianAuth(db)(TechnicianServiceBillingHandler(db, c))
	body := fmt.Sprintf(`{"rate_id":%q,"target_kind":"code","target_id":%q,"client_agreed":true}`, rate.ID, code.Code)
	res := serviceTestCall(h, "POST", "/api/v1/technician/service-billing", token, body)
	if res.Code != 201 {
		t.Fatal(res.Code, res.Body.String())
	}
	var work dbpkg.ServiceWork
	json.Unmarshal(res.Body.Bytes(), &work)
	other, _ := dbpkg.CreateLicense(db, "other@example.test", 365, 5, "Pro")
	otherToken, _ := dbpkg.CreateTechnicianSession(db, other.LicenseID)
	res = serviceTestCall(h, "GET", "/api/v1/technician/service-billing/"+work.ID, otherToken, "")
	if res.Code != 404 {
		t.Fatal("tenant leak", res.Code)
	}
	c.ServicePaymentsEnabled = false
	res = serviceTestCall(h, "POST", "/api/v1/technician/service-billing", token, body)
	if res.Code != 503 {
		t.Fatal("disabled sales accepted")
	}
	c.ServicePaymentsEnabled = true
	res = serviceTestCall(h, "POST", "/api/v1/technician/service-billing/"+work.ID+"/checkout", token, `{}`)
	if res.Code != 409 {
		t.Fatal("hourly payment before completion")
	}
	res = serviceTestCall(h, "POST", "/api/v1/technician/service-billing", token, strings.Replace(body, "true", "false", 1))
	if res.Code != 400 {
		t.Fatal("missing client agreement accepted")
	}
}

func TestServiceOwnerTermsGateAndExpiredAccess(t *testing.T) {
	db, lic, _, c := serviceTestFixture(t)
	if _, err := db.Exec(`DELETE FROM service_terms_acceptances`); err != nil {
		t.Fatal(err)
	}
	h := CustomerServiceBillingHandler(db, c)
	call := func(role, method, suffix, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "/api/v1/customer/service-billing"+suffix, strings.NewReader(body))
		r = r.WithContext(context.WithValue(r.Context(), middleware.CustomerIdentityContextKey, &dbpkg.CustomerIdentity{ID: lic.CustomerID, Role: role}))
		w := httptest.NewRecorder()
		h(w, r)
		return w
	}
	old := serviceHTTPClient
	defer func() { serviceHTTPClient = old }()
	serviceHTTPClient = &http.Client{Transport: serviceTransport(func(r *http.Request) (*http.Response, error) {
		t.Fatal("Stripe called before legal checks")
		return nil, fmt.Errorf("unexpected call")
	})}
	for _, suffix := range []string{"/onboarding", "/rates"} {
		if res := call("owner", "POST", suffix, `{}`); res.Code != 409 {
			t.Fatal(suffix, res.Code, res.Body.String())
		}
	}
	if res := call("owner", "PUT", "", `{"enabled":true}`); res.Code != 409 {
		t.Fatal("enabled without terms", res.Code)
	}
	for _, body := range []string{`{"accepted":false}`, `{"accepted":true,"version":"old"}`, fmt.Sprintf(`{"accepted":true,"version":%q,"sha256":"wrong"}`, servicelegal.Version)} {
		if res := call("owner", "POST", "/terms/accept", body); res.Code != 409 {
			t.Fatal("invalid proof accepted", res.Code)
		}
	}
	body := fmt.Sprintf(`{"accepted":true,"version":%q,"sha256":%q}`, servicelegal.Version, servicelegal.Hash())
	if res := call("member", "POST", "/terms/accept", body); res.Code != 403 {
		t.Fatal("member accepted owner contract")
	}
	for i := 0; i < 2; i++ {
		if res := call("owner", "POST", "/terms/accept", body); res.Code != 200 {
			t.Fatal(res.Body.String())
		}
	}
	if _, err := db.Exec(`UPDATE licences SET expires_at=datetime('now','-1 day') WHERE license_id=?`, lic.LicenseID); err != nil {
		t.Fatal(err)
	}
	if res := call("owner", "GET", "", ""); res.Code != 200 || !strings.Contains(res.Body.String(), `"active_license":false`) {
		t.Fatal("expired owner lost access", res.Body.String())
	}
	if res := call("owner", "POST", "/onboarding", `{}`); res.Code != 409 {
		t.Fatal("expired owner can configure")
	}
	c.ServicePaymentsEnabled = false
	if res := call("owner", "PUT", "", `{"enabled":false}`); res.Code != 200 {
		t.Fatal("cannot disable after global stop", res.Body.String())
	}
}
func TestServiceCheckoutAndWebhook(t *testing.T) {
	db, lic, _, c := serviceTestFixture(t)
	rate, _ := dbpkg.CreateServiceRate(db, lic.CustomerID, dbpkg.ServiceRate{Label: "Assistance", Mode: "prepaid", Cents: 6000})
	work, e := dbpkg.CreateServiceWork(db, lic.CustomerID, lic.LicenseID, "", "device", "DEV-TEST-1234", "123456789", rate.ID)
	if e != nil {
		t.Fatal(e)
	}
	old := serviceHTTPClient
	defer func() { serviceHTTPClient = old }()
	created := 0
	amount := int64(6000)
	serviceHTTPClient = &http.Client{Transport: serviceTransport(func(r *http.Request) (*http.Response, error) {
		body := ""
		if strings.HasPrefix(r.URL.Path, "/v1/accounts/") {
			body = `{"id":"acct_merchant","charges_enabled":true,"payouts_enabled":true,"details_submitted":true}`
		} else {
			if r.Header.Get("Stripe-Account") != "acct_merchant" {
				t.Fatal("charge not on merchant")
			}
			if r.Method == "POST" {
				created++
				r.ParseForm()
				if r.Form.Get("line_items[0][price_data][unit_amount]") != "6000" || r.Form.Get("payment_intent_data[application_fee_amount]") != "" || r.Header.Get("Idempotency-Key") == "" {
					t.Fatal("incorrect charge")
				}
			}
			body = fmt.Sprintf(`{"id":"cs_fixture","url":"https://checkout.stripe.com/c/pay/cs_fixture","status":"complete","payment_status":"paid","mode":"payment","currency":"eur","amount_total":%d,"client_reference_id":%q,"livemode":false}`, amount, work.ID)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	if e = serviceCheckout(db, c, work); e != nil {
		t.Fatal(e)
	}
	if e = serviceCheckout(db, c, work); e != nil {
		t.Fatal(e)
	}
	if created != 1 {
		t.Fatal("double session")
	}
	webhook := ServiceStripeWebhookHandler(db, c)
	call := func(account, signature string) *httptest.ResponseRecorder {
		raw := fmt.Sprintf(`{"type":"checkout.session.completed","account":%q,"livemode":false,"data":{"object":{"id":"cs_fixture"}}}`, account)
		at := fmt.Sprint(time.Now().Unix())
		mac := hmac.New(sha256.New, []byte(c.StripeConnectWebhookSecret))
		mac.Write([]byte(at + "." + raw))
		sig := "t=" + at + ",v1=" + hex.EncodeToString(mac.Sum(nil))
		if signature != "" {
			sig = signature
		}
		r := httptest.NewRequest("POST", "/api/v1/stripe/connect-webhook", strings.NewReader(raw))
		r.Header.Set("Stripe-Signature", sig)
		res := httptest.NewRecorder()
		webhook(res, r)
		return res
	}
	if res := call("acct_merchant", "bad"); res.Code != 400 {
		t.Fatal("unsigned webhook accepted")
	}
	call("acct_other", "")
	fresh, _ := dbpkg.GetServiceWork(db, work.ID)
	if fresh.Paid {
		t.Fatal("wrong account settled")
	}
	amount = 1
	if res := call("acct_merchant", ""); res.Code != 400 {
		t.Fatal("wrong amount accepted")
	}
	amount = 6000
	for i := 0; i < 2; i++ {
		if res := call("acct_merchant", ""); res.Code != 200 {
			t.Fatal(res.Code, res.Body.String())
		}
	}
	fresh, _ = dbpkg.GetServiceWork(db, work.ID)
	if !fresh.Paid {
		t.Fatal("payment missing")
	}
}

func TestServiceTeamFolderAndOwnerAuthorization(t *testing.T) {
	db, lic, _, c := serviceTestFixture(t)
	folder, err := dbpkg.CreateDeviceFolder(db, lic.CustomerID, lic.LicenseID, "Allowed", "")
	if err != nil {
		t.Fatal(err)
	}
	memberToken := func(email string) string {
		m, e := dbpkg.CreateTeamInvitation(db, lic.CustomerID, lic.LicenseID, email, []string{folder.FolderID})
		if e != nil {
			t.Fatal(e)
		}
		invitation, _, e := dbpkg.IssueTeamInvitationToken(db, lic.CustomerID, m.MemberID)
		if e != nil {
			t.Fatal(e)
		}
		if _, _, e = dbpkg.AcceptTeamInvitation(db, invitation); e != nil {
			t.Fatal(e)
		}
		token, e := dbpkg.CreatePersonalTechnicianSession(db, email, lic.LicenseID)
		if e != nil {
			t.Fatal(e)
		}
		return token
	}
	token, other := memberToken("member1@example.test"), memberToken("member2@example.test")
	d, err := dbpkg.CreatePermanentEnrollment(db, lic.CustomerID, lic.LicenseID, "Poste", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE devices SET enrollment_version=2,peer_auth_version=1,rustdesk_id='123456789',folder_id=? WHERE device_id=?`, folder.FolderID, d.DeviceID); err != nil {
		t.Fatal(err)
	}
	rate, err := dbpkg.CreateServiceRate(db, lic.CustomerID, dbpkg.ServiceRate{Label: "Hourly", Mode: "hourly", Cents: 6000})
	if err != nil {
		t.Fatal(err)
	}
	h := middleware.TechnicianAuth(db)(TechnicianServiceBillingHandler(db, c))
	body := fmt.Sprintf(`{"rate_id":%q,"target_kind":"device","target_id":%q,"client_agreed":true}`, rate.ID, d.DeviceID)
	res := serviceTestCall(h, "POST", "/api/v1/technician/service-billing", token, body)
	if res.Code != 201 {
		t.Fatal(res.Code, res.Body.String())
	}
	var work dbpkg.ServiceWork
	if err = json.Unmarshal(res.Body.Bytes(), &work); err != nil {
		t.Fatal(err)
	}
	if res = serviceTestCall(h, "GET", "/api/v1/technician/service-billing/"+work.ID, other, ""); res.Code != 404 {
		t.Fatal("colleague work leaked", res.Code)
	}
	if _, err = db.Exec(`UPDATE devices SET folder_id='' WHERE device_id=?`, d.DeviceID); err != nil {
		t.Fatal(err)
	}
	if res = serviceTestCall(h, "GET", "/api/v1/technician/service-billing/"+work.ID, token, ""); res.Code != 403 {
		t.Fatal("moved target still accessible")
	}
	if res = serviceTestCall(h, "POST", "/api/v1/technician/service-billing", token, body); res.Code != 403 {
		t.Fatal("unauthorized target billed")
	}
	if _, err = db.Exec(`UPDATE devices SET folder_id=?,is_active=0 WHERE device_id=?`, folder.FolderID, d.DeviceID); err != nil {
		t.Fatal(err)
	}
	if res = serviceTestCall(h, "POST", "/api/v1/technician/service-billing", token, body); res.Code != 403 {
		t.Fatal("revoked target billed")
	}
	ownerHandler := CustomerServiceBillingHandler(db, c)
	ownerCall := func(role string, customer int64, path string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", path, strings.NewReader(`{}`))
		r = r.WithContext(context.WithValue(r.Context(), middleware.CustomerIdentityContextKey, &dbpkg.CustomerIdentity{ID: customer, Role: role}))
		w := httptest.NewRecorder()
		ownerHandler(w, r)
		return w
	}
	path := "/api/v1/customer/service-billing/work/" + work.ID + "/finish"
	if res = ownerCall("member", lic.CustomerID, path); res.Code != 403 {
		t.Fatal("member acquired owner actions")
	}
	c.ServicePaymentsEnabled = false
	if res = ownerCall("owner", lic.CustomerID, path); res.Code != 200 {
		t.Fatal("owner cannot close orphaned work when disabled", res.Body.String())
	}
	otherLic, err := dbpkg.CreateLicense(db, "other-owner@example.test", 365, 5, "Pro")
	if err != nil {
		t.Fatal(err)
	}
	var otherCustomer int64
	if err = db.QueryRow(`SELECT customer_id FROM licences WHERE license_id=?`, otherLic.LicenseID).Scan(&otherCustomer); err != nil {
		t.Fatal(err)
	}
	if res = ownerCall("owner", otherCustomer, path); res.Code != 404 {
		t.Fatal("foreign owner action accepted")
	}
}
