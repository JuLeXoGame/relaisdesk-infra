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
