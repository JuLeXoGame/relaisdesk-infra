//go:build windows

package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestEmbeddedRustDeskRequiresAnInjectedPinnedSHA256(t *testing.T) {
	previous := RUSTDESK_EXPECTED_SHA256
	defer func() { RUSTDESK_EXPECTED_SHA256 = previous }()
	RUSTDESK_EXPECTED_SHA256 = ""
	if bytesMatchRustDeskSHA256(embeddedRustDesk) {
		t.Fatal("an empty build-time hash must fail closed")
	}
	sum := sha256.Sum256(embeddedRustDesk)
	RUSTDESK_EXPECTED_SHA256 = fmt.Sprintf("%x", sum)
	if !bytesMatchRustDeskSHA256(embeddedRustDesk) {
		t.Fatal("the injected hash should authorize exactly the embedded artifact")
	}
}

func TestGeneratedViewerTomlUsesCommunityServersDirectly(t *testing.T) {
	content := generateViewerRustDesk2Toml(
		"api.relaisdesk.fr:21116",
		"api.relaisdesk.fr",
		"api.relaisdesk.fr",
		"qdCKy9ILJQmM40BjSBn3s+7KMN+YR37CU7hWfYLmz74=",
		`C:\Users\test\AppData\Roaming\RelaisDesk\authorization\viewer-network-token`,
		`C:\Users\test\AppData\Roaming\RelaisDesk\authorization\viewer-proof-key`,
	)

	expected := []string{
		"rendezvous_server = 'api.relaisdesk.fr:21116'",
		"nat_type = 1",
		"serial = 0",
		"[options]",
		"custom-rendezvous-server = 'api.relaisdesk.fr'",
		"relay-server = 'api.relaisdesk.fr'",
		"api-server = 'https://api.relaisdesk.fr'",
		"key = 'qdCKy9ILJQmM40BjSBn3s+7KMN+YR37CU7hWfYLmz74='",
		"relaisdesk-token-file = 'C:\\Users\\test\\AppData\\Roaming\\RelaisDesk\\authorization\\viewer-network-token'",
		"relaisdesk-proof-key-file = 'C:\\Users\\test\\AppData\\Roaming\\RelaisDesk\\authorization\\viewer-proof-key'",
	}
	for _, value := range expected {
		if !strings.Contains(content, value) {
			t.Fatalf("missing %q in generated config:\n%s", value, content)
		}
	}

	forbidden := []string{"proxy-", "socks", "disable-udp", "1080"}
	for _, value := range forbidden {
		if strings.Contains(content, value) {
			t.Fatalf("obsolete setting %q found in generated config:\n%s", value, content)
		}
	}
}

func TestParseNumericRustDeskID(t *testing.T) {
	packerOutput := `skip .\dylib_virtual_display.dll
skip .\rustdesk.exe
skip .\sciter.dll
executing C:\Users\Administrator\AppData\Local\rustdesk\.\rustdesk.exe
Windows version: 10.0
is windows7: false
1248626563
`
	if id := parseNumericRustDeskID(packerOutput); id != "1248626563" {
		t.Fatalf("expected 1248626563, got %q", id)
	}

	cleanOutput := "987654321\r\n"
	if id := parseNumericRustDeskID(cleanOutput); id != "987654321" {
		t.Fatalf("expected 987654321, got %q", id)
	}

	invalidOutput := "some error occurred\r\nDone!\r\n"
	if id := parseNumericRustDeskID(invalidOutput); id != "" {
		t.Fatalf("expected empty for invalid output, got %q", id)
	}
}

func TestVerifiedInnerRustDeskPathRejectsUnpinnedBinary(t *testing.T) {
	previous := RUSTDESK_EXPECTED_SHA256
	defer func() { RUSTDESK_EXPECTED_SHA256 = previous }()
	sum := sha256.Sum256(embeddedRustDesk)
	RUSTDESK_EXPECTED_SHA256 = fmt.Sprintf("%x", sum)

	fakeAppData := t.TempDir()
	innerDir := fakeAppData + `\rustdesk`
	if err := os.MkdirAll(innerDir, 0700); err != nil {
		t.Fatal(err)
	}
	planted := innerDir + `\rustdesk.exe`
	// Planted same-user binary: must never be selected for execution.
	if err := os.WriteFile(planted, []byte("malicious-binary"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := verifiedInnerRustDeskPath(fakeAppData); got != "" {
		t.Fatalf("unpinned binary accepted: %q", got)
	}
	// Exact pinned bytes: accepted.
	if err := os.WriteFile(planted, embeddedRustDesk, 0600); err != nil {
		t.Fatal(err)
	}
	if got := verifiedInnerRustDeskPath(fakeAppData); got == "" {
		t.Fatal("pinned binary rejected")
	}
	if got := verifiedInnerRustDeskPath(""); got != "" {
		t.Fatalf("empty LOCALAPPDATA accepted: %q", got)
	}
}

func TestFileMatchesFleetServiceRejectsPathOutsideProgramFiles(t *testing.T) {
	if fileMatchesFleetService(`C:\Windows\System32\cmd.exe`) {
		t.Fatal("file outside Program Files must be rejected")
	}
	if fileMatchesFleetService(`C:\Users\Public\rustdesk.exe`) {
		t.Fatal("file in user directory must be rejected")
	}
}

func TestEnsureRustDeskServiceInstalledLive(t *testing.T) {
	if os.Getenv("RELAISDESK_LIVE_SERVICE_TEST") != "1" {
		t.Skip("opt-in integration test for a disposable Windows VM only")
	}
	if !isElevated() {
		t.Skip("requires elevated privileges")
	}
	if rustDeskServiceExists() || fleetServiceExists() {
		t.Fatal("refusing to modify a machine with an existing RustDesk/Fleet service")
	}
	prevExpected := RUSTDESK_SERVICE_EXPECTED_SHA256
	defer func() { RUSTDESK_SERVICE_EXPECTED_SHA256 = prevExpected }()
	RUSTDESK_SERVICE_EXPECTED_SHA256 = strings.TrimSpace(os.Getenv("RELAISDESK_TEST_NATIVE_SHA256"))
	if _, err := verifiedNativePayload(); err != nil {
		t.Fatal(err)
	}

	targetExe, err := ensureRustDeskServiceInstalled()
	if err != nil {
		t.Fatalf("ensureRustDeskServiceInstalled failed: %v", err)
	}
	t.Cleanup(func() {
		_ = uninstallFleet()
	})

	if !fileMatchesFleetService(targetExe) {
		t.Fatalf("targetExe %s failed fileMatchesFleetService", targetExe)
	}
	if !fleetRustDeskRunning() {
		t.Fatalf("RustDesk service is not running")
	}
}

func TestWindowsFleetConfigEnforcesUnattendedPasswordMode(t *testing.T) {
	relay := "79.72.27.213"
	config := generateViewerRustDesk2Toml("79.72.27.213:21116", "79.72.27.213", relay, "pubkey", `C:\test\token`, `C:\test\key`)
	config += "verification-method = 'use-permanent-password'\r\n"
	config += "approve-mode = 'password'\r\n"
	config += "allow-hide-cm = 'Y'\r\n"
	config += "enable-keyboard = 'Y'\r\n"
	config += "enable-clipboard = 'Y'\r\n"
	config += "enable-file-transfer = 'Y'\r\n"
	config += "enable-audio = 'Y'\r\n"
	config += "enable-remote-restart = 'Y'\r\n"

	for _, expected := range []string{
		"verification-method = 'use-permanent-password'",
		"approve-mode = 'password'",
		"allow-hide-cm = 'Y'",
		"enable-keyboard = 'Y'",
		"enable-clipboard = 'Y'",
		"enable-file-transfer = 'Y'",
	} {
		if !strings.Contains(config, expected) {
			t.Fatalf("missing unattended option %q", expected)
		}
	}
}
