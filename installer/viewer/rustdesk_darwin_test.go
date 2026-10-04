//go:build darwin

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDarwinGeneratedToml(t *testing.T) {
	content := generateViewerRustDesk2Toml(
		"api.relaisdesk.fr:21116",
		"api.relaisdesk.fr",
		"api.relaisdesk.fr",
		"qdCKy9ILJQmM40BjSBn3s+7KMN+YR37CU7hWfYLmz74=",
		"/Users/test/Library/Application Support/RelaisDesk/network-token",
		"/Users/test/Library/Application Support/RelaisDesk/proof-key",
	)

	expected := []string{
		"rendezvous_server = 'api.relaisdesk.fr:21116'",
		"nat_type = 1",
		"serial = 0",
		"[options]",
		"custom-rendezvous-server = 'api.relaisdesk.fr'",
		"relay-server = 'api.relaisdesk.fr'",
		"key = 'qdCKy9ILJQmM40BjSBn3s+7KMN+YR37CU7hWfYLmz74='",
		"relaisdesk-token-file = '/Users/test/Library",
		"relaisdesk-proof-key-file = '/Users/test/Library",
	}
	for _, value := range expected {
		if !strings.Contains(content, value) {
			t.Fatalf("missing %q in generated config:\n%s", value, content)
		}
	}
}

func TestDarwinParseNumericRustDeskID(t *testing.T) {
	out := "1248626563\n"
	if id := parseNumericRustDeskID(out); id != "1248626563" {
		t.Fatalf("expected 1248626563, got %q", id)
	}

	invalid := "error: no connection\n"
	if id := parseNumericRustDeskID(invalid); id != "" {
		t.Fatalf("expected empty for invalid output, got %q", id)
	}
}

func TestDarwinCandidateUsable(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "rustdesk")
	payload := []byte("fake-macos-binary")
	if err := os.WriteFile(good, payload, 0600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(payload)
	pin := hex.EncodeToString(sum[:])
	cases := []struct {
		name       string
		path, sha  string
		exists     bool
		override   bool
		want       bool
	}{
		{"matching pin wins anywhere", good, pin, true, false, true},
		{"wrong pin rejects", good, strings.Repeat("0", 64), true, false, false},
		{"missing file rejects", good, pin, false, false, false},
		{"/Applications allowed unpinned", "/Applications/RelaisDesk.app/Contents/MacOS/RelaisDesk", "", true, false, true},
		{"user path rejected unpinned", "/Users/moi/rustdesk", "", true, false, false},
		{"user path allowed with override", "/Users/moi/rustdesk", "", true, true, true},
		{"sibling of Applications rejected", "/ApplicationsX/rustdesk", "", true, false, false},
	}
	for _, tc := range cases {
		if got := darwinCandidateUsable(tc.path, tc.sha, tc.exists, tc.override); got != tc.want {
			t.Errorf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
}
