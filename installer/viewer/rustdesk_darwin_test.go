//go:build darwin

package main

import (
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
