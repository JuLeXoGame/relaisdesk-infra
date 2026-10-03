//go:build darwin

package main

import (
	"strings"
	"testing"
)

func TestDarwinTechnicianGeneratedToml(t *testing.T) {
	content := generateTechnicianRustDesk2Toml(
		"api.relaisdesk.fr:21116",
		"api.relaisdesk.fr",
		"api.relaisdesk.fr",
		"qdCKy9ILJQmM40BjSBn3s+7KMN+YR37CU7hWfYLmz74=",
		"/Users/test/Library/Application Support/RelaisDesk/tech-network-token",
		"/Users/test/Library/Application Support/RelaisDesk/tech-proof-key",
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

func TestTomlEscapeDarwin(t *testing.T) {
	cases := map[string]string{
		`plain`:          `plain`,
		`C:\Programmes`:  `C:\\Programmes`,
		`l'outil`:        `l\'outil`,
		`a\'b`:           `a\\\'b`,
		`key'with\both'`: `key\'with\\both\'`,
	}
	for input, expected := range cases {
		if got := tomlEscapeDarwin(input); got != expected {
			t.Errorf("tomlEscapeDarwin(%q) = %q, want %q", input, got, expected)
		}
	}
}

func TestDarwinLaunchRustDeskSessionRejectsEmptyID(t *testing.T) {
	if err := launchRustDeskSession("/usr/bin/false", ""); err == nil {
		t.Fatal("expected error when launching session with empty targetID")
	}
	if err := launchRustDeskSession("/usr/bin/false", "   "); err == nil {
		t.Fatal("expected error when launching session with whitespace targetID")
	}
}

func TestDarwinLaunchRustDeskSessionCmdRejectsEmptyID(t *testing.T) {
	if _, err := launchRustDeskSessionCmd("/usr/bin/false", ""); err == nil {
		t.Fatal("expected error from session cmd with empty targetID")
	}
	if _, err := launchRustDeskSessionCmd("/usr/bin/false", " \t "); err == nil {
		t.Fatal("expected error from session cmd with whitespace targetID")
	}
}
