package database

import (
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"
)

const viewerCodeCharset = "23456789ABCDEFGHJKMNPQRSTUVWXYZ"

type ViewerCode struct {
	ID                  int        `json:"id"`
	Code                string     `json:"code"`
	TechnicianLicenseID string     `json:"technician_license_id"`
	ClientEmail         string     `json:"client_email"`
	ClientRustDeskID    string     `json:"client_rustdesk_id,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
	ExpiresAt           time.Time  `json:"expires_at"`
	UsedAt              *time.Time `json:"used_at,omitempty"`
	IsActive            bool       `json:"is_active"`
	MaxConnections      int        `json:"max_connections"`
	CurrentConnections  int        `json:"current_connections"`
	FirstClientIP       string     `json:"first_client_ip,omitempty"`
	LastConnectionAt    *time.Time `json:"last_connection_at,omitempty"`
	RevokedReason       string     `json:"revoked_reason,omitempty"`
	Status              string     `json:"status,omitempty"`
}

type ViewerConfig struct {
	ServerIP            string
	RendezvousPort      int
	RelayPort           int
	PublicKey           string
	ExpiresAt           time.Time
	ViewerID            int
	TechnicianLicenseID string
	MaxConnections      int
}

type SecurityAlert struct {
	ID         int        `json:"id"`
	Type       string     `json:"type"`
	Code       string     `json:"code,omitempty"`
	FirstIP    string     `json:"first_ip,omitempty"`
	SecondIP   string     `json:"second_ip,omitempty"`
	Message    string     `json:"message"`
	CreatedAt  time.Time  `json:"created_at"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
}

// GenerateRandomViewerCode generates a code in the format XXXX-XXXX.
func GenerateRandomViewerCode() (string, error) {
	bytes := make([]byte, 8)
	for i := range bytes {
		for {
			var candidate [1]byte
			if _, err := rand.Read(candidate[:]); err != nil {
				return "", err
			}
			limit := 256 - (256 % len(viewerCodeCharset))
			if int(candidate[0]) < limit {
				bytes[i] = viewerCodeCharset[int(candidate[0])%len(viewerCodeCharset)]
				break
			}
		}
	}
	return fmt.Sprintf("%s-%s", bytes[0:4], bytes[4:8]), nil
}

func NormalizeViewerCode(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}

func IsViewerCodeFormat(code string) bool {
	code = NormalizeViewerCode(code)
	if len(code) != 9 || code[4] != '-' {
		return false
	}
	for i, r := range code {
		if i == 4 {
			continue
		}
		if !strings.ContainsRune(viewerCodeCharset, r) {
			return false
		}
	}
	return true
}

func MaskViewerCode(code string) string {
	code = NormalizeViewerCode(code)
	if len(code) != 9 {
		return "****"
	}
	return code[:2] + "**-**" + code[7:]
}

// CreateViewerCode creates a new viewer code, ensuring the technician exists and is active.
func CreateViewerCode(db *sql.DB, technicianLicenseID string, clientEmail string) (*ViewerCode, error) {
	clientEmail = strings.TrimSpace(clientEmail)
	if len(clientEmail) > 254 || strings.ContainsAny(clientEmail, "\r\n\x00") {
		return nil, errors.New("identifiant client invalide")
	}
	var status string
	var expiresAtRaw any
	err := db.QueryRow("SELECT status, expires_at FROM licences WHERE license_id = ?", technicianLicenseID).Scan(&status, &expiresAtRaw)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("licence technicien introuvable")
		}
		return nil, fmt.Errorf("erreur verification licence technicien: %v", err)
	}

	expiresAt, err := ParseSQLiteTime(expiresAtRaw)
	if err != nil {
		return nil, fmt.Errorf("erreur parsing date expiration licence: %v", err)
	}

	if status != "active" || time.Now().UTC().After(expiresAt) {
		return nil, errors.New("la licence du technicien n'est pas active ou est expirée")
	}

	code, err := GenerateRandomViewerCode()
	if err != nil {
		return nil, fmt.Errorf("erreur generation code: %v", err)
	}

	expires := time.Now().UTC().Add(12 * time.Hour)
	res, err := db.Exec(`
		INSERT INTO viewer_codes (code, technician_license_id, client_email, expires_at, max_connections)
		VALUES (?, ?, ?, ?, 1)
	`, code, technicianLicenseID, clientEmail, expires.Format("2006-01-02 15:04:05"))
	if err != nil {
		return nil, fmt.Errorf("erreur insertion code: %v", err)
	}

	id, _ := res.LastInsertId()

	viewer := &ViewerCode{
		ID:                  int(id),
		Code:                code,
		TechnicianLicenseID: technicianLicenseID,
		ClientEmail:         clientEmail,
		CreatedAt:           time.Now().UTC(),
		ExpiresAt:           expires,
		IsActive:            true,
		MaxConnections:      1,
	}
	if _, err := EnsureViewerCodeIntervention(db, viewer); err != nil {
		_, _ = db.Exec(`DELETE FROM viewer_codes WHERE id = ?`, id)
		return nil, fmt.Errorf("création historique intervention: %v", err)
	}
	return viewer, nil
}

// ValidateViewerCode validates a code and returns the ViewerConfig if valid.
func ValidateViewerCode(db *sql.DB, code string) (*ViewerConfig, error) {
	vc, err := getViewerCodeForValidation(db, NormalizeViewerCode(code))
	if err != nil {
		return nil, err
	}
	if err := validateViewerCodeState(db, vc); err != nil {
		return nil, err
	}

	pubKey, err := GetActiveServerPublicKey(db)
	if err != nil {
		return nil, err
	}

	_, _ = db.Exec("UPDATE viewer_codes SET used_at = CURRENT_TIMESTAMP WHERE id = ? AND used_at IS NULL", vc.ID)
	_ = MarkViewerInterventionReady(db, vc.ID)

	return &ViewerConfig{
		PublicKey:           pubKey,
		ExpiresAt:           vc.ExpiresAt,
		ViewerID:            vc.ID,
		TechnicianLicenseID: vc.TechnicianLicenseID,
		MaxConnections:      vc.MaxConnections,
	}, nil
}

func getViewerCodeForValidation(db queryer, code string) (*ViewerCode, error) {
	var vc ViewerCode
	var (
		createdAtRaw        any
		expiresAtRaw        any
		usedAtRaw           any
		rustdeskID          sql.NullString
		firstIP             sql.NullString
		lastConnectionAtRaw any
		revokedReason       sql.NullString
	)
	err := db.QueryRow(`
		SELECT id, code, technician_license_id, client_email, client_rustdesk_id, created_at, expires_at, used_at,
		       is_active, max_connections, current_connections, first_client_ip,
		       last_connection_at, revoked_reason
		FROM viewer_codes
		WHERE code = ?`, code).Scan(
		&vc.ID,
		&vc.Code,
		&vc.TechnicianLicenseID,
		&vc.ClientEmail,
		&rustdeskID,
		&createdAtRaw,
		&expiresAtRaw,
		&usedAtRaw,
		&vc.IsActive,
		&vc.MaxConnections,
		&vc.CurrentConnections,
		&firstIP,
		&lastConnectionAtRaw,
		&revokedReason,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("code inexistant")
		}
		return nil, fmt.Errorf("erreur base de données: %v", err)
	}
	if createdAtRaw != nil {
		if t, err := ParseSQLiteTime(createdAtRaw); err == nil {
			vc.CreatedAt = t
		}
	}
	if expiresAtRaw != nil {
		if t, err := ParseSQLiteTime(expiresAtRaw); err == nil {
			vc.ExpiresAt = t
		}
	}
	if usedAtRaw != nil {
		if t, err := ParseSQLiteTime(usedAtRaw); err == nil {
			vc.UsedAt = &t
		}
	}
	if rustdeskID.Valid {
		vc.ClientRustDeskID = rustdeskID.String
	}
	if firstIP.Valid {
		vc.FirstClientIP = firstIP.String
	}
	if lastConnectionAtRaw != nil {
		if t, err := ParseSQLiteTime(lastConnectionAtRaw); err == nil {
			vc.LastConnectionAt = &t
		}
	}
	if revokedReason.Valid {
		vc.RevokedReason = revokedReason.String
	}
	if vc.MaxConnections <= 0 {
		vc.MaxConnections = 1
	}
	return &vc, nil
}

func validateViewerCodeState(db queryer, vc *ViewerCode) error {
	if !vc.IsActive {
		return errors.New("code revoqué")
	}
	if !vc.ExpiresAt.After(time.Now().UTC()) {
		return errors.New("code expiré")
	}

	var status string
	var techExpiresRaw any
	err := db.QueryRow("SELECT status, expires_at FROM licences WHERE license_id = ?", vc.TechnicianLicenseID).Scan(&status, &techExpiresRaw)
	if err != nil {
		return errors.New("licence technicien invalide ou expirée")
	}
	techExpires, err := ParseSQLiteTime(techExpiresRaw)
	if err != nil || status != "active" || !techExpires.After(time.Now().UTC()) {
		return errors.New("licence technicien invalide ou expirée")
	}
	return nil
}

// SetViewerRustDeskID associates a RustDesk ID announced by the viewer with
// the viewer code. The announcement must come from the same network device key
// that claimed the code, otherwise a copied code could replace the ID shown to
// the technician.
func SetViewerRustDeskID(db *sql.DB, code string, rustdeskID string, devicePublicKey string) error {
	code = NormalizeViewerCode(code)
	rustdeskID = strings.TrimSpace(rustdeskID)
	if !IsViewerCodeFormat(code) || !isValidRustDeskID(rustdeskID) {
		return errors.New("code ou ID RustDesk invalide")
	}
	devicePublicKey, err := normalizeNetworkDevicePublicKey(devicePublicKey)
	if err != nil {
		return err
	}
	if _, err := ValidateViewerCode(db, code); err != nil {
		return err
	}

	res, err := db.Exec(`
		UPDATE viewer_codes
		SET client_rustdesk_id = ?
		WHERE code = ? AND is_active = 1
		  AND expires_at > CURRENT_TIMESTAMP
		  AND network_device_public_key = ?
		  AND (client_rustdesk_id IS NULL OR client_rustdesk_id = '' OR client_rustdesk_id = ?)
	`, rustdeskID, code, devicePublicKey, rustdeskID)
	if err != nil {
		return err
	}
	return ensureAffected(res, "code introuvable ou inactif")
}

// BindViewerNetworkDeviceKey permanently associates a viewer code with the
// Ed25519 device key that first requested a network token. Repeating the call
// with the same key is allowed so short-lived tokens can be renewed, while a
// copied code cannot be moved to another device.
func BindViewerNetworkDeviceKey(db *sql.DB, viewerID int, devicePublicKey string) error {
	devicePublicKey, err := normalizeNetworkDevicePublicKey(devicePublicKey)
	if err != nil {
		return err
	}
	res, err := db.Exec(`
		UPDATE viewer_codes
		SET network_device_public_key = ?
		WHERE id = ? AND is_active = 1
		  AND expires_at > CURRENT_TIMESTAMP
		  AND (network_device_public_key IS NULL OR network_device_public_key = '' OR network_device_public_key = ?)
	`, devicePublicKey, viewerID, devicePublicKey)
	if err != nil {
		return fmt.Errorf("liaison de l'appareil viewer: %w", err)
	}
	return ensureAffected(res, "code viewer déjà lié à un autre appareil ou inactif")
}

func normalizeNetworkDevicePublicKey(devicePublicKey string) (string, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(devicePublicKey))
	if err != nil || len(decoded) != ed25519.PublicKeySize {
		return "", errors.New("clé publique d'appareil invalide")
	}
	return base64.RawURLEncoding.EncodeToString(decoded), nil
}

func isValidRustDeskID(id string) bool {
	if len(id) < 6 || len(id) > 16 {
		return false
	}
	for _, r := range id {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func ResetTransientConnections(db *sql.DB) error {
	if _, err := db.Exec("UPDATE licences SET current_connections = 0"); err != nil {
		return err
	}
	_, err := db.Exec("UPDATE viewer_codes SET current_connections = 0")
	return err
}

// GetActiveServerPublicKey returns the latest active RustDesk server public key.
func GetActiveServerPublicKey(db *sql.DB) (string, error) {
	var pubKey string
	err := db.QueryRow("SELECT public_key FROM server_keys WHERE is_active = 1 ORDER BY id DESC LIMIT 1").Scan(&pubKey)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", errors.New("clé publique serveur active non configurée")
		}
		return "", fmt.Errorf("erreur recupération clé publique: %v", err)
	}
	if pubKey == "" {
		return "", errors.New("clé publique serveur active vide")
	}
	return pubKey, nil
}

// RevokeViewerCode revokes a given code.
func RevokeViewerCode(db *sql.DB, code string) error {
	res, err := db.Exec(`
		UPDATE viewer_codes
		SET is_active = 0,
		    current_connections = 0,
		    revoked_reason = COALESCE(revoked_reason, 'révocation manuelle')
		WHERE code = ?
	`, NormalizeViewerCode(code))
	if err != nil {
		return err
	}
	return ensureAffected(res, "aucun code trouvé")
}

// DeleteViewerCode permanently removes a viewer code from the database.
func DeleteViewerCode(db *sql.DB, code string, technicianLicenseID string) error {
	res, err := db.Exec(`
		DELETE FROM viewer_codes
		WHERE code = ? AND technician_license_id = ?
	`, NormalizeViewerCode(code), technicianLicenseID)
	if err != nil {
		return err
	}
	return ensureAffected(res, "aucun code trouvé")
}

// ListViewerCodes returns all viewer codes for a specific technician.
func ListViewerCodes(db *sql.DB, technicianLicenseID string) ([]ViewerCode, error) {
	rows, err := db.Query(`
		SELECT id, code, technician_license_id, client_email, client_rustdesk_id, created_at, expires_at, used_at,
		       is_active, max_connections, current_connections, first_client_ip,
		       last_connection_at, revoked_reason
		FROM viewer_codes
		WHERE technician_license_id = ?
		ORDER BY created_at DESC
	`, technicianLicenseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var codes []ViewerCode
	for rows.Next() {
		var vc ViewerCode
		var (
			createdAtRaw        any
			expiresAtRaw        any
			usedAtRaw           any
			email               sql.NullString
			rustdeskID          sql.NullString
			firstIP             sql.NullString
			lastConnectionAtRaw any
			revokedReason       sql.NullString
		)

		if err := rows.Scan(
			&vc.ID,
			&vc.Code,
			&vc.TechnicianLicenseID,
			&email,
			&rustdeskID,
			&createdAtRaw,
			&expiresAtRaw,
			&usedAtRaw,
			&vc.IsActive,
			&vc.MaxConnections,
			&vc.CurrentConnections,
			&firstIP,
			&lastConnectionAtRaw,
			&revokedReason,
		); err != nil {
			return nil, err
		}

		if email.Valid {
			vc.ClientEmail = email.String
		}
		if rustdeskID.Valid {
			vc.ClientRustDeskID = rustdeskID.String
		}
		if createdAtRaw != nil {
			if t, err := ParseSQLiteTime(createdAtRaw); err == nil {
				vc.CreatedAt = t
			}
		}
		if expiresAtRaw != nil {
			if t, err := ParseSQLiteTime(expiresAtRaw); err == nil {
				vc.ExpiresAt = t
			}
		}
		if usedAtRaw != nil {
			if t, err := ParseSQLiteTime(usedAtRaw); err == nil {
				vc.UsedAt = &t
			}
		}
		if firstIP.Valid {
			vc.FirstClientIP = firstIP.String
		}
		if lastConnectionAtRaw != nil {
			if t, err := ParseSQLiteTime(lastConnectionAtRaw); err == nil {
				vc.LastConnectionAt = &t
			}
		}
		if revokedReason.Valid {
			vc.RevokedReason = revokedReason.String
		}
		codes = append(codes, vc)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return codes, nil
}

func ListSecurityAlerts(db *sql.DB, limit int) ([]SecurityAlert, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := db.Query(`
		SELECT id, type, code, first_ip, second_ip, message, created_at, resolved_at
		FROM security_alerts
		ORDER BY created_at DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	alerts := []SecurityAlert{}
	for rows.Next() {
		var alert SecurityAlert
		var (
			code          sql.NullString
			firstIP       sql.NullString
			secondIP      sql.NullString
			createdAtRaw  any
			resolvedAtRaw any
		)
		if err := rows.Scan(&alert.ID, &alert.Type, &code, &firstIP, &secondIP, &alert.Message, &createdAtRaw, &resolvedAtRaw); err != nil {
			return nil, err
		}
		if code.Valid {
			alert.Code = code.String
		}
		if firstIP.Valid {
			alert.FirstIP = firstIP.String
		}
		if secondIP.Valid {
			alert.SecondIP = secondIP.String
		}
		if createdAtRaw != nil {
			if t, err := ParseSQLiteTime(createdAtRaw); err == nil {
				alert.CreatedAt = t
			}
		}
		if resolvedAtRaw != nil {
			if t, err := ParseSQLiteTime(resolvedAtRaw); err == nil {
				alert.ResolvedAt = &t
			}
		}
		alerts = append(alerts, alert)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return alerts, nil
}

type queryer interface {
	QueryRow(query string, args ...any) *sql.Row
}
