package main

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"runtime"
	"strconv"
	"strings"
	"time"
)

var (
	APP_VERSION        = "1.0.0"
	RELEASE_PUBLIC_KEY = "K3k6oko00jMzl7hN3poS6KYjJzZvjNz9Tgdz73E2duo"
)

type releaseArtifact struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

type releaseManifest struct {
	Version     string            `json:"version"`
	PublishedAt string            `json:"published_at"`
	KeyID       string            `json:"key_id"`
	Artifacts   []releaseArtifact `json:"artifacts"`
	Signature   string            `json:"signature"`
}

type releasePayload struct {
	Version     string            `json:"version"`
	PublishedAt string            `json:"published_at"`
	KeyID       string            `json:"key_id"`
	Artifacts   []releaseArtifact `json:"artifacts"`
}

type UpdateInfo struct {
	CurrentVersion string
	LatestVersion  string
	Available      bool
	DownloadURL    string
}

func checkForUpdate(ctx context.Context, apiBase string) (*UpdateInfo, error) {
	if len(strings.TrimSpace(RELEASE_PUBLIC_KEY)) == 0 {
		return nil, errors.New("clé publique des mises à jour absente de cette compilation")
	}
	base, err := url.Parse(strings.TrimSpace(apiBase))
	if err != nil || (base.Scheme != "https" && base.Scheme != "http") || base.Host == "" || base.User != nil {
		return nil, errors.New("adresse API invalide")
	}
	base.Path = strings.TrimRight(base.Path, "/") + "/api/v1/public/releases/latest"
	base.RawQuery = ""
	base.Fragment = ""
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", PRODUCT_NAME+"-Updater/"+APP_VERSION)
	resp, err := (&http.Client{Timeout: 8 * time.Second}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("canal de mise à jour inaccessible: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("canal de mise à jour indisponible (HTTP %d)", resp.StatusCode)
	}
	var manifest releaseManifest
	decoder := json.NewDecoder(io.LimitReader(resp.Body, (1<<20)+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return nil, errors.New("manifeste de mise à jour invalide")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, errors.New("manifeste de mise à jour ambigu")
	}
	if err := verifyReleaseManifest(manifest); err != nil {
		return nil, err
	}

	info := &UpdateInfo{CurrentVersion: APP_VERSION, LatestVersion: manifest.Version}
	comparison, err := compareVersions(manifest.Version, APP_VERSION)
	if err != nil {
		return nil, err
	}
	info.Available = comparison > 0
	targetName := "RelaisDesk_Technicien_Portable.exe"
	if runtime.GOOS == "linux" {
		targetName = "RelaisDesk_Technicien.deb"
	}
	for _, artifact := range manifest.Artifacts {
		if artifact.Name == targetName {
			info.DownloadURL = artifact.URL
			break
		}
	}
	if info.Available && info.DownloadURL == "" {
		return nil, errors.New("mise à jour disponible mais artefact technicien absent")
	}
	return info, nil
}

func verifyReleaseManifest(manifest releaseManifest) error {
	publicKey, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(RELEASE_PUBLIC_KEY))
	if err != nil || len(publicKey) != ed25519.PublicKeySize {
		return errors.New("clé publique des mises à jour invalide")
	}
	signature, err := base64.RawURLEncoding.DecodeString(manifest.Signature)
	if err != nil || len(signature) != ed25519.SignatureSize {
		return errors.New("signature de mise à jour invalide")
	}
	if _, err := time.Parse(time.RFC3339, manifest.PublishedAt); err != nil || len(manifest.Artifacts) == 0 {
		return errors.New("métadonnées de mise à jour invalides")
	}
	for _, artifact := range manifest.Artifacts {
		parsedURL, urlErr := url.Parse(artifact.URL)
		_, hashErr := hex.DecodeString(artifact.SHA256)
		if artifact.Name == "" || strings.ContainsAny(artifact.Name, `/\\`) || urlErr != nil || parsedURL.Scheme != "https" || parsedURL.Host == "" || parsedURL.User != nil || parsedURL.Fragment != "" ||
			len(artifact.SHA256) != 64 || hashErr != nil || artifact.Size < 1 {
			return errors.New("artefact de mise à jour invalide")
		}
	}
	payload, err := json.Marshal(releasePayload{
		Version: manifest.Version, PublishedAt: manifest.PublishedAt, KeyID: manifest.KeyID, Artifacts: manifest.Artifacts,
	})
	if err != nil || !ed25519.Verify(ed25519.PublicKey(publicKey), payload, signature) {
		return errors.New("signature de mise à jour refusée")
	}
	return nil
}

func compareVersions(left, right string) (int, error) {
	parse := func(value string) ([3]int, error) {
		var parsed [3]int
		parts := strings.Split(strings.TrimSpace(strings.TrimPrefix(value, "v")), ".")
		if len(parts) != 3 {
			return parsed, errors.New("format de version invalide")
		}
		for index, part := range parts {
			number, err := strconv.Atoi(part)
			if err != nil || number < 0 {
				return parsed, errors.New("format de version invalide")
			}
			parsed[index] = number
		}
		return parsed, nil
	}
	leftVersion, err := parse(left)
	if err != nil {
		return 0, err
	}
	rightVersion, err := parse(right)
	if err != nil {
		return 0, err
	}
	for index := range leftVersion {
		if leftVersion[index] > rightVersion[index] {
			return 1, nil
		}
		if leftVersion[index] < rightVersion[index] {
			return -1, nil
		}
	}
	return 0, nil
}

// IsDeviceUpdateAvailable détermine si un poste distant doit être mis à jour.
// IsDeviceUpdateAvailable détermine si un poste distant doit être mis à jour.
// Si sa version actuelle est vide ou invalide, aucune mise à jour n'est signalée (false).
// Si targetVersion est omis, la version courante de référence (APP_VERSION) est utilisée.
// Renvoie true uniquement si la version actuelle est connue et que la version cible est strictement supérieure.
func IsDeviceUpdateAvailable(currentVersion string, targetVersion ...string) bool {
	cur := strings.TrimSpace(currentVersion)
	if cur == "" {
		return false
	}
	target := APP_VERSION
	if len(targetVersion) > 0 && strings.TrimSpace(targetVersion[0]) != "" {
		target = strings.TrimSpace(targetVersion[0])
	}
	cmp, err := compareVersions(target, cur)
	if err != nil {
		return false
	}
	return cmp > 0
}

