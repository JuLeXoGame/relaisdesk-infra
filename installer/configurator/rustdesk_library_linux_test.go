//go:build linux

package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestRustDeskLibraryRequiresPinnedCurrentEngine(t *testing.T) {
	previous := RUSTDESK_SO_EXPECTED_SHA256
	t.Cleanup(func() { RUSTDESK_SO_EXPECTED_SHA256 = previous })
	path := filepath.Join(t.TempDir(), "librustdesk.so")
	if err := os.WriteFile(path, []byte("old engine"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, pin := range []string{"", "invalid", fmt.Sprintf("%x", sha256.Sum256([]byte("new engine")))} {
		RUSTDESK_SO_EXPECTED_SHA256 = pin
		if matchesRustDeskLibrary(path) {
			t.Fatal("accepted an unpinned or outdated native library")
		}
	}
	RUSTDESK_SO_EXPECTED_SHA256 = fmt.Sprintf("%x", sha256.Sum256([]byte("old engine")))
	if !matchesRustDeskLibrary(path) {
		t.Fatal("rejected the exactly pinned library")
	}
	if matchesRustDeskLibrary(filepath.Join(t.TempDir(), "missing.so")) {
		t.Fatal("accepted missing library")
	}
}
