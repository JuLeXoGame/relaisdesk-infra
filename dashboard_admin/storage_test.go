package main

import (
	"os"
	"strings"
	"testing"
)

func TestAdminSessionRoundTripNeverStoresTokenPlaintext(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	t.Setenv("USERPROFILE", dir)
	saved := AdminStoredSession{Token: "top-secret-admin-token-xyz", LicenseID: "LIC-1", Email: "admin@example.com"}
	if err := SaveAdminSession(saved); err != nil {
		t.Fatalf("SaveAdminSession: %v", err)
	}
	path, err := getSessionFilePath()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), saved.Token) {
		t.Fatal("admin token stored in plaintext")
	}
	loaded, err := LoadAdminSession()
	if err != nil {
		t.Fatalf("LoadAdminSession: %v", err)
	}
	if loaded.Email != saved.Email || loaded.LicenseID != saved.LicenseID {
		t.Fatalf("identifiers lost: %+v", loaded)
	}
}

func TestAdminSessionMigratesLegacyPlaintext(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	t.Setenv("USERPROFILE", dir)
	path, err := getSessionFilePath()
	if err != nil {
		t.Fatal(err)
	}
	legacy := `{"token":"legacy-plaintext-token-abc","license_id":"LIC-9","email":"old@example.com"}`
	if err := os.WriteFile(path, []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadAdminSession()
	if err != nil {
		t.Fatalf("LoadAdminSession: %v", err)
	}
	if loaded.Email != "old@example.com" || loaded.LicenseID != "LIC-9" {
		t.Fatalf("identifiers lost: %+v", loaded)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "legacy-plaintext-token-abc") {
		t.Fatal("legacy plaintext token survived migration")
	}
}
