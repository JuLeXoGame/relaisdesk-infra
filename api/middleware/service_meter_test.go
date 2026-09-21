package middleware

import (
	dbpkg "database"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func TestServiceMeterUsesAuthenticatedLicenseBudget(t *testing.T) {
	db, err := dbpkg.InitDatabase(filepath.Join(t.TempDir(), "meter.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	lic, err := dbpkg.CreateLicense(db, "meter@example.test", 365, 1, "Starter")
	if err != nil {
		t.Fatal(err)
	}
	token, err := dbpkg.CreateTechnicianSession(db, lic.LicenseID)
	if err != nil {
		t.Fatal(err)
	}
	reached := 0
	h := ServiceAwareRateLimit(db, 2, time.Minute)(ServiceOperationRateLimit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached++; w.WriteHeader(200) })))
	path := "/api/v1/technician/service-billing/SVC-0123456789abcdef0123456789abcdef/pulse"
	call := func(path, token string) int {
		r := httptest.NewRequest("POST", path, nil)
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}
	if call(path, "wrong") != 401 || reached != 0 {
		t.Fatal("pulse bypassed authentication")
	}
	for i := 0; i < 120; i++ {
		if code := call(path, token); code != 200 {
			t.Fatal("licence capacity unexpectedly limited", i, code)
		}
	}
	if call(path, token) != 429 {
		t.Fatal("pulse budget unlimited")
	}
	for i := 0; i < 2; i++ {
		if call("/ordinary", token) != 200 {
			t.Fatal("pulses consumed ordinary IP budget")
		}
	}
	if call("/ordinary", token) != 429 || call(path+"/extra", token) != 429 {
		t.Fatal("non-pulse route bypassed IP budget")
	}
	for _, bad := range []string{"/api/v1/technician/service-billing/SVC-short/pulse", "/api/v1/technician/service-billing/SVC-zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz/pulse"} {
		if IsServiceMeterPulse(httptest.NewRequest("POST", bad, nil)) {
			t.Fatal("invalid ID recognized")
		}
	}
}
