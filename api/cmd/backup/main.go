package main

import (
	"archive/tar"
	"crypto/sha256"
	dbpkg "database"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		fatal("usage: relaisdesk-backup <keygen|create|verify|restore|seal|open>")
	}
	var err error
	switch os.Args[1] {
	case "keygen":
		err = keygen(os.Args[2:])
	case "create":
		err = create(os.Args[2:])
	case "verify":
		err = verify(os.Args[2:])
	case "restore":
		err = restore(os.Args[2:])
	case "seal":
		err = seal(os.Args[2:])
	case "open":
		err = openArchive(os.Args[2:])
	default:
		err = fmt.Errorf("commande inconnue %q", os.Args[1])
	}
	if err != nil {
		fatal(err.Error())
	}
}

func keygen(args []string) error {
	set := flag.NewFlagSet("keygen", flag.ContinueOnError)
	out := set.String("out", env("BACKUP_KEY_FILE", "/etc/relaisdesk/backup.key"), "fichier de clé")
	if err := set.Parse(args); err != nil {
		return err
	}
	if err := dbpkg.GenerateBackupKeyFile(*out); err != nil {
		return err
	}
	fmt.Printf("Clé de sauvegarde créée dans %s. Conservez une copie hors ligne.\n", *out)
	return nil
}

func create(args []string) error {
	set := flag.NewFlagSet("create", flag.ContinueOnError)
	dbPath := set.String("db", env("BACKUP_DB_PATH", "/data/relaisdesk/licences.db"), "base SQLite")
	invoicesDir := set.String("invoices-dir", env("INVOICES_DIR", "/data/relaisdesk/invoices"), "dossier des factures (conservation fiscale)")
	outDir := set.String("out-dir", env("BACKUP_DIR", "/var/backups/relaisdesk"), "répertoire local")
	keyFile := set.String("key-file", env("BACKUP_KEY_FILE", "/etc/relaisdesk/backup.key"), "clé AES-256")
	mirrorDir := set.String("mirror-dir", env("BACKUP_MIRROR_DIR", ""), "répertoire hors site monté")
	retention := set.Int("retention-days", envInt("BACKUP_RETENTION_DAYS", 30), "rétention locale")
	if err := set.Parse(args); err != nil {
		return err
	}
	// Échec rapide si les factures sont inaccessibles : elles suivent une
	// obligation fiscale de dix ans et ne doivent pas être oubliées en silence.
	if info, err := os.Stat(*invoicesDir); err != nil || !info.IsDir() {
		return fmt.Errorf("dossier des factures inaccessible: %s", *invoicesDir)
	}
	key, err := dbpkg.LoadBackupKey(*keyFile)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(*outDir, 0700); err != nil {
		return err
	}
	stamp := time.Now().UTC().Format("20060102T150405Z")
	base := "relaisdesk-" + stamp + ".db"
	snapshot := filepath.Join(*outDir, "."+base+".tmp")
	verifyPath := filepath.Join(*outDir, "."+base+".verify.tmp")
	encrypted := filepath.Join(*outDir, base+".aesgcm")
	invoicesTar := filepath.Join(*outDir, ".relaisdesk-"+stamp+".invoices.tar.tmp")
	invoicesVerify := filepath.Join(*outDir, ".relaisdesk-"+stamp+".invoices.verify.tmp")
	invoicesEncrypted := filepath.Join(*outDir, "relaisdesk-"+stamp+".invoices.aesgcm")
	defer os.Remove(snapshot)
	defer os.Remove(verifyPath)
	defer os.Remove(invoicesTar)
	defer os.Remove(invoicesVerify)

	db, err := dbpkg.OpenBackupSource(*dbPath)
	if err != nil {
		return err
	}
	if err := dbpkg.CreateConsistentSnapshot(db, snapshot); err != nil {
		_ = db.Close()
		return err
	}
	if err := db.Close(); err != nil {
		return err
	}
	if err := dbpkg.VerifySQLiteBackup(snapshot); err != nil {
		return err
	}
	if err := dbpkg.EncryptBackupFile(snapshot, encrypted, key); err != nil {
		return err
	}
	if err := dbpkg.DecryptBackupFile(encrypted, verifyPath, key); err != nil {
		return err
	}
	if err := dbpkg.VerifySQLiteBackup(verifyPath); err != nil {
		return err
	}
	if err := buildInvoicesTarball(*invoicesDir, invoicesTar); err != nil {
		return err
	}
	if err := dbpkg.EncryptBackupFile(invoicesTar, invoicesEncrypted, key); err != nil {
		return err
	}
	if err := dbpkg.DecryptBackupFile(invoicesEncrypted, invoicesVerify, key); err != nil {
		return err
	}
	if err := verifyTarArchive(invoicesVerify); err != nil {
		return err
	}
	same, err := sameFileContent(invoicesTar, invoicesVerify)
	if err != nil {
		return err
	}
	if !same {
		return fmt.Errorf("contrôle à blanc : le déchiffrement des factures diffère de l'original")
	}
	if strings.TrimSpace(*mirrorDir) != "" {
		if err := os.MkdirAll(*mirrorDir, 0700); err != nil {
			return err
		}
		for _, archive := range []string{encrypted, invoicesEncrypted} {
			if err := dbpkg.CopyBackupFile(archive, filepath.Join(*mirrorDir, filepath.Base(archive))); err != nil {
				return fmt.Errorf("copie hors site: %w", err)
			}
		}
	}
	if err := purgeOldBackups(*outDir, *retention); err != nil {
		return err
	}
	latest := []string{filepath.Base(encrypted), filepath.Base(invoicesEncrypted)}
	if err := writeBackupManifest(*outDir, latest, strings.TrimSpace(*mirrorDir) != "", *retention); err != nil {
		return err
	}
	fmt.Printf("Sauvegarde créée et restaurée à blanc avec succès: %s\n", encrypted)
	fmt.Printf("Factures scellées et contrôlées à blanc: %s\n", invoicesEncrypted)
	return nil
}

// buildInvoicesTarball archives a directory into a deterministic tarball:
// entries triées, chemins relatifs, refus des liens et fichiers spéciaux.
func buildInvoicesTarball(srcDir, dest string) error {
	var files []string
	var dirs []string
	err := filepath.WalkDir(srcDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == srcDir {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() && !info.IsDir() {
			return fmt.Errorf("contenu inattendu dans les factures: %s", path)
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil || filepath.IsAbs(rel) || strings.HasPrefix(rel, "..") {
			return fmt.Errorf("chemin de facture invalide: %s", path)
		}
		if info.IsDir() {
			dirs = append(dirs, rel)
			return nil
		}
		files = append(files, rel)
		return nil
	})
	if err != nil {
		return err
	}
	slices.Sort(dirs)
	slices.Sort(files)
	out, err := os.OpenFile(filepath.Clean(dest), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		_ = out.Close()
		if !committed {
			_ = os.Remove(filepath.Clean(dest))
		}
	}()
	writer := tar.NewWriter(out)
	writeEntry := func(rel string, info os.FileInfo) error {
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(rel)
		header.Format = tar.FormatPAX
		if info.IsDir() && !strings.HasSuffix(header.Name, "/") {
			header.Name += "/"
		}
		if err := writer.WriteHeader(header); err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		in, err := os.Open(filepath.Join(srcDir, rel))
		if err != nil {
			return err
		}
		defer in.Close()
		_, err = io.Copy(writer, in)
		return err
	}
	for _, rel := range dirs {
		info, err := os.Stat(filepath.Join(srcDir, rel))
		if err != nil {
			return err
		}
		if err := writeEntry(rel, info); err != nil {
			return err
		}
	}
	for _, rel := range files {
		info, err := os.Stat(filepath.Join(srcDir, rel))
		if err != nil {
			return err
		}
		if err := writeEntry(rel, info); err != nil {
			return err
		}
	}
	if err := writer.Close(); err != nil {
		return err
	}
	if err := out.Sync(); err != nil {
		return err
	}
	committed = true
	return nil
}

// verifyTarArchive relit intégralement une archive. L'itération seule ne
// suffit pas : une coupe franche au milieu d'un bloc d'en-tête passe
// inaperçue, d'où le contrôle de taille et de bande-annonce. La suppression
// d'un suffixe entier (fichier final + bande-annonce) reste indétectable ici ;
// create() la couvre par comparaison à blanc avec l'original.
func verifyTarArchive(path string) error {
	info, err := os.Stat(filepath.Clean(path))
	if err != nil {
		return err
	}
	if info.Size() < 1024 || info.Size()%512 != 0 {
		return fmt.Errorf("archive des factures tronquée: taille %d", info.Size())
	}
	in, err := os.Open(filepath.Clean(path))
	if err != nil {
		return err
	}
	defer in.Close()
	trailer := make([]byte, 1024)
	if _, err := in.ReadAt(trailer, info.Size()-1024); err != nil {
		return fmt.Errorf("archive des factures illisible: %w", err)
	}
	for _, b := range trailer {
		if b != 0 {
			return fmt.Errorf("archive des factures tronquée: bande-annonce manquante")
		}
	}
	if _, err := in.Seek(0, io.SeekStart); err != nil {
		return err
	}
	reader := tar.NewReader(in)
	for {
		_, err := reader.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("archive des factures illisible: %w", err)
		}
		if _, err := io.Copy(io.Discard, reader); err != nil {
			return fmt.Errorf("archive des factures tronquée: %w", err)
		}
	}
}

// backupManifestEntry describes one retained encrypted backup for the admin
// supervision panel. Verified is true only for a backup whose round-trip
// decrypt and SQLite integrity check just succeeded.
type backupManifestEntry struct {
	File      string `json:"file"`
	Size      int64  `json:"size"`
	SHA256    string `json:"sha256"`
	CreatedAt string `json:"created_at"`
	Verified  bool   `json:"verified"`
	Mirrored  bool   `json:"mirrored"`
}

type backupManifest struct {
	GeneratedAt   string                `json:"generated_at"`
	RetentionDays int                   `json:"retention_days"`
	Backups       []backupManifestEntry `json:"backups"`
}

// isBackupArchive reconnaît les deux archives produites par create : base et factures.
func isBackupArchive(name string) bool {
	return strings.HasSuffix(name, ".db.aesgcm") || strings.HasSuffix(name, ".invoices.aesgcm")
}

func writeBackupManifest(outDir string, latest []string, mirrored bool, retention int) error {
	entries, err := os.ReadDir(outDir)
	if err != nil {
		return err
	}
	fresh := make(map[string]bool, len(latest))
	for _, name := range latest {
		fresh[name] = true
	}
	manifest := backupManifest{GeneratedAt: time.Now().UTC().Format(time.RFC3339), RetentionDays: retention}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasPrefix(name, "relaisdesk-") || !isBackupArchive(name) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		digest, err := sha256File(filepath.Join(outDir, name))
		if err != nil {
			return err
		}
		manifest.Backups = append(manifest.Backups, backupManifestEntry{
			File:      name,
			Size:      info.Size(),
			SHA256:    digest,
			CreatedAt: info.ModTime().UTC().Format(time.RFC3339),
			Verified:  fresh[name],
			Mirrored:  fresh[name] && mirrored,
		})
	}
	slices.SortFunc(manifest.Backups, func(a, b backupManifestEntry) int { return strings.Compare(a.File, b.File) })
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(outDir, ".manifest-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(append(encoded, '\n')); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, 0600); err != nil {
		return err
	}
	return os.Rename(tmpPath, filepath.Join(outDir, "manifest.json"))
}

func sha256File(path string) (string, error) {
	stream, err := os.Open(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	defer stream.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, stream); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func verify(args []string) error {
	set := flag.NewFlagSet("verify", flag.ContinueOnError)
	file := set.String("file", "", "sauvegarde chiffrée")
	keyFile := set.String("key-file", env("BACKUP_KEY_FILE", "/etc/relaisdesk/backup.key"), "clé AES-256")
	if err := set.Parse(args); err != nil {
		return err
	}
	if *file == "" {
		return fmt.Errorf("-file est obligatoire")
	}
	key, err := dbpkg.LoadBackupKey(*keyFile)
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp("", "relaisdesk-verify-*.db")
	if err != nil {
		return err
	}
	path := temp.Name()
	_ = temp.Close()
	_ = os.Remove(path)
	defer os.Remove(path)
	if err := dbpkg.DecryptBackupFile(*file, path, key); err != nil {
		return err
	}
	if strings.HasSuffix(*file, ".invoices.aesgcm") {
		if err := verifyTarArchive(path); err != nil {
			return err
		}
		fmt.Println("Sauvegarde authentique et archive des factures intègre.")
		return nil
	}
	if err := dbpkg.VerifySQLiteBackup(path); err != nil {
		return err
	}
	fmt.Println("Sauvegarde authentique et base SQLite intègre.")
	return nil
}

func restore(args []string) error {
	set := flag.NewFlagSet("restore", flag.ContinueOnError)
	file := set.String("file", "", "sauvegarde chiffrée")
	out := set.String("out", "", "nouveau fichier SQLite (ne doit pas exister)")
	keyFile := set.String("key-file", env("BACKUP_KEY_FILE", "/etc/relaisdesk/backup.key"), "clé AES-256")
	if err := set.Parse(args); err != nil {
		return err
	}
	if *file == "" || *out == "" {
		return fmt.Errorf("-file et -out sont obligatoires")
	}
	if _, err := os.Stat(*out); err == nil {
		return fmt.Errorf("le fichier de destination existe déjà; aucun écrasement n'est autorisé")
	} else if !os.IsNotExist(err) {
		return err
	}
	key, err := dbpkg.LoadBackupKey(*keyFile)
	if err != nil {
		return err
	}
	if err := dbpkg.DecryptBackupFile(*file, *out, key); err != nil {
		return err
	}
	if err := dbpkg.VerifySQLiteBackup(*out); err != nil {
		_ = os.Remove(*out)
		return err
	}
	fmt.Printf("Restauration vérifiée dans %s. Arrêtez l'API avant tout remplacement de la base active.\n", *out)
	return nil
}

// seal chiffre un fichier quelconque (ex. archive des factures) avec le
// même format vérifié que les sauvegardes de base. Aucun écrasement.
func seal(args []string) error {
	set := flag.NewFlagSet("seal", flag.ContinueOnError)
	in := set.String("in", "", "fichier à chiffrer")
	out := set.String("out", "", "archive chiffrée (ne doit pas exister)")
	keyFile := set.String("key-file", env("BACKUP_KEY_FILE", "/etc/relaisdesk/backup.key"), "clé AES-256")
	if err := set.Parse(args); err != nil {
		return err
	}
	if *in == "" || *out == "" {
		return fmt.Errorf("-in et -out sont obligatoires")
	}
	if _, err := os.Stat(*out); err == nil {
		return fmt.Errorf("le fichier de destination existe déjà; aucun écrasement n'est autorisé")
	} else if !os.IsNotExist(err) {
		return err
	}
	key, err := dbpkg.LoadBackupKey(*keyFile)
	if err != nil {
		return err
	}
	if err := dbpkg.EncryptBackupFile(*in, *out, key); err != nil {
		return err
	}
	temp, err := os.CreateTemp("", "relaisdesk-seal-*.tmp")
	if err != nil {
		return err
	}
	path := temp.Name()
	_ = temp.Close()
	_ = os.Remove(path)
	defer os.Remove(path)
	if err := dbpkg.DecryptBackupFile(*out, path, key); err != nil {
		return err
	}
	same, err := sameFileContent(*in, path)
	if err != nil {
		return err
	}
	if !same {
		return fmt.Errorf("contrôle à blanc : le déchiffrement diffère de l'original")
	}
	fmt.Printf("Fichier scellé et contrôlé à blanc: %s\n", *out)
	return nil
}

// openArchive déchiffre une archive scellée par seal. Aucun écrasement,
// aucun contrôle SQLite : le format du contenu reste à la charge de l'appelant.
func openArchive(args []string) error {
	set := flag.NewFlagSet("open", flag.ContinueOnError)
	file := set.String("file", "", "archive chiffrée")
	out := set.String("out", "", "fichier déchiffré (ne doit pas exister)")
	keyFile := set.String("key-file", env("BACKUP_KEY_FILE", "/etc/relaisdesk/backup.key"), "clé AES-256")
	if err := set.Parse(args); err != nil {
		return err
	}
	if *file == "" || *out == "" {
		return fmt.Errorf("-file et -out sont obligatoires")
	}
	if _, err := os.Stat(*out); err == nil {
		return fmt.Errorf("le fichier de destination existe déjà; aucun écrasement n'est autorisé")
	} else if !os.IsNotExist(err) {
		return err
	}
	key, err := dbpkg.LoadBackupKey(*keyFile)
	if err != nil {
		return err
	}
	if err := dbpkg.DecryptBackupFile(*file, *out, key); err != nil {
		return err
	}
	fmt.Printf("Archive ouverte dans %s.\n", *out)
	return nil
}

func sameFileContent(first, second string) (bool, error) {
	digest := func(path string) ([32]byte, error) {
		var empty [32]byte
		stream, err := os.Open(filepath.Clean(path))
		if err != nil {
			return empty, err
		}
		defer stream.Close()
		hasher := sha256.New()
		if _, err := io.Copy(hasher, stream); err != nil {
			return empty, err
		}
		var sum [32]byte
		copy(sum[:], hasher.Sum(nil))
		return sum, nil
	}
	a, err := digest(first)
	if err != nil {
		return false, err
	}
	b, err := digest(second)
	if err != nil {
		return false, err
	}
	return a == b, nil
}

func purgeOldBackups(directory string, days int) error {
	if days < 1 {
		return fmt.Errorf("la rétention doit être d'au moins un jour")
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}
	cutoff := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasPrefix(name, "relaisdesk-") || !isBackupArchive(name) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.ModTime().UTC().Before(cutoff) {
			target := filepath.Join(directory, name)
			rel, err := filepath.Rel(directory, target)
			if err != nil || filepath.IsAbs(rel) || strings.HasPrefix(rel, "..") {
				return fmt.Errorf("cible de rétention invalide")
			}
			if err := os.Remove(target); err != nil {
				return err
			}
		}
	}
	return nil
}

func env(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func envInt(name string, fallback int) int {
	var value int
	if _, err := fmt.Sscanf(env(name, ""), "%d", &value); err == nil && value > 0 {
		return value
	}
	return fallback
}

func fatal(message string) {
	fmt.Fprintln(os.Stderr, "Erreur:", message)
	os.Exit(1)
}
