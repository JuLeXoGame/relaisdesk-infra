package handlers

import (
	"bytes"
	"context"
	dbpkg "database"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"api/middleware"
)

func TestEnrollmentQuotaEnforcedForCustomerAndTechnician(t *testing.T) {
	db, err := dbpkg.InitDatabase(filepath.Join(t.TempDir(), "quota-api.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	lic, err := dbpkg.CreateLicense(db, "quota-api@example.com", 30, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`WITH RECURSIVE numbers(n) AS (SELECT 1 UNION ALL SELECT n+1 FROM numbers WHERE n<500)
		INSERT INTO devices(device_id,license_id,permanent_code,enrollment_version) SELECT 'fixture-'||n,?,'hash-'||n,2 FROM numbers`, lic.LicenseID)
	if err != nil {
		t.Fatal(err)
	}
	q, err := dbpkg.ListFleetQuotas(db, 0, lic.LicenseID)
	if err != nil || len(q) != 1 {
		t.Fatalf("quota lookup %v %v", q, err)
	}
	for _, customer := range []bool{false, true} {
		body, _ := json.Marshal(map[string]string{"license_id": lic.LicenseID, "alias": "one too many"})
		if !customer {
			body = []byte(`{"alias":"one too many"}`)
		}
		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
		rec := httptest.NewRecorder()
		if customer {
			ctx := context.WithValue(req.Context(), middleware.CustomerIdentityContextKey, &dbpkg.CustomerIdentity{ID: q[0].CustomerID})
			CustomerCreateDeviceEnrollmentHandler(db)(rec, req.WithContext(ctx))
		} else {
			ctx := context.WithValue(req.Context(), middleware.TechnicianLicenseContextKey, lic.LicenseID)
			TechnicianCreateDeviceEnrollmentHandler(db)(rec, req.WithContext(ctx))
		}
		if rec.Code != http.StatusConflict {
			t.Fatalf("customer=%v status=%d body=%s", customer, rec.Code, rec.Body.String())
		}
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM devices").Scan(&count); err != nil || count != 500 {
		t.Fatalf("quota bypass %d %v", count, err)
	}
}

func TestPublicPricingIncludesFleetQuotas(t *testing.T) {
	rec := httptest.NewRecorder()
	PublicPricingHandler()(rec, httptest.NewRequest(http.MethodGet, "/api/v1/public/pricing", nil))
	var catalog map[string]struct {
		Limit int `json:"managed_devices"`
		Extra int `json:"managed_devices_per_extra_technician"`
		Max   int `json:"max_managed_devices"`
		Bonus int `json:"managed_devices_max_tier_bonus"`
	}
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Code)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &catalog); err != nil {
		t.Fatal(err)
	}
	for plan, want := range map[string]int{"starter": 500, "pro": 1000, "ultra": 2000} {
		if catalog[plan].Limit != want {
			t.Fatalf("%s: %+v", plan, catalog[plan])
		}
	}
	if catalog["ultra"].Extra != 5 || catalog["ultra"].Max != 4500 || catalog["ultra"].Bonus != 50 {
		t.Fatal(catalog["ultra"])
	}
}
