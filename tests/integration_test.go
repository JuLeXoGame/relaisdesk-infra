package tests

import (
	"bytes"
	dbpkg "database"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"api/handlers"
	"api/middleware"
)

type testEnv struct {
	db       *sql.DB
	settings handlers.ServerSettings
}

func setupDB(t *testing.T) *testEnv {
	t.Helper()

	db, err := dbpkg.InitDatabase(filepath.Join(t.TempDir(), "licences.db"))
	if err != nil {
		t.Fatalf("InitDatabase failed: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec("INSERT INTO server_keys (public_key, is_active) VALUES (?, 1)", "test-public-key"); err != nil {
		t.Fatalf("insert server key failed: %v", err)
	}

	return &testEnv{
		db: db,
		settings: handlers.ServerSettings{
			ServerIP:       "79.72.27.213",
			RendezvousPort: 21116,
			RelayPort:      21117,
		},
	}
}

func TestLicenseLifecycle(t *testing.T) {
	env := setupDB(t)

	lic, err := dbpkg.CreateLicense(env.db, "client@example.com", 365, 1, "integration")
	if err != nil {
		t.Fatalf("CreateLicense failed: %v", err)
	}

	activateResp := postJSON(t, handlers.ActivateHandler(env.db, env.settings), map[string]string{
		"license_key": lic.LicenseKey,
	})
	if activateResp.Code != http.StatusOK {
		t.Fatalf("activate status = %d, body=%s", activateResp.Code, activateResp.Body.String())
	}

	var activation map[string]any
	if err := json.Unmarshal(activateResp.Body.Bytes(), &activation); err != nil {
		t.Fatalf("invalid activation JSON: %v", err)
	}
	if activation["status"] != "valid" || activation["server_ip"] != "79.72.27.213" {
		t.Fatalf("unexpected activation response: %+v", activation)
	}
	if _, exists := activation["proxy_port"]; exists {
		t.Fatalf("activation response must not expose obsolete proxy settings: %+v", activation)
	}
	if enabled, ok := activation["udp_enabled"].(bool); !ok || !enabled {
		t.Fatalf("activation response must enable RustDesk UDP: %+v", activation)
	}

	validateResp := postJSON(t, handlers.ValidateHandler(env.db), map[string]string{
		"license_id":  lic.LicenseID,
		"license_key": lic.LicenseKey,
	})
	if validateResp.Code != http.StatusOK {
		t.Fatalf("validate status = %d, body=%s", validateResp.Code, validateResp.Body.String())
	}

	if err := dbpkg.IncrementConnections(env.db, lic.LicenseID); err != nil {
		t.Fatalf("IncrementConnections failed: %v", err)
	}
	if err := dbpkg.DecrementConnections(env.db, lic.LicenseID); err != nil {
		t.Fatalf("DecrementConnections failed: %v", err)
	}
	if err := dbpkg.RevokeLicense(env.db, lic.LicenseID, "integration revoke"); err != nil {
		t.Fatalf("RevokeLicense failed: %v", err)
	}
	if _, err := dbpkg.ValidateLicense(env.db, lic.LicenseID, lic.LicenseKey); err == nil {
		t.Fatal("ValidateLicense should fail after revocation")
	}
}

func TestExpiredLicense(t *testing.T) {
	env := setupDB(t)

	lic, err := dbpkg.CreateLicense(env.db, "expired@example.com", 0, 1, "")
	if err != nil {
		t.Fatalf("CreateLicense failed: %v", err)
	}
	time.Sleep(10 * time.Millisecond)

	resp := postJSON(t, handlers.ActivateHandler(env.db, env.settings), map[string]string{
		"license_key": lic.LicenseKey,
	})
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for expired license, got %d", resp.Code)
	}
}

func TestMaxConnectionsExceeded(t *testing.T) {
	env := setupDB(t)

	lic, err := dbpkg.CreateLicense(env.db, "limited@example.com", 365, 1, "")
	if err != nil {
		t.Fatalf("CreateLicense failed: %v", err)
	}

	if err := dbpkg.IncrementConnections(env.db, lic.LicenseID); err != nil {
		t.Fatalf("first increment failed: %v", err)
	}
	if err := dbpkg.IncrementConnections(env.db, lic.LicenseID); err == nil {
		t.Fatal("second increment should fail when max_connections=1")
	}
	if err := dbpkg.DecrementConnections(env.db, lic.LicenseID); err != nil {
		t.Fatalf("decrement failed: %v", err)
	}
	if err := dbpkg.IncrementConnections(env.db, lic.LicenseID); err != nil {
		t.Fatalf("increment after decrement should succeed: %v", err)
	}
}

func TestLegacyDatabaseCounterDoesNotLimitInstallations(t *testing.T) {
	env := setupDB(t)
	lic, err := dbpkg.CreateLicense(env.db, "installations@example.com", 365, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := dbpkg.IncrementConnections(env.db, lic.LicenseID); err != nil {
		t.Fatal(err)
	}

	response := postJSON(t, handlers.ActivateHandler(env.db, env.settings), map[string]string{"license_key": lic.LicenseKey})
	if response.Code != http.StatusOK {
		t.Fatalf("an installation was rejected by the legacy counter: %d %s", response.Code, response.Body.String())
	}
}

func TestPerformance(t *testing.T) {
	env := setupDB(t)

	const n = 100
	licenses := make([]*dbpkg.License, 0, n)
	for i := 0; i < n; i++ {
		lic, err := dbpkg.CreateLicense(env.db, "perf@example.com", 365, 5, "")
		if err != nil {
			t.Fatalf("CreateLicense %d failed: %v", i, err)
		}
		licenses = append(licenses, lic)
	}

	start := time.Now()
	for _, lic := range licenses {
		if _, err := dbpkg.ValidateLicense(env.db, lic.LicenseID, lic.LicenseKey); err != nil {
			t.Fatalf("ValidateLicense failed: %v", err)
		}
	}
	avg := time.Since(start) / n
	if avg > 10*time.Millisecond {
		t.Fatalf("average validation time %v exceeds 10ms", avg)
	}
}

func TestResilience(t *testing.T) {
	env := setupDB(t)
	lic, err := dbpkg.CreateLicense(env.db, "resilience@example.com", 365, 1, "")
	if err != nil {
		t.Fatalf("CreateLicense failed: %v", err)
	}

	if _, err := dbpkg.ValidateLicense(env.db, lic.LicenseID, lic.LicenseKey); err != nil {
		t.Fatalf("initial validation failed: %v", err)
	}
	env.db.Close()

	if _, err := dbpkg.ValidateLicense(env.db, lic.LicenseID, lic.LicenseKey); err == nil {
		t.Fatal("validation should fail after DB close")
	}
}

func postJSON(t *testing.T, handler http.HandlerFunc, payload any) *httptest.ResponseRecorder {
	t.Helper()

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("json marshal failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler(rec, req)
	return rec
}

func TestViewerCodeLifecycle(t *testing.T) {
	env := setupDB(t)

	// Create tech license
	tech, err := dbpkg.CreateLicense(env.db, "tech@example.com", 365, 5, "")
	if err != nil {
		t.Fatalf("CreateLicense failed: %v", err)
	}

	// Generate viewer code
	vc, err := dbpkg.CreateViewerCode(env.db, tech.LicenseID, "client@viewer.com")
	if err != nil {
		t.Fatalf("CreateViewerCode failed: %v", err)
	}

	if !regexp.MustCompile(`^[23456789ABCDEFGHJKMNPQRSTUVWXYZ]{4}-[23456789ABCDEFGHJKMNPQRSTUVWXYZ]{4}$`).MatchString(vc.Code) {
		t.Fatalf("viewer code %q does not match XXXX-XXXX format", vc.Code)
	}

	// Validate viewer code
	config, err := dbpkg.ValidateViewerCode(env.db, vc.Code)
	if err != nil {
		t.Fatalf("ValidateViewerCode failed: %v", err)
	}
	if config.PublicKey == "" {
		t.Fatalf("ValidateViewerCode returned empty config")
	}

	// Revoke code
	if err := dbpkg.RevokeViewerCode(env.db, vc.Code); err != nil {
		t.Fatalf("RevokeViewerCode failed: %v", err)
	}

	// Validate should fail now
	_, err = dbpkg.ValidateViewerCode(env.db, vc.Code)
	if err == nil {
		t.Fatal("ValidateViewerCode should fail after revocation")
	}
}

func TestViewerCodesAreUnlimitedPerTechnician(t *testing.T) {
	env := setupDB(t)

	// Create tech license
	tech, err := dbpkg.CreateLicense(env.db, "tech2@example.com", 365, 5, "")
	if err != nil {
		t.Fatalf("CreateLicense failed: %v", err)
	}

	for i := 0; i < 25; i++ {
		if _, err := dbpkg.CreateViewerCode(env.db, tech.LicenseID, ""); err != nil {
			t.Fatalf("Failed to create code %d: %v", i+1, err)
		}
	}
}

func TestTechnicianAPI(t *testing.T) {
	env := setupDB(t)

	// 1. Create a tech license
	tech, err := dbpkg.CreateLicense(env.db, "tech3@example.com", 365, 5, "")
	if err != nil {
		t.Fatalf("CreateLicense failed: %v", err)
	}

	// 2. Login
	loginResp := postJSON(t, handlers.TechnicianLoginHandler(env.db, env.settings), map[string]string{
		"license_id":  tech.LicenseID,
		"license_key": tech.LicenseKey,
	})
	if loginResp.Code != http.StatusOK {
		t.Fatalf("login status = %d", loginResp.Code)
	}

	var loginData map[string]any
	if err := json.Unmarshal(loginResp.Body.Bytes(), &loginData); err != nil {
		t.Fatalf("json unmarshal failed: %v", err)
	}
	token, ok := loginData["token"].(string)
	if !ok || token == "" {
		t.Fatalf("missing token in login response")
	}

	// Helper for authenticated requests
	authReq := func(method, path string, body map[string]string) *httptest.ResponseRecorder {
		var reqBody *bytes.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			reqBody = bytes.NewReader(b)
		} else {
			reqBody = bytes.NewReader(nil)
		}

		req := httptest.NewRequest(method, path, reqBody)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()

		// Run through middleware
		handler := middleware.TechnicianAuth(env.db)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "generate") {
				handlers.TechnicianGenerateCodeHandler(env.db)(w, r)
			} else if strings.HasSuffix(r.URL.Path, "dashboard") {
				handlers.TechnicianDashboardHandler(env.db)(w, r)
			} else if strings.HasSuffix(r.URL.Path, "revoke") {
				handlers.TechnicianRevokeCodeHandler(env.db)(w, r)
			}
		}))
		handler.ServeHTTP(rec, req)
		return rec
	}

	// 3. Generate a code via API
	genResp := authReq(http.MethodPost, "/api/v1/technician/viewer-codes/generate", map[string]string{"client_email": "client_api@example.com"})
	if genResp.Code != http.StatusOK {
		t.Fatalf("generate status = %d: %s", genResp.Code, genResp.Body.String())
	}

	var genData map[string]any
	json.Unmarshal(genResp.Body.Bytes(), &genData)
	code := genData["code"].(string)

	// 4. Get Dashboard
	dashResp := authReq(http.MethodGet, "/api/v1/technician/dashboard", nil)
	if dashResp.Code != http.StatusOK {
		t.Fatalf("dashboard status = %d", dashResp.Code)
	}
	var dashData map[string]any
	json.Unmarshal(dashResp.Body.Bytes(), &dashData)
	if dashData["total_codes"].(float64) != 1 {
		t.Fatalf("expected 1 code, got %v", dashData["total_codes"])
	}

	// 5. Revoke code via API
	revResp := authReq(http.MethodPut, "/api/v1/technician/viewer-codes/"+code+"/revoke", nil)
	if revResp.Code != http.StatusOK {
		t.Fatalf("revoke status = %d: %s", revResp.Code, revResp.Body.String())
	}

	// 6. Verify Dashboard shows revoked
	dashResp2 := authReq(http.MethodGet, "/api/v1/technician/dashboard", nil)
	var dashData2 map[string]any
	json.Unmarshal(dashResp2.Body.Bytes(), &dashData2)
	if dashData2["active_codes"].(float64) != 0 {
		t.Fatalf("expected 0 active codes, got %v", dashData2["active_codes"])
	}
	codesList := dashData2["codes"].([]any)
	if len(codesList) != 1 || codesList[0].(map[string]any)["status"] != "revoked" {
		t.Fatalf("expected code status to be 'revoked', got %v", codesList)
	}
}

func TestDashboardCodeStatusLogic(t *testing.T) {
	env := setupDB(t)

	// 1. Create a technician license
	tech, err := dbpkg.CreateLicense(env.db, "tech_status@example.com", 365, 5, "")
	if err != nil {
		t.Fatalf("CreateLicense failed: %v", err)
	}

	token, err := dbpkg.CreateTechnicianSession(env.db, tech.LicenseID)
	if err != nil {
		t.Fatalf("CreateTechnicianSession failed: %v", err)
	}

	now := time.Now().UTC()

	// Code 1: is_active = 1, expires_at = now + 12h -> "active"
	expiresActive := now.Add(12 * time.Hour).Format("2006-01-02 15:04:05")
	_, err = env.db.Exec(`
		INSERT INTO viewer_codes (code, technician_license_id, client_email, created_at, expires_at, is_active)
		VALUES ('AAAA-1111', ?, 'active@example.com', ?, ?, 1)
	`, tech.LicenseID, now.Format("2006-01-02 15:04:05"), expiresActive)
	if err != nil {
		t.Fatalf("insert active code failed: %v", err)
	}

	// Code 2: is_active = 1, expires_at = now - 1m -> "expired"
	expiresExpired := now.Add(-1 * time.Minute).Format("2006-01-02 15:04:05")
	_, err = env.db.Exec(`
		INSERT INTO viewer_codes (code, technician_license_id, client_email, created_at, expires_at, is_active)
		VALUES ('BBBB-2222', ?, 'expired@example.com', ?, ?, 1)
	`, tech.LicenseID, now.Format("2006-01-02 15:04:05"), expiresExpired)
	if err != nil {
		t.Fatalf("insert expired code failed: %v", err)
	}

	// Code 3: is_active = 0, expires_at = now + 12h -> "revoked"
	_, err = env.db.Exec(`
		INSERT INTO viewer_codes (code, technician_license_id, client_email, created_at, expires_at, is_active, revoked_reason)
		VALUES ('CCCC-3333', ?, 'revoked@example.com', ?, ?, 0, 'révocation manuelle')
	`, tech.LicenseID, now.Format("2006-01-02 15:04:05"), expiresActive)
	if err != nil {
		t.Fatalf("insert revoked code failed: %v", err)
	}

	// Call dashboard endpoint
	req := httptest.NewRequest(http.MethodGet, "/api/v1/technician/dashboard", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	handler := middleware.TechnicianAuth(env.db)(handlers.TechnicianDashboardHandler(env.db))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("dashboard status = %d, body = %s", rec.Code, rec.Body.String())
	}

	type codeItem struct {
		Code        string `json:"code"`
		ClientEmail string `json:"client_email"`
		IsActive    bool   `json:"is_active"`
		Status      string `json:"status"`
	}

	var dashResp struct {
		TotalCodes   int        `json:"total_codes"`
		ActiveCodes  int        `json:"active_codes"`
		ExpiredCodes int        `json:"expired_codes"`
		Codes        []codeItem `json:"codes"`
	}

	if err := json.Unmarshal(rec.Body.Bytes(), &dashResp); err != nil {
		t.Fatalf("unmarshal dashboard response failed: %v", err)
	}

	if dashResp.TotalCodes != 3 {
		t.Errorf("expected 3 total codes, got %d", dashResp.TotalCodes)
	}
	if dashResp.ActiveCodes != 1 {
		t.Errorf("expected 1 active code, got %d", dashResp.ActiveCodes)
	}
	if dashResp.ExpiredCodes != 1 { // 1 code expiré (le 3e étant révoqué)
		t.Errorf("expected 1 expired code, got %d", dashResp.ExpiredCodes)
	}

	codeMap := make(map[string]codeItem)
	for _, c := range dashResp.Codes {
		codeMap[c.Code] = c
	}

	// Verify Code 1 (Active)
	c1, ok := codeMap["AAAA-1111"]
	if !ok {
		t.Fatal("code AAAA-1111 missing")
	}
	if c1.Status != "active" || !c1.IsActive {
		t.Errorf("AAAA-1111: expected status='active' and is_active=true, got status=%q, is_active=%v", c1.Status, c1.IsActive)
	}

	// Verify Code 2 (Expired)
	c2, ok := codeMap["BBBB-2222"]
	if !ok {
		t.Fatal("code BBBB-2222 missing")
	}
	if c2.Status != "expired" || c2.IsActive {
		t.Errorf("BBBB-2222: expected status='expired' and is_active=false, got status=%q, is_active=%v", c2.Status, c2.IsActive)
	}

	// Verify Code 3 (Revoked)
	c3, ok := codeMap["CCCC-3333"]
	if !ok {
		t.Fatal("code CCCC-3333 missing")
	}
	if c3.Status != "revoked" || c3.IsActive {
		t.Errorf("CCCC-3333: expected status='revoked' and is_active=false, got status=%q, is_active=%v", c3.Status, c3.IsActive)
	}
}
