package main

import (
	dbpkg "database"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func setupTestDB(t *testing.T) *dbpkg.License {
	t.Helper()
	db, err := dbpkg.InitDatabase(filepath.Join(t.TempDir(), "licences.db"))
	if err != nil {
		t.Fatalf("InitDatabase failed: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	lic, err := CreateLicense(db, "test@example.com", 365, 1, "test")
	if err != nil {
		t.Fatalf("CreateLicense failed: %v", err)
	}
	return lic
}

func TestLicenseGeneration(t *testing.T) {
	lic := setupTestDB(t)

	if lic.Email != "test@example.com" {
		t.Errorf("Expected email test@example.com, got %s", lic.Email)
	}
	if lic.Status != "active" {
		t.Errorf("Expected status active, got %s", lic.Status)
	}
	if lic.LicenseID == "" || lic.LicenseKey == "" {
		t.Fatal("expected generated license identifiers")
	}
}

func TestRevokeAndExtendLicense(t *testing.T) {
	db, err := dbpkg.InitDatabase(filepath.Join(t.TempDir(), "licences.db"))
	if err != nil {
		t.Fatalf("InitDatabase failed: %v", err)
	}
	defer db.Close()

	lic, err := CreateLicense(db, "test@example.com", 30, 1, "test")
	if err != nil {
		t.Fatalf("CreateLicense failed: %v", err)
	}

	if err := ExtendLicense(db, lic.LicenseID, 15); err != nil {
		t.Fatalf("ExtendLicense failed: %v", err)
	}
	if err := RevokeLicense(db, lic.LicenseID, "reason"); err != nil {
		t.Fatalf("RevokeLicense failed: %v", err)
	}
}

func TestPricingCalculation(t *testing.T) {
	// 1. Starter
	starter, err := CalculatePlan("starter", 1)
	if err != nil {
		t.Fatalf("CalculatePlan starter failed: %v", err)
	}
	if starter.MonthlyPrice != 24.90 || starter.MaxConnections != 1 {
		t.Errorf("Starter attendu 24.90€/1 connexion, obtenu %.2f€/%d", starter.MonthlyPrice, starter.MaxConnections)
	}

	// 2. Pro
	pro, err := CalculatePlan("pro", 5)
	if err != nil {
		t.Fatalf("CalculatePlan pro failed: %v", err)
	}
	if pro.MonthlyPrice != 110.00 || pro.MaxConnections != 5 {
		t.Errorf("Pro attendu 110.00€/5 connexions, obtenu %.2f€/%d", pro.MonthlyPrice, pro.MaxConnections)
	}

	// 3. Ultra tiers
	tests := []struct {
		techs         int
		expectedPrice float64
	}{
		{10, 199.00},
		{20, 339.00},
		{50, 759.00},
		{100, 1259.00},
		{200, 1959.00},
		{500, 4059.00},
	}

	for _, tc := range tests {
		price := CalculateUltraPrice(tc.techs)
		if math.Abs(price-tc.expectedPrice) > 0.001 {
			t.Errorf("Ultra %d techniciens : attendu %.2f€, obtenu %.2f€", tc.techs, tc.expectedPrice, price)
		}

		ultraPlan, err := CalculatePlan("ultra", tc.techs)
		if err != nil {
			t.Errorf("CalculatePlan ultra %d failed: %v", tc.techs, err)
		} else if math.Abs(ultraPlan.MonthlyPrice-tc.expectedPrice) > 0.001 || ultraPlan.MaxConnections != tc.techs {
			t.Errorf("Plan Ultra %d : attendu %.2f€/%d, obtenu %.2f€/%d", tc.techs, tc.expectedPrice, tc.techs, ultraPlan.MonthlyPrice, ultraPlan.MaxConnections)
		}
	}
}

func TestAddServerKey(t *testing.T) {
	db, err := dbpkg.InitDatabase(filepath.Join(t.TempDir(), "licences.db"))
	if err != nil {
		t.Fatalf("InitDatabase failed: %v", err)
	}
	defer db.Close()

	if err := AddServerKey(db, "recette-server-key"); err != nil {
		t.Fatalf("AddServerKey failed: %v", err)
	}
	got, err := dbpkg.GetActiveServerPublicKey(db)
	if err != nil || got != "recette-server-key" {
		t.Fatalf("GetActiveServerPublicKey = %q, %v", got, err)
	}
	if err := AddServerKey(db, "  "); err == nil {
		t.Fatal("expected error for empty server key")
	}
}

func TestCalculatePlanRejectsOutOfRange(t *testing.T) {
	if _, err := CalculatePlan("ultra", 501); err == nil {
		t.Error("expected error for ultra with 501 technicians")
	}
	if _, err := CalculatePlan("unknown-plan", 1); err == nil {
		t.Error("expected error for unknown plan")
	}
}

func TestCustomPricingMatchesServerEveryCapacity(t *testing.T) {
	for capacity := 10; capacity <= 500; capacity++ {
		price, _, _, err := dbpkg.CalculateServerPrice("ultra", capacity)
		if err != nil || CalculateUltraPrice(capacity) != price {
			t.Fatalf("keygen/server price mismatch at %d: %v", capacity, err)
		}
	}
}

func TestExportCSVUsesPrivatePermissions(t *testing.T) {
	db, err := dbpkg.InitDatabase(filepath.Join(t.TempDir(), "licences.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := CreateLicense(db, "private@example.com", 30, 1, "test"); err != nil {
		t.Fatal(err)
	}

	exportPath := filepath.Join(t.TempDir(), "licences.csv")
	if err := os.WriteFile(exportPath, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := ExportCSV(db, exportPath); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(exportPath)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0600 {
			t.Fatalf("permissions = %o, want 600", got)
		}
	}
}
