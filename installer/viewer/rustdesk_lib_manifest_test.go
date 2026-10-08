//go:build linux

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The manifest contents themselves are verified against the embedded deb by
// TestViewerBundledLibManifest in installer/configurator
// (pin_consistency_test.go), which reuses the deb readers there.

func TestCheckRustDeskBundledLibs(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"libone.so", "libtwo.so"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := checkRustDeskBundledLibs(dir, []string{"libone.so", "libtwo.so"}); err != nil {
		t.Fatalf("all present, want nil: %v", err)
	}
	err := checkRustDeskBundledLibs(dir, []string{"libone.so", "libmissing.so"})
	if err == nil || !strings.Contains(err.Error(), "libmissing.so") {
		t.Fatalf("missing lib, want error naming libmissing.so: %v", err)
	}
}

func TestCheckRustDeskBundledLibsFailClosed(t *testing.T) {
	if err := checkRustDeskBundledLibs(t.TempDir(), nil); err == nil {
		t.Fatal("empty manifest, want fail-closed error")
	}
}
