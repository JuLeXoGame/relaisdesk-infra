package database

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestConsistentEncryptedBackupRoundTrip(t *testing.T) {
	dir := t.TempDir()
	db, err := InitDatabase(filepath.Join(dir, "source.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CreateLicense(db, "backup@example.com", 30, 1, "backup-test"); err != nil {
		t.Fatal(err)
	}
	snapshot := filepath.Join(dir, "snapshot.db")
	if err := CreateConsistentSnapshot(db, snapshot); err != nil {
		t.Fatal(err)
	}
	if err := VerifySQLiteBackup(snapshot); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	key := bytes.Repeat([]byte{0x42}, 32)
	encrypted := filepath.Join(dir, "backup.db.aesgcm")
	if err := EncryptBackupFile(snapshot, encrypted, key); err != nil {
		t.Fatal(err)
	}
	restored := filepath.Join(dir, "restored.db")
	if err := DecryptBackupFile(encrypted, restored, key); err != nil {
		t.Fatal(err)
	}
	if err := VerifySQLiteBackup(restored); err != nil {
		t.Fatal(err)
	}
	if err := DecryptBackupFile(encrypted, filepath.Join(dir, "wrong.db"), bytes.Repeat([]byte{0x24}, 32)); err == nil {
		t.Fatal("a backup was decrypted with the wrong key")
	}
	raw, err := os.ReadFile(encrypted)
	if err != nil {
		t.Fatal(err)
	}
	truncated := filepath.Join(dir, "truncated.db.aesgcm")
	if err := os.WriteFile(truncated, raw[:len(raw)-1], 0600); err != nil {
		t.Fatal(err)
	}
	if err := DecryptBackupFile(truncated, filepath.Join(dir, "truncated.db"), key); err == nil {
		t.Fatal("a truncated backup was accepted")
	}
	withTrailingData := filepath.Join(dir, "trailing.db.aesgcm")
	if err := os.WriteFile(withTrailingData, append(raw, 0x42), 0600); err != nil {
		t.Fatal(err)
	}
	if err := DecryptBackupFile(withTrailingData, filepath.Join(dir, "trailing.db"), key); err == nil {
		t.Fatal("a backup with unauthenticated trailing data was accepted")
	}
	info, err := os.Stat(encrypted)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
		t.Fatalf("backup permissions=%v", info.Mode().Perm())
	}
}
