//go:build windows

package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGeneratedTomlMatchesExpected(t *testing.T) {
	rendezvousHost := "api.relaisdesk.fr"
	publicKey := "qdCKy9ILJQmM40BjSBn3s+7KMN+YR37CU7hWfYLmz74="
	rendezvousWithPort := rendezvousHost + ":21116"

	content := generateRustDesk2Toml(
		rendezvousWithPort,
		rendezvousHost,
		rendezvousHost,
		publicKey,
		`C:\Users\test\AppData\Roaming\RelaisDesk\authorization\technician-network-token`,
		`C:\Users\test\AppData\Roaming\RelaisDesk\authorization\technician-proof-key`,
	)

	expectedSnippets := []string{
		"rendezvous_server = 'api.relaisdesk.fr:21116'",
		"nat_type = 1",
		"serial = 0",
		"[options]",
		"custom-rendezvous-server = 'api.relaisdesk.fr'",
		"relay-server = 'api.relaisdesk.fr'",
		"api-server = 'https://api.relaisdesk.fr'",
		"key = 'qdCKy9ILJQmM40BjSBn3s+7KMN+YR37CU7hWfYLmz74='",
		"relaisdesk-token-file = 'C:\\\\Users\\\\test",
		"relaisdesk-proof-key-file = 'C:\\\\Users\\\\test",
	}

	for _, expected := range expectedSnippets {
		if !strings.Contains(content, expected) {
			t.Errorf("missing expected snippet in generated toml: %q\nFull content:\n%s", expected, content)
		}
	}

	// Interdictions absolues
	forbiddenSnippets := []string{
		"127.0.0.1",
		"localhost",
		"proxy-url",
		"proxy-username",
		"proxy-password",
		"disable-udp",
		"mpsk_",
	}

	for _, forbidden := range forbiddenSnippets {
		if strings.Contains(content, forbidden) {
			t.Errorf("forbidden snippet found in generated toml: %q\nFull content:\n%s", forbidden, content)
		}
	}
}

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

func TestCleanupCorruptedConfig(t *testing.T) {
	tempDir := t.TempDir()

	// RustDesk.toml valide avec key_confirmed = true et key_pair rempli doit être PRÉSERVÉ
	validToml := `enc_id = '00AbDIzvlm...'
password = ''
salt = 'zjimmbhxkf...'
key_pair = [
    [
    220, 246, 115, 158, 242, 50, 69, 200, 172, 110, 223, 184, 57, 140, 136, 73,
    ],
    [
    176, 55, 208, 151, 95, 35, 183, 96, 215, 19, 135, 70, 193, 160, 102, 70,
    ],
]
key_confirmed = true
`
	validPath := filepath.Join(tempDir, RUSTDESK_CONFIG_FILE)
	if err := os.WriteFile(validPath, []byte(validToml), 0644); err != nil {
		t.Fatalf("failed to write toml: %v", err)
	}

	cleanupCorruptedConfig(tempDir)

	if _, err := os.Stat(validPath); err != nil {
		t.Errorf("Valid RustDesk.toml should be preserved: %v", err)
	}

	// RustDesk.toml corrompu avec key_pair vide ou key_confirmed = false doit être SUPPRIMÉ
	corruptToml := `enc_id = '00AYpJ...'
password = ''
salt = 'j9mrbkey...'
key_pair = [
    [],
    [],
]
key_confirmed = false
`
	if err := os.WriteFile(validPath, []byte(corruptToml), 0644); err != nil {
		t.Fatalf("failed to write corrupt toml: %v", err)
	}

	cleanupCorruptedConfig(tempDir)

	if _, err := os.Stat(validPath); !os.IsNotExist(err) {
		t.Errorf("Corrupt RustDesk.toml with empty key_pair should have been deleted")
	}
}

func TestLaunchRustDeskSessionRejectsEmptyID(t *testing.T) {
	if err := launchRustDeskSession("dummy.exe", ""); err == nil {
		t.Fatal("expected error when launching session with empty targetID")
	}
	if err := launchRustDeskSession("dummy.exe", "   "); err == nil {
		t.Fatal("expected error when launching session with whitespace targetID")
	}
}

