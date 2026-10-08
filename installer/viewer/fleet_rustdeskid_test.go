package main

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// Regression: the fleet once enrolled a stale RustDesk.toml `id` on the
// first transient --get-id failure, registering a ghost the engine no longer
// held (reachable nowhere, offline forever). The live engine stays
// authoritative; the toml is a last resort, and heartbeats propagate later
// rotations.

func TestParseRustDeskTomlID(t *testing.T) {
	cases := []struct {
		name string
		toml string
		want string
	}{
		{"double quotes", "id = \"123456789\"\n", "123456789"},
		{"single quotes", "id = '987654321'\n", "987654321"},
		{"bare", "id = 555666777\n", "555666777"},
		{"no spaces", "id=\"111222333\"\n", "111222333"},
		{"tabs", "\tid\t=\t\"444555666\"\n", "444555666"},
		{"crlf", "id = \"777888999\"\r\n", "777888999"},
		{"among others", "key_confirmed = false\nid = \"123123123\"\npassword = 'x'\n", "123123123"},
		{"identity ignored", "identity = \"123456789\"\n", ""},
		{"myid ignored", "myid = \"123456789\"\n", ""},
		{"uppercase ignored", "ID = \"123456789\"\n", ""},
		{"empty", "id = \"\"\n", ""},
		{"non numeric", "id = abcdef\n", ""},
		{"too short", "id = \"123\"\n", ""},
		{"missing", "key_confirmed = false\n", ""},
		{"garbage", "\x00\x01\x02", ""},
		{"empty file", "", ""},
	}
	for _, tc := range cases {
		if got := parseRustDeskTomlID([]byte(tc.toml)); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestResolveEnrolledRustDeskIDPrefersLiveOverStored(t *testing.T) {
	getCalls, tomlReads, sleeps := 0, 0, 0
	getID := func() (string, error) {
		getCalls++
		if getCalls < 4 {
			return "", errors.New("transient")
		}
		return "1027287547\n", nil
	}
	got := resolveEnrolledRustDeskID(getID, func() string {
		tomlReads++
		return "1261345246"
	}, 20, func() { sleeps++ })
	if got != "1027287547" {
		t.Fatalf("got %q, want live 1027287547", got)
	}
	if getCalls != 4 {
		t.Fatalf("getID called %d times, want 4 (transients retried)", getCalls)
	}
	if tomlReads != 0 {
		t.Fatalf("stored toml read %d times, want 0 (live wins)", tomlReads)
	}
	if sleeps != 3 {
		t.Fatalf("sleeps %d, want 3", sleeps)
	}
}

func TestResolveEnrolledRustDeskIDFallsBackAfterExhaustion(t *testing.T) {
	getCalls, tomlReads, sleeps := 0, 0, 0
	got := resolveEnrolledRustDeskID(func() (string, error) {
		getCalls++
		return "", errors.New("down")
	}, func() string {
		tomlReads++
		return "1261345246"
	}, 3, func() { sleeps++ })
	if got != "1261345246" {
		t.Fatalf("got %q, want stored 1261345246", got)
	}
	if getCalls != 3 || tomlReads != 1 || sleeps != 2 {
		t.Fatalf("calls get=%d toml=%d sleeps=%d, want 3/1/2", getCalls, tomlReads, sleeps)
	}
}

func TestResolveEnrolledRustDeskIDFailClosed(t *testing.T) {
	fail := func() (string, error) { return "", errors.New("down") }
	if got := resolveEnrolledRustDeskID(fail, nil, 3, nil); got != "" {
		t.Fatalf("nil fallback: got %q, want empty", got)
	}
	if got := resolveEnrolledRustDeskID(fail, func() string { return "bogus" }, 3, nil); got != "" {
		t.Fatalf("invalid stored: got %q, want empty", got)
	}
	if got := resolveEnrolledRustDeskID(func() (string, error) { return "not-an-id\n", nil }, nil, 2, nil); got != "" {
		t.Fatalf("invalid live: got %q, want empty", got)
	}
}

func TestRustDeskIDDrifted(t *testing.T) {
	cases := []struct {
		enrolled, live string
		want           bool
	}{
		{"123456789", "123456789", false},
		{"1261345246", "1027287547", true},
		{"1261345246", "", false},
		{"", "1027287547", false},
		{"", "", false},
	}
	for _, tc := range cases {
		if got := rustDeskIDDrifted(tc.enrolled, tc.live); got != tc.want {
			t.Errorf("enrolled=%q live=%q: got %v, want %v", tc.enrolled, tc.live, got, tc.want)
		}
	}
}

func TestWriteFileIfChangedAtomically(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "RustDesk2.toml")

	wrote, err := writeFileIfChangedAtomically(path, []byte("a=1\n"), 0600)
	if err != nil || !wrote {
		t.Fatalf("create: wrote=%v err=%v", wrote, err)
	}
	if runtime.GOOS != "windows" {
		if mode := fileModePerm(t, path); mode != 0600 {
			t.Fatalf("perm=%o, want 600", mode)
		}
	}

	wrote, err = writeFileIfChangedAtomically(path, []byte("a=1\n"), 0600)
	if err != nil || wrote {
		t.Fatalf("identical: wrote=%v err=%v, want no write", wrote, err)
	}

	wrote, err = writeFileIfChangedAtomically(path, []byte("a=2\n"), 0600)
	if err != nil || !wrote {
		t.Fatalf("replace: wrote=%v err=%v", wrote, err)
	}
	if content, _ := os.ReadFile(path); string(content) != "a=2\n" {
		t.Fatalf("content=%q", content)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() != "RustDesk2.toml" {
			t.Fatalf("leftover temp file: %s", entry.Name())
		}
	}

	if _, err := writeFileIfChangedAtomically(filepath.Join(dir, "no-such-dir", "x.toml"), []byte("x"), 0600); err == nil {
		t.Fatal("missing dir accepted")
	}
}

func fileModePerm(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode().Perm()
}
