package main

import (
	"strings"
	"testing"
)

func TestReadSecretStdin(t *testing.T) {
	pass, err := readSecretStdin(strings.NewReader("s3cret-pass\n"))
	if err != nil {
		t.Fatal(err)
	}
	if pass != "s3cret-pass" {
		t.Fatalf("pass = %q", pass)
	}
	if _, err := readSecretStdin(strings.NewReader("  \n")); err == nil {
		t.Fatal("mot de passe vide accepté")
	}
}
