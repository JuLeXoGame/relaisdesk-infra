package database

import (
	"database/sql"
	"errors"
	"strings"
	"time"
)

// Park (multi-use) enrollment tokens for mass deployment.
//
// Unlike per-device PERM codes (one code, one machine, 15-minute TTL), a park
// token enrolls up to max_uses machines over a long window (days). It is
// scoped to one licence and optionally one folder, revocable at any time, and
// stored hashed: the plaintext is shown only at creation. Each enrolled
// device still gets its own ed25519 identity and consumes one fleet slot.
const (
	// ParkTokenDefaultTTL is deliberately long: a rollout wave (GPO, Intune,
	// SCCM) spans days, and machines enroll when they come online.
	ParkTokenDefaultTTL = 30 * 24 * time.Hour
	ParkTokenMaxTTL     = 365 * 24 * time.Hour
	ParkTokenMaxUses    = 100000
)

// ParkEnrollmentToken is the stored metadata of a park token. The plaintext
// token is never persisted; TokenPrefix allows operators to tell tokens apart.
type ParkEnrollmentToken struct {
	ID         int64     `json:"id"`
	Prefix     string    `json:"prefix"`
	CustomerID int64     `json:"-"`
	LicenseID  string    `json:"license_id"`
	FolderID   string    `json:"folder_id"`
	Label      string    `json:"label"`
	MaxUses    int       `json:"max_uses"`
	UseCount   int       `json:"use_count"`
	ExpiresAt  time.Time `json:"expires_at"`
	IsActive   bool      `json:"is_active"`
	CreatedAt  time.Time `json:"created_at"`
}

// GenerateParkToken returns a PARK- token with ~157 bits of entropy
// (32 chars over the 30-symbol viewer alphabet).
func GenerateParkToken() (string, error) { return randomDeviceCode("PARK-", 32) }

func parkTokenPrefix(token string) string {
	token = NormalizePermanentCode(token)
	if len(token) > 13 {
		return token[:13]
	}
	return token
}

// CreateParkEnrollmentToken mints a multi-use token. maxUses <= 0 selects a
// safe default of 100; ttlDays <= 0 selects the 30-day default.
func CreateParkEnrollmentToken(db *sql.DB, customerID int64, licenseID, label, folderID string, maxUses, ttlDays int) (*ParkEnrollmentToken, string, error) {
	if db == nil {
		return nil, "", ErrDeviceAuthorization
	}
	licenseID = strings.ToUpper(strings.TrimSpace(licenseID))
	label = strings.TrimSpace(label)
	folderID = strings.TrimSpace(folderID)
	if !validDeviceText(label, 100) {
		return nil, "", errors.New("libellé invalide (100 caractères maximum)")
	}
	if maxUses <= 0 {
		maxUses = 100
	}
	if maxUses > ParkTokenMaxUses {
		return nil, "", errors.New("nombre d'utilisations maximal dépassé")
	}
	ttl := ParkTokenDefaultTTL
	if ttlDays > 0 {
		ttl = time.Duration(ttlDays) * 24 * time.Hour
		if ttl > ParkTokenMaxTTL {
			return nil, "", errors.New("durée de validité maximale dépassée (365 jours)")
		}
	}
	lic, err := GetLicenseByID(db, licenseID)
	if err != nil || lic.Status != "active" || !lic.ExpiresAt.After(time.Now()) {
		return nil, "", ErrDeviceAuthorization
	}
	if customerID == 0 {
		customerID = lic.CustomerID
	}
	if folderID != "" {
		if _, err := GetDeviceFolder(db, customerID, licenseID, folderID); err != nil {
			return nil, "", errors.New("dossier de parc inconnu pour cette licence")
		}
	}
	token, err := GenerateParkToken()
	if err != nil {
		return nil, "", err
	}
	now := time.Now().UTC()
	expires := now.Add(ttl)
	var cust any
	if customerID > 0 {
		cust = customerID
	}
	res, err := db.Exec("INSERT INTO device_enrollment_tokens(token_hash,token_prefix,customer_id,license_id,folder_id,label,max_uses,use_count,expires_at,is_active,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)",
		deviceCodeHash(token), parkTokenPrefix(token), cust, licenseID, folderID, label, maxUses, 0, expires.Format(time.RFC3339), 1, now.Format(time.RFC3339))
	if err != nil {
		return nil, "", err
	}
	rowID, err := res.LastInsertId()
	if err != nil {
		return nil, "", err
	}
	return &ParkEnrollmentToken{ID: rowID, Prefix: parkTokenPrefix(token), CustomerID: customerID, LicenseID: licenseID, FolderID: folderID, Label: label, MaxUses: maxUses, ExpiresAt: expires, IsActive: true, CreatedAt: now}, token, nil
}

// ListParkEnrollmentTokens returns token metadata (never plaintext).
func ListParkEnrollmentTokens(db *sql.DB, customerID int64, licenseID string) ([]ParkEnrollmentToken, error) {
	licenseID = strings.ToUpper(strings.TrimSpace(licenseID))
	rows, err := db.Query("SELECT id,token_prefix,customer_id,license_id,folder_id,label,max_uses,use_count,expires_at,is_active,created_at FROM device_enrollment_tokens WHERE license_id=? ORDER BY id DESC", licenseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ParkEnrollmentToken
	for rows.Next() {
		var t ParkEnrollmentToken
		var customer sql.NullInt64
		var expiresRaw, createdRaw string
		var active int
		if err := rows.Scan(&t.ID, &t.Prefix, &customer, &t.LicenseID, &t.FolderID, &t.Label, &t.MaxUses, &t.UseCount, &expiresRaw, &active, &createdRaw); err != nil {
			return nil, err
		}
		t.CustomerID = customer.Int64
		t.IsActive = active == 1
		if exp, err := time.Parse(time.RFC3339, expiresRaw); err == nil {
			t.ExpiresAt = exp
		}
		if created, err := time.Parse(time.RFC3339, createdRaw); err == nil {
			t.CreatedAt = created
		}
		if customerID > 0 && t.CustomerID != customerID {
			continue
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// enrollDeviceWithParkTokenTx enrolls a device presenting a park token instead
// of a per-device code. The device row is created directly in the enrolled
// state (version 2) and the token use counter is bumped atomically, all
// inside the caller's transaction. Commits on success.
func enrollDeviceWithParkTokenTx(tx *sql.Tx, db *sql.DB, code, rustdeskID, hostname, osName, publicKey, ip, macAddress, subnetBroadcast, agentVersion string, p DeviceProof) (*Device, error) {
	var tokID int64
	var tokCustomer sql.NullInt64
	var licenseID, folderID string
	var maxUses, useCount int
	var expiresRaw string
	if err := tx.QueryRow("SELECT id,customer_id,license_id,folder_id,max_uses,use_count,expires_at FROM device_enrollment_tokens WHERE token_hash=? AND is_active=1", deviceCodeHash(code)).Scan(&tokID, &tokCustomer, &licenseID, &folderID, &maxUses, &useCount, &expiresRaw); err != nil {
		return nil, ErrDeviceAuthorization
	}
	exp, err := time.Parse(time.RFC3339, expiresRaw)
	if err != nil || !exp.After(time.Now()) {
		return nil, ErrDeviceAuthorization
	}
	if useCount >= maxUses {
		return nil, ErrDeviceLimit
	}
	if !activeDeviceLicense(tx, licenseID) {
		return nil, ErrDeviceAuthorization
	}
	quotas, err := listFleetQuotas(tx, 0, licenseID)
	if err != nil || len(quotas) != 1 {
		return nil, ErrDeviceAuthorization
	}
	// Same downgrade guard as the per-device path.
	if quotas[0].Used > quotas[0].Limit {
		return nil, ErrDeviceLimit
	}
	// Idempotent replay: a runner that lost the first response (GPO/Intune
	// retry, operator re-run, crash before saving state) must not burn a
	// second fleet slot and token use. Same licence + same RustDesk ID +
	// same proof key = same install (a reinstall mints a fresh key, so it
	// still enrolls anew). The proof was already verified by the caller.
	if rustdeskID != "" && publicKey != "" {
		var existingID string
		if err := tx.QueryRow("SELECT device_id FROM devices WHERE license_id=? AND rustdesk_id=? AND device_public_key=? AND enrollment_version=2 AND is_active=1", licenseID, rustdeskID, publicKey).Scan(&existingID); err == nil {
			if err := consumeDeviceNonce(tx, existingID, p); err != nil {
				return nil, err
			}
			if err := tx.Commit(); err != nil {
				return nil, err
			}
			return GetDeviceByID(db, existingID)
		}
	}
	// Atomic cap: concurrent waves cannot overshoot max_uses (immediate
	// transactions serialize writers, and the conditional UPDATE is the
	// arbiter).
	res, err := tx.Exec("UPDATE device_enrollment_tokens SET use_count=use_count+1 WHERE id=? AND use_count<?", tokID, maxUses)
	if err != nil {
		return nil, err
	}
	if n, err := res.RowsAffected(); err != nil || n != 1 {
		return nil, ErrDeviceLimit
	}
	id, err := GenerateDeviceID()
	if err != nil {
		return nil, err
	}
	// No per-device code exists here; keep the UNIQUE column meaningful.
	parkedCode := "PARKED-" + id
	now := time.Now().UTC()
	alias := sanitizeDeviceText(hostname, 100)
	if alias == "" {
		alias = id
	}
	var cust any
	if tokCustomer.Valid && tokCustomer.Int64 > 0 {
		cust = tokCustomer.Int64
	}
	if _, err := tx.Exec("INSERT INTO devices(device_id,customer_id,license_id,permanent_code,rustdesk_id,device_public_key,enrollment_version,folder_id,alias,hostname,os,status,last_seen_at,last_ip,notes,is_active,created_at,updated_at,mac_address,subnet_broadcast,agent_version) VALUES(?,?,?,?,?,?,2,?,?,?,?,'offline',?,?,?,1,?,?,?,?,?)",
		id, cust, licenseID, parkedCode, rustdeskID, publicKey, folderID, alias, hostname, osName, now, ip, "", now, now, macAddress, subnetBroadcast, agentVersion); err != nil {
		return nil, err
	}
	if err := consumeDeviceNonce(tx, id, p); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return GetDeviceByID(db, id)
}

// ParkTokenLicense resolves the licence owning a token row ("" if none).
func ParkTokenLicense(db *sql.DB, id int64) string {
	var licenseID string
	if err := db.QueryRow("SELECT license_id FROM device_enrollment_tokens WHERE id=?", id).Scan(&licenseID); err != nil {
		return ""
	}
	return licenseID
}

// RevokeParkEnrollmentToken immediately disables a token; already enrolled
// devices keep working (revoke them individually to cut access).
func RevokeParkEnrollmentToken(db *sql.DB, customerID int64, licenseID string, id int64) error {
	licenseID = strings.ToUpper(strings.TrimSpace(licenseID))
	// Ownership is enforced inside the UPDATE so a mismatched call revokes
	// nothing.
	query := "UPDATE device_enrollment_tokens SET is_active=0,revoked_at=? WHERE id=? AND license_id=? AND is_active=1"
	args := []any{time.Now().UTC().Format(time.RFC3339), id, licenseID}
	if customerID > 0 {
		query += " AND customer_id=?"
		args = append(args, customerID)
	}
	res, err := db.Exec(query, args...)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil || n == 0 {
		return ErrDeviceAuthorization
	}
	return nil
}
