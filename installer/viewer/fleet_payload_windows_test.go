//go:build windows

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativePayloadIgnoresUnverifiedUserCache(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	dir := filepath.Join(os.Getenv("LOCALAPPDATA"), "rustdesk")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"rustdesk.exe", "sciter.dll"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("INERT TEST DATA"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	saved := RUSTDESK_SERVICE_EXPECTED_SHA256
	RUSTDESK_SERVICE_EXPECTED_SHA256 = ""
	t.Cleanup(func() { RUSTDESK_SERVICE_EXPECTED_SHA256 = saved })
	// Missing embedded identity must fail before any system file/service operation.
	if _, _, err := extractNativeRustDeskPayload(); err == nil {
		t.Fatal("accepted unverified payload")
	}
}

func TestReleaseNativePayloadMatchesInjectedIdentity(t *testing.T) {
	prevExpected := RUSTDESK_SERVICE_EXPECTED_SHA256
	defer func() { RUSTDESK_SERVICE_EXPECTED_SHA256 = prevExpected }()
	if injected := strings.TrimSpace(os.Getenv("RELAISDESK_TEST_NATIVE_SHA256")); injected != "" {
		RUSTDESK_SERVICE_EXPECTED_SHA256 = injected
	}
	if RUSTDESK_SERVICE_EXPECTED_SHA256 == "" {
		t.Skip("requires release payload and build-time identity")
	}
	payload, err := verifiedNativePayload()
	if err != nil {
		t.Fatal(err)
	}
	if len(payload) != 3 {
		t.Fatal("unexpected native payload component count")
	}
	if err := installNativePayload(t.TempDir(), payload); err != nil {
		t.Fatal(err)
	}
}

func TestNativePayloadDLLIntegrityAndUnknownDLL(t *testing.T) {
	dir := t.TempDir()
	payload := map[string][]byte{"rustdesk.exe": []byte("INERT EXE"), "sciter.dll": []byte("INERT DLL"), "dylib_virtual_display.dll": []byte("INERT DISPLAY")}
	if err := installNativePayload(dir, payload); err != nil {
		t.Fatal(err)
	}
	if !nativeDirectoryMatches(dir, payload) {
		t.Fatal("complete payload rejected")
	}
	if err := os.WriteFile(filepath.Join(dir, "sciter.dll"), []byte("TAMPERING"), 0600); err != nil {
		t.Fatal(err)
	}
	if nativeDirectoryMatches(dir, payload) {
		t.Fatal("modified DLL accepted")
	}
	if err := installNativePayload(dir, payload); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "foreign.dll"), []byte("UNRELATED FILE"), 0600); err != nil {
		t.Fatal(err)
	}
	if nativeDirectoryMatches(dir, payload) {
		t.Fatal("unknown DLL accepted")
	}
	if err := installNativePayload(dir, payload); err == nil {
		t.Fatal("unknown DLL did not block installation")
	}
	data, err := os.ReadFile(filepath.Join(dir, "foreign.dll"))
	if err != nil || !bytes.Equal(data, []byte("UNRELATED FILE")) {
		t.Fatal("unrelated file was deleted")
	}
}

func TestFleetServiceDoesNotOwnAnIndependentInstallation(t *testing.T) {
	target := `C:\Program Files\RelaisDeskEngine\rustdesk.exe`
	for _, command := range []string{
		`"C:\Program Files\RustDesk\rustdesk.exe" --service`,
		`"C:\Users\Public\rustdesk.exe" --service`,
		`rustdesk.exe --service`,
		`"C:\Program Files\RelaisDeskEngine\rustdesk.exe" --service --extra`,
		`C:\Program Files\RelaisDeskEngine\rustdesk.exe --service`,
	} {
		if fleetServiceCommandMatches(command, target) {
			t.Fatalf("unsafe service command accepted: %s", command)
		}
	}
	if !fleetServiceCommandMatches(`"C:\Program Files\RelaisDeskEngine\rustdesk.exe" --service`, target) {
		t.Fatal("owned service rejected")
	}
}
