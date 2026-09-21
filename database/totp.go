package database

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"strings"
	"time"
)

const (
	totpDigits     = 6
	totpPeriodSecs = 30
	totpIssuer     = "RelaisDesk"
	recoveryCount  = 8
)

// GenerateTOTPSecret creates a new cryptographically secure 20-byte base32 secret.
func GenerateTOTPSecret() (string, error) {
	bytes := make([]byte, 20)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("génération clé secrète: %w", err)
	}
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(bytes)
	return secret, nil
}

// GenerateTOTPURL builds an otpauth:// URL suitable for QR codes and authenticator apps.
func GenerateTOTPURL(accountEmail, secret string) string {
	accountEmail = strings.TrimSpace(accountEmail)
	label := fmt.Sprintf("%s:%s", totpIssuer, accountEmail)
	params := url.Values{}
	params.Set("secret", secret)
	params.Set("issuer", totpIssuer)
	params.Set("algorithm", "SHA1")
	params.Set("digits", fmt.Sprintf("%d", totpDigits))
	params.Set("period", fmt.Sprintf("%d", totpPeriodSecs))

	return fmt.Sprintf("otpauth://totp/%s?%s", url.PathEscape(label), params.Encode())
}

// CalculateTOTP generates the 6-digit TOTP code for a secret at a given timestamp.
func CalculateTOTP(secret string, t time.Time) (string, error) {
	cleanSecret := strings.ToUpper(strings.ReplaceAll(strings.ReplaceAll(secret, " ", ""), "-", ""))
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(cleanSecret)
	if err != nil {
		key, err = base32.StdEncoding.DecodeString(cleanSecret)
		if err != nil {
			return "", fmt.Errorf("clé secrète base32 invalide: %w", err)
		}
	}

	counter := uint64(t.Unix() / totpPeriodSecs)
	counterBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(counterBytes, counter)

	mac := hmac.New(sha1.New, key)
	mac.Write(counterBytes)
	hash := mac.Sum(nil)

	offset := hash[len(hash)-1] & 0x0f
	binaryCode := (uint32(hash[offset]&0x7f) << 24) |
		(uint32(hash[offset+1]&0xff) << 16) |
		(uint32(hash[offset+2]&0xff) << 8) |
		(uint32(hash[offset+3] & 0xff))

	otp := binaryCode % uint32(math.Pow10(totpDigits))
	return fmt.Sprintf("%06d", otp), nil
}

// ValidateTOTPCode checks if the provided 6-digit code matches the secret,
// allowing a window of ±1 step (±30 seconds) to account for client clock skew.
func ValidateTOTPCode(secret, inputCode string, at time.Time) bool {
	inputCode = strings.TrimSpace(inputCode)
	if len(inputCode) != totpDigits {
		return false
	}

	for _, offset := range []int64{-1, 0, 1} {
		testTime := at.Add(time.Duration(offset*totpPeriodSecs) * time.Second)
		expected, err := CalculateTOTP(secret, testTime)
		if err == nil && subtle.ConstantTimeCompare([]byte(expected), []byte(inputCode)) == 1 {
			return true
		}
	}
	return false
}

// GenerateRecoveryCodes produces count recovery codes and their SHA-256 hashes.
func GenerateRecoveryCodes(count int) (plainCodes []string, hashedCodes []string, err error) {
	if count <= 0 {
		count = recoveryCount
	}
	const alphabet = "23456789ABCDEFGHJKLMNPQRSTUVWXYZ"
	plainCodes = make([]string, count)
	hashedCodes = make([]string, count)

	buf := make([]byte, 8)
	for i := 0; i < count; i++ {
		if _, err := rand.Read(buf); err != nil {
			return nil, nil, fmt.Errorf("génération code de secours: %w", err)
		}
		var sb strings.Builder
		for j := 0; j < 8; j++ {
			if j == 4 {
				sb.WriteByte('-')
			}
			idx := int(buf[j]) % len(alphabet)
			sb.WriteByte(alphabet[idx])
		}
		code := sb.String()
		plainCodes[i] = code
		hashedCodes[i] = HashRecoveryCode(code)
	}
	return plainCodes, hashedCodes, nil
}

// HashRecoveryCode hashes a recovery code with SHA-256 for secure database storage.
func HashRecoveryCode(code string) string {
	normalized := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(code), "-", ""))
	h := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(h[:])
}

// ValidateAndConsumeRecoveryCode verifies if inputCode matches one of the stored hashed recovery codes.
// If valid, it removes the code and returns the updated JSON string.
func ValidateAndConsumeRecoveryCode(storedJSON, inputCode string) (valid bool, updatedJSON string, err error) {
	inputCode = strings.TrimSpace(inputCode)
	if inputCode == "" {
		return false, storedJSON, nil
	}

	var hashes []string
	if strings.TrimSpace(storedJSON) != "" {
		if err := json.Unmarshal([]byte(storedJSON), &hashes); err != nil {
			return false, storedJSON, err
		}
	}
	if len(hashes) == 0 {
		return false, storedJSON, nil
	}

	inputHash := HashRecoveryCode(inputCode)
	matchedIndex := -1
	for i, h := range hashes {
		if subtle.ConstantTimeCompare([]byte(h), []byte(inputHash)) == 1 {
			matchedIndex = i
			break
		}
	}

	if matchedIndex == -1 {
		return false, storedJSON, nil
	}

	remaining := append(hashes[:matchedIndex], hashes[matchedIndex+1:]...)
	newJSONBytes, err := json.Marshal(remaining)
	if err != nil {
		return false, storedJSON, err
	}

	return true, string(newJSONBytes), nil
}

// CountRemainingRecoveryCodes returns how many unused recovery codes are left.
func CountRemainingRecoveryCodes(storedJSON string) int {
	if strings.TrimSpace(storedJSON) == "" {
		return 0
	}
	var hashes []string
	if err := json.Unmarshal([]byte(storedJSON), &hashes); err != nil {
		return 0
	}
	return len(hashes)
}
