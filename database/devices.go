package database

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	DeviceStatusOnline  = "online"
	DeviceStatusOffline = "offline"
	DeviceEnrollmentTTL = 15 * time.Minute
)

var ErrDeviceAuthorization = errors.New("autorisation de poste invalide, expirée ou révoquée")

type Device struct {
	ID                  int        `json:"id"`
	DeviceID            string     `json:"device_id"`
	CustomerID          int64      `json:"customer_id"`
	LicenseID           string     `json:"license_id"`
	PermanentCode       string     `json:"permanent_code,omitempty"`
	RustDeskID          string     `json:"rustdesk_id"`
	DevicePublicKey     string     `json:"-"`
	EnrollmentVersion   int        `json:"-"`
	FolderID            string     `json:"folder_id"`
	EnrollmentState     string     `json:"enrollment_state"`
	EnrollmentExpiresAt *time.Time `json:"enrollment_expires_at,omitempty"`
	Alias               string     `json:"alias"`
	Hostname            string     `json:"hostname"`
	OS                  string     `json:"os"`
	Status              string     `json:"status"`
	LastSeenAt          *time.Time `json:"last_seen_at,omitempty"`
	LastIP              string     `json:"last_ip,omitempty"`
	MACAddress          string     `json:"mac_address,omitempty"`
	SubnetBroadcast     string     `json:"subnet_broadcast,omitempty"`
	AgentVersion        string     `json:"agent_version,omitempty"`
	Notes               string     `json:"notes,omitempty"`
	IsActive            bool       `json:"is_active"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

type DeviceProof struct {
	Timestamp int64  `json:"timestamp"`
	Nonce     string `json:"nonce"`
	Signature string `json:"signature"`
	Ready     bool   `json:"ready"`
}

// This ordered array is the versioned Viewer wire protocol.
func DeviceProofMessage(action, identifier, rustdeskID, hostname, osName, publicKey string, p DeviceProof) []byte {
	b, _ := json.Marshal([]any{"relaisdesk-fleet-v1", action, identifier, rustdeskID, hostname, osName, publicKey, p.Ready, p.Timestamp, p.Nonce})
	return b
}
func deviceProofValid(key string, p DeviceProof, message []byte) bool {
	pub, e := base64.RawURLEncoding.DecodeString(key)
	sig, se := base64.RawURLEncoding.DecodeString(p.Signature)
	nonce, ne := base64.RawURLEncoding.DecodeString(p.Nonce)
	now := time.Now().Unix()
	return e == nil && len(pub) == ed25519.PublicKeySize && se == nil && len(sig) == ed25519.SignatureSize && ne == nil && len(nonce) == 24 && p.Timestamp >= now-90 && p.Timestamp <= now+30 && ed25519.Verify(pub, message, sig)
}
func randomDeviceCode(prefix string, count int) (string, error) {
	raw := make([]byte, count)
	for i := range raw {
		for {
			var b [1]byte
			if _, err := rand.Read(b[:]); err != nil {
				return "", err
			}
			if int(b[0]) < 256-256%len(viewerCodeCharset) {
				raw[i] = viewerCodeCharset[int(b[0])%len(viewerCodeCharset)]
				break
			}
		}
	}
	var groups []string
	for i := 0; i < len(raw); i += 4 {
		groups = append(groups, string(raw[i:i+4]))
	}
	return prefix + strings.Join(groups, "-"), nil
}
func GenerateDeviceID() (string, error)         { return randomDeviceCode("DEV-", 16) }
func GeneratePermanentCode() (string, error)    { return randomDeviceCode("PERM-", 24) }
func NormalizePermanentCode(code string) string { return strings.ToUpper(strings.TrimSpace(code)) }
func deviceCodeHash(code string) string {
	sum := sha256.Sum256([]byte(NormalizePermanentCode(code)))
	return hex.EncodeToString(sum[:])
}
func validDeviceText(s string, max int) bool {
	return utf8.ValidString(s) && utf8.RuneCountInString(s) <= max && !strings.ContainsFunc(s, unicode.IsControl)
}

// sanitizeDeviceText strips control characters and caps length for free-form
// device fields (agent version, subnet). Unlike validDeviceText it never
// rejects: heartbeats must not flap on a weird agent build string, and the
// values land in server logs where CR/LF or ANSI escapes would forge lines.
func sanitizeDeviceText(s string, max int) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, strings.TrimSpace(s))
	if utf8.RuneCountInString(s) > max {
		s = string([]rune(s)[:max])
	}
	return s
}
func ValidFleetRustDeskID(id string) bool {
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
func validDeviceMetadata(alias, notes string) bool {
	return validDeviceText(alias, 100) && validDeviceText(notes, 254)
}

func CreatePermanentEnrollment(db *sql.DB, customerID int64, licenseID, alias, notes string) (*Device, error) {
	if db == nil {
		return nil, ErrDeviceAuthorization
	}
	licenseID = strings.ToUpper(strings.TrimSpace(licenseID))
	alias = strings.TrimSpace(alias)
	notes = strings.TrimSpace(notes)
	if !validDeviceMetadata(alias, notes) {
		return nil, errors.New("alias ou notes invalides (100 / 254 caractères maximum)")
	}
	// InitDatabase uses immediate SQLite transactions. Reserving the quota and
	// inserting the invitation must hold the same write lock (including across
	// different API processes), otherwise simultaneous requests could exceed it.
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	quotas, err := listFleetQuotas(tx, 0, licenseID)
	if err != nil {
		return nil, err
	}
	if len(quotas) != 1 || quotas[0].Status != "active" || !quotas[0].ExpiresAt.After(time.Now()) {
		return nil, ErrDeviceAuthorization
	}
	quota := quotas[0]
	if customerID > 0 && quota.CustomerID != customerID {
		return nil, ErrDeviceAuthorization
	}
	if quota.Remaining == 0 {
		return nil, ErrDeviceLimit
	}
	if customerID == 0 {
		customerID = quota.CustomerID
	}
	var cust any
	if customerID > 0 {
		cust = customerID
	}
	for attempts := 0; attempts < 5; attempts++ {
		id, err := GenerateDeviceID()
		if err != nil {
			return nil, err
		}
		code, err := GeneratePermanentCode()
		if err != nil {
			return nil, err
		}
		now := time.Now().UTC()
		expires := now.Add(DeviceEnrollmentTTL)
		res, err := tx.Exec("INSERT INTO devices(device_id,customer_id,license_id,permanent_code,enrollment_version,folder_id,alias,notes,status,is_active,created_at,updated_at) VALUES(?,?,?,?,1,'',?,?,'offline',1,?,?)", id, cust, licenseID, deviceCodeHash(code), alias, notes, now, now)
		if err != nil {
			if strings.Contains(err.Error(), "UNIQUE constraint failed") {
				continue
			}
			return nil, err
		}
		rowID, err := res.LastInsertId()
		if err != nil {
			return nil, err
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return &Device{ID: int(rowID), DeviceID: id, CustomerID: customerID, LicenseID: licenseID, PermanentCode: code, EnrollmentVersion: 1, EnrollmentState: "pending", EnrollmentExpiresAt: &expires, Alias: alias, Notes: notes, Status: DeviceStatusOffline, IsActive: true, CreatedAt: now, UpdatedAt: now}, nil
	}
	return nil, errors.New("impossible de réserver un identifiant unique")
}

const deviceColumns = "id,device_id,customer_id,license_id,rustdesk_id,device_public_key,enrollment_version,folder_id,alias,hostname,os,status,last_seen_at,last_ip,notes,is_active,created_at,updated_at,mac_address,subnet_broadcast,agent_version"

type deviceScanner interface{ Scan(...any) error }

func NormalizeMAC(mac string) string {
	mac = strings.ToLower(strings.TrimSpace(mac))
	if len(mac) > 32 {
		return ""
	}
	hw, err := net.ParseMAC(mac)
	if err != nil || len(hw) != 6 {
		return ""
	}
	return hw.String()
}

func scanDevice(row deviceScanner) (Device, error) {
	var d Device
	var customer sql.NullInt64
	var seen sql.NullTime
	var folderID sql.NullString
	var mac, subnet, agentVersion sql.NullString
	err := row.Scan(&d.ID, &d.DeviceID, &customer, &d.LicenseID, &d.RustDeskID, &d.DevicePublicKey, &d.EnrollmentVersion, &folderID, &d.Alias, &d.Hostname, &d.OS, &d.Status, &seen, &d.LastIP, &d.Notes, &d.IsActive, &d.CreatedAt, &d.UpdatedAt, &mac, &subnet, &agentVersion)
	if err != nil {
		return d, err
	}
	d.CustomerID = customer.Int64
	d.FolderID = folderID.String
	d.MACAddress = mac.String
	d.SubnetBroadcast = subnet.String
	d.AgentVersion = agentVersion.String
	if seen.Valid {
		d.LastSeenAt = &seen.Time
	}
	if !seen.Valid || time.Since(seen.Time) > 3*time.Minute {
		d.Status = DeviceStatusOffline
	}
	switch d.EnrollmentVersion {
	case 2:
		d.EnrollmentState = "enrolled"
	case 1:
		exp := d.CreatedAt.Add(DeviceEnrollmentTTL)
		d.EnrollmentExpiresAt = &exp
		d.EnrollmentState = "pending"
		if time.Now().After(exp) {
			d.EnrollmentState = "expired"
		}
	default:
		d.EnrollmentState = "reenrollment_required"
		d.Status = DeviceStatusOffline
	}
	return d, nil
}
func consumeDeviceNonce(tx *sql.Tx, id string, p DeviceProof) error {
	if _, err := tx.Exec("DELETE FROM device_auth_nonces WHERE expires_at < ?", time.Now().Unix()); err != nil {
		return err
	}
	if _, err := tx.Exec("INSERT INTO device_auth_nonces(device_id,nonce,expires_at) VALUES(?,?,?)", id, p.Nonce, p.Timestamp+91); err != nil {
		return ErrDeviceAuthorization
	}
	return nil
}

func activeDeviceLicense(tx *sql.Tx, id string) bool {
	var status string
	var expiry time.Time
	return tx.QueryRow("SELECT status,expires_at FROM licences WHERE license_id=?", id).Scan(&status, &expiry) == nil && status == "active" && expiry.After(time.Now())
}

func EnrollDeviceWithFullInfo(db *sql.DB, code, rustdeskID, hostname, osName, publicKey, ip, macAddress, subnetBroadcast, agentVersion string, proof ...DeviceProof) (*Device, error) {
	code = NormalizePermanentCode(code)
	if db == nil || len(proof) != 1 || len(code) > 128 || !ValidFleetRustDeskID(rustdeskID) || !validDeviceText(hostname, 253) || (osName != "windows" && osName != "linux" && osName != "darwin") {
		return nil, ErrDeviceAuthorization
	}
	p := proof[0]
	if !deviceProofValid(publicKey, p, DeviceProofMessage("enroll", code, rustdeskID, hostname, osName, publicKey, p)) {
		return nil, ErrDeviceAuthorization
	}
	macAddress = NormalizeMAC(macAddress)
	subnetBroadcast = sanitizeDeviceText(subnetBroadcast, 64)
	agentVersion = sanitizeDeviceText(agentVersion, 64)
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	d, err := scanDevice(tx.QueryRow("SELECT "+deviceColumns+" FROM devices WHERE permanent_code=? AND is_active=1", deviceCodeHash(code)))
	if err != nil {
		// No per-device invitation: fall back to park (multi-use) tokens.
		return enrollDeviceWithParkTokenTx(tx, db, code, rustdeskID, hostname, osName, publicKey, ip, macAddress, subnetBroadcast, agentVersion, p)
	}
	if d.EnrollmentVersion == 0 || !d.CreatedAt.Add(DeviceEnrollmentTTL).After(time.Now()) {
		return nil, ErrDeviceAuthorization
	}
	// Lost responses can only be retried by the same cryptographic identity.
	if d.EnrollmentVersion == 2 && (d.DevicePublicKey != publicKey || d.RustDeskID != rustdeskID) {
		return nil, ErrDeviceAuthorization
	}
	if !activeDeviceLicense(tx, d.LicenseID) {
		return nil, ErrDeviceAuthorization
	}
	if d.EnrollmentVersion == 1 {
		quotas, err := listFleetQuotas(tx, 0, d.LicenseID)
		if err != nil {
			return nil, err
		}
		if len(quotas) != 1 {
			return nil, ErrDeviceAuthorization
		}
		// A plan may have been downgraded since the invitation was reserved.
		if quotas[0].Used > quotas[0].Limit {
			return nil, ErrDeviceLimit
		}
	}
	if err := consumeDeviceNonce(tx, d.DeviceID, p); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	_, err = tx.Exec("UPDATE devices SET rustdesk_id=?,hostname=?,os=?,device_public_key=?,enrollment_version=2,status='offline',last_seen_at=?,last_ip=?,mac_address=?,subnet_broadcast=?,agent_version=?,updated_at=? WHERE id=?", rustdeskID, hostname, osName, publicKey, now, ip, macAddress, subnetBroadcast, agentVersion, now, d.ID)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return GetDeviceByID(db, d.DeviceID)
}

func EnrollDeviceWithNetwork(db *sql.DB, code, rustdeskID, hostname, osName, publicKey, ip, macAddress, subnetBroadcast string, proof ...DeviceProof) (*Device, error) {
	return EnrollDeviceWithFullInfo(db, code, rustdeskID, hostname, osName, publicKey, ip, macAddress, subnetBroadcast, "", proof...)
}

func EnrollDevice(db *sql.DB, code, rustdeskID, hostname, osName, publicKey, ip string, proof ...DeviceProof) (*Device, error) {
	return EnrollDeviceWithNetwork(db, code, rustdeskID, hostname, osName, publicKey, ip, "", "", proof...)
}

func DeviceHeartbeatWithFullInfo(db *sql.DB, identifier, ip, macAddress, subnetBroadcast, agentVersion, rustdeskID string, proof ...DeviceProof) error {
	if db == nil || len(proof) != 1 {
		return ErrDeviceAuthorization
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	d, err := scanDevice(tx.QueryRow("SELECT "+deviceColumns+" FROM devices WHERE device_id=? AND is_active=1 AND enrollment_version=2", identifier))
	if err != nil {
		return ErrDeviceAuthorization
	}
	p := proof[0]
	if !deviceProofValid(d.DevicePublicKey, p, DeviceProofMessage("heartbeat", identifier, "", "", "", d.DevicePublicKey, p)) {
		return ErrDeviceAuthorization
	}
	if !activeDeviceLicense(tx, d.LicenseID) {
		return ErrDeviceAuthorization
	}
	if err = consumeDeviceNonce(tx, identifier, p); err != nil {
		return err
	}
	status := DeviceStatusOffline
	if p.Ready {
		status = DeviceStatusOnline
	}
	now := time.Now().UTC()
	macAddress = NormalizeMAC(macAddress)
	subnetBroadcast = sanitizeDeviceText(subnetBroadcast, 64)
	agentVersion = sanitizeDeviceText(agentVersion, 64)
	if agentVersion != "" && macAddress != "" {
		_, err = tx.Exec("UPDATE devices SET status=?,last_seen_at=?,last_ip=?,mac_address=?,subnet_broadcast=?,agent_version=?,updated_at=? WHERE id=?", status, now, ip, macAddress, subnetBroadcast, agentVersion, now, d.ID)
	} else if agentVersion != "" {
		_, err = tx.Exec("UPDATE devices SET status=?,last_seen_at=?,last_ip=?,agent_version=?,updated_at=? WHERE id=?", status, now, ip, agentVersion, now, d.ID)
	} else if macAddress != "" {
		_, err = tx.Exec("UPDATE devices SET status=?,last_seen_at=?,last_ip=?,mac_address=?,subnet_broadcast=?,updated_at=? WHERE id=?", status, now, ip, macAddress, subnetBroadcast, now, d.ID)
	} else {
		_, err = tx.Exec("UPDATE devices SET status=?,last_seen_at=?,last_ip=?,updated_at=? WHERE id=?", status, now, ip, now, d.ID)
	}
	if err != nil {
		return err
	}
	// Self-heal: the engine id can rotate under a running agent (config
	// rebuilt, keys regenerated). The agent reports its live id every
	// cycle; adopt it so the fleet never keeps pointing at a ghost.
	// Proof-checked above, own row only; invalid values never wipe.
	rustdeskID = strings.TrimSpace(rustdeskID)
	if ValidFleetRustDeskID(rustdeskID) && rustdeskID != d.RustDeskID {
		if _, err = tx.Exec("UPDATE devices SET rustdesk_id=?,updated_at=? WHERE id=?", rustdeskID, now, d.ID); err != nil {
			return err
		}
		log.Printf("[devices] %s: rustdesk_id %s -> %s (heartbeat)", identifier, d.RustDeskID, rustdeskID)
	}
	return tx.Commit()
}

func DeviceHeartbeatWithNetwork(db *sql.DB, identifier, ip, macAddress, subnetBroadcast string, proof ...DeviceProof) error {
	return DeviceHeartbeatWithFullInfo(db, identifier, ip, macAddress, subnetBroadcast, "", "", proof...)
}

func DeviceHeartbeat(db *sql.DB, identifier, ip string, proof ...DeviceProof) error {
	return DeviceHeartbeatWithNetwork(db, identifier, ip, "", "", proof...)
}
func GetDeviceByID(db *sql.DB, id string) (*Device, error) {
	if db == nil {
		return nil, ErrDeviceAuthorization
	}
	d, err := scanDevice(db.QueryRow("SELECT "+deviceColumns+" FROM devices WHERE device_id=? AND is_active=1", id))
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// Cursor pagination is independent of the commercial managed-device quota.
func ListDevicePage(db *sql.DB, customerID int64, licenseID string, after, limit int) ([]Device, error) {
	if db == nil || after < 0 || limit < 1 || limit > 200 {
		return nil, errors.New("pagination invalide")
	}
	where := "customer_id=?"
	var owner any = customerID
	if licenseID != "" {
		where = "license_id=?"
		owner = strings.ToUpper(licenseID)
	}
	rows, err := db.Query("SELECT "+deviceColumns+" FROM devices WHERE "+where+" AND is_active=1 AND id>? ORDER BY id LIMIT ?", owner, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Device{}
	for rows.Next() {
		d, err := scanDevice(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, d)
	}
	return result, rows.Err()
}
func ListDevicesByCustomer(db *sql.DB, id int64) ([]Device, error) {
	return ListDevicePage(db, id, "", 0, 200)
}
func ListDevicesByLicense(db *sql.DB, id string) ([]Device, error) {
	return ListDevicePage(db, 0, id, 0, 200)
}
func updateDevice(db *sql.DB, ownerField string, owner any, id, alias, notes string, folderID ...string) error {
	alias = strings.TrimSpace(alias)
	notes = strings.TrimSpace(notes)
	if db == nil || !validDeviceMetadata(alias, notes) {
		return errors.New("alias ou notes invalides")
	}

	now := time.Now().UTC()
	var res sql.Result
	var err error

	if len(folderID) > 0 {
		fID := strings.TrimSpace(folderID[0])
		if fID != "" {
			var exists int
			if ownerField == "license_id" {
				err = db.QueryRow("SELECT 1 FROM device_folders WHERE folder_id = ? AND license_id = ?", fID, owner).Scan(&exists)
			} else {
				err = db.QueryRow("SELECT 1 FROM device_folders WHERE folder_id = ? AND customer_id = ?", fID, owner).Scan(&exists)
			}
			if err != nil || exists != 1 {
				return ErrFolderNotFound
			}
		}
		res, err = db.Exec("UPDATE devices SET alias=?,notes=?,folder_id=?,updated_at=? WHERE device_id=? AND "+ownerField+"=? AND is_active=1", alias, notes, fID, now, id, owner)
	} else {
		res, err = db.Exec("UPDATE devices SET alias=?,notes=?,updated_at=? WHERE device_id=? AND "+ownerField+"=? AND is_active=1", alias, notes, now, id, owner)
	}

	return deviceAffected(res, err)
}
func deviceAffected(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrDeviceAuthorization
	}
	return nil
}
func UpdateDeviceAlias(db *sql.DB, id int64, device, alias, notes string, folderID ...string) error {
	return updateDevice(db, "customer_id", id, device, alias, notes, folderID...)
}
func TechnicianUpdateDevice(db *sql.DB, id, device, alias, notes string, folderID ...string) error {
	return updateDevice(db, "license_id", strings.ToUpper(id), device, alias, notes, folderID...)
}
func DeleteDevice(db *sql.DB, id int64, device string) error {
	if db == nil {
		return ErrDeviceAuthorization
	}
	res, err := db.Exec("DELETE FROM devices WHERE device_id=? AND customer_id=?", device, id)
	return deviceAffected(res, err)
}
func TechnicianDeleteDevice(db *sql.DB, id, device string) error {
	if db == nil {
		return ErrDeviceAuthorization
	}
	res, err := db.Exec("DELETE FROM devices WHERE device_id=? AND license_id=?", device, strings.ToUpper(id))
	return deviceAffected(res, err)
}
