package networkauth

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"
)

const (
	Issuer          = "relaisdesk-api"
	Audience        = "rustdesk-network"
	TokenPrefix     = "rd1"
	MinTTL          = 60 * time.Second
	MaxTTL          = 15 * time.Minute
	maxKeyFileBytes = 256
)

type Signer struct {
	privateKey ed25519.PrivateKey
	keyID      string
	ttl        time.Duration
	now        func() time.Time
}

type Claims struct {
	Issuer          string   `json:"iss"`
	Audience        string   `json:"aud"`
	Subject         string   `json:"sub"`
	Tenant          string   `json:"tenant"`
	Role            string   `json:"role"`
	TokenID         string   `json:"jti"`
	DevicePublicKey string   `json:"device_public_key"`
	KeyID           string   `json:"kid"`
	IssuedAt        int64    `json:"iat"`
	NotBefore       int64    `json:"nbf"`
	ExpiresAt       int64    `json:"exp"`
	MaxSessions     int      `json:"max_sessions"`
	FolderAccess    []string `json:"folder_access,omitempty"`
	FolderPath      []string `json:"folder_path,omitempty"`
	VerificationKey string   `json:"verification_key"`
	PeerAuthVersion int      `json:"peer_auth_version,omitempty"`
}

type IssueRequest struct {
	Subject         string
	Tenant          string
	Role            string
	DevicePublicKey string
	MaxSessions     int
	EntitlementEnds time.Time
	FolderAccess    []string
	FolderPath      []string
	PeerAuthVersion int
}

type IssuedToken struct {
	Token     string
	ExpiresAt time.Time
	KeyID     string
}

func LoadSigner(path, keyID string, ttl time.Duration) (*Signer, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("NETWORK_AUTH_PRIVATE_KEY_FILE est obligatoire")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("lecture de la clé privée d'autorisation: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() > maxKeyFileBytes {
		return nil, errors.New("fichier de clé privée d'autorisation invalide")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return nil, errors.New("la clé privée d'autorisation doit être accessible uniquement par son propriétaire (mode 0600)")
	}
	encoded, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("lecture de la clé privée d'autorisation: %w", err)
	}
	privateKey, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(string(encoded)))
	if err != nil || len(privateKey) != ed25519.PrivateKeySize {
		return nil, errors.New("clé privée d'autorisation Ed25519 invalide")
	}
	return NewSigner(ed25519.PrivateKey(privateKey), keyID, ttl)
}

func NewSigner(privateKey ed25519.PrivateKey, keyID string, ttl time.Duration) (*Signer, error) {
	if len(privateKey) != ed25519.PrivateKeySize {
		return nil, errors.New("clé privée Ed25519 invalide")
	}
	keyID = strings.TrimSpace(keyID)
	if !validIdentifier(keyID, 64) {
		return nil, errors.New("NETWORK_AUTH_KEY_ID invalide")
	}
	if ttl < MinTTL || ttl > MaxTTL {
		return nil, fmt.Errorf("durée des jetons hors limites (%s-%s)", MinTTL, MaxTTL)
	}
	keyCopy := append(ed25519.PrivateKey(nil), privateKey...)
	return &Signer{privateKey: keyCopy, keyID: keyID, ttl: ttl, now: time.Now}, nil
}

func (s *Signer) Issue(req IssueRequest) (*IssuedToken, error) {
	if s == nil {
		return nil, errors.New("autorité de jetons indisponible")
	}
	if !validValue(req.Subject, 128) || !validValue(req.Tenant, 128) {
		return nil, errors.New("sujet ou locataire invalide")
	}
	if req.Role != "technician" && req.Role != "viewer" && req.Role != "folder_technician" {
		return nil, errors.New("rôle d'autorisation invalide")
	}
	if len(req.FolderAccess) > 100 || len(req.FolderPath) > 32 || (req.Role != "folder_technician" && len(req.FolderAccess) > 0) || (req.Role != "viewer" && len(req.FolderPath) > 0) {
		return nil, errors.New("portée de dossiers invalide")
	}
	for _, list := range [][]string{req.FolderAccess, req.FolderPath} {
		for _, id := range list {
			if !validIdentifier(id, 32) || !strings.HasPrefix(id, "FLD-") {
				return nil, errors.New("identifiant de dossier invalide")
			}
		}
	}
	devicePublicKey, err := NormalizeDevicePublicKey(req.DevicePublicKey)
	if err != nil {
		return nil, err
	}
	if req.MaxSessions < 1 || req.MaxSessions > 10_000 {
		return nil, errors.New("limite de sessions invalide")
	}

	now := s.now().UTC().Truncate(time.Second)
	expiresAt := now.Add(s.ttl)
	if !req.EntitlementEnds.IsZero() && req.EntitlementEnds.UTC().Before(expiresAt) {
		expiresAt = req.EntitlementEnds.UTC().Truncate(time.Second)
	}
	if !expiresAt.After(now) {
		return nil, errors.New("droit d'accès expiré")
	}
	tokenID, err := randomTokenID()
	if err != nil {
		return nil, err
	}
	claims := Claims{
		Issuer:          Issuer,
		Audience:        Audience,
		Subject:         req.Subject,
		Tenant:          req.Tenant,
		Role:            req.Role,
		TokenID:         tokenID,
		DevicePublicKey: devicePublicKey,
		KeyID:           s.keyID,
		IssuedAt:        now.Unix(),
		NotBefore:       now.Unix(),
		ExpiresAt:       expiresAt.Unix(),
		MaxSessions:     req.MaxSessions,
		FolderAccess:    append([]string(nil), req.FolderAccess...),
		FolderPath:      append([]string(nil), req.FolderPath...),
		VerificationKey: s.PublicKey(),
		PeerAuthVersion: req.PeerAuthVersion,
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return nil, fmt.Errorf("encodage du jeton: %w", err)
	}
	payloadSegment := base64.RawURLEncoding.EncodeToString(payload)
	if len(payload) > 4096 {
		return nil, errors.New("trop d'autorisations de dossiers pour un jeton")
	}
	signed := TokenPrefix + "." + payloadSegment
	signature := ed25519.Sign(s.privateKey, []byte(signed))
	token := signed + "." + base64.RawURLEncoding.EncodeToString(signature)
	return &IssuedToken{Token: token, ExpiresAt: expiresAt, KeyID: s.keyID}, nil
}

func (s *Signer) PublicKey() string {
	if s == nil {
		return ""
	}
	publicKey := s.privateKey.Public().(ed25519.PublicKey)
	return base64.RawURLEncoding.EncodeToString(publicKey)
}

func NormalizeDevicePublicKey(value string) (string, error) {
	value = strings.TrimSpace(value)
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(decoded) != ed25519.PublicKeySize {
		return "", errors.New("clé publique d'appareil invalide")
	}
	return base64.RawURLEncoding.EncodeToString(decoded), nil
}

func randomTokenID() (string, error) {
	random := make([]byte, 18)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("génération de l'identifiant du jeton: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(random), nil
}

func validIdentifier(value string, max int) bool {
	if value == "" || len(value) > max {
		return false
	}
	for _, char := range value {
		if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') &&
			(char < '0' || char > '9') && char != '-' && char != '_' {
			return false
		}
	}
	return true
}

func validValue(value string, max int) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > max {
		return false
	}
	for _, char := range value {
		if char < 0x20 || char == 0x7f {
			return false
		}
	}
	return true
}
