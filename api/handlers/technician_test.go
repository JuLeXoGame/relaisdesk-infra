package handlers

import (
	"bytes"
	dbpkg "database"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestTechnicianLoginFailureIsGeneric(t *testing.T) {
	db, err := dbpkg.InitDatabase(filepath.Join(t.TempDir(), "tech-login.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	handler := TechnicianLoginHandler(db, ServerSettings{})
	for _, body := range []string{
		`{"license_id":"MP-UNKNOWN","license_key":"mpsk_nope"}`,
		`{"email":"nobody@example.com","password":"WrongPassword123!"}`,
	} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/technician/login", bytes.NewBufferString(body))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("body %s: status = %d, want 401", body, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "Identifiants incorrects") {
			t.Fatalf("body %s: generic message missing: %s", body, rec.Body.String())
		}
		for _, leak := range []string{"invalid license", "unknown", "no account", "mot de passe"} {
			if strings.Contains(strings.ToLower(rec.Body.String()), leak) {
				t.Fatalf("body %s: backend detail leaked (%q): %s", body, leak, rec.Body.String())
			}
		}
	}
}
