package main

import (
	"os"
	"runtime"
	"strings"
	"testing"
)

func TestEnrollPasswordFileRoundtrip(t *testing.T) {
	path, err := writeEnrollPasswordFile("S3cret-Password!")
	if err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && st.Mode().Perm() != 0600 {
		t.Fatalf("permissions = %o, want 600", st.Mode().Perm())
	}
	got, err := readEnrollPasswordFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != "S3cret-Password!" {
		t.Fatalf("contenu = %q", got)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("fichier one-shot non supprimé après lecture")
	}
	// Second read must fail (one-shot).
	if _, err := readEnrollPasswordFile(path); err == nil {
		t.Fatal("relecture acceptée")
	}
}

func TestParseEnrollCLI(t *testing.T) {
	code, src, silent := parseEnrollCLI([]string{"park-abc", "@pwfile"})
	if code != "PARK-ABC" || src != "@pwfile" || silent {
		t.Fatalf("classique = %q %q %v", code, src, silent)
	}
	code, src, silent = parseEnrollCLI([]string{"--silent", "perm-x"})
	if code != "PERM-X" || src != "" || !silent {
		t.Fatalf("silent avant = %q %q %v", code, src, silent)
	}
	code, _, silent = parseEnrollCLI([]string{"perm-x", "/batch"})
	if code != "PERM-X" || !silent {
		t.Fatalf("batch après = %q %v", code, silent)
	}
	if c, _, s := parseEnrollCLI(nil); c != "" || s {
		t.Fatalf("vide = %q %v", c, s)
	}
	// One-shot @file code handoff is consumed and deleted.
	path, err := writeEnrollPasswordFile("park-handoff")
	if err != nil {
		t.Fatal(err)
	}
	code, _, _ = parseEnrollCLI([]string{"@" + path})
	if code != "PARK-HANDOFF" {
		t.Fatalf("handoff = %q", code)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("fichier one-shot non supprimé")
	}
	// Unreadable handoff yields an empty code (caller falls back to env).
	code, src, _ = parseEnrollCLI([]string{"@" + path, "@pw"})
	if code != "" || src != "@pw" {
		t.Fatalf("handoff illisible = %q %q", code, src)
	}
}

func TestReadEnrollPasswordFileRejectsInvalid(t *testing.T) {
	empty := t.TempDir() + "/empty"
	if err := os.WriteFile(empty, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readEnrollPasswordFile(empty); err == nil {
		t.Error("fichier vide accepté")
	}
	big := t.TempDir() + "/big"
	if err := os.WriteFile(big, []byte(strings.Repeat("x", 4097)), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readEnrollPasswordFile(big); err == nil {
		t.Error("fichier surdimensionné accepté")
	}
	if _, err := readEnrollPasswordFile(t.TempDir() + "/absent"); err == nil {
		t.Error("fichier absent accepté")
	}
}
