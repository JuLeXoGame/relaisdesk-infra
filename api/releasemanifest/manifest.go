package releasemanifest

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"
)

const MaxManifestBytes = 1 << 20

type Artifact struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

type Manifest struct {
	Version     string     `json:"version"`
	PublishedAt string     `json:"published_at"`
	KeyID       string     `json:"key_id"`
	Artifacts   []Artifact `json:"artifacts"`
	Signature   string     `json:"signature"`
}

type signedPayload struct {
	Version     string     `json:"version"`
	PublishedAt string     `json:"published_at"`
	KeyID       string     `json:"key_id"`
	Artifacts   []Artifact `json:"artifacts"`
}

func (m Manifest) payload() ([]byte, error) {
	return json.Marshal(signedPayload{Version: m.Version, PublishedAt: m.PublishedAt, KeyID: m.KeyID, Artifacts: m.Artifacts})
}

func Sign(manifest *Manifest, privateKey ed25519.PrivateKey) error {
	if manifest == nil || len(privateKey) != ed25519.PrivateKeySize {
		return errors.New("manifest ou clé privée Ed25519 invalide")
	}
	sort.Slice(manifest.Artifacts, func(i, j int) bool { return manifest.Artifacts[i].Name < manifest.Artifacts[j].Name })
	if err := validateUnsigned(*manifest); err != nil {
		return err
	}
	payload, err := manifest.payload()
	if err != nil {
		return err
	}
	manifest.Signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(privateKey, payload))
	return nil
}

func Verify(manifest Manifest, encodedPublicKey, keyID string) error {
	if err := validateUnsigned(manifest); err != nil {
		return err
	}
	if strings.TrimSpace(manifest.KeyID) == "" || manifest.KeyID != keyID {
		return errors.New("identifiant de clé de version inattendu")
	}
	publicKey, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(encodedPublicKey))
	if err != nil || len(publicKey) != ed25519.PublicKeySize {
		return errors.New("clé publique de version invalide")
	}
	signature, err := base64.RawURLEncoding.DecodeString(manifest.Signature)
	if err != nil || len(signature) != ed25519.SignatureSize {
		return errors.New("signature de version invalide")
	}
	payload, err := manifest.payload()
	if err != nil {
		return err
	}
	if !ed25519.Verify(ed25519.PublicKey(publicKey), payload, signature) {
		return errors.New("signature de version refusée")
	}
	return nil
}

func LoadVerified(path, encodedPublicKey, keyID string) (*Manifest, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("ouverture du manifeste: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || info.Size() < 1 || info.Size() > MaxManifestBytes {
		return nil, errors.New("taille du manifeste invalide")
	}
	var manifest Manifest
	decoder := json.NewDecoder(io.LimitReader(file, MaxManifestBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return nil, fmt.Errorf("lecture du manifeste: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, errors.New("contenu supplémentaire dans le manifeste")
	}
	if err := Verify(manifest, encodedPublicKey, keyID); err != nil {
		return nil, err
	}
	return &manifest, nil
}

func (m Manifest) Artifact(name string) (Artifact, bool) {
	for _, artifact := range m.Artifacts {
		if artifact.Name == name {
			return artifact, true
		}
	}
	return Artifact{}, false
}

func VerifyArtifact(path string, artifact Artifact) error {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("artefact de version introuvable")
	}
	if info.Size() != artifact.Size {
		return errors.New("taille de l'artefact différente du manifeste")
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return err
	}
	if !strings.EqualFold(hex.EncodeToString(hash.Sum(nil)), artifact.SHA256) {
		return errors.New("empreinte de l'artefact différente du manifeste")
	}
	return nil
}

func validateUnsigned(manifest Manifest) error {
	if !validIdentifier(manifest.Version, ".-+") || !validIdentifier(manifest.KeyID, "-_") {
		return errors.New("version ou identifiant de clé invalide")
	}
	if _, err := time.Parse(time.RFC3339, manifest.PublishedAt); err != nil {
		return errors.New("date de publication invalide")
	}
	if len(manifest.Artifacts) == 0 || len(manifest.Artifacts) > 100 {
		return errors.New("liste d'artefacts invalide")
	}
	seen := make(map[string]bool, len(manifest.Artifacts))
	for _, artifact := range manifest.Artifacts {
		if artifact.Name == "" || artifact.Name != strings.TrimSpace(artifact.Name) || strings.ContainsAny(artifact.Name, `/\\`) || seen[artifact.Name] {
			return errors.New("nom d'artefact invalide ou dupliqué")
		}
		seen[artifact.Name] = true
		parsedURL, urlErr := url.Parse(artifact.URL)
		if urlErr != nil || parsedURL.Scheme != "https" || parsedURL.Host == "" || parsedURL.User != nil || parsedURL.Fragment != "" ||
			len(artifact.SHA256) != sha256.Size*2 || artifact.Size < 1 {
			return errors.New("métadonnées d'artefact invalides")
		}
		if _, err := hex.DecodeString(artifact.SHA256); err != nil {
			return errors.New("empreinte d'artefact invalide")
		}
	}
	return nil
}

func validIdentifier(value, punctuation string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 64 {
		return false
	}
	for _, char := range value {
		if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') &&
			(char < '0' || char > '9') && !strings.ContainsRune(punctuation, char) {
			return false
		}
	}
	return true
}
