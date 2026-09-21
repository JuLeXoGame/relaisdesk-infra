package database

import (
	"database/sql"
	"errors"
	"strings"
	"time"
)

var (
	ErrDeviceNoMAC       = errors.New("adresse MAC non renseignée pour ce poste")
	ErrDeviceWakePending = errors.New("un ordre de réveil a déjà été envoyé récemment pour ce poste")
)

// QueueDeviceWake reserves a Wake-on-LAN request for an offline fleet device.
// Returns the target device and the number of online relay peers available on the same license.
func QueueDeviceWake(db *sql.DB, targetDeviceID string, customerID int64, licenseID string) (*Device, int, error) {
	if db == nil {
		return nil, 0, ErrDeviceAuthorization
	}
	targetDeviceID = strings.TrimSpace(targetDeviceID)
	if targetDeviceID == "" {
		return nil, 0, ErrDeviceAuthorization
	}

	dev, err := GetDeviceByID(db, targetDeviceID)
	if err != nil {
		return nil, 0, err
	}

	// Verify caller rights
	if customerID > 0 && dev.CustomerID != customerID {
		return nil, 0, ErrDeviceAuthorization
	}
	if licenseID != "" && !strings.EqualFold(dev.LicenseID, licenseID) {
		return nil, 0, ErrDeviceAuthorization
	}

	mac := NormalizeMAC(dev.MACAddress)
	if mac == "" {
		return dev, 0, ErrDeviceNoMAC
	}

	// Count online peers on the same license
	var onlinePeers int
	err = db.QueryRow(
		"SELECT COUNT(*) FROM devices WHERE license_id = ? AND status = 'online' AND is_active = 1 AND device_id != ?",
		dev.LicenseID, dev.DeviceID,
	).Scan(&onlinePeers)
	if err != nil {
		return nil, 0, err
	}

	// Throttle: don't insert duplicate pending requests within the last 30 seconds
	var recentCount int
	cutoff := time.Now().UTC().Add(-30 * time.Second)
	_ = db.QueryRow(
		"SELECT COUNT(*) FROM device_wake_requests WHERE target_device_id = ? AND status = 'pending' AND created_at > ?",
		dev.DeviceID, cutoff,
	).Scan(&recentCount)
	if recentCount > 0 {
		return dev, onlinePeers, ErrDeviceWakePending
	}

	now := time.Now().UTC()
	_, err = db.Exec(
		"INSERT INTO device_wake_requests(target_device_id, license_id, mac_address, status, created_at) VALUES(?, ?, ?, 'pending', ?)",
		dev.DeviceID, dev.LicenseID, mac, now,
	)
	if err != nil {
		return nil, 0, err
	}

	return dev, onlinePeers, nil
}

// PopPendingDeviceWakes retrieves and dispatches pending Wake-on-LAN requests for a given license.
// It is called during fleet peer heartbeat to instruct online machines to broadcast the magic packet.
func PopPendingDeviceWakes(tx *sql.Tx, licenseID string) ([]string, error) {
	if tx == nil {
		return nil, errors.New("transaction required")
	}
	licenseID = strings.ToUpper(strings.TrimSpace(licenseID))
	if licenseID == "" {
		return nil, nil
	}

	cutoff := time.Now().UTC().Add(-5 * time.Minute)
	rows, err := tx.Query(
		"SELECT id, mac_address FROM device_wake_requests WHERE license_id = ? AND status = 'pending' AND created_at > ? ORDER BY id ASC LIMIT 10",
		licenseID, cutoff,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int
	var macs []string
	seen := make(map[string]bool)

	for rows.Next() {
		var id int
		var mac string
		if err := rows.Scan(&id, &mac); err != nil {
			return nil, err
		}
		ids = append(ids, id)
		norm := NormalizeMAC(mac)
		if norm != "" && !seen[norm] {
			seen[norm] = true
			macs = append(macs, norm)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Mark the requests as dispatched so subsequent heartbeats won't spam
	if len(ids) > 0 {
		now := time.Now().UTC()
		for _, id := range ids {
			if _, err := tx.Exec("UPDATE device_wake_requests SET status = 'dispatched', dispatched_at = ? WHERE id = ?", now, id); err != nil {
				return nil, err
			}
		}
	}

	return macs, nil
}

// UpdateDeviceMAC updates the MAC address of a device.
func UpdateDeviceMAC(db *sql.DB, deviceID, mac string) error {
	if db == nil {
		return ErrDeviceAuthorization
	}
	norm := NormalizeMAC(mac)
	if norm == "" {
		return ErrDeviceNoMAC
	}
	now := time.Now().UTC()
	res, err := db.Exec("UPDATE devices SET mac_address = ?, updated_at = ? WHERE device_id = ? AND is_active = 1", norm, now, deviceID)
	return deviceAffected(res, err)
}
