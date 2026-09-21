package database

import (
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

var (
	ErrFolderNotFound = errors.New("dossier introuvable")
	ErrInvalidFolder  = errors.New("nom de dossier invalide")
)

type DeviceFolder struct {
	ID             int       `json:"id"`
	FolderID       string    `json:"folder_id"`
	CustomerID     int64     `json:"customer_id"`
	LicenseID      string    `json:"license_id"`
	ParentFolderID string    `json:"parent_folder_id"`
	Name           string    `json:"name"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func GenerateFolderID() (string, error) {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return fmt.Sprintf("FLD-%04X-%04X", b[0:2], b[2:4]), nil
}

func validFolderName(name string) bool {
	clean := strings.TrimSpace(name)
	if clean == "" || utf8.RuneCountInString(clean) > 64 {
		return false
	}
	return !strings.ContainsFunc(clean, unicode.IsControl)
}

func CreateDeviceFolder(db *sql.DB, customerID int64, licenseID, name, parentFolderID string) (*DeviceFolder, error) {
	if db == nil {
		return nil, errors.New("base de données non initialisée")
	}
	name = strings.TrimSpace(name)
	if !validFolderName(name) {
		return nil, ErrInvalidFolder
	}
	parentFolderID = strings.TrimSpace(parentFolderID)

	licenseID = strings.ToUpper(strings.TrimSpace(licenseID))

	// Si un dossier parent est spécifié, vérifier qu'il existe et appartient au même propriétaire
	if parentFolderID != "" {
		var exists int
		var err error
		if licenseID != "" {
			err = db.QueryRow("SELECT 1 FROM device_folders WHERE folder_id = ? AND license_id = ?", parentFolderID, licenseID).Scan(&exists)
		} else if customerID > 0 {
			err = db.QueryRow("SELECT 1 FROM device_folders WHERE folder_id = ? AND customer_id = ?", parentFolderID, customerID).Scan(&exists)
		}
		if err != nil || exists != 1 {
			return nil, fmt.Errorf("dossier parent introuvable ou non autorisé")
		}
	}

	var cust any = nil
	if customerID > 0 {
		cust = customerID
	}
	var lic any = nil
	if licenseID != "" {
		lic = licenseID
	}

	now := time.Now().UTC()
	for attempt := 0; attempt < 5; attempt++ {
		fID, err := GenerateFolderID()
		if err != nil {
			return nil, err
		}

		res, err := db.Exec(`
			INSERT INTO device_folders (folder_id, customer_id, license_id, parent_folder_id, name, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, fID, cust, lic, parentFolderID, name, now, now)
		if err != nil {
			if strings.Contains(err.Error(), "UNIQUE constraint failed") {
				continue
			}
			return nil, fmt.Errorf("erreur création dossier: %w", err)
		}

		lastID, _ := res.LastInsertId()
		return &DeviceFolder{
			ID:             int(lastID),
			FolderID:       fID,
			CustomerID:     customerID,
			LicenseID:      licenseID,
			ParentFolderID: parentFolderID,
			Name:           name,
			CreatedAt:      now,
			UpdatedAt:      now,
		}, nil
	}

	return nil, errors.New("impossible de générer un identifiant de dossier unique")
}

func ListDeviceFolders(db *sql.DB, customerID int64, licenseID string) ([]DeviceFolder, error) {
	if db == nil {
		return nil, errors.New("base de données non initialisée")
	}

	licenseID = strings.ToUpper(strings.TrimSpace(licenseID))
	var rows *sql.Rows
	var err error

	if licenseID != "" {
		rows, err = db.Query(`
			SELECT id, folder_id, COALESCE(customer_id, 0), COALESCE(license_id, ''), parent_folder_id, name, created_at, updated_at
			FROM device_folders
			WHERE license_id = ?
			ORDER BY name COLLATE NOCASE ASC
		`, licenseID)
	} else if customerID > 0 {
		rows, err = db.Query(`
			SELECT id, folder_id, COALESCE(customer_id, 0), COALESCE(license_id, ''), parent_folder_id, name, created_at, updated_at
			FROM device_folders
			WHERE customer_id = ?
			ORDER BY name COLLATE NOCASE ASC
		`, customerID)
	} else {
		return []DeviceFolder{}, nil
	}

	if err != nil {
		return nil, fmt.Errorf("erreur récupération dossiers: %w", err)
	}
	defer rows.Close()

	folders := []DeviceFolder{}
	for rows.Next() {
		var f DeviceFolder
		if err := rows.Scan(&f.ID, &f.FolderID, &f.CustomerID, &f.LicenseID, &f.ParentFolderID, &f.Name, &f.CreatedAt, &f.UpdatedAt); err != nil {
			return nil, err
		}
		folders = append(folders, f)
	}

	return folders, rows.Err()
}

func GetDeviceFolder(db *sql.DB, customerID int64, licenseID, folderID string) (*DeviceFolder, error) {
	if db == nil {
		return nil, errors.New("base de données non initialisée")
	}

	folderID = strings.TrimSpace(folderID)
	licenseID = strings.ToUpper(strings.TrimSpace(licenseID))

	var row *sql.Row
	if licenseID != "" {
		row = db.QueryRow(`
			SELECT id, folder_id, COALESCE(customer_id, 0), COALESCE(license_id, ''), parent_folder_id, name, created_at, updated_at
			FROM device_folders
			WHERE folder_id = ? AND license_id = ?
		`, folderID, licenseID)
	} else if customerID > 0 {
		row = db.QueryRow(`
			SELECT id, folder_id, COALESCE(customer_id, 0), COALESCE(license_id, ''), parent_folder_id, name, created_at, updated_at
			FROM device_folders
			WHERE folder_id = ? AND customer_id = ?
		`, folderID, customerID)
	} else {
		return nil, ErrFolderNotFound
	}

	var f DeviceFolder
	if err := row.Scan(&f.ID, &f.FolderID, &f.CustomerID, &f.LicenseID, &f.ParentFolderID, &f.Name, &f.CreatedAt, &f.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrFolderNotFound
		}
		return nil, err
	}

	return &f, nil
}

func UpdateDeviceFolder(db *sql.DB, customerID int64, licenseID, folderID, newName string) error {
	if db == nil {
		return errors.New("base de données non initialisée")
	}
	newName = strings.TrimSpace(newName)
	if !validFolderName(newName) {
		return ErrInvalidFolder
	}
	folderID = strings.TrimSpace(folderID)
	licenseID = strings.ToUpper(strings.TrimSpace(licenseID))
	now := time.Now().UTC()

	var res sql.Result
	var err error
	if licenseID != "" {
		res, err = db.Exec(`
			UPDATE device_folders SET name = ?, updated_at = ?
			WHERE folder_id = ? AND license_id = ?
		`, newName, now, folderID, licenseID)
	} else if customerID > 0 {
		res, err = db.Exec(`
			UPDATE device_folders SET name = ?, updated_at = ?
			WHERE folder_id = ? AND customer_id = ?
		`, newName, now, folderID, customerID)
	} else {
		return ErrFolderNotFound
	}

	if err != nil {
		return fmt.Errorf("erreur renommage dossier: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrFolderNotFound
	}

	return nil
}

// DeleteDeviceFolder supprime un dossier et tous ses sous-dossiers récursivement.
// Tous les postes rattachés à ces dossiers sont réinitialisés à la racine (folder_id = '')
// pour qu'aucune machine ne soit perdue ou supprimée par mégarde.
func DeleteDeviceFolder(db *sql.DB, customerID int64, licenseID, folderID string) error {
	if db == nil {
		return errors.New("base de données non initialisée")
	}
	folderID = strings.TrimSpace(folderID)
	if folderID == "" {
		return ErrFolderNotFound
	}
	licenseID = strings.ToUpper(strings.TrimSpace(licenseID))

	// Vérifier l'existence du dossier cible
	allFolders, err := ListDeviceFolders(db, customerID, licenseID)
	if err != nil {
		return err
	}

	targetExists := false
	for _, f := range allFolders {
		if f.FolderID == folderID {
			targetExists = true
			break
		}
	}
	if !targetExists {
		return ErrFolderNotFound
	}

	// Trouver récursivement tous les identifiants de dossiers et sous-dossiers à supprimer
	foldersToDelete := make(map[string]bool)
	foldersToDelete[folderID] = true

	// Parcours pour inclure les sous-dossiers
	changed := true
	for changed {
		changed = false
		for _, f := range allFolders {
			if f.ParentFolderID != "" && foldersToDelete[f.ParentFolderID] && !foldersToDelete[f.FolderID] {
				foldersToDelete[f.FolderID] = true
				changed = true
			}
		}
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := time.Now().UTC()
	for fID := range foldersToDelete {
		// Réinitialiser les machines contenues vers la racine
		if licenseID != "" {
			_, err = tx.Exec("UPDATE devices SET folder_id = '', updated_at = ? WHERE folder_id = ? AND license_id = ?", now, fID, licenseID)
		} else {
			_, err = tx.Exec("UPDATE devices SET folder_id = '', updated_at = ? WHERE folder_id = ? AND customer_id = ?", now, fID, customerID)
		}
		if err != nil {
			return fmt.Errorf("erreur réinitialisation postes: %w", err)
		}

		// Supprimer le dossier
		if licenseID != "" {
			_, err = tx.Exec("DELETE FROM device_folders WHERE folder_id = ? AND license_id = ?", fID, licenseID)
		} else {
			_, err = tx.Exec("DELETE FROM device_folders WHERE folder_id = ? AND customer_id = ?", fID, customerID)
		}
		if err != nil {
			return fmt.Errorf("erreur suppression dossier %s: %w", fID, err)
		}
	}

	return tx.Commit()
}

// SetDeviceFolder classe un poste dans un dossier spécifié (ou à la racine si folderID = '').
func SetDeviceFolder(db *sql.DB, customerID int64, licenseID, deviceID, folderID string) error {
	if db == nil {
		return errors.New("base de données non initialisée")
	}
	deviceID = strings.TrimSpace(deviceID)
	folderID = strings.TrimSpace(folderID)
	licenseID = strings.ToUpper(strings.TrimSpace(licenseID))

	// Si un dossier est spécifié, vérifier qu'il existe et appartient à l'utilisateur
	if folderID != "" {
		var exists int
		var err error
		if licenseID != "" {
			err = db.QueryRow("SELECT 1 FROM device_folders WHERE folder_id = ? AND license_id = ?", folderID, licenseID).Scan(&exists)
		} else if customerID > 0 {
			err = db.QueryRow("SELECT 1 FROM device_folders WHERE folder_id = ? AND customer_id = ?", folderID, customerID).Scan(&exists)
		}
		if err != nil || exists != 1 {
			return ErrFolderNotFound
		}
	}

	now := time.Now().UTC()
	var res sql.Result
	var err error

	if licenseID != "" {
		res, err = db.Exec("UPDATE devices SET folder_id = ?, updated_at = ? WHERE device_id = ? AND license_id = ? AND is_active = 1", folderID, now, deviceID, licenseID)
	} else if customerID > 0 {
		res, err = db.Exec("UPDATE devices SET folder_id = ?, updated_at = ? WHERE device_id = ? AND customer_id = ? AND is_active = 1", folderID, now, deviceID, customerID)
	} else {
		return ErrDeviceAuthorization
	}

	if err != nil {
		return fmt.Errorf("erreur classement poste: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrDeviceAuthorization
	}

	return nil
}
