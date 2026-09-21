package main

import (
	"bytes"
	"encoding/json"
	"os"
	"runtime"
	"testing"
)

func TestCredentialsNeverPersistPlaintext(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	const password = "SyntheticStorageSecret2026!"
	if err := SaveCredentials("storage@example.invalid", password); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(getLicenseFilePath())
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte(password)) || bytes.Contains(data, []byte(`"password"`)) {
		t.Fatal("plaintext credential persisted")
	}
	credentials, err := LoadCredentials()
	if err != nil {
		t.Fatal(err)
	}
	if credentials.Email != "storage@example.invalid" {
		t.Fatal("identity lost")
	}
	if runtime.GOOS == "windows" && credentials.Password != password {
		t.Fatal("DPAPI roundtrip failed")
	}
	if runtime.GOOS != "windows" && credentials.Password != "" {
		t.Fatal("password retained without system vault")
	}
}

func TestLegacyPlaintextIsMigratedBeforeUse(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	path := getLicenseFilePath()
	const key = "SyntheticLegacyKey2026!"
	raw, err := json.Marshal(SavedCredentials{LicenseID: "SYNTHETIC-LICENCE", LicenseKey: key})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	credentials, err := LoadCredentials()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte(key)) {
		t.Fatal("legacy plaintext remains")
	}
	if credentials.LicenseID != "SYNTHETIC-LICENCE" {
		t.Fatal("legacy identity lost")
	}
}
