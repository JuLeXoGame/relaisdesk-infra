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

	"api/middleware"
)

func TestCustomer2FAAPIEndpoints(t *testing.T) {
	db, err := dbpkg.InitDatabase(filepath.Join(t.TempDir(), "cust-2fa-api.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	email := "api.cust2fa@relaisdesk.fr"
	password := "SecurePassword123!"

	// Create customer and licence so account is eligible
	order, err := dbpkg.CreateOrderWithBilling(db, email, "starter", 1, "stripe", "", "", &dbpkg.BillingDetails{
		Name: email, Address: "1 rue de Paris", PostalCode: "75001", City: "Paris", CustomerType: "business",
		TermsVersion: "v1", TermsAccepted: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = dbpkg.FulfillPendingOrder(db, order.OrderID, "test")
	if err != nil {
		t.Fatal(err)
	}

	// Set password
	err = dbpkg.SetCustomerPassword(db, email, password)
	if err != nil {
		t.Fatal(err)
	}

	// Login before 2FA
	loginPayload, _ := json.Marshal(map[string]string{
		"email":    email,
		"password": password,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/customer/login", bytes.NewReader(loginPayload))
	rec := httptest.NewRecorder()
	CustomerLoginWithPasswordHandler(db).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("login failed: %d, %s", rec.Code, rec.Body.String())
	}
	var loginResp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &loginResp)
	if loginResp["requires_2fa"] == true || loginResp["token"] == "" {
		t.Fatalf("expected requires_2fa=false and session token, got %v", loginResp)
	}
	sessionToken := loginResp["token"].(string)

	// Setup 2FA
	setupReq := httptest.NewRequest(http.MethodPost, "/api/v1/customer/2fa/setup", nil)
	setupReq.Header.Set("Authorization", "Bearer "+sessionToken)
	setupRec := httptest.NewRecorder()
	middleware.CustomerAuth(db)(Customer2FASetupHandler(db)).ServeHTTP(setupRec, setupReq)

	if setupRec.Code != http.StatusOK {
		t.Fatalf("setup failed: %d, %s", setupRec.Code, setupRec.Body.String())
	}
	var setupResp map[string]any
	_ = json.Unmarshal(setupRec.Body.Bytes(), &setupResp)
	secret := setupResp["secret"].(string)
	rawRecCodes := setupResp["recovery_codes"].([]any)
	recCodes := make([]string, len(rawRecCodes))
	for i, c := range rawRecCodes {
		recCodes[i] = c.(string)
	}

	// Enable 2FA with calculated code
	totpCode, err := dbpkg.CalculateTOTP(secret, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}

	enablePayload, _ := json.Marshal(map[string]any{
		"password":       password,
		"secret":         secret,
		"code":           totpCode,
		"recovery_codes": recCodes,
	})
	enableReq := httptest.NewRequest(http.MethodPost, "/api/v1/customer/2fa/enable", bytes.NewReader(enablePayload))
	enableReq.Header.Set("Authorization", "Bearer "+sessionToken)
	enableRec := httptest.NewRecorder()
	middleware.CustomerAuth(db)(Customer2FAEnableHandler(db)).ServeHTTP(enableRec, enableReq)

	if enableRec.Code != http.StatusOK {
		t.Fatalf("enable failed: %d, %s", enableRec.Code, enableRec.Body.String())
	}

	// Login again -> Must return challenge token
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/customer/login", bytes.NewReader(loginPayload))
	rec2 := httptest.NewRecorder()
	CustomerLoginWithPasswordHandler(db).ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Fatalf("login 2FA challenge failed: %d, %s", rec2.Code, rec2.Body.String())
	}
	var login2Resp map[string]any
	_ = json.Unmarshal(rec2.Body.Bytes(), &login2Resp)
	if login2Resp["requires_2fa"] != true || login2Resp["challenge_token"] == "" {
		t.Fatalf("expected requires_2fa=true and challenge token, got %v", login2Resp)
	}
	challengeToken := login2Resp["challenge_token"].(string)

	// Complete 2FA login with code
	newTotpCode, _ := dbpkg.CalculateTOTP(secret, time.Now().UTC())
	verifyPayload, _ := json.Marshal(map[string]string{
		"challenge_token": challengeToken,
		"code":            newTotpCode,
	})
	verifyReq := httptest.NewRequest(http.MethodPost, "/api/v1/customer/login/2fa", bytes.NewReader(verifyPayload))
	verifyRec := httptest.NewRecorder()
	CustomerLogin2FAHandler(db).ServeHTTP(verifyRec, verifyReq)

	if verifyRec.Code != http.StatusOK {
		t.Fatalf("login 2fa verify failed: %d, %s", verifyRec.Code, verifyRec.Body.String())
	}
	var verifyResp map[string]any
	_ = json.Unmarshal(verifyRec.Body.Bytes(), &verifyResp)
	if verifyResp["token"] == "" {
		t.Fatalf("expected session token from 2fa verify: %v", verifyResp)
	}

	sessionToken = verifyResp["token"].(string)
	// Check 2FA status
	statusReq := httptest.NewRequest(http.MethodGet, "/api/v1/customer/2fa/status", nil)
	statusReq.Header.Set("Authorization", "Bearer "+sessionToken)
	statusRec := httptest.NewRecorder()
	middleware.CustomerAuth(db)(Customer2FAStatusHandler(db)).ServeHTTP(statusRec, statusReq)

	if statusRec.Code != http.StatusOK {
		t.Fatalf("status failed: %d, %s", statusRec.Code, statusRec.Body.String())
	}
	var statusResp map[string]any
	_ = json.Unmarshal(statusRec.Body.Bytes(), &statusResp)
	if statusResp["enabled"] != true || statusResp["remaining_recovery_codes"].(float64) != 8 {
		t.Fatalf("unexpected status: %v", statusResp)
	}

	// Disable 2FA
	disablePayload, _ := json.Marshal(map[string]string{
		"password": password,
		"code":     recCodes[0],
	})
	disableReq := httptest.NewRequest(http.MethodPost, "/api/v1/customer/2fa/disable", bytes.NewReader(disablePayload))
	disableReq.Header.Set("Authorization", "Bearer "+sessionToken)
	disableRec := httptest.NewRecorder()
	middleware.CustomerAuth(db)(Customer2FADisableHandler(db)).ServeHTTP(disableRec, disableReq)

	if disableRec.Code != http.StatusOK {
		t.Fatalf("disable failed: %d, %s", disableRec.Code, disableRec.Body.String())
	}
}

func TestCustomer2FAEmailCodeAndTrustedDeviceHandlers(t *testing.T) {
	db, err := dbpkg.InitDatabase(filepath.Join(t.TempDir(), "cust-device-api.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	email := "device2fa@example.com"
	password := "SecretPass1234!"

	order, err := dbpkg.CreateOrderWithBilling(db, email, "starter", 1, "stripe", "", "", &dbpkg.BillingDetails{
		Name: email, Address: "1 rue de Paris", PostalCode: "75001", City: "Paris", CustomerType: "business",
		TermsVersion: "v1", TermsAccepted: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dbpkg.FulfillPendingOrder(db, order.OrderID, "test"); err != nil {
		t.Fatal(err)
	}
	if err := dbpkg.SetCustomerPassword(db, email, password); err != nil {
		t.Fatalf("failed to set customer password: %v", err)
	}

	// Login before 2FA
	loginPrePayload, _ := json.Marshal(map[string]string{
		"email":    email,
		"password": password,
	})
	loginPreReq := httptest.NewRequest(http.MethodPost, "/api/v1/customer/login", bytes.NewReader(loginPrePayload))
	loginPreRec := httptest.NewRecorder()
	CustomerLoginWithPasswordHandler(db).ServeHTTP(loginPreRec, loginPreReq)
	if loginPreRec.Code != http.StatusOK {
		t.Fatalf("pre-login failed: %d, %s", loginPreRec.Code, loginPreRec.Body.String())
	}
	var loginPreResp map[string]any
	_ = json.Unmarshal(loginPreRec.Body.Bytes(), &loginPreResp)
	sessionToken := loginPreResp["token"].(string)

	// Setup 2FA
	setupReq := httptest.NewRequest(http.MethodPost, "/api/v1/customer/2fa/setup", nil)
	setupReq.Header.Set("Authorization", "Bearer "+sessionToken)
	setupRec := httptest.NewRecorder()
	middleware.CustomerAuth(db)(Customer2FASetupHandler(db)).ServeHTTP(setupRec, setupReq)
	if setupRec.Code != http.StatusOK {
		t.Fatalf("setup failed: %d, %s", setupRec.Code, setupRec.Body.String())
	}
	var setupResp map[string]any
	_ = json.Unmarshal(setupRec.Body.Bytes(), &setupResp)
	secret := setupResp["secret"].(string)
	rawRecCodes := setupResp["recovery_codes"].([]any)
	recCodes := make([]string, len(rawRecCodes))
	for i, c := range rawRecCodes {
		recCodes[i] = c.(string)
	}

	// Enable 2FA
	totpCode, err := dbpkg.CalculateTOTP(secret, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	enablePayload, _ := json.Marshal(map[string]any{
		"password":       password,
		"secret":         secret,
		"code":           totpCode,
		"recovery_codes": recCodes,
	})
	enableReq := httptest.NewRequest(http.MethodPost, "/api/v1/customer/2fa/enable", bytes.NewReader(enablePayload))
	enableReq.Header.Set("Authorization", "Bearer "+sessionToken)
	enableRec := httptest.NewRecorder()
	middleware.CustomerAuth(db)(Customer2FAEnableHandler(db)).ServeHTTP(enableRec, enableReq)
	if enableRec.Code != http.StatusOK {
		t.Fatalf("enable failed: %d, %s", enableRec.Code, enableRec.Body.String())
	}

	// 1. Password login returns requires_2fa=true
	loginPayload, _ := json.Marshal(map[string]string{
		"email":    email,
		"password": password,
	})
	loginReq := httptest.NewRequest(http.MethodPost, "/api/v1/customer/login", bytes.NewReader(loginPayload))
	loginRec := httptest.NewRecorder()
	CustomerLoginWithPasswordHandler(db).ServeHTTP(loginRec, loginReq)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("login failed: %d, %s", loginRec.Code, loginRec.Body.String())
	}
	var loginResp map[string]any
	_ = json.Unmarshal(loginRec.Body.Bytes(), &loginResp)
	if loginResp["requires_2fa"] != true || loginResp["challenge_token"] == "" {
		t.Fatalf("expected requires_2fa=true and challenge token, got %v", loginResp)
	}
	challengeToken := loginResp["challenge_token"].(string)

	// 2. Request email code via Customer2FASendEmailCodeHandler
	emailCodePayload, _ := json.Marshal(map[string]string{
		"challenge_token": challengeToken,
	})
	emailCodeReq := httptest.NewRequest(http.MethodPost, "/api/v1/customer/login/2fa/send-email-code", bytes.NewReader(emailCodePayload))
	emailCodeRec := httptest.NewRecorder()
	Customer2FASendEmailCodeHandler(db, nil).ServeHTTP(emailCodeRec, emailCodeReq)
	if emailCodeRec.Code != http.StatusOK {
		t.Fatalf("send email code failed: %d, %s", emailCodeRec.Code, emailCodeRec.Body.String())
	}
	var emailCodeResp map[string]any
	_ = json.Unmarshal(emailCodeRec.Body.Bytes(), &emailCodeResp)
	if emailCodeResp["success"] != true || emailCodeResp["email_masked"] == "" {
		t.Fatalf("unexpected send email code resp: %v", emailCodeResp)
	}

	// 3. Immediate resend should be throttled (30s)
	emailCodeReq2 := httptest.NewRequest(http.MethodPost, "/api/v1/customer/login/2fa/send-email-code", bytes.NewReader(emailCodePayload))
	emailCodeRec2 := httptest.NewRecorder()
	Customer2FASendEmailCodeHandler(db, nil).ServeHTTP(emailCodeRec2, emailCodeReq2)
	if emailCodeRec2.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for throttled resend, got %d", emailCodeRec2.Code)
	}

	// 4. Verify 2FA challenge with TOTP and remember_device = true
	newTotp, _ := dbpkg.CalculateTOTP(secret, time.Now().UTC())
	verifyPayload, _ := json.Marshal(map[string]any{
		"challenge_token": challengeToken,
		"code":            newTotp,
		"remember_device": true,
		"device_name":     "Mozilla/5.0 Test Browser",
	})
	verifyReq := httptest.NewRequest(http.MethodPost, "/api/v1/customer/login/2fa", bytes.NewReader(verifyPayload))
	verifyRec := httptest.NewRecorder()
	CustomerLogin2FAHandler(db).ServeHTTP(verifyRec, verifyReq)
	if verifyRec.Code != http.StatusOK {
		t.Fatalf("verify 2fa failed: %d, %s", verifyRec.Code, verifyRec.Body.String())
	}
	var verifyResp map[string]any
	_ = json.Unmarshal(verifyRec.Body.Bytes(), &verifyResp)
	deviceToken, ok := verifyResp["device_token"].(string)
	if !ok || deviceToken == "" {
		t.Fatalf("expected device_token in verify response, got %v", verifyResp)
	}

	// 5. Subsequent password login with device_token should bypass 2FA
	loginPayloadWithDevice, _ := json.Marshal(map[string]string{
		"email":        email,
		"password":     password,
		"device_token": deviceToken,
	})
	loginReq2 := httptest.NewRequest(http.MethodPost, "/api/v1/customer/login", bytes.NewReader(loginPayloadWithDevice))
	loginRec2 := httptest.NewRecorder()
	CustomerLoginWithPasswordHandler(db).ServeHTTP(loginRec2, loginReq2)
	if loginRec2.Code != http.StatusOK {
		t.Fatalf("login with device token failed: %d, %s", loginRec2.Code, loginRec2.Body.String())
	}
	var loginResp2 map[string]any
	_ = json.Unmarshal(loginRec2.Body.Bytes(), &loginResp2)
	if loginResp2["requires_2fa"] == true || loginResp2["token"] == "" {
		t.Fatalf("expected 2fa bypass with device token, got %v", loginResp2)
	}

	// 6. Revoke trusted devices
	if err := dbpkg.RevokeCustomerTrustedDevices(db, email); err != nil {
		t.Fatalf("failed to revoke devices: %v", err)
	}

	// 7. Login with old device token now requires 2FA again
	loginReq3 := httptest.NewRequest(http.MethodPost, "/api/v1/customer/login", bytes.NewReader(loginPayloadWithDevice))
	loginRec3 := httptest.NewRecorder()
	CustomerLoginWithPasswordHandler(db).ServeHTTP(loginRec3, loginReq3)
	var loginResp3 map[string]any
	_ = json.Unmarshal(loginRec3.Body.Bytes(), &loginResp3)
	if loginResp3["requires_2fa"] != true {
		t.Fatalf("expected requires_2fa=true after device revocation, got %v", loginResp3)
	}
}
