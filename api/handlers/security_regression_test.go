package handlers_test

// Regression tests for the September 2026 security review, using synthetic data only.
import (
	"api/handlers"
	"api/middleware"
	"bytes"
	dbpkg "database"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const auditPassword = "SyntheticAuditOnly2026!"

func auditFixture(t *testing.T) (*sql.DB, *dbpkg.License) {
	t.Helper()
	db, err := dbpkg.InitDatabase(filepath.Join(t.TempDir(), "audit.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	lic, err := dbpkg.CreateLicense(db, "audit@example.invalid", 30, 5, "ADMIN")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dbpkg.SetAdminPasswordWithLicense(db, lic.LicenseID, lic.LicenseKey, auditPassword); err != nil {
		t.Fatal(err)
	}
	return db, lic
}

func auditEnable(t *testing.T, db *sql.DB, email string) (string, []string) {
	t.Helper()
	secret, _, recovery, err := dbpkg.SetupCustomerTOTP(db, email)
	if err != nil {
		t.Fatal(err)
	}
	code, err := dbpkg.CalculateTOTP(secret, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := dbpkg.EnableCustomerTOTP(db, email, secret, code, recovery); err != nil {
		t.Fatal(err)
	}
	return secret, recovery
}

func auditRequest(t *testing.T, h http.Handler, body any, token string) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "https://audit.invalid/", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func auditToken(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	if w.Code != 200 {
		t.Fatalf("unexpected HTTP %d", w.Code)
	}
	var result map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	token, ok := result["token"].(string)
	if !ok || token == "" {
		t.Fatal("no full session token")
	}
	return token
}

func auditAuthorized(t *testing.T, db *sql.DB, token string, admin bool) bool {
	t.Helper()
	sentinel := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	var h http.Handler = middleware.CustomerAuth(db)(sentinel)
	if admin {
		h = middleware.AdminAuth("synthetic-master-unused", db)(sentinel)
	}
	return auditRequest(t, h, map[string]string{}, token).Code == 204
}

func auditEmailToken(t *testing.T, db *sql.DB, email string) string {
	t.Helper()
	tok, eligible, err := dbpkg.CreateCustomerLoginToken(db, email)
	if err != nil || !eligible || tok == "" {
		t.Fatalf("cannot create synthetic email token: %v", err)
	}
	return tok
}

func TestSecurityAdminMFAAndRevocation(t *testing.T) {
	db, lic := auditFixture(t)
	_, codes := auditEnable(t, db, lic.Email)
	w := auditRequest(t, handlers.AdminLoginHandler(db, "synthetic-master-unused"), map[string]string{"license_id": lic.LicenseID, "license_key": lic.LicenseKey}, "")
	var challenge map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &challenge); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || challenge["requires_2fa"] != true || challenge["token"] != nil {
		t.Fatal("licence login did not require MFA")
	}
	tok, ok := challenge["challenge_token"].(string)
	if !ok {
		t.Fatal("missing challenge")
	}
	token := auditToken(t, auditRequest(t, handlers.AdminLoginHandler(db, "synthetic-master-unused"), map[string]string{"challenge_token": tok, "code": codes[0]}, ""))
	if !auditAuthorized(t, db, token, true) {
		t.Fatal("legitimate MFA admin login failed")
	}
	if err := dbpkg.RevokeLicense(db, lic.LicenseID, "test"); err != nil {
		t.Fatal(err)
	}
	if auditAuthorized(t, db, token, true) {
		t.Fatal("revoked administrator still authorized")
	}
}
func TestSecurityAdminPasswordSessionIsBoundToLicense(t *testing.T) {
	db, lic := auditFixture(t)
	auth, _, err := dbpkg.ValidateAdminEmailPassword(db, lic.Email, auditPassword)
	if err != nil {
		t.Fatal(err)
	}
	if !auditAuthorized(t, db, auth.SessionToken, true) {
		t.Fatal("valid admin rejected")
	}
	if _, err = db.Exec("UPDATE licences SET notes='' WHERE license_id=?", lic.LicenseID); err != nil {
		t.Fatal(err)
	}
	if auditAuthorized(t, db, auth.SessionToken, true) {
		t.Fatal("removed admin role still authorized")
	}
}
func TestSecurityEmailLinkRequiresExistingMFA(t *testing.T) {
	db, lic := auditFixture(t)
	_, codes := auditEnable(t, db, lic.Email)
	emailToken := auditEmailToken(t, db, lic.Email)
	w := auditRequest(t, handlers.CustomerLoginVerifyHandler(db), map[string]string{"token": emailToken}, "")
	var challenge map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &challenge); err != nil {
		t.Fatal(err)
	}
	// MFA still required (no session issued), but signaled with 200 like
	// every other 2FA challenge: a 401 here would burn the IP-ban budget
	// on each legitimate MFA login.
	if w.Code != 200 || challenge["requires_2fa"] != true || challenge["token"] != nil {
		t.Fatal("email link bypassed MFA")
	}
	session := auditToken(t, auditRequest(t, handlers.CustomerLoginVerifyHandler(db), map[string]string{"token": emailToken, "code": codes[0]}, ""))
	if !auditAuthorized(t, db, session, false) {
		t.Fatal("legitimate email+MFA login failed")
	}
}
func TestSecurityResetPreservesMFAAndRevokesSessions(t *testing.T) {
	db, lic := auditFixture(t)
	old, _, err := dbpkg.ConsumeCustomerLoginToken(db, auditEmailToken(t, db, lic.Email))
	if err != nil {
		t.Fatal(err)
	}
	oldSecret, codes := auditEnable(t, db, lic.Email)
	if auditAuthorized(t, db, old, false) {
		t.Fatal("pre-enrollment session not revoked")
	}
	auth, err := dbpkg.ValidateCustomerPasswordWith2FA(db, lic.Email, auditPassword)
	if err != nil {
		t.Fatal(err)
	}
	existing, _, _, err := dbpkg.VerifyCustomer2FAChallenge(db, auth.ChallengeToken, codes[0])
	if err != nil {
		t.Fatal(err)
	}
	reset := auditEmailToken(t, db, lic.Email)
	w := auditRequest(t, handlers.CustomerSetPasswordWithTokenHandler(db), map[string]string{"token": reset, "password": "NewSyntheticPassword2026!"}, "")
	if w.Code == 200 {
		t.Fatal("reset bypassed MFA")
	}
	newSecret, err := dbpkg.GenerateTOTPSecret()
	if err != nil {
		t.Fatal(err)
	}
	code, err := dbpkg.CalculateTOTP(newSecret, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	w = auditRequest(t, handlers.CustomerSetPasswordWithTokenHandler(db), map[string]any{"token": reset, "password": "NewSyntheticPassword2026!", "current_2fa_code": codes[1], "totp_secret": newSecret, "totp_code": code, "recovery_codes": []string{"attacker-code"}}, "")
	if w.Code == 200 {
		t.Fatal("reset replaced enrolled factor")
	}
	token := auditToken(t, auditRequest(t, handlers.CustomerSetPasswordWithTokenHandler(db), map[string]string{"token": reset, "password": "NewSyntheticPassword2026!", "current_2fa_code": codes[1]}, ""))
	var stored string
	if err = db.QueryRow("SELECT totp_secret FROM customer_accounts WHERE email=?", lic.Email).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != oldSecret || !auditAuthorized(t, db, token, false) || auditAuthorized(t, db, existing, false) {
		t.Fatal("reset did not preserve MFA and revoke old sessions")
	}
}
func TestSecurityExistingFactorCannotBeOverwritten(t *testing.T) {
	db, lic := auditFixture(t)
	oldSecret, codes := auditEnable(t, db, lic.Email)
	auth, err := dbpkg.ValidateCustomerPasswordWith2FA(db, lic.Email, auditPassword)
	if err != nil {
		t.Fatal(err)
	}
	token, _, _, err := dbpkg.VerifyCustomer2FAChallenge(db, auth.ChallengeToken, codes[0])
	if err != nil {
		t.Fatal(err)
	}
	newSecret, err := dbpkg.GenerateTOTPSecret()
	if err != nil {
		t.Fatal(err)
	}
	code, err := dbpkg.CalculateTOTP(newSecret, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	w := auditRequest(t, middleware.CustomerAuth(db)(handlers.Customer2FAEnableHandler(db)), map[string]any{"password": auditPassword, "secret": newSecret, "code": code, "recovery_codes": []string{"not-generated-by-server"}}, token)
	if w.Code == 200 {
		t.Fatal("existing factor overwritten")
	}
	var stored string
	if err = db.QueryRow("SELECT totp_secret FROM customer_accounts WHERE email=?", lic.Email).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != oldSecret {
		t.Fatal("factor changed")
	}
}
func TestSecurityPasswordResetRevokesCustomerAndTechnician(t *testing.T) {
	db, lic := auditFixture(t)
	old, _, err := dbpkg.ConsumeCustomerLoginToken(db, auditEmailToken(t, db, lic.Email))
	if err != nil {
		t.Fatal(err)
	}
	admin, _, err := dbpkg.ValidateAdminEmailPassword(db, lic.Email, auditPassword)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err = dbpkg.SetCustomerPasswordWithToken(db, auditEmailToken(t, db, lic.Email), "ChangedSyntheticPassword2026!"); err != nil {
		t.Fatal(err)
	}
	if auditAuthorized(t, db, old, false) || auditAuthorized(t, db, admin.SessionToken, true) {
		t.Fatal("pre-reset sessions survived")
	}
}
func TestSecurityRecoveryCodeConsumedAtomicallyAcrossLogins(t *testing.T) {
	db, lic := auditFixture(t)
	_, codes := auditEnable(t, db, lic.Email)
	auth, err := dbpkg.ValidateCustomerPasswordWith2FA(db, lic.Email, auditPassword)
	if err != nil {
		t.Fatal(err)
	}
	gate := make(chan struct{})
	var wg sync.WaitGroup
	var successes atomic.Int32
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-gate
			if _, _, _, err := dbpkg.VerifyCustomer2FAChallenge(db, auth.ChallengeToken, codes[0]); err == nil {
				successes.Add(1)
			}
		}()
	}
	close(gate)
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatalf("expected exactly one successful consumption, got %d", successes.Load())
	}
	if _, _, err := dbpkg.ValidateTechnicianCredentialsWith2FA(db, lic.LicenseID, lic.LicenseKey, codes[0]); err == nil {
		t.Fatal("recovery code reused on technician endpoint")
	}
}
func TestSecuritySpoofedCloudflareHeaderCannotBypassLimit(t *testing.T) {
	h := middleware.StrictAuthLimiter(2, time.Hour)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(401) }))
	for i := 0; i < 12; i++ {
		req := httptest.NewRequest("POST", "https://audit.invalid/", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		req.Header.Set("X-Real-IP", "192.0.2.10")
		req.Header.Set("CF-Connecting-IP", fmt.Sprintf("198.51.100.%d", i+1))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if i < 2 && w.Code != 401 || i >= 2 && w.Code != 429 {
			t.Fatalf("request %d: HTTP %d", i, w.Code)
		}
	}
}
func TestSecurityAdminBootstrapCannotResetExistingPassword(t *testing.T) {
	db, lic := auditFixture(t)
	if _, err := dbpkg.SetAdminPasswordWithLicense(db, lic.LicenseID, lic.LicenseKey, "AnotherSyntheticPassword2026!"); err == nil {
		t.Fatal("bootstrap reset an existing password")
	}
	if _, _, err := dbpkg.ValidateAdminEmailPassword(db, lic.Email, auditPassword); err != nil {
		t.Fatal("existing password changed")
	}
}

func TestSecurityAdminPasswordSetupFailuresAre401(t *testing.T) {
	db, _ := auditFixture(t)
	// A valid but non-admin license must be indistinguishable from garbage:
	// same 401, same generic message (no oracle, and the limiter can ban).
	plain, err := dbpkg.CreateLicense(db, "plain@example.invalid", 30, 1, "note")
	if err != nil {
		t.Fatal(err)
	}
	handler := handlers.AdminPasswordSetupHandler(db)
	bodies := []map[string]string{
		{"license_id": plain.LicenseID, "license_key": plain.LicenseKey, "new_password": "NewPassword2026!"},
		{"license_id": "MP-UNKNOWN-0000-0000", "license_key": "garbage", "new_password": "NewPassword2026!"},
	}
	var first string
	for i, body := range bodies {
		w := auditRequest(t, handler, body, "")
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("case %d: status = %d, want 401", i, w.Code)
		}
		var decoded map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &decoded); err != nil {
			t.Fatal(err)
		}
		if decoded["error"] != "Identifiants incorrects" {
			t.Fatalf("case %d: error = %q, want generic", i, decoded["error"])
		}
		if i == 0 {
			first = w.Body.String()
		} else if w.Body.String() != first {
			t.Fatal("valid non-admin license distinguishable from garbage")
		}
	}
	// Malformed requests stay 400.
	w := auditRequest(t, handler, map[string]string{"license_id": "", "license_key": "", "new_password": "short"}, "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("malformed: status = %d, want 400", w.Code)
	}
}
