package main

import (
	dbpkg "database"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		fatal("usage: relaisdesk-backup <keygen|create|verify|restore>")
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
	outDir := set.String("out-dir", env("BACKUP_DIR", "/var/backups/relaisdesk"), "répertoire local")
	keyFile := set.String("key-file", env("BACKUP_KEY_FILE", "/etc/relaisdesk/backup.key"), "clé AES-256")
	mirrorDir := set.String("mirror-dir", env("BACKUP_MIRROR_DIR", ""), "répertoire hors site monté")
	retention := set.Int("retention-days", envInt("BACKUP_RETENTION_DAYS", 30), "rétention locale")
	if err := set.Parse(args); err != nil {
		return err
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
	defer os.Remove(snapshot)
	defer os.Remove(verifyPath)

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
	if strings.TrimSpace(*mirrorDir) != "" {
		if err := os.MkdirAll(*mirrorDir, 0700); err != nil {
			return err
		}
		if err := dbpkg.CopyBackupFile(encrypted, filepath.Join(*mirrorDir, filepath.Base(encrypted))); err != nil {
			return fmt.Errorf("copie hors site: %w", err)
		}
	}
	if err := purgeOldBackups(*outDir, *retention); err != nil {
		return err
	}
	fmt.Printf("Sauvegarde créée et restaurée à blanc avec succès: %s\n", encrypted)
	return nil
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
		if entry.IsDir() || !strings.HasPrefix(name, "relaisdesk-") || !strings.HasSuffix(name, ".db.aesgcm") {
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
