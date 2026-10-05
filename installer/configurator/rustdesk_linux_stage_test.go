//go:build linux

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"testing"
)

func TestStageVerifiedDebRoundtrip(t *testing.T) {
	data := []byte("fake-deb-payload-for-hash-test")
	sum := sha256.Sum256(data)
	expected := hex.EncodeToString(sum[:])
	path, err := stageVerifiedDeb(data, expected, "relaisdesk-test-*.deb")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)
	onDisk, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(onDisk) != string(data) {
		t.Fatal("contenu staged différent des octets vérifiés")
	}
	if !fileMatchesSHA256(path, expected) {
		t.Fatal("relecture disque ne correspond pas à l'empreinte")
	}
}

func TestStageVerifiedDebRejectsMismatch(t *testing.T) {
	data := []byte("fake-deb-payload-for-hash-test")
	wrong := "0011c9e2d4b6a3f6f0e6b6a3f6f0e6b6a3f6f0e6b6a3f6f0e6b6a3f6f0e6b6a3"
	if _, err := stageVerifiedDeb(data, wrong, "relaisdesk-test-*.deb"); err == nil {
		t.Fatal("empreinte divergente acceptée")
	}
	if _, err := stageVerifiedDeb(data, "not-hex", "relaisdesk-test-*.deb"); err == nil {
		t.Fatal("empreinte malformée acceptée")
	}
	if _, err := stageVerifiedDeb(nil, wrong, "relaisdesk-test-*.deb"); err == nil {
		t.Fatal("paquet vide accepté")
	}
}
