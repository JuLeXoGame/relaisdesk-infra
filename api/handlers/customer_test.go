package handlers

import (
	"bytes"
	dbpkg "database"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"api/config"
	"api/middleware"
)

func TestCustomerDashboardIsStrictlyScopedAndNeverReturnsLicenseKey(t *testing.T) {
	db, err := dbpkg.InitDatabase(filepath.Join(t.TempDir(), "customer-api.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	create := func(email string) (*dbpkg.Order, *dbpkg.License) {
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
		return order, lic
	}
	ownerOrder, ownerLicense := create("owner@example.com")
	otherOrder, otherLicense := create("other@example.com")
	token, eligible, err := dbpkg.CreateCustomerLoginToken(db, "owner@example.com")
	if err != nil || !eligible {
		t.Fatal(err)
	}
	session, _, err := dbpkg.ConsumeCustomerLoginToken(db, token)
	if err != nil {
		t.Fatal(err)
	}

	handler := middleware.CustomerAuth(db)(CustomerDashboardHandler(db))
	request := httptest.NewRequest(http.MethodGet, "/api/v1/customer/dashboard", nil)
	request.Header.Set("Authorization", "Bearer "+session)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	if !strings.Contains(body, ownerOrder.OrderID) || !strings.Contains(body, ownerLicense.LicenseID) {
		t.Fatalf("owner data missing: %s", body)
	}
	if strings.Contains(body, otherOrder.OrderID) || strings.Contains(body, otherLicense.LicenseID) {
		t.Fatalf("cross-customer data leak: %s", body)
	}
	if strings.Contains(body, ownerLicense.LicenseKey) || strings.Contains(body, "license_key") {
		t.Fatalf("license secret leaked in commercial dashboard: %s", body)
	}
}

func TestCustomerLoginRequestDoesNotEnumerateAccounts(t *testing.T) {
	db, err := dbpkg.InitDatabase(filepath.Join(t.TempDir(), "login-api.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	lic, err := dbpkg.CreateLicense(db, "known@example.com", 30, 1, "")
	if err != nil || lic == nil {
		t.Fatal(err)
	}
	handler := CustomerLoginRequestHandler(db, &config.Config{DevHTTP: true, PublicWebsiteURL: "https://relaisdesk.fr"}, nil)
	call := func(email string) (int, string) {
		body, _ := json.Marshal(map[string]string{"email": email})
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/customer/login/request", bytes.NewReader(body)))
		return recorder.Code, recorder.Body.String()
	}
	knownStatus, knownBody := call("known@example.com")
	unknownStatus, unknownBody := call("unknown@example.com")
	if knownStatus != http.StatusAccepted || unknownStatus != http.StatusAccepted || knownBody != unknownBody {
		t.Fatalf("account enumeration possible: known=(%d,%q) unknown=(%d,%q)", knownStatus, knownBody, unknownStatus, unknownBody)
	}
}

func TestCustomerCannotRenewAnotherCustomersLicense(t *testing.T) {
	db, err := dbpkg.InitDatabase(filepath.Join(t.TempDir(), "renew-api.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ownerLicense, err := dbpkg.CreateLicense(db, "owner@example.com", 30, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	otherLicense, err := dbpkg.CreateLicense(db, "other@example.com", 30, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := dbpkg.CreateCustomerLoginToken(db, ownerLicense.Email)
	if err != nil {
		t.Fatal(err)
	}
	session, _, err := dbpkg.ConsumeCustomerLoginToken(db, token)
	if err != nil {
		t.Fatal(err)
	}
	body := bytes.NewBufferString(`{"payment_method":"stripe","terms_version":"` + publicTermsVersion + `","terms_accepted":true}`)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/customer/licenses/"+otherLicense.LicenseID+"/renew", body)
	request.Header.Set("Authorization", "Bearer "+session)
	recorder := httptest.NewRecorder()
	middleware.CustomerAuth(db)(CustomerRenewLicenseHandler(db, &config.Config{DevHTTP: true, StripeMock: true}, nil)).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status=%d want=404 body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestCustomerPasswordLoginAndResetFlow(t *testing.T) {
	db, err := dbpkg.InitDatabase(filepath.Join(t.TempDir(), "customer-pwd-api.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	email := "client-pwd@example.com"
	_, err = dbpkg.CreateLicense(db, email, 30, 1, "")
	if err != nil {
		t.Fatal(err)
	}

	loginHandler := CustomerLoginWithPasswordHandler(db)
	resetHandler := CustomerSetPasswordWithTokenHandler(db)
	changeHandler := middleware.CustomerAuth(db)(CustomerChangePasswordHandler(db))

	// 1. Attempt login before setting password -> generic 401 like any bad
	// credentials (revealing "no password set" would enumerate accounts).
	loginBody := bytes.NewBufferString(`{"email":"` + email + `","password":"SomePassword123"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/customer/login", loginBody)
	rec := httptest.NewRecorder()
	loginHandler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Body.String(), "Identifiants incorrects") {
		t.Fatalf("expected 401 generic, got status %d: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "need_password_setup") {
		t.Fatalf("password-setup state leaked: %s", rec.Body.String())
	}

	// 2. Request reset token and set password
	token, eligible, err := dbpkg.CreateCustomerLoginToken(db, email)
	if err != nil || !eligible {
		t.Fatalf("failed to create token: %v", err)
	}

	setBody := bytes.NewBufferString(`{"token":"` + token + `","password":"MySecurePassword2026!"}`)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/customer/password/reset", setBody)
	rec = httptest.NewRecorder()
	resetHandler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on reset, got %d: %s", rec.Code, rec.Body.String())
	}

	var resetResp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resetResp); err != nil || resetResp["token"] == "" {
		t.Fatalf("invalid reset response: %v", err)
	}

	// 3. Login with wrong password -> 401
	loginBody = bytes.NewBufferString(`{"email":"` + email + `","password":"WrongPassword!"}`)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/customer/login", loginBody)
	rec = httptest.NewRecorder()
	loginHandler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 on bad password, got %d", rec.Code)
	}

	// 4. Login with correct password -> 200
	loginBody = bytes.NewBufferString(`{"email":"` + email + `","password":"MySecurePassword2026!"}`)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/customer/login", loginBody)
	rec = httptest.NewRecorder()
	loginHandler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on good login, got %d: %s", rec.Code, rec.Body.String())
	}
	var loginResp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &loginResp); err != nil || loginResp["token"] == "" {
		t.Fatalf("invalid login response: %v", err)
	}
	sessionToken := loginResp["token"]

	// 5. Change password using authenticated session
	changeBody := bytes.NewBufferString(`{"old_password":"MySecurePassword2026!","new_password":"BrandNewPassword2026!"}`)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/customer/preferences/password", changeBody)
	req.Header.Set("Authorization", "Bearer "+sessionToken)
	rec = httptest.NewRecorder()
	changeHandler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on change password, got %d: %s", rec.Code, rec.Body.String())
	}

	// 6. Verify new password logs in
	loginBody = bytes.NewBufferString(`{"email":"` + email + `","password":"BrandNewPassword2026!"}`)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/customer/login", loginBody)
	rec = httptest.NewRecorder()
	loginHandler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on login with new password, got %d", rec.Code)
	}
}

func TestCustomerViewerCodesLifecycleAndScoping(t *testing.T) {
	db, err := dbpkg.InitDatabase(filepath.Join(t.TempDir(), "customer-codes.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Helper to create customer and session
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

	sessionA, _ := setupCustomer("clientA@example.com")
	sessionB, _ := setupCustomer("clientB@example.com")

	authGen := middleware.CustomerAuth(db)(CustomerGenerateViewerCodeHandler(db))
	authList := middleware.CustomerAuth(db)(CustomerListViewerCodesHandler(db))
	authRevoke := middleware.CustomerAuth(db)(CustomerRevokeViewerCodeHandler(db))

	// 1. Client A generates a viewer code
	genBody := bytes.NewBufferString(`{"client_email":"target@client.fr"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/customer/viewer-codes/generate", genBody)
	req.Header.Set("Authorization", "Bearer "+sessionA)
	rec := httptest.NewRecorder()
	authGen.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("generate code failed: %d: %s", rec.Code, rec.Body.String())
	}
	var genResp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &genResp); err != nil || genResp["code"] == "" {
		t.Fatalf("invalid generate response: %v", err)
	}
	codeA := genResp["code"].(string)

	// 2. Client A lists viewer codes
	req = httptest.NewRequest(http.MethodGet, "/api/v1/customer/viewer-codes", nil)
	req.Header.Set("Authorization", "Bearer "+sessionA)
	rec = httptest.NewRecorder()
	authList.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list codes failed: %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), codeA) {
		t.Fatalf("code %s not in list: %s", codeA, rec.Body.String())
	}

	// 3. Client B cannot see Client A's code
	req = httptest.NewRequest(http.MethodGet, "/api/v1/customer/viewer-codes", nil)
	req.Header.Set("Authorization", "Bearer "+sessionB)
	rec = httptest.NewRecorder()
	authList.ServeHTTP(rec, req)
	if strings.Contains(rec.Body.String(), codeA) {
		t.Fatalf("cross customer code leak: client B saw %s", codeA)
	}

	// 4. Client B cannot revoke Client A's code
	req = httptest.NewRequest(http.MethodPut, "/api/v1/customer/viewer-codes/"+codeA+"/revoke", nil)
	req.Header.Set("Authorization", "Bearer "+sessionB)
	rec = httptest.NewRecorder()
	authRevoke.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 on unauthorized revoke, got %d", rec.Code)
	}

	// 5. Client A revokes their code
	req = httptest.NewRequest(http.MethodPut, "/api/v1/customer/viewer-codes/"+codeA+"/revoke", nil)
	req.Header.Set("Authorization", "Bearer "+sessionA)
	rec = httptest.NewRecorder()
	authRevoke.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on legitimate revoke, got %d", rec.Code)
	}

	// 6. Verify status is revoked in list
	req = httptest.NewRequest(http.MethodGet, "/api/v1/customer/viewer-codes", nil)
	req.Header.Set("Authorization", "Bearer "+sessionA)
	rec = httptest.NewRecorder()
	authList.ServeHTTP(rec, req)
	if !strings.Contains(rec.Body.String(), `"status":"revoked"`) {
		t.Fatalf("expected status revoked in list: %s", rec.Body.String())
	}
}

func TestCustomerGoogleLogin(t *testing.T) {
	db, err := dbpkg.InitDatabase(filepath.Join(t.TempDir(), "customer-google-test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	email := "googleuser@example.com"
	_, err = dbpkg.CreateLicense(db, email, 30, 1, "Test Google Customer")
	if err != nil {
		t.Fatal(err)
	}

	clientID := "test-client-id-12345"

	// Mock verifyGoogleToken
	origVerify := verifyGoogleToken
	defer func() { verifyGoogleToken = origVerify }()

	verifyGoogleToken = func(credential, expectedClientID string) (*googleTokenInfo, error) {
		if credential == "valid-token" {
			return &googleTokenInfo{
				Aud:           expectedClientID,
				Email:         email,
				EmailVerified: true,
			}, nil
		}
		if credential == "unknown-user-token" {
			return &googleTokenInfo{
				Aud:           expectedClientID,
				Email:         "nonexistent@google.com",
				EmailVerified: true,
			}, nil
		}
		return nil, errors.New("invalid token")
	}

	handler := CustomerGoogleLoginHandler(db, clientID)

	// 1. Success case
	body, _ := json.Marshal(map[string]string{"credential": "valid-token"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/customer/login/google", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var res map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
		t.Fatal(err)
	}
	if res["token"] == "" || res["email"] != email || res["customer_id"] == "" {
		t.Fatalf("unexpected response: %+v", res)
	}

	// 2. Unknown user case -> 404
	body, _ = json.Marshal(map[string]string{"credential": "unknown-user-token"})
	req = httptest.NewRequest(http.MethodPost, "/api/v1/customer/login/google", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown user, got %d: %s", rec.Code, rec.Body.String())
	}

	// 3. Invalid token case -> 401
	body, _ = json.Marshal(map[string]string{"credential": "bad-token"})
	req = httptest.NewRequest(http.MethodPost, "/api/v1/customer/login/google", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for bad token, got %d: %s", rec.Code, rec.Body.String())
	}

	// 4. Missing body / malformed -> 400
	req = httptest.NewRequest(http.MethodPost, "/api/v1/customer/login/google", strings.NewReader("bad json"))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad json, got %d: %s", rec.Code, rec.Body.String())
	}
}

