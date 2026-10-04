package database

import (
	"database/sql"
	"errors"
	"strings"
	"time"
)

var (
	ErrDeviceNotFound       = errors.New("poste introuvable")
	ErrUpdateAlreadyPending = errors.New("une mise à jour est déjà programmée pour ce poste")
)

// QueueDeviceUpdate schedules an over-the-air update for a permanent fleet device.
func QueueDeviceUpdate(db *sql.DB, targetDeviceID string, customerID int64, licenseID string, targetVersion string) error {
	if db == nil {
		return ErrDeviceAuthorization
	}
	targetDeviceID = strings.TrimSpace(targetDeviceID)
	if targetDeviceID == "" {
		return errors.New("identifiant de poste manquant")
	}
	licenseID = strings.ToUpper(strings.TrimSpace(licenseID))
	targetVersion = strings.TrimSpace(targetVersion)
	if targetVersion == "" {
		targetVersion = "1.0.0"
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 1. Verify device exists and belongs to this customer/license
	query := "SELECT id, customer_id, license_id, agent_version FROM devices WHERE device_id=? AND is_active=1"
	var rowID int
	var devCust sql.NullInt64
	var devLic, curVer string
	err = tx.QueryRow(query, targetDeviceID).Scan(&rowID, &devCust, &devLic, &curVer)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrDeviceNotFound
		}
		return err
	}

	if customerID > 0 && devCust.Int64 != customerID {
		return ErrDeviceAuthorization
	}
	if licenseID != "" && devLic != licenseID {
		return ErrDeviceAuthorization
	}

	// 2. Throttle: check if a pending request was already queued in the last 15 seconds
	var recentCount int
	throttleLimit := time.Now().UTC().Add(-15 * time.Second)
	err = tx.QueryRow(`
		SELECT COUNT(*) FROM device_update_requests 
		WHERE device_id=? AND status='pending' AND created_at > ?`,
		targetDeviceID, throttleLimit,
	).Scan(&recentCount)
	if err != nil {
		return err
	}
	if recentCount > 0 {
		return ErrUpdateAlreadyPending
	}

	// 3. Insert the update request
	_, err = tx.Exec(`
		INSERT INTO device_update_requests (device_id, target_version, status, created_at)
		VALUES (?, ?, 'pending', ?)`,
		targetDeviceID, targetVersion, time.Now().UTC(),
	)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// PopPendingDeviceUpdate retrieves and marks as dispatched any pending update for a device.
func PopPendingDeviceUpdate(tx *sql.Tx, deviceID string) (string, error) {
	if tx == nil {
		return "", errors.New("transaction requise")
	}
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return "", nil
	}

	var reqID int
	var targetVersion string
	err := tx.QueryRow(`
		SELECT id, target_version FROM device_update_requests
		WHERE device_id=? AND status='pending'
		ORDER BY id ASC LIMIT 1`,
		deviceID,
	).Scan(&reqID, &targetVersion)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", err
	}

	now := time.Now().UTC()
	_, err = tx.Exec(`
		UPDATE device_update_requests
		SET status='dispatched', dispatched_at=?
		WHERE id=?`,
		now, reqID,
	)
	if err != nil {
		return "", err
	}

	targetVersion = strings.TrimSpace(targetVersion)
	if targetVersion == "" {
		targetVersion = "1.0.0"
	}
	return targetVersion, nil
}

// UpdateDeviceAgentVersion updates the reported agent version of a device.
func UpdateDeviceAgentVersion(db *sql.DB, deviceID, version string) error {
	if db == nil {
		return ErrDeviceAuthorization
	}
	deviceID = strings.TrimSpace(deviceID)
	version = sanitizeDeviceText(version, 64)
	if deviceID == "" || version == "" {
		return errors.New("identifiant ou version manquant")
	}

	now := time.Now().UTC()
	_, err := db.Exec("UPDATE devices SET agent_version=?, updated_at=? WHERE device_id=? AND is_active=1", version, now, deviceID)
	return err
}
