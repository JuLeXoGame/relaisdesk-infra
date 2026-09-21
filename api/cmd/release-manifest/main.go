package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"api/releasemanifest"
)

func main() {
	downloads := flag.String("downloads", "", "répertoire contenant les artefacts")
	version := flag.String("version", "", "version publiée")
	baseURL := flag.String("base-url", "", "URL HTTPS de téléchargement")
	keyID := flag.String("key-id", "release-1", "identifiant public de la clé")
	privateKeyPath := flag.String("private-key", "", "fichier privé Ed25519 base64url")
	publicKeyExpected := flag.String("public-key", "", "clé publique attendue base64url")
	include := flag.String("include", "", "liste exacte de fichiers à inclure, séparés par des virgules")
	output := flag.String("out", "", "fichier JSON de sortie")
	flag.Parse()

	if *downloads == "" || *version == "" || *baseURL == "" || *privateKeyPath == "" || *publicKeyExpected == "" {
		log.Fatal("-downloads, -version, -base-url, -private-key et -public-key sont obligatoires")
	}
	parsedBase, err := url.Parse(strings.TrimRight(*baseURL, "/") + "/")
	if err != nil || parsedBase.Scheme != "https" || parsedBase.Host == "" || parsedBase.User != nil {
		log.Fatal("-base-url doit être une URL HTTPS publique")
	}
	privateKey := loadPrivateKey(*privateKeyPath)
	actualPublic := base64.RawURLEncoding.EncodeToString(privateKey.Public().(ed25519.PublicKey))
	if actualPublic != strings.TrimSpace(*publicKeyExpected) {
		log.Fatal("la clé publique fournie ne correspond pas à la clé privée")
	}
	includeNames, err := parseArtifactInclude(*include)
	if err != nil {
		log.Fatal(err)
	}

	entries, err := os.ReadDir(*downloads)
	if err != nil {
		log.Fatal(err)
	}
	artifacts := make([]releasemanifest.Artifact, 0, len(entries))
	found := make(map[string]struct{}, len(includeNames))
	for _, entry := range entries {
		if entry.IsDir() || !isReleaseArtifact(entry.Name(), includeNames) {
			continue
		}
		path := filepath.Join(*downloads, entry.Name())
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() || info.Size() < 1 {
			continue
		}
		hash, err := hashFile(path)
		if err != nil {
			log.Fatal(err)
		}
		artifacts = append(artifacts, releasemanifest.Artifact{
			Name: entry.Name(), URL: parsedBase.String() + url.PathEscape(entry.Name()), SHA256: hash, Size: info.Size(),
		})
		found[entry.Name()] = struct{}{}
	}
	if len(includeNames) > 0 && len(found) != len(includeNames) {
		missing := make([]string, 0, len(includeNames)-len(found))
		for name := range includeNames {
			if _, ok := found[name]; !ok {
				missing = append(missing, name)
			}
		}
		sort.Strings(missing)
		log.Fatalf("artefacts demandés absents ou invalides : %s", strings.Join(missing, ", "))
	}
	manifest := releasemanifest.Manifest{
		Version: *version, PublishedAt: time.Now().UTC().Format(time.RFC3339), KeyID: *keyID, Artifacts: artifacts,
	}
	if err := releasemanifest.Sign(&manifest, privateKey); err != nil {
		log.Fatal(err)
	}
	if err := releasemanifest.Verify(manifest, *publicKeyExpected); err != nil {
		log.Fatalf("auto-vérification du manifeste: %v", err)
	}
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	target := *output
	if target == "" {
		target = filepath.Join(*downloads, "release-manifest.json")
	}
	if err := writeAtomically(target, append(encoded, '\n')); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Manifeste signé: %s (%d artefacts, clé %s)\n", target, len(artifacts), *keyID)
}

func parseArtifactInclude(raw string) (map[string]struct{}, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	allowed := make(map[string]struct{})
	for _, value := range strings.Split(raw, ",") {
		name := strings.TrimSpace(value)
		if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\`) ||
			name == "release-manifest.json" || strings.HasPrefix(name, ".") {
			return nil, fmt.Errorf("nom d'artefact interdit dans -include : %q", name)
		}
		if _, duplicate := allowed[name]; duplicate {
			return nil, fmt.Errorf("nom d'artefact dupliqué dans -include : %q", name)
		}
		allowed[name] = struct{}{}
	}
	return allowed, nil
}

func isReleaseArtifact(name string, allowed map[string]struct{}) bool {
	if name == "release-manifest.json" || strings.HasPrefix(name, ".") {
		return false
	}
	if len(allowed) == 0 {
		return true
	}
	_, ok := allowed[name]
	return ok
}

func loadPrivateKey(path string) ed25519.PrivateKey {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 256 {
		log.Fatal("fichier de clé privée invalide")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		log.Fatal("le fichier de clé privée doit être en mode 0600")
	}
	encoded, err := os.ReadFile(path)
	if err != nil {
		log.Fatal(err)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(string(encoded)))
	if err != nil || len(decoded) != ed25519.PrivateKeySize {
		log.Fatal("clé privée Ed25519 invalide")
	}
	return ed25519.PrivateKey(decoded)
}

func hashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func writeAtomically(path string, content []byte) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, "release-manifest-*.json")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.Write(content); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Chmod(temporaryPath, 0o644); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return os.Rename(temporaryPath, path)
}
