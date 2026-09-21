package database

import (
	"database/sql"
	"errors"
	"strings"
	"time"
)

const (
	StarterManagedDevices          = 500
	ProManagedDevices              = 1000
	UltraBaseManagedDevices        = 2000
	UltraDevicesPerExtraTechnician = 5
	UltraMaxManagedDevices         = 4500
)

var ErrDeviceLimit = errors.New("quota de postes enregistrés atteint : supprimez un poste du parc ou choisissez une offre supérieure")

// ManagedDeviceLimit is separate from concurrent technician sessions and from
// temporary assistance codes. Commercial order/trial names take precedence over
// legacy capacity-based inference. No price or connection limit is changed.
func ManagedDeviceLimit(plan string, technicians int) int {
	switch strings.ToLower(strings.TrimSpace(plan)) {
	case "starter":
		return StarterManagedDevices
	case "pro":
		return ProManagedDevices
	case "ultra", "custom", "personnalise", "personnalisé":
	default:
		if technicians <= 1 {
			return StarterManagedDevices
		}
		if technicians < 10 {
			return ProManagedDevices
		}
	}
	if technicians < 10 {
		technicians = 10
	}
	if technicians >= 500 {
		// The maximum tier includes a 50-device rounding bonus (4450 -> 4500).
		return UltraMaxManagedDevices
	}
	return UltraBaseManagedDevices + (technicians-10)*UltraDevicesPerExtraTechnician
}

type FleetQuota struct {
	LicenseID   string    `json:"license_id"`
	CustomerID  int64     `json:"-"`
	Plan        string    `json:"plan"`
	Technicians int       `json:"technicians"`
	Status      string    `json:"license_status"`
	ExpiresAt   time.Time `json:"expires_at"`
	Limit       int       `json:"limit"`
	Registered  int       `json:"registered"`
	Reserved    int       `json:"reserved"`
	Used        int       `json:"used"`
	Remaining   int       `json:"remaining"`
	CanEnroll   bool      `json:"can_enroll"`
}

type fleetQuerier interface {
	Query(string, ...any) (*sql.Rows, error)
}

// Expired pending invitations do not consume a slot. Offline and legacy
// registered devices do; deleting/revoking a device immediately releases it.
func listFleetQuotas(q fleetQuerier, customerID int64, licenseID string) ([]FleetQuota, error) {
	where := "l.customer_id=?"
	var owner any = customerID
	if licenseID != "" {
		where = "l.license_id=?"
		owner = strings.ToUpper(strings.TrimSpace(licenseID))
	}
	now := time.Now().UTC()
	rows, err := q.Query(`SELECT l.license_id,COALESCE(l.customer_id,0),l.max_connections,l.status,l.expires_at,
		COALESCE((SELECT o.plan FROM orders o WHERE o.license_id=l.license_id AND o.status='paid' ORDER BY o.id DESC LIMIT 1),
		(SELECT t.plan FROM trial_applications t WHERE t.license_id=l.license_id LIMIT 1),'') AS plan,
		SUM(CASE WHEN d.id IS NOT NULL AND d.enrollment_version<>1 THEN 1 ELSE 0 END),
		SUM(CASE WHEN d.enrollment_version=1 AND d.created_at>? THEN 1 ELSE 0 END)
		FROM licences l LEFT JOIN devices d ON d.license_id=l.license_id AND d.is_active=1
		WHERE `+where+` GROUP BY l.license_id ORDER BY l.id`, now.Add(-DeviceEnrollmentTTL), owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []FleetQuota{}
	for rows.Next() {
		var quota FleetQuota
		if err := rows.Scan(&quota.LicenseID, &quota.CustomerID, &quota.Technicians, &quota.Status, &quota.ExpiresAt, &quota.Plan, &quota.Registered, &quota.Reserved); err != nil {
			return nil, err
		}
		quota.Limit = ManagedDeviceLimit(quota.Plan, quota.Technicians)
		quota.Used = quota.Registered + quota.Reserved
		quota.Remaining = max(0, quota.Limit-quota.Used)
		quota.CanEnroll = quota.Status == "active" && quota.ExpiresAt.After(now) && quota.Remaining > 0
		result = append(result, quota)
	}
	return result, rows.Err()
}

func ListFleetQuotas(db *sql.DB, customerID int64, licenseID string) ([]FleetQuota, error) {
	if db == nil {
		return nil, ErrDeviceAuthorization
	}
	return listFleetQuotas(db, customerID, licenseID)
}
