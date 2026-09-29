package main

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	dbpkg "database"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWriteBackupManifest(t *testing.T) {
	dir := t.TempDir()
	oldPayload := bytes.Repeat([]byte("a"), 100)
	newPayload := bytes.Repeat([]byte("b"), 300)
	if err := os.WriteFile(filepath.Join(dir, "relaisdesk-20200101T000000Z.db.aesgcm"), oldPayload, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("ignored"), 0600); err != nil {
		t.Fatal(err)
	}
	latest := "relaisdesk-20260927T000000Z.db.aesgcm"
	if err := os.WriteFile(filepath.Join(dir, latest), newPayload, 0600); err != nil {
		t.Fatal(err)
	}
	latestInvoices := "relaisdesk-20260927T000000Z.invoices.aesgcm"
	if err := os.WriteFile(filepath.Join(dir, latestInvoices), newPayload, 0600); err != nil {
		t.Fatal(err)
	}
	if err := writeBackupManifest(dir, []string{latest, latestInvoices}, true, 30); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest backupManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("manifest illisible: %v\n%s", err, raw)
	}
	if manifest.RetentionDays != 30 || len(manifest.Backups) != 3 {
		t.Fatalf("manifest = %+v, want 3 backups and retention 30", manifest)
	}
	got := []string{manifest.Backups[0].File, manifest.Backups[1].File, manifest.Backups[2].File}
	want := []string{"relaisdesk-20200101T000000Z.db.aesgcm", latest, latestInvoices}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("backups = %v, want sorted %v", got, want)
		}
	}
	if !manifest.Backups[2].Verified || !manifest.Backups[2].Mirrored {
		t.Errorf("latest invoices archive must be verified+mirrored: %+v", manifest.Backups[2])
	}
	sum := sha256.Sum256(newPayload)
	if manifest.Backups[1].SHA256 != hex.EncodeToString(sum[:]) || manifest.Backups[1].Size != int64(len(newPayload)) {
		t.Errorf("latest digest/size = %s/%d", manifest.Backups[1].SHA256, manifest.Backups[1].Size)
	}
	if !manifest.Backups[1].Verified || !manifest.Backups[1].Mirrored {
		t.Errorf("latest must be verified+mirrored: %+v", manifest.Backups[1])
	}
	if manifest.Backups[0].Verified || manifest.Backups[0].Mirrored {
		t.Errorf("old backup must not be verified+mirrored: %+v", manifest.Backups[0])
	}
	info, err := os.Stat(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("manifest perms = %o, want 600", info.Mode().Perm())
	}
}

func TestBuildInvoicesTarball(t *testing.T) {
	src := t.TempDir()
	contents := map[string]string{
		"FAC-2026-0002.pdf":     "facture-deux",
		"FAC-2026-0001.pdf":     "facture-une",
		"archive/OLD-2025.html": "<html>vieille</html>",
	}
	for name, body := range contents {
		p := filepath.Join(src, name)
		if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	dest := filepath.Join(t.TempDir(), "invoices.tar")
	if err := buildInvoicesTarball(src, dest); err != nil {
		t.Fatal(err)
	}
	in, err := os.Open(dest)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	var names []string
	got := map[string]string{}
	reader := tar.NewReader(in)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, header.Name)
		if header.Typeflag == tar.TypeReg {
			body, err := io.ReadAll(reader)
			if err != nil {
				t.Fatal(err)
			}
			got[header.Name] = string(body)
		}
	}
	wantOrder := []string{"archive/", "FAC-2026-0001.pdf", "FAC-2026-0002.pdf", "archive/OLD-2025.html"}
	// Les dossiers d'abord (triés), puis les fichiers (triés).
	if len(names) != len(wantOrder) {
		t.Fatalf("entries = %v, want %v", names, wantOrder)
	}
	for i := range wantOrder {
		if names[i] != wantOrder[i] {
			t.Fatalf("entries = %v, want %v", names, wantOrder)
		}
	}
	for name, body := range contents {
		if got[name] != body {
			t.Errorf("content of %s = %q, want %q", name, got[name], body)
		}
	}
	if err := verifyTarArchive(dest); err != nil {
		t.Fatalf("verifyTarArchive: %v", err)
	}
	// Refus des liens symboliques et des fichiers spéciaux.
	if err := os.Symlink("FAC-2026-0001.pdf", filepath.Join(src, "lien.pdf")); err != nil {
		t.Fatal(err)
	}
	if err := buildInvoicesTarball(src, dest+"-lien.tar"); err == nil {
		t.Fatal("buildInvoicesTarball doit refuser les liens symboliques")
	}
	// Archive corrompue détectée : coupe franche au milieu d'une entrée
	// (ni sur une frontière de bloc ni dans la bande-annonce finale) et
	// octet d'en-tête altéré.
	raw, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) < 2048 {
		t.Fatalf("archive de test inattendue: %d octets", len(raw))
	}
	truncated := filepath.Join(t.TempDir(), "truncated.tar")
	if err := os.WriteFile(truncated, raw[:len(raw)-1500], 0600); err != nil {
		t.Fatal(err)
	}
	if err := verifyTarArchive(truncated); err == nil {
		t.Fatal("verifyTarArchive doit détecter la troncature")
	}
	damaged := append([]byte{}, raw...)
	damaged[100] ^= 0xff
	damagedPath := filepath.Join(t.TempDir(), "damaged.tar")
	if err := os.WriteFile(damagedPath, damaged, 0600); err != nil {
		t.Fatal(err)
	}
	if err := verifyTarArchive(damagedPath); err == nil {
		t.Fatal("verifyTarArchive doit détecter la corruption")
	}
}

func TestPurgeOldBackupsKeepsRecentPairs(t *testing.T) {
	dir := t.TempDir()
	old := time.Now().UTC().Add(-31 * 24 * time.Hour)
	oldDB := "relaisdesk-20200101T000000Z.db.aesgcm"
	oldInv := "relaisdesk-20200101T000000Z.invoices.aesgcm"
	newDB := "relaisdesk-20260927T000000Z.db.aesgcm"
	newInv := "relaisdesk-20260927T000000Z.invoices.aesgcm"
	for _, name := range []string{oldDB, oldInv, newDB, newInv, "manifest.json", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{oldDB, oldInv} {
		if err := os.Chtimes(filepath.Join(dir, name), old, old); err != nil {
			t.Fatal(err)
		}
	}
	if err := purgeOldBackups(dir, 30); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{oldDB, oldInv} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Errorf("%s aurait dû être purgé", name)
		}
	}
	for _, name := range []string{newDB, newInv, "manifest.json", "notes.txt"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("%s aurait dû être conservé: %v", name, err)
		}
	}
}

func TestCreateBacksUpDatabaseAndInvoices(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "licences.db")
	db, err := dbpkg.InitDatabase(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	invoicesDir := filepath.Join(dir, "invoices")
	if err := os.MkdirAll(invoicesDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(invoicesDir, "FAC-2026-0001.pdf"), []byte("%PDF-facture"), 0600); err != nil {
		t.Fatal(err)
	}
	keyFile := filepath.Join(dir, "backup.key")
	if err := dbpkg.GenerateBackupKeyFile(keyFile); err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(dir, "out")
	mirrorDir := filepath.Join(dir, "mirror")
	err = create([]string{
		"-db", dbPath, "-invoices-dir", invoicesDir, "-out-dir", outDir,
		"-key-file", keyFile, "-mirror-dir", mirrorDir, "-retention-days", "30",
	})
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(outDir)
	if err != nil {
		t.Fatal(err)
	}
	var dbArchive, invArchive string
	for _, entry := range entries {
		switch {
		case strings.HasSuffix(entry.Name(), ".db.aesgcm"):
			dbArchive = entry.Name()
		case strings.HasSuffix(entry.Name(), ".invoices.aesgcm"):
			invArchive = entry.Name()
		}
	}
	if dbArchive == "" || invArchive == "" {
		t.Fatalf("archives manquantes dans %s", outDir)
	}
	for _, archive := range []string{dbArchive, invArchive} {
		if _, err := os.Stat(filepath.Join(mirrorDir, archive)); err != nil {
			t.Errorf("miroir manquant pour %s: %v", archive, err)
		}
	}
	raw, err := os.ReadFile(filepath.Join(outDir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest backupManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	if len(manifest.Backups) != 2 {
		t.Fatalf("manifest = %+v, want 2 archives", manifest)
	}
	for _, entry := range manifest.Backups {
		if !entry.Verified || !entry.Mirrored {
			t.Errorf("%s doit être vérifiée+miroir: %+v", entry.File, entry)
		}
	}
	// Contenu des factures retrouvable après déchiffrement.
	key, err := dbpkg.LoadBackupKey(keyFile)
	if err != nil {
		t.Fatal(err)
	}
	restored := filepath.Join(dir, "restored.tar")
	if err := dbpkg.DecryptBackupFile(filepath.Join(outDir, invArchive), restored, key); err != nil {
		t.Fatal(err)
	}
	in, err := os.Open(restored)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	found := false
	reader := tar.NewReader(in)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if header.Name == "FAC-2026-0001.pdf" {
			body, _ := io.ReadAll(reader)
			if string(body) != "%PDF-facture" {
				t.Fatalf("contenu restauré = %q", body)
			}
			found = true
		}
	}
	if !found {
		t.Fatal("FAC-2026-0001.pdf introuvable après restauration")
	}
	// Dossier factures manquant : échec rapide, sans rien écrire.
	if err := create([]string{
		"-db", dbPath, "-invoices-dir", filepath.Join(dir, "absent"), "-out-dir", filepath.Join(dir, "out2"),
		"-key-file", keyFile, "-retention-days", "30",
	}); err == nil {
		t.Fatal("create doit échouer si les factures sont inaccessibles")
	}
}

func TestSealOpenRoundTrip(t *testing.T) {
	dir := t.TempDir()
	keyFile := filepath.Join(dir, "backup.key")
	if err := dbpkg.GenerateBackupKeyFile(keyFile); err != nil {
		t.Fatal(err)
	}
	payload := bytes.Repeat([]byte("facture-2026-"), 200000) // ~2.6 Mio : plusieurs blocs
	plain := filepath.Join(dir, "invoices.tar.gz")
	if err := os.WriteFile(plain, payload, 0600); err != nil {
		t.Fatal(err)
	}
	sealed := filepath.Join(dir, "relaisdesk-20260924T000000Z.invoices.aesgcm")
	if err := seal([]string{"-in", plain, "-out", sealed, "-key-file", keyFile}); err != nil {
		t.Fatal(err)
	}
	opened := filepath.Join(dir, "invoices-out.tar.gz")
	if err := openArchive([]string{"-file", sealed, "-out", opened, "-key-file", keyFile}); err != nil {
		t.Fatal(err)
	}
	restored, err := os.ReadFile(opened)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(restored, payload) {
		t.Fatal("l'aller-retour seal/open altère le contenu")
	}
}

func TestSealAndOpenRefuseOverwrite(t *testing.T) {
	dir := t.TempDir()
	keyFile := filepath.Join(dir, "backup.key")
	if err := dbpkg.GenerateBackupKeyFile(keyFile); err != nil {
		t.Fatal(err)
	}
	plain := filepath.Join(dir, "invoices.tar.gz")
	if err := os.WriteFile(plain, []byte("donnees"), 0600); err != nil {
		t.Fatal(err)
	}
	sealed := filepath.Join(dir, "out.aesgcm")
	if err := seal([]string{"-in", plain, "-out", sealed, "-key-file", keyFile}); err != nil {
		t.Fatal(err)
	}
	if err := seal([]string{"-in", plain, "-out", sealed, "-key-file", keyFile}); err == nil {
		t.Fatal("seal doit refuser l'écrasement")
	}
	opened := filepath.Join(dir, "out.tar.gz")
	if err := openArchive([]string{"-file", sealed, "-out", opened, "-key-file", keyFile}); err != nil {
		t.Fatal(err)
	}
	if err := openArchive([]string{"-file", sealed, "-out", opened, "-key-file", keyFile}); err == nil {
		t.Fatal("open doit refuser l'écrasement")
	}
}
