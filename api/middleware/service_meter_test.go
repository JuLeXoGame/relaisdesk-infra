package middleware

import (
	dbpkg "database"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFleetHeartbeatKeepsItsOwnBudgetBehindSharedIP(t *testing.T) {
	const path = "/api/v1/devices/heartbeat"
	mux := http.NewServeMux()
	mux.Handle(path, DeviceRateLimit(4, 2, time.Minute)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})))
	mux.HandleFunc("/ordinary", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	h := ServiceAwareRateLimit(nil, 2, time.Minute)(mux)
	call := func(method, url, device string) int {
		r := httptest.NewRequest(method, url, strings.NewReader(`{"device_id":"`+device+`"}`))
		r.RemoteAddr = "198.51.100.9:1234"
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}
	for _, device := range []string{"DEV-A", "DEV-B", "DEV-C", "DEV-D"} {
		if code := call(http.MethodPost, path, device); code != http.StatusNoContent {
			t.Fatalf("heartbeat %s blocked by ordinary budget: %d", device, code)
		}
	}
	if code := call(http.MethodPost, path, "DEV-E"); code != http.StatusTooManyRequests {
		t.Fatalf("fleet IP flood guard bypassed: %d", code)
	}
	for i := 0; i < 2; i++ {
		if code := call(http.MethodGet, "/ordinary", ""); code != http.StatusNoContent {
			t.Fatalf("fleet consumed ordinary request budget: %d", code)
		}
	}
	for _, target := range []struct{ method, path string }{
		{http.MethodGet, "/ordinary"},
		{http.MethodGet, path},
		{http.MethodPost, path + "/extra"},
	} {
		if code := call(target.method, target.path, "DEV-F"); code != http.StatusTooManyRequests {
			t.Fatalf("non-heartbeat bypassed ordinary budget: %s %s = %d", target.method, target.path, code)
		}
	}
}

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
