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

// The permanent password must never transit on argv: it goes through a
// one-shot handoff file that is always removed, even when the core exec
// fails (no /usr/bin/rustdesk on this machine).
func TestSetFleetPermanentPasswordCleansHandoff(t *testing.T) {
	if _, err := os.Stat("/usr/bin/rustdesk"); err == nil {
		t.Skip("rustdesk core présent : ce test couvre le chemin d'échec")
	}
	before, err := filepath.Glob(filepath.Join(os.TempDir(), "relaisdesk-enroll-*"))
	if err != nil {
		t.Fatal(err)
	}
	if err := setFleetPermanentPassword("mot-de-passe-valide"); err == nil {
		t.Fatal("succès inattendu sans binaire rustdesk")
	}
	after, err := filepath.Glob(filepath.Join(os.TempDir(), "relaisdesk-enroll-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Fatalf("fichiers handoff restants: avant=%d après=%d", len(before), len(after))
	}
	if err := setFleetPermanentPassword("court"); err == nil {
		t.Fatal("mot de passe trop court accepté")
	}
}
