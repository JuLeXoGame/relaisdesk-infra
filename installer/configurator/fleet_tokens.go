package main

import (
	"errors"
	"net/http"
	"strings"
)

// ParkToken mirrors database.ParkEnrollmentToken (metadata only; the
// plaintext token is returned once, at creation).
type ParkToken struct {
	ID        int64  `json:"id"`
	Prefix    string `json:"prefix"`
	LicenseID string `json:"license_id"`
	FolderID  string `json:"folder_id"`
	Label     string `json:"label"`
	MaxUses   int    `json:"max_uses"`
	UseCount  int    `json:"use_count"`
	ExpiresAt string `json:"expires_at"`
	IsActive  bool   `json:"is_active"`
	CreatedAt string `json:"created_at"`
}

// ParkTokenCreated is the one-time creation response carrying the secret.
type ParkTokenCreated struct {
	Token string    `json:"token"`
	Park  ParkToken `json:"park_token"`
}

func createTechnicianParkToken(token, label, folderID string, maxUses, ttlDays int) (*ParkTokenCreated, error) {
	req := map[string]interface{}{
		"label":     strings.TrimSpace(label),
		"folder_id": strings.TrimSpace(folderID),
		"max_uses":  maxUses,
		"ttl_days":  ttlDays,
	}
	var envelope struct {
		Token string    `json:"token"`
		Park  ParkToken `json:"park_token"`
	}
	if err := doTechnicianReq(http.MethodPost, "/api/v1/technician/device-park-tokens", token, req, &envelope); err != nil {
		return nil, err
	}
	if envelope.Token == "" || envelope.Park.ID == 0 {
		return nil, errors.New("réponse de création de token invalide")
	}
	return &ParkTokenCreated{Token: envelope.Token, Park: envelope.Park}, nil
}

func listTechnicianParkTokens(token string) ([]ParkToken, error) {
	var envelope struct {
		Tokens []ParkToken `json:"park_tokens"`
	}
	if err := doTechnicianReq(http.MethodGet, "/api/v1/technician/device-park-tokens", token, nil, &envelope); err != nil {
		return nil, err
	}
	return envelope.Tokens, nil
}

func revokeTechnicianParkToken(token string, id int64) error {
	var res map[string]interface{}
	return doTechnicianReq(http.MethodPut, "/api/v1/technician/device-park-tokens/"+itoa64(id)+"/revoke", token, nil, &res)
}

func itoa64(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
