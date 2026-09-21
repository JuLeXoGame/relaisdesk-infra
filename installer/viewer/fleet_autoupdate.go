package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	ReleaseSigningPublicKey = "K3k6oko00jMzl7hN3poS6KYjJzZvjNz9Tgdz73E2duo"
	MaxManifestBytes        = 1 << 20
	MaxDownloadBytes        = 250 << 20 // 250 MB
)

type manifestArtifact struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

type releaseManifest struct {
	Version     string             `json:"version"`
	PublishedAt string             `json:"published_at"`
	KeyID       string             `json:"key_id"`
	Artifacts   []manifestArtifact `json:"artifacts"`
	Signature   string             `json:"signature"`
}

type manifestPayload struct {
	Version     string             `json:"version"`
	PublishedAt string             `json:"published_at"`
	KeyID       string             `json:"key_id"`
	Artifacts   []manifestArtifact `json:"artifacts"`
}

var (
	updateLock            sync.Mutex
	lastFailedUpdateMu    sync.Mutex
	lastFailedVersion     string
	lastFailedAttemptTime time.Time
)

func verifyReleaseManifest(m *releaseManifest, encodedPublicKey string) error {
	if m == nil {
		return errors.New("manifeste manquant")
	}
	if len(m.Artifacts) == 0 {
		return errors.New("aucun artefact dans le manifeste")
	}
	publicKey, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(encodedPublicKey))
	if err != nil || len(publicKey) != ed25519.PublicKeySize {
		return errors.New("clé publique de signature invalide")
	}
	signature, err := base64.RawURLEncoding.DecodeString(m.Signature)
	if err != nil || len(signature) != ed25519.SignatureSize {
		return errors.New("signature du manifeste invalide")
	}
	sortedArtifacts := make([]manifestArtifact, len(m.Artifacts))
	copy(sortedArtifacts, m.Artifacts)
	sort.Slice(sortedArtifacts, func(i, j int) bool { return sortedArtifacts[i].Name < sortedArtifacts[j].Name })

	payload, err := json.Marshal(manifestPayload{
		Version:     m.Version,
		PublishedAt: m.PublishedAt,
		KeyID:       m.KeyID,
		Artifacts:   sortedArtifacts,
	})
	if err != nil {
		return err
	}
	if !ed25519.Verify(ed25519.PublicKey(publicKey), payload, signature) {
		return errors.New("signature Ed25519 du manifeste rejetée")
	}
	return nil
}

func fetchVerifiedManifest(apiURL, encodedPublicKey string) (*releaseManifest, error) {
	endpoint := strings.TrimRight(apiURL, "/") + "/api/v1/public/releases/latest"
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("requête manifeste: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("manifeste indisponible (HTTP %d)", resp.StatusCode)
	}
	var m releaseManifest
	if err := json.NewDecoder(io.LimitReader(resp.Body, MaxManifestBytes)).Decode(&m); err != nil {
		return nil, fmt.Errorf("décodage du manifeste: %w", err)
	}
	if err := verifyReleaseManifest(&m, encodedPublicKey); err != nil {
		return nil, fmt.Errorf("vérification cryptographique: %w", err)
	}
	return &m, nil
}

func downloadAndVerifyArtifact(url, expectedSHA string, maxSize int64) ([]byte, error) {
	client := &http.Client{Timeout: 5 * time.Minute}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("téléchargement échoué: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("téléchargement refusé (HTTP %d)", resp.StatusCode)
	}

	hasher := sha256.New()
	var buf bytes.Buffer
	w := io.MultiWriter(&buf, hasher)
	written, err := io.Copy(w, io.LimitReader(resp.Body, maxSize+1))
	if err != nil {
		return nil, fmt.Errorf("erreur de lecture: %w", err)
	}
	if written > maxSize {
		return nil, fmt.Errorf("taille du téléchargement (%d octets) dépasse la limite", written)
	}
	calculatedSHA := hex.EncodeToString(hasher.Sum(nil))
	if !strings.EqualFold(calculatedSHA, strings.TrimSpace(expectedSHA)) {
		return nil, fmt.Errorf("somme de contrôle SHA256 non conforme: obtenu %s, attendu %s", calculatedSHA, expectedSHA)
	}
	return buf.Bytes(), nil
}

// extractBinaryFromDeb extracts /usr/bin/<binName> from a Debian package .deb archive in memory.
func extractBinaryFromDeb(debData []byte, binName string) ([]byte, error) {
	if !bytes.HasPrefix(debData, []byte("!<arch>\n")) {
		if bytes.HasPrefix(debData, []byte("\x7fELF")) {
			return debData, nil
		}
		return nil, errors.New("format d'archive non reconnu")
	}

	offset := 8
	var dataTarGz []byte
	for offset+60 <= len(debData) {
		hdr := debData[offset : offset+60]
		name := strings.TrimRight(string(hdr[0:16]), " /")
		sizeStr := strings.TrimSpace(string(hdr[48:58]))
		size, err := strconv.ParseInt(sizeStr, 10, 64)
		if err != nil || size < 0 {
			break
		}
		offset += 60
		if offset+int(size) > len(debData) {
			break
		}
		memberData := debData[offset : offset+int(size)]
		if name == "data.tar.gz" {
			dataTarGz = memberData
			break
		}
		offset += int(size)
		if size%2 != 0 {
			offset++
		}
	}

	if len(dataTarGz) == 0 {
		return nil, errors.New("data.tar.gz introuvable dans le paquet .deb")
	}

	gzr, err := gzip.NewReader(bytes.NewReader(dataTarGz))
	if err != nil {
		return nil, fmt.Errorf("lecture gzip data.tar: %w", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("lecture tar data: %w", err)
		}
		cleanName := strings.TrimPrefix(header.Name, ".")
		cleanName = strings.TrimPrefix(cleanName, "/")
		if cleanName == "usr/bin/"+binName || strings.HasSuffix(cleanName, "/"+binName) {
			extracted, err := io.ReadAll(io.LimitReader(tr, 100<<20))
			if err != nil {
				return nil, err
			}
			if !bytes.HasPrefix(extracted, []byte("\x7fELF")) {
				return nil, errors.New("le binaire extrait n'est pas un exécutable ELF valide")
			}
			return extracted, nil
		}
	}

	return nil, fmt.Errorf("binaire %s introuvable dans data.tar.gz", binName)
}

func checkAndApplyFleetUpdate(dir string, target *DeviceUpdateTarget) {
	if target == nil || target.Version == "" || target.Version == APP_VERSION {
		return
	}

	if !updateLock.TryLock() {
		return
	}
	defer updateLock.Unlock()

	lastFailedUpdateMu.Lock()
	if lastFailedVersion == target.Version && time.Since(lastFailedAttemptTime) < 15*time.Minute {
		lastFailedUpdateMu.Unlock()
		return
	}
	lastFailedUpdateMu.Unlock()

	recordFailure := func(v string) {
		lastFailedUpdateMu.Lock()
		lastFailedVersion = v
		lastFailedAttemptTime = time.Now()
		lastFailedUpdateMu.Unlock()
	}

	log.Printf("Parc : mise à jour distante détectée vers la version %s", target.Version)

	manifest, err := fetchVerifiedManifest(APIURL, ReleaseSigningPublicKey)
	if err != nil {
		log.Printf("Parc : échec de récupération ou vérification du manifeste de version: %v", err)
		recordFailure(target.Version)
		return
	}

	if err := validateFleetUpdateVersion(APP_VERSION, target.Version, manifest.Version); err != nil {
		log.Printf("Parc : mise à jour refusée : %v", err)
		recordFailure(target.Version)
		return
	}

	var targetArtifactName string
	switch runtime.GOOS {
	case "windows":
		targetArtifactName = "RelaisDesk_Portable.exe"
	case "linux":
		targetArtifactName = "RelaisDesk_viewer.deb"
	default:
		log.Printf("Parc : mise à jour automatique non prise en charge sur l'OS %s", runtime.GOOS)
		recordFailure(target.Version)
		return
	}

	var matchedArtifact *manifestArtifact
	for _, a := range manifest.Artifacts {
		if strings.EqualFold(a.Name, targetArtifactName) {
			matchedArtifact = &a
			break
		}
	}
	if matchedArtifact == nil {
		log.Printf("Parc : aucun artefact '%s' dans le manifeste de version %s", targetArtifactName, manifest.Version)
		recordFailure(target.Version)
		return
	}

	log.Printf("Parc : téléchargement de %s (%s)...", matchedArtifact.Name, matchedArtifact.URL)
	rawBytes, err := downloadAndVerifyArtifact(matchedArtifact.URL, matchedArtifact.SHA256, MaxDownloadBytes)
	if err != nil {
		log.Printf("Parc : échec du téléchargement ou contrôle d'intégrité SHA256: %v", err)
		recordFailure(target.Version)
		return
	}

	var finalBinary []byte
	switch runtime.GOOS {
	case "windows":
		if !bytes.HasPrefix(rawBytes, []byte("MZ")) {
			log.Print("Parc : le fichier téléchargé n'est pas un exécutable Windows valide (en-tête MZ manquant)")
			recordFailure(target.Version)
			return
		}
		finalBinary = rawBytes
	case "linux":
		extracted, err := extractBinaryFromDeb(rawBytes, "relaisdesk-viewer")
		if err != nil {
			log.Printf("Parc : extraction du binaire Linux depuis le paquet: %v", err)
			recordFailure(target.Version)
			return
		}
		finalBinary = extracted
	}

	newBinaryPath := filepath.Join(dir, "viewer-agent.new")
	file, err := os.CreateTemp(dir, ".update-*.tmp")
	if err != nil {
		recordFailure(target.Version)
		return
	}
	tmpPath := file.Name()
	defer os.Remove(tmpPath)
	err = file.Chmod(0700)
	if err == nil {
		_, err = file.Write(finalBinary)
	}
	err = errors.Join(err, file.Sync(), file.Close())
	if err != nil {
		log.Printf("Parc : écriture du binaire temporaire impossible: %v", err)
		recordFailure(target.Version)
		return
	}
	_ = os.Remove(newBinaryPath)
	if err := os.Rename(tmpPath, newBinaryPath); err != nil {
		_ = os.Remove(tmpPath)
		log.Printf("Parc : renommage du binaire mis à jour impossible: %v", err)
		recordFailure(target.Version)
		return
	}

	log.Printf("Parc : binaire version %s prêt, application du remplacement et redémarrage du service...", target.Version)
	if err := applyServiceUpdate(dir, newBinaryPath); err != nil {
		log.Printf("Parc : échec de l'application de la mise à jour: %v", err)
		recordFailure(target.Version)
		return
	}
}

// A signature authenticates an old release too: reject replayed downgrades and
// mismatches before downloading or executing anything as the system service.
func validateFleetUpdateVersion(current, requested, published string) error {
	if published != requested {
		return errors.New("version publiée différente de la version demandée")
	}
	parse := func(value string) ([3]uint64, error) {
		var version [3]uint64
		parts := strings.Split(strings.TrimPrefix(value, "v"), ".")
		if len(parts) != 3 {
			return version, errors.New("version invalide")
		}
		for i, part := range parts {
			if part == "" {
				return version, errors.New("version invalide")
			}
			for _, c := range part {
				if c < '0' || c > '9' {
					return version, errors.New("version invalide")
				}
			}
			n, err := strconv.ParseUint(part, 10, 32)
			if err != nil {
				return version, errors.New("version invalide")
			}
			version[i] = n
		}
		return version, nil
	}
	old, err := parse(current)
	if err != nil {
		return err
	}
	next, err := parse(published)
	if err != nil {
		return err
	}
	for i := range old {
		if next[i] > old[i] {
			return nil
		}
		if next[i] < old[i] {
			break
		}
	}
	return errors.New("retour vers une version ancienne ou identique interdit")
}
