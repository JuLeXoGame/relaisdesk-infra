package tests

import (
	"bytes"
	dbpkg "database"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"api/handlers"
	"api/middleware"
)

func TestAdminLoginAndDashboardFlow(t *testing.T) {
	env := setupDB(t)

	// 1. Insert the Master Admin license from licenseAdmin.txt
	adminLicenseID := "MP-1CE6-0489-5AF5"
	adminLicenseKey := "mpsk_00000000000000000000000000000000"
	adminEmail := "admin@example.com"
	expiresFuture := time.Now().UTC().Add(36500 * 24 * time.Hour).Format("2006-01-02 15:04:05")

	_, err := env.db.Exec(`
		INSERT INTO licences (license_id, email, license_key, status, created_at, expires_at, max_connections, notes)
		VALUES (?, ?, ?, 'active', CURRENT_TIMESTAMP, ?, 100, 'ADMIN')
	`, adminLicenseID, adminEmail, adminLicenseKey, expiresFuture)
	if err != nil {
		t.Fatalf("Failed to insert admin license: %v", err)
	}

	// 2. Test Admin Login with valid Master License credentials
	loginPayload, _ := json.Marshal(map[string]string{
		"license_id":  adminLicenseID,
		"license_key": adminLicenseKey,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/login", bytes.NewReader(loginPayload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	adminToken := "test-admin-secret-token"

	handlers.AdminLoginHandler(env.db, adminToken)(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Admin login failed: status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var loginResp struct {
		Valid bool   `json:"valid"`
		Token string `json:"token"`
		Role  string `json:"role"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &loginResp); err != nil {
		t.Fatalf("Unmarshal admin login failed: %v", err)
	}
	if !loginResp.Valid || loginResp.Token == "" {
		t.Fatalf("Expected valid admin session token, got %+v", loginResp)
	}

	adminSessionToken := loginResp.Token

	// Helper for admin authenticated requests using the session token
	adminReq := func(method, path string, body any) *httptest.ResponseRecorder {
		var reqBody *bytes.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			reqBody = bytes.NewReader(b)
		} else {
			reqBody = bytes.NewReader(nil)
		}

		r := httptest.NewRequest(method, path, reqBody)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Authorization", "Bearer "+adminSessionToken)
		w := httptest.NewRecorder()

		// Route handler through admin middleware
		middleware.AdminAuth(adminToken, env.db)(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
			if req.URL.Path == "/api/v1/admin/stats" {
				handlers.AdminStatsHandler(env.db)(rw, req)
			} else if req.URL.Path == "/api/v1/admin/licences" && req.Method == http.MethodGet {
				handlers.AdminListLicensesHandler(env.db)(rw, req)
			} else if req.URL.Path == "/api/v1/admin/licences" && req.Method == http.MethodPost {
				handlers.AdminCreateLicenseHandler(env.db)(rw, req)
			} else if req.Method == http.MethodPost && (len(req.URL.Path) > len("/api/v1/admin/licences/") && req.URL.Path[len(req.URL.Path)-7:] == "/revoke") {
				handlers.AdminRevokeHandler(env.db)(rw, req)
			} else if req.Method == http.MethodPost && (len(req.URL.Path) > len("/api/v1/admin/licences/") && req.URL.Path[len(req.URL.Path)-7:] == "/extend") {
				handlers.AdminExtendHandler(env.db)(rw, req)
			}
		})).ServeHTTP(w, r)

		return w
	}

	// 3. Test Admin Stats
	statsRec := adminReq(http.MethodGet, "/api/v1/admin/stats", nil)
	if statsRec.Code != http.StatusOK {
		t.Fatalf("admin stats status = %d: %s", statsRec.Code, statsRec.Body.String())
	}

	// 4. Create commercial licenses (Starter, Pro, Ultra)
	// 4.A Starter Plan
	starterRec := adminReq(http.MethodPost, "/api/v1/admin/licences", map[string]any{
		"email": "client_starter@example.com",
		"plan":  "Starter",
		"days":  30,
	})
	if starterRec.Code != http.StatusCreated {
		t.Fatalf("create starter status = %d: %s", starterRec.Code, starterRec.Body.String())
	}
	var starterLic map[string]any
	json.Unmarshal(starterRec.Body.Bytes(), &starterLic)
	starterID := starterLic["license_id"].(string)
	if starterLic["max_connections"].(float64) != 1 {
		t.Errorf("expected max_connections=1 for Starter, got %v", starterLic["max_connections"])
	}

	// 4.B Pro Plan
	proRec := adminReq(http.MethodPost, "/api/v1/admin/licences", map[string]any{
		"email": "client_pro@example.com",
		"plan":  "Pro",
		"days":  30,
	})
	if proRec.Code != http.StatusCreated {
		t.Fatalf("create pro status = %d", proRec.Code)
	}
	var proLic map[string]any
	json.Unmarshal(proRec.Body.Bytes(), &proLic)
	if proLic["max_connections"].(float64) != 10 {
		t.Errorf("expected max_connections=10 for Pro, got %v", proLic["max_connections"])
	}

	// 4.C Ultra Plan (50 techniciens)
	ultraRec := adminReq(http.MethodPost, "/api/v1/admin/licences", map[string]any{
		"email":       "client_ultra@example.com",
		"plan":        "Ultra",
		"technicians": 50,
		"days":        30,
	})
	if ultraRec.Code != http.StatusCreated {
		t.Fatalf("create ultra status = %d", ultraRec.Code)
	}
	var ultraLic map[string]any
	json.Unmarshal(ultraRec.Body.Bytes(), &ultraLic)
	if ultraLic["max_connections"].(float64) != 50 {
		t.Errorf("expected max_connections=50 for Ultra, got %v", ultraLic["max_connections"])
	}

	// 5. Test List Licenses with unmask=true
	listRec := adminReq(http.MethodGet, "/api/v1/admin/licences?unmask=true", nil)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list licences status = %d: %s", listRec.Code, listRec.Body.String())
	}
	var listResp struct {
		Total    int              `json:"total"`
		Licences []map[string]any `json:"licences"`
	}
	json.Unmarshal(listRec.Body.Bytes(), &listResp)
	if listResp.Total < 4 {
		t.Errorf("expected at least 4 licenses, got %d", listResp.Total)
	}
	// Verify unmasked key is full mpsk_ string (not masked with ****)
	var foundStarterKey string
	for _, l := range listResp.Licences {
		if l["license_id"] == starterID {
			foundStarterKey = l["license_key"].(string)
		}
	}
	if len(foundStarterKey) < 20 || foundStarterKey[:5] != "mpsk_" {
		t.Errorf("expected unmasked key starting with mpsk_, got %q", foundStarterKey)
	}

	// 6. Test Extend License
	extendRec := adminReq(http.MethodPost, "/api/v1/admin/licences/"+starterID+"/extend", map[string]int{"days": 30})
	if extendRec.Code != http.StatusOK {
		t.Fatalf("extend license status = %d: %s", extendRec.Code, extendRec.Body.String())
	}

	// 7. Test Revoke License
	revokeRec := adminReq(http.MethodPost, "/api/v1/admin/licences/"+starterID+"/revoke", map[string]string{"reason": "Test révocation"})
	if revokeRec.Code != http.StatusOK {
		t.Fatalf("revoke license status = %d: %s", revokeRec.Code, revokeRec.Body.String())
	}

	licAfter, _ := dbpkg.GetLicense(env.db, starterID)
	if licAfter.Status != "revoked" {
		t.Errorf("expected status revoked, got %s", licAfter.Status)
	}
}

func TestFormerMasterLicenseIDDoesNotGrantAdminRights(t *testing.T) {
	env := setupDB(t)
	licenseID := "MP-1CE6-0489-5AF5"
	licenseKey := "mpsk_11111111111111111111111111111111"
	expiresFuture := time.Now().UTC().Add(24 * time.Hour).Format("2006-01-02 15:04:05")
	if _, err := env.db.Exec(`
		INSERT INTO licences (license_id, email, license_key, status, created_at, expires_at, max_connections, notes)
		VALUES (?, 'ordinary@example.com', ?, 'active', CURRENT_TIMESTAMP, ?, 1, '')
	`, licenseID, licenseKey, expiresFuture); err != nil {
		t.Fatal(err)
	}

	payload, _ := json.Marshal(map[string]string{"license_id": licenseID, "license_key": licenseKey})
	recorder := httptest.NewRecorder()
	handlers.AdminLoginHandler(env.db, "different-master-secret")(
		recorder,
		httptest.NewRequest(http.MethodPost, "/api/v1/admin/login", bytes.NewReader(payload)),
	)
	// Uniform 401 (not 403): a distinct status would oracle license
	// credential validity. No admin session must be issued.
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("former master ID login status = %d, want %d; body=%s", recorder.Code, http.StatusUnauthorized, recorder.Body.String())
	}
	var denied map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &denied); err != nil || denied["token"] != nil {
		t.Fatalf("non-admin login issued a session: %s", recorder.Body.String())
	}

	session, err := dbpkg.CreateTechnicianSession(env.db, licenseID)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/stats", nil)
	request.Header.Set("Authorization", "Bearer "+session)
	recorder = httptest.NewRecorder()
	middleware.AdminAuth("different-master-secret", env.db)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("ordinary license reached an admin handler")
	})).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("former master ID session status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
}

func TestAdminLoginWithEmailPasswordAndSetup(t *testing.T) {
	env := setupDB(t)

	adminLicenseID := "MP-ADMN-LIFE-9999"
	adminLicenseKey := "mpsk_lifetimeadmin000000000000000001"
	adminEmail := "operator@relaisdesk.fr"
	adminPassword := "AdminSuperSecret2026!"
	expiresFuture := time.Now().UTC().Add(36500 * 24 * time.Hour).Format("2006-01-02 15:04:05")

	// 1. Insert lifetime admin license
	_, err := env.db.Exec(`
		INSERT INTO licences (license_id, email, license_key, status, created_at, expires_at, max_connections, notes)
		VALUES (?, ?, ?, 'active', CURRENT_TIMESTAMP, ?, 100, 'ADMIN')
	`, adminLicenseID, adminEmail, adminLicenseKey, expiresFuture)
	if err != nil {
		t.Fatalf("Failed to insert admin license: %v", err)
	}

	// 2. Setup password using admin license credentials
	setupPayload, _ := json.Marshal(map[string]string{
		"license_id":   adminLicenseID,
		"license_key":  adminLicenseKey,
		"new_password": adminPassword,
	})
	setupReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/password/setup", bytes.NewReader(setupPayload))
	setupReq.Header.Set("Content-Type", "application/json")
	setupRec := httptest.NewRecorder()
	handlers.AdminPasswordSetupHandler(env.db)(setupRec, setupReq)
	if setupRec.Code != http.StatusOK {
		t.Fatalf("Admin password setup failed: code=%d body=%s", setupRec.Code, setupRec.Body.String())
	}

	// 3. Login with Email & Password
	loginPayload, _ := json.Marshal(map[string]string{
		"email":    adminEmail,
		"password": adminPassword,
	})
	loginReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/login", bytes.NewReader(loginPayload))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()
	handlers.AdminLoginHandler(env.db, "master-secret")(loginRec, loginReq)

	if loginRec.Code != http.StatusOK {
		t.Fatalf("Admin email login failed: code=%d body=%s", loginRec.Code, loginRec.Body.String())
	}

	var resp struct {
		Valid     bool   `json:"valid"`
		Token     string `json:"token"`
		Role      string `json:"role"`
		Email     string `json:"email"`
		LicenseID string `json:"license_id"`
	}
	if err := json.Unmarshal(loginRec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if !resp.Valid || resp.Token == "" || resp.Role != "admin" || resp.Email != adminEmail || resp.LicenseID != adminLicenseID {
		t.Fatalf("Unexpected admin login response: %+v", resp)
	}

	// 4. Verify admin session allows access to protected admin route
	statsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/stats", nil)
	statsReq.Header.Set("Authorization", "Bearer "+resp.Token)
	statsRec := httptest.NewRecorder()
	middleware.AdminAuth("master-secret", env.db)(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		handlers.AdminStatsHandler(env.db)(rw, req)
	})).ServeHTTP(statsRec, statsReq)

	if statsRec.Code != http.StatusOK {
		t.Fatalf("Admin stats with session token failed: code=%d body=%s", statsRec.Code, statsRec.Body.String())
	}
}

func TestAdminLoginWithEmailBadPasswordAndMissingRights(t *testing.T) {
	env := setupDB(t)

	adminLicenseID := "MP-ADMN-BAD-0001"
	adminLicenseKey := "mpsk_badpassadmin0000000000000000001"
	adminEmail := "boss@relaisdesk.fr"
	adminPassword := "CorrectPassword2026!"
	expiresFuture := time.Now().UTC().Add(36500 * 24 * time.Hour).Format("2006-01-02 15:04:05")

	_, err := env.db.Exec(`
		INSERT INTO licences (license_id, email, license_key, status, created_at, expires_at, max_connections, notes)
		VALUES (?, ?, ?, 'active', CURRENT_TIMESTAMP, ?, 100, 'ADMIN')
	`, adminLicenseID, adminEmail, adminLicenseKey, expiresFuture)
	if err != nil {
		t.Fatalf("Failed to insert admin license: %v", err)
	}

	_, err = dbpkg.SetAdminPasswordWithLicense(env.db, adminLicenseID, adminLicenseKey, adminPassword)
	if err != nil {
		t.Fatalf("SetAdminPasswordWithLicense failed: %v", err)
	}

	// 1. Wrong password -> 401
	badPayload, _ := json.Marshal(map[string]string{
		"email":    adminEmail,
		"password": "WrongPassword2026!",
	})
	badReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/login", bytes.NewReader(badPayload))
	badReq.Header.Set("Content-Type", "application/json")
	badRec := httptest.NewRecorder()
	handlers.AdminLoginHandler(env.db, "master-secret")(badRec, badReq)
	if badRec.Code != http.StatusUnauthorized {
		t.Fatalf("Expected 401 for bad password, got %d", badRec.Code)
	}

	// 2. Email without admin license -> uniform 401 with a generic message
	// (a distinct 403 would oracle credential validity; the real reason
	// stays server-side in the logs).
	userEmail := "normal.tech@example.com"
	userPassword := "NormalUserPass2026!"
	_, err = env.db.Exec(`
		INSERT INTO licences (license_id, email, license_key, status, created_at, expires_at, max_connections, notes)
		VALUES ('MP-NORM-0001', ?, 'mpsk_norm00000000000000000000000001', 'active', CURRENT_TIMESTAMP, ?, 2, 'starter')
	`, userEmail, expiresFuture)
	if err != nil {
		t.Fatalf("Failed to insert normal license: %v", err)
	}
	_ = dbpkg.SetCustomerPassword(env.db, userEmail, userPassword)

	noAdminPayload, _ := json.Marshal(map[string]string{
		"email":    userEmail,
		"password": userPassword,
	})
	noAdminReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/login", bytes.NewReader(noAdminPayload))
	noAdminReq.Header.Set("Content-Type", "application/json")
	noAdminRec := httptest.NewRecorder()
	handlers.AdminLoginHandler(env.db, "master-secret")(noAdminRec, noAdminReq)
	if noAdminRec.Code != http.StatusUnauthorized {
		t.Fatalf("Expected uniform 401 for email without admin license, got %d", noAdminRec.Code)
	}
	var noAdminBody map[string]string
	if err := json.Unmarshal(noAdminRec.Body.Bytes(), &noAdminBody); err != nil || noAdminBody["error"] != "Identifiants incorrects" {
		t.Fatalf("Expected generic error message, got %q", noAdminRec.Body.String())
	}
}
