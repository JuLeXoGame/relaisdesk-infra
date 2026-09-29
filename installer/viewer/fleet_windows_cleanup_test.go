//go:build windows

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// The automatic replacement relies on removeFleetStateFiles always clearing
// our own enrollment, even when the engine cleanup later reports a problem.
func TestRemoveFleetStateFiles(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "fleet-state")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"network-token", "proof-key", "state.json", "viewer-agent.exe", "ready"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := removeFleetStateFiles(dir); err != nil {
		t.Fatalf("removeFleetStateFiles: %v", err)
	}
	if _, err := os.Lstat(dir); !os.IsNotExist(err) {
		t.Fatalf("state dir still present: %v", err)
	}
}

func TestRemoveFleetStateFilesIdempotent(t *testing.T) {
	if err := removeFleetStateFiles(filepath.Join(t.TempDir(), "missing")); err != nil {
		t.Fatalf("removeFleetStateFiles on missing dir: %v", err)
	}
}
