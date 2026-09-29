package handlers

import (
	"bytes"
	dbpkg "database"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestAdminCreateLicenseProMatchesCatalogue(t *testing.T) {
	db, err := dbpkg.InitDatabase(filepath.Join(t.TempDir(), "admin-license.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	body, _ := json.Marshal(map[string]any{"email": "pro@example.com", "days": 30, "plan": "pro"})
	req := httptest.NewRequest(http.MethodPost, "/admin/licenses", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	AdminCreateLicenseHandler(db)(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if got := payload["max_connections"]; got != float64(5) {
		t.Errorf("pro max_connections = %v, want 5", got)
	}
	if got := payload["notes"]; got != "Plan Pro (110€/mois)" {
		t.Errorf("pro notes = %q", got)
	}
}
