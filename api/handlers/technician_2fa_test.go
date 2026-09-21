package handlers

import (
	"bytes"
	dbpkg "database"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func TestTechnician2FAAPIEndpoints(t *testing.T) {
	db, err := dbpkg.InitDatabase(filepath.Join(t.TempDir(), "tech-2fa-api.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	settings := ServerSettings{
		ServerIP:       "127.0.0.1",
		RendezvousPort: 21116,
		RelayPort:      21117,
	}

	// Insert active server public key
	_, err = db.Exec(`INSERT INTO server_keys (public_key, is_active) VALUES ('TEST_PUB_KEY_ED25519', 1)`)
	if err != nil {
		t.Fatal(err)
	}

	email := "tech.api2fa@relaisdesk.fr"
	licID := "LIC-API-2FA"
	licKey := "KEY-API-2FA"

	// Create customer and licence
	order, err := dbpkg.CreateOrderWithBilling(db, email, "pro", 1, "stripe", "", "", &dbpkg.BillingDetails{
		Name: email, Address: "1 rue Test", PostalCode: "75001", City: "Paris", CustomerType: "business",
		TermsVersion: "v1", TermsAccepted: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	lic, err := dbpkg.FulfillPendingOrder(db, order.OrderID, "test")
	if err != nil {
		t.Fatal(err)
	}
	licID = lic.LicenseID
	licKey = lic.LicenseKey

	// 1. Login before 2FA -> 200 with credentials directly
	loginPayload, _ := json.Marshal(map[string]string{
		"license_id":  licID,
		"license_key": licKey,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/technician/login", bytes.NewReader(loginPayload))
	rec := httptest.NewRecorder()
	TechnicianLoginHandler(db, settings).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var loginResp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &loginResp)
	if loginResp["requires_2fa"] == true || loginResp["token"] == "" {
		t.Fatalf("unexpected login resp: %v", loginResp)
	}

	// 2. Enable 2FA on the customer account
	secret, _, recoveryCodes, err := dbpkg.SetupCustomerTOTP(db, email)
	if err != nil {
		t.Fatal(err)
	}
	totpCode, _ := dbpkg.CalculateTOTP(secret, time.Now().UTC())
	err = dbpkg.EnableCustomerTOTP(db, email, secret, totpCode, recoveryCodes)
	if err != nil {
		t.Fatal(err)
	}

	// 3. Login again -> Must return requires_2fa: true and challenge_token
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/technician/login", bytes.NewReader(loginPayload))
	rec2 := httptest.NewRecorder()
	TechnicianLoginHandler(db, settings).ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200 with challenge, got %d: %s", rec2.Code, rec2.Body.String())
	}
	var challengeResp map[string]any
	_ = json.Unmarshal(rec2.Body.Bytes(), &challengeResp)
	if challengeResp["requires_2fa"] != true || challengeResp["challenge_token"] == "" {
		t.Fatalf("expected 2FA challenge, got: %v", challengeResp)
	}
	challengeToken := challengeResp["challenge_token"].(string)

	// 4. Verify 2FA challenge with invalid code -> 401
	badVerifyPayload, _ := json.Marshal(map[string]string{
		"challenge_token": challengeToken,
		"code":            "999999",
	})
	badReq := httptest.NewRequest(http.MethodPost, "/api/v1/technician/login/2fa", bytes.NewReader(badVerifyPayload))
	badRec := httptest.NewRecorder()
	TechnicianLogin2FAHandler(db, settings).ServeHTTP(badRec, badReq)

	if badRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 on bad code, got %d", badRec.Code)
	}

	// 5. Request Email Code
	emailCodeReqPayload, _ := json.Marshal(map[string]string{
		"challenge_token": challengeToken,
	})
	emailCodeReq := httptest.NewRequest(http.MethodPost, "/api/v1/technician/login/email-code", bytes.NewReader(emailCodeReqPayload))
	emailCodeRec := httptest.NewRecorder()
	Technician2FASendEmailCodeHandler(db, nil).ServeHTTP(emailCodeRec, emailCodeReq)
	if emailCodeRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on email code request, got %d: %s", emailCodeRec.Code, emailCodeRec.Body.String())
	}
	var emailCodeResp map[string]any
	_ = json.Unmarshal(emailCodeRec.Body.Bytes(), &emailCodeResp)
	if emailCodeResp["success"] != true {
		t.Fatalf("expected success=true, got %v", emailCodeResp)
	}

	// Fetch generated email code directly from DB for test verification
	var dbCodeHash string
	err = db.QueryRow(`SELECT email_code_hash FROM technician_2fa_challenges WHERE challenge_token = ?`, dbpkg.HashSessionToken(challengeToken)).Scan(&dbCodeHash)
	if err != nil || dbCodeHash == "" {
		t.Fatalf("email_code_hash not saved in DB: %v", err)
	}

	// 6. Verify 2FA challenge with valid code + remember_device -> 200 with credentials and device_token
	validCode, _ := dbpkg.CalculateTOTP(secret, time.Now().UTC())
	goodVerifyPayload, _ := json.Marshal(map[string]any{
		"challenge_token": challengeToken,
		"code":            validCode,
		"remember_device": true,
		"device_name":     "Test Device",
	})
	goodReq := httptest.NewRequest(http.MethodPost, "/api/v1/technician/login/2fa", bytes.NewReader(goodVerifyPayload))
	goodRec := httptest.NewRecorder()
	TechnicianLogin2FAHandler(db, settings).ServeHTTP(goodRec, goodReq)

	if goodRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on valid 2FA, got %d: %s", goodRec.Code, goodRec.Body.String())
	}
	var finalResp map[string]any
	_ = json.Unmarshal(goodRec.Body.Bytes(), &finalResp)
	if finalResp["token"] == "" || finalResp["public_key"] != "TEST_PUB_KEY_ED25519" {
		t.Fatalf("unexpected final response: %v", finalResp)
	}
	deviceToken, ok := finalResp["device_token"].(string)
	if !ok || deviceToken == "" {
		t.Fatalf("expected device_token in response when remember_device=true")
	}

	// 7. Login with device_token -> Should bypass 2FA
	loginWithDevicePayload, _ := json.Marshal(map[string]string{
		"license_id":   licID,
		"license_key":  licKey,
		"device_token": deviceToken,
	})
	reqDev := httptest.NewRequest(http.MethodPost, "/api/v1/technician/login", bytes.NewReader(loginWithDevicePayload))
	recDev := httptest.NewRecorder()
	TechnicianLoginHandler(db, settings).ServeHTTP(recDev, reqDev)

	if recDev.Code != http.StatusOK {
		t.Fatalf("expected 200 with device token bypass, got %d: %s", recDev.Code, recDev.Body.String())
	}
	var devLoginResp map[string]any
	_ = json.Unmarshal(recDev.Body.Bytes(), &devLoginResp)
	if devLoginResp["requires_2fa"] == true || devLoginResp["token"] == "" {
		t.Fatalf("expected 2FA bypass with device token, got: %v", devLoginResp)
	}
}

func TestTechnicianEmailPasswordAPIEndpoint(t *testing.T) {
	db, err := dbpkg.InitDatabase(filepath.Join(t.TempDir(), "tech-pwd-api.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	settings := ServerSettings{
		ServerIP:       "127.0.0.1",
		RendezvousPort: 21116,
		RelayPort:      21117,
	}

	_, err = db.Exec(`INSERT INTO server_keys (public_key, is_active) VALUES ('TEST_PUB_KEY_ED25519', 1)`)
	if err != nil {
		t.Fatal(err)
	}

	email := "tech.pwd@relaisdesk.fr"
	password := "MonMotDePasseSecret2026!"

	// 1. Create customer and license
	_, err = dbpkg.EnsureCustomer(db, email, "business", "Test Corp")
	if err != nil {
		t.Fatal(err)
	}
	if err := dbpkg.SetCustomerPassword(db, email, password); err != nil {
		t.Fatal(err)
	}
	_, err = dbpkg.CreateLicense(db, email, 30, 1, "Tech license")
	if err != nil {
		t.Fatal(err)
	}

	// 2. Login with wrong password -> 401
	badPayload, _ := json.Marshal(map[string]string{
		"email":    email,
		"password": "WrongPassword!",
	})
	badReq := httptest.NewRequest(http.MethodPost, "/api/v1/technician/login", bytes.NewReader(badPayload))
	badRec := httptest.NewRecorder()
	TechnicianLoginHandler(db, settings).ServeHTTP(badRec, badReq)
	if badRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 on bad password, got %d", badRec.Code)
	}

	// 3. Login with valid email & password -> 200 with credentials
	goodPayload, _ := json.Marshal(map[string]string{
		"email":    email,
		"password": password,
	})
	goodReq := httptest.NewRequest(http.MethodPost, "/api/v1/technician/login", bytes.NewReader(goodPayload))
	goodRec := httptest.NewRecorder()
	TechnicianLoginHandler(db, settings).ServeHTTP(goodRec, goodReq)

	if goodRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on valid email/password login, got %d: %s", goodRec.Code, goodRec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(goodRec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if resp["valid"] != true || resp["token"] == "" || resp["email"] != email || resp["public_key"] != "TEST_PUB_KEY_ED25519" {
		t.Fatalf("unexpected login response: %v", resp)
	}
}

