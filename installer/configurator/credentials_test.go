package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestStorageCredentialsRoundtrip(t *testing.T) {
	tempDir := t.TempDir()
	origAppData := os.Getenv("APPDATA")
	origHome := os.Getenv("HOME")
	os.Setenv("APPDATA", tempDir)
	defer func() {
		os.Setenv("APPDATA", origAppData)
		os.Setenv("HOME", origHome)
	}()

	// 1. Save new email/pwd credentials
	err := SaveCredentials("tech@example.com", "my-secure-pwd")
	if err != nil {
		t.Fatalf("SaveCredentials failed: %v", err)
	}

	loaded, err := LoadCredentials()
	if err != nil {
		t.Fatalf("LoadCredentials failed: %v", err)
	}
	expectedPassword := ""
	if runtime.GOOS == "windows" {
		expectedPassword = "my-secure-pwd"
	}
	if loaded.Email != "tech@example.com" || loaded.Password != expectedPassword {
		t.Errorf("Unexpected loaded credentials: %+v", loaded)
	}

	// 2. Clear credentials
	if err := ClearLicense(); err != nil {
		t.Fatalf("ClearLicense failed: %v", err)
	}

	// 3. Test legacy license.json compatibility
	legacyPath := filepath.Join(tempDir, "RelaisDesk", "license.json")
	_ = os.MkdirAll(filepath.Dir(legacyPath), 0700)
	legacyData := map[string]string{
		"license_id":  "MP-OLD-1234-5678",
		"license_key": "mpsk_legacy_secret",
	}
	raw, _ := json.Marshal(legacyData)
	_ = os.WriteFile(legacyPath, raw, 0600)

	loadedLegacy, err := LoadLicense()
	if err != nil {
		t.Fatalf("LoadLicense failed for legacy format: %v", err)
	}
	expectedKey := ""
	if runtime.GOOS == "windows" {
		expectedKey = "mpsk_legacy_secret"
	}
	if loadedLegacy.LicenseID != "MP-OLD-1234-5678" || loadedLegacy.LicenseKey != expectedKey {
		t.Errorf("Unexpected legacy license data: %+v", loadedLegacy)
	}
}

func TestLoginTechnicianEmailVsLicenseDispatch(t *testing.T) {
	var receivedBody map[string]string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/technician/login" {
			_ = json.NewDecoder(r.Body).Decode(&receivedBody)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(TechnicianLoginResponse{
				Valid:     true,
				Token:     "test-token",
				LicenseID: "MP-INTERNAL-001",
				Email:     "tech@example.com",
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	origAPI := APIURL
	APIURL = ts.URL
	defer func() { APIURL = origAPI }()

	// Test 1: Email + Password dispatch
	resp, err := loginTechnician("admin@domain.com", "pass123")
	if err != nil {
		t.Fatalf("loginTechnician with email failed: %v", err)
	}
	if !resp.Valid || resp.LicenseID != "MP-INTERNAL-001" {
		t.Errorf("Unexpected response: %+v", resp)
	}
	if receivedBody["email"] != "admin@domain.com" || receivedBody["password"] != "pass123" {
		t.Errorf("Server did not receive expected email payload: %+v", receivedBody)
	}

	// Test 2: License ID + License Key dispatch
	respLic, err := loginTechnician("MP-ABCD-1234-5678", "secret-key")
	if err != nil {
		t.Fatalf("loginTechnician with license failed: %v", err)
	}
	if !respLic.Valid {
		t.Errorf("Unexpected response: %+v", respLic)
	}
	if receivedBody["license_id"] != "MP-ABCD-1234-5678" || receivedBody["license_key"] != "secret-key" {
		t.Errorf("Server did not receive expected license payload: %+v", receivedBody)
	}
}
