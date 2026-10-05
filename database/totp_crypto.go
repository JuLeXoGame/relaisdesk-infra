package database

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"
	"sync"
)

// TOTP secrets and recovery-code blobs are encrypted at rest with AES-256-GCM.
// The data key is provisioned once at API startup via SetTOTPDataKey (32 bytes
// from TOTP_DATA_KEY). Without a key, writes fail closed while legacy
// plaintext rows still verify (read-only compat) until re-protected on next
// read.
const totpProtectedPrefix = "t1:"

var (
	totpDataKeyMu sync.RWMutex
	totpDataKey   []byte
)

// SetTOTPDataKey provisions the at-rest encryption key (the slice is copied).
// A nil or short key disables protection: new secrets cannot be written.
func SetTOTPDataKey(key []byte) {
	totpDataKeyMu.Lock()
	defer totpDataKeyMu.Unlock()
	totpDataKey = append([]byte(nil), key...)
}

// totpKeyAvailable reports whether a usable data key is provisioned.
func totpKeyAvailable() bool {
	totpDataKeyMu.RLock()
	defer totpDataKeyMu.RUnlock()
	return len(totpDataKey) == 32
}

func totpCipher() (cipher.AEAD, error) {
	totpDataKeyMu.RLock()
	defer totpDataKeyMu.RUnlock()
	if len(totpDataKey) != 32 {
		return nil, errors.New("clé de chiffrement TOTP absente ou invalide (TOTP_DATA_KEY, 32 octets base64url)")
	}
	block, err := aes.NewCipher(totpDataKey)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// protectTOTPSecret seals a plaintext secret for storage. Every call uses a
// fresh random nonce, so equal plaintexts produce distinct ciphertexts.
func protectTOTPSecret(plain string) (string, error) {
	if plain == "" || len(plain) > 4096 {
		return "", errors.New("secret TOTP invalide")
	}
	aead, err := totpCipher()
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := aead.Seal(nonce, nonce, []byte(plain), nil)
	return totpProtectedPrefix + base64.RawURLEncoding.EncodeToString(sealed), nil
}

// openTOTPSecret unseals a stored value. Legacy plaintext rows (no prefix) are
// returned as-is with legacy=true so they keep verifying until re-protected.
func openTOTPSecret(stored string) (plain string, legacy bool, err error) {
	if !strings.HasPrefix(stored, totpProtectedPrefix) {
		return stored, true, nil
	}
	aead, err := totpCipher()
	if err != nil {
		return "", false, err
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(stored, totpProtectedPrefix))
	if err != nil || len(raw) < aead.NonceSize()+1 {
		return "", false, errors.New("secret TOTP protégé illisible")
	}
	nonce, ct := raw[:aead.NonceSize()], raw[aead.NonceSize():]
	opened, err := aead.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", false, errors.New("secret TOTP protégé illisible")
	}
	return string(opened), false, nil
}

// OpenStoredTOTPSecret unseals a stored secret for callers outside this package
// (admin tooling, tests). Protected rows fail closed when the key is missing;
// legacy plaintext rows are returned as-is.
func OpenStoredTOTPSecret(stored string) (string, error) {
	plain, _, err := openTOTPSecret(stored)
	return plain, err
}
