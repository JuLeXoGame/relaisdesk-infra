//go:build linux

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLinuxFleetMarkerAcrossReadBoundary(t *testing.T) {
	payload := strings.Repeat("x", 32760) + linuxFleetProtocol + strings.Repeat("y", 200)
	if !linuxFleetContains(strings.NewReader(payload), []byte(linuxFleetProtocol)) {
		t.Fatal("split marker missed")
	}
	if linuxFleetContains(strings.NewReader("old fork"), []byte(linuxFleetProtocol)) {
		t.Fatal("old fork accepted")
	}
}

func TestLinuxFleetRefusesUnprivilegedPathsAndPeers(t *testing.T) {
	if err := checkLinuxFleetPath("relative/path"); err == nil {
		t.Fatal("relative path accepted")
	}
	if err := checkLinuxFleetPath("/tmp"); err == nil {
		t.Fatal("world-writable root path accepted")
	}
	if err := checkLinuxFleetPath("/proc/self/exe"); err == nil {
		t.Fatal("symlink accepted")
	}
	info, err := os.Stat("/proc/self/exe")
	if err != nil {
		t.Fatal(err)
	}
	if linuxFleetPeerAllowed(os.Getpid(), info) {
		t.Fatal("standalone non-service caller accepted")
	}
	if linuxFleetPeerAllowed(999999999, info) {
		t.Fatal("nonexistent caller accepted")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "example")
	if err = os.WriteFile(path, []byte("example"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = checkLinuxFleetPath(path); err == nil {
		t.Fatal("file under writable /tmp trusted")
	}
}
