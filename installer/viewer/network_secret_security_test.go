package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestViewerSecretsStayPrivate(t *testing.T) {
	dir := t.TempDir()
	keyPath, tokenPath := filepath.Join(dir, "proof-key"), filepath.Join(dir, "network-token")
	key, err := loadOrCreateViewerProofKey(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(keyPath, 0644); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadOrCreateViewerProofKey(keyPath)
	if err != nil || !bytes.Equal(key, loaded) {
		t.Fatalf("identity changed during permission repair: %v", err)
	}
	for _, token := range []string{"rd1.first.signature", "rd1.second.signature"} {
		if err = writeViewerSecretAtomically(tokenPath, token); err != nil {
			t.Fatal(err)
		}
		for _, path := range []string{keyPath, tokenPath} {
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
				t.Fatalf("secret accessible to other users: %s mode %o", path, info.Mode().Perm())
			}
		}
	}
}
