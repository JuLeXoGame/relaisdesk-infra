package handlers

import (
	dbpkg "database"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestHealthHandlerStatusAndPrivacy(t *testing.T) {
	db, err := dbpkg.InitDatabase(filepath.Join(t.TempDir(), "health.db"))
	if err != nil {
		t.Fatal(err)
	}

	handler := HealthHandler(db)
	recorder := httptest.NewRecorder()
	handler(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/health", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("healthy status = %d, body = %q", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "licences_active") {
		t.Fatal("health response leaks business metrics")
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	recorder = httptest.NewRecorder()
	handler(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/health", nil))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("unhealthy status = %d, want %d; body = %q", recorder.Code, http.StatusServiceUnavailable, recorder.Body.String())
	}
}
