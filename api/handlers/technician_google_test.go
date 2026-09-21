package handlers

import (
	"bytes"
	dbpkg "database"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestTechnicianGoogleLoginHandler(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "tech_google_handler_test.db")
	db, err := dbpkg.InitDatabase(dbPath)
	if err != nil {
		t.Fatalf("InitDatabase failed: %v", err)
	}
	defer db.Close()

	testEmail := "tech-google-handler@example.com"
	clientID := "test-google-client-id.apps.googleusercontent.com"
	settings := ServerSettings{
		ServerIP:       "192.168.1.100",
		RendezvousPort: 21116,
		RelayPort:      21117,
	}

	// Insert active server public key
	_, err = db.Exec(`INSERT INTO server_keys (public_key, is_active) VALUES ('TEST_PUB_KEY_ED25519', 1)`)
	if err != nil {
		t.Fatal(err)
	}

	// Create customer and active license
	_, err = dbpkg.EnsureCustomer(db, testEmail, "business", "Google Tech Co")
	if err != nil {
		t.Fatalf("EnsureCustomer: %v", err)
	}
	lic, err := dbpkg.CreateLicense(db, testEmail, 30, 2, "Test License")
	if err != nil {
		t.Fatalf("CreateLicense: %v", err)
	}

	// Mock verifyGoogleToken
	origVerify := verifyGoogleToken
	defer func() { verifyGoogleToken = origVerify }()

	verifyGoogleToken = func(credential, expectedClientID string) (*googleTokenInfo, error) {
		if credential == "valid-token" {
			return &googleTokenInfo{
				Email:         testEmail,
				EmailVerified: true,
				Iss:           "https://accounts.google.com",
				Aud:           expectedClientID,
				Exp:           strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10),
				Sub:           "google-sub-12345",
			}, nil
		}
		if credential == "unknown-user-token" {
			return &googleTokenInfo{
				Email:         "nobody@unknown.com",
				EmailVerified: true,
				Iss:           "https://accounts.google.com",
				Aud:           expectedClientID,
				Exp:           strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10),
				Sub:           "google-sub-67890",
			}, nil
		}
		return nil, errors.New("jeton invalide")
	}

	handler := TechnicianGoogleLoginHandler(db, settings, clientID)

	// 1. Success case without 2FA
	body, _ := json.Marshal(map[string]string{"credential": "valid-token"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/technician/login/google", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var res map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
		t.Fatal(err)
	}
	if res["valid"] != true || res["token"] == "" || res["license_id"] != lic.LicenseID || res["email"] != testEmail {
		t.Fatalf("unexpected success response: %+v", res)
	}

	// 2. Unknown user / no active license -> 401
	body, _ = json.Marshal(map[string]string{"credential": "unknown-user-token"})
	req = httptest.NewRequest(http.MethodPost, "/api/v1/technician/login/google", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unknown user, got %d: %s", rec.Code, rec.Body.String())
	}

	// 3. Bad token -> 401
	body, _ = json.Marshal(map[string]string{"credential": "bad-token"})
	req = httptest.NewRequest(http.MethodPost, "/api/v1/technician/login/google", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for bad token, got %d: %s", rec.Code, rec.Body.String())
	}

	// 4. Malformed JSON -> 400
	req = httptest.NewRequest(http.MethodPost, "/api/v1/technician/login/google", strings.NewReader("bad json"))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad json, got %d: %s", rec.Code, rec.Body.String())
	}

	// 5. Google unconfigured -> 503
	unconfiguredHandler := TechnicianGoogleLoginHandler(db, settings, "")
	body, _ = json.Marshal(map[string]string{"credential": "valid-token"})
	req = httptest.NewRequest(http.MethodPost, "/api/v1/technician/login/google", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	unconfiguredHandler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for unconfigured Google, got %d: %s", rec.Code, rec.Body.String())
	}

	// 6. 2FA flow: Enable TOTP on account
	totpSecret := "JBSWY3DPEHPK3PXP"
	_, err = db.Exec(`UPDATE customer_accounts SET totp_enabled = 1, totp_secret = ? WHERE email = ?`, totpSecret, testEmail)
	if err != nil {
		t.Fatalf("Failed to enable TOTP: %v", err)
	}

	body, _ = json.Marshal(map[string]string{"credential": "valid-token"})
	req = httptest.NewRequest(http.MethodPost, "/api/v1/technician/login/google", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for 2FA challenge, got %d: %s", rec.Code, rec.Body.String())
	}
	var res2FA map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&res2FA); err != nil {
		t.Fatal(err)
	}
	if res2FA["valid"] != true || res2FA["requires_2fa"] != true || res2FA["challenge_token"] == "" {
		t.Fatalf("expected requires_2fa=true and challenge_token, got %+v", res2FA)
	}
}
