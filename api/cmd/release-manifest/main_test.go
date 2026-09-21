package main

import "testing"

func TestIsReleaseArtifact(t *testing.T) {
	allowed := map[string]struct{}{
		"RelaisDesk_Setup.exe":      {},
		"RelaisDesk_Technicien.deb": {},
		"SHA256SUMS.txt":            {},
	}
	tests := []struct {
		name string
		want bool
	}{
		{name: "RelaisDesk_Setup.exe", want: true},
		{name: "RelaisDesk_Technicien.deb", want: true},
		{name: "SHA256SUMS.txt", want: true},
		{name: "stale-rustdesk.exe", want: false},
		{name: "release-manifest.json", want: false},
		{name: ".htaccess", want: false},
		{name: ".env", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isReleaseArtifact(tt.name, allowed); got != tt.want {
				t.Fatalf("isReleaseArtifact(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestParseArtifactInclude(t *testing.T) {
	allowed, err := parseArtifactInclude("RelaisDesk_Setup.exe, SHA256SUMS.txt")
	if err != nil {
		t.Fatal(err)
	}
	if len(allowed) != 2 {
		t.Fatalf("len(allowed) = %d, want 2", len(allowed))
	}
	for _, invalid := range []string{".env", "../secret", `dir\secret`, "a.exe,a.exe", "a.exe,"} {
		if _, err := parseArtifactInclude(invalid); err == nil {
			t.Fatalf("parseArtifactInclude(%q) succeeded, want error", invalid)
		}
	}
}
