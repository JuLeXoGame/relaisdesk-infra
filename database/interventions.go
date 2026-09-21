package database

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
)

type Intervention struct {
	ID              int        `json:"id"`
	InterventionID  string     `json:"intervention_id"`
	LicenseID       string     `json:"license_id"`
	ViewerCodeID    *int       `json:"viewer_code_id,omitempty"`
	ClientReference string     `json:"client_reference"`
	Title           string     `json:"title"`
	Status          string     `json:"status"`
	StartedAt       *time.Time `json:"started_at,omitempty"`
	EndedAt         *time.Time `json:"ended_at,omitempty"`
	DurationMinutes *int       `json:"duration_minutes,omitempty"`
	Summary         string     `json:"summary"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

func generateInterventionID() (string, error) {
	buffer := make([]byte, 6)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return "INT-" + strings.ToUpper(hex.EncodeToString(buffer)), nil
}

func validateInterventionText(clientReference, title, summary string) error {
	if len(clientReference) > 254 || len(title) > 160 || len(summary) > 4000 ||
		strings.ContainsAny(clientReference+title+summary, "\x00") {
		return errors.New("contenu d'intervention invalide ou trop long")
	}
	if strings.TrimSpace(title) == "" {
		return errors.New("titre d'intervention requis")
	}
	return nil
}

// CreateIntervention creates a professional metadata record. It deliberately
// has no field for screen contents, transferred files, passwords or IP data.
func CreateIntervention(db *sql.DB, licenseID string, viewerCodeID *int, clientReference, title string) (*Intervention, error) {
	licenseID = strings.TrimSpace(licenseID)
	clientReference = strings.TrimSpace(clientReference)
	title = strings.TrimSpace(title)
	if title == "" {
		title = "Assistance à distance"
	}
	if err := validateInterventionText(clientReference, title, ""); err != nil {
		return nil, err
	}
	var exists int
	if err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM licences WHERE license_id = ?)`, licenseID).Scan(&exists); err != nil || exists == 0 {
		return nil, errors.New("licence introuvable")
	}

	for attempt := 0; attempt < 5; attempt++ {
		interventionID, err := generateInterventionID()
		if err != nil {
			return nil, err
		}
		result, err := db.Exec(`
			INSERT INTO interventions (intervention_id, license_id, viewer_code_id, client_reference, title)
			VALUES (?, ?, ?, ?, ?)
		`, interventionID, licenseID, viewerCodeID, clientReference, title)
		if err != nil {
			continue
		}
		id, _ := result.LastInsertId()
		return GetIntervention(db, licenseID, interventionID, int(id))
	}
	return nil, errors.New("impossible de générer la référence d'intervention")
}

func EnsureViewerCodeIntervention(db *sql.DB, code *ViewerCode) (*Intervention, error) {
	if code == nil {
		return nil, errors.New("code viewer requis")
	}
	existing, err := getInterventionByViewerCode(db, code.TechnicianLicenseID, code.ID)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	viewerID := code.ID
	return CreateIntervention(db, code.TechnicianLicenseID, &viewerID, code.ClientEmail, "Assistance à distance")
}

func MarkViewerInterventionReady(db *sql.DB, viewerCodeID int) error {
	_, err := db.Exec(`
		UPDATE interventions
		SET status = CASE WHEN status = 'planned' THEN 'client_ready' ELSE status END,
		    updated_at = CURRENT_TIMESTAMP
		WHERE viewer_code_id = ?
	`, viewerCodeID)
	return err
}

func StartIntervention(db *sql.DB, licenseID, interventionID string) (*Intervention, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := db.Exec(`
		UPDATE interventions
		SET status = 'in_progress', started_at = COALESCE(started_at, ?), ended_at = NULL,
		    duration_minutes = NULL, updated_at = ?
		WHERE intervention_id = ? AND license_id = ? AND status IN ('planned', 'client_ready', 'in_progress')
	`, now, now, interventionID, licenseID)
	if err != nil {
		return nil, err
	}
	if err := ensureInterventionAffected(result); err != nil {
		return nil, err
	}
	return GetIntervention(db, licenseID, interventionID, 0)
}

func CompleteIntervention(db *sql.DB, licenseID, interventionID, clientReference, title, summary string) (*Intervention, error) {
	existing, err := GetIntervention(db, licenseID, interventionID, 0)
	if err != nil {
		return nil, err
	}
	clientReference = strings.TrimSpace(clientReference)
	title = strings.TrimSpace(title)
	summary = strings.TrimSpace(summary)
	if clientReference == "" {
		clientReference = existing.ClientReference
	}
	if title == "" {
		title = existing.Title
	}
	if summary == "" {
		summary = existing.Summary
		if summary == "" {
			summary = "Session d'accès à distance terminée"
		}
	}
	if err := validateInterventionText(clientReference, title, summary); err != nil {
		return nil, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := db.Exec(`
		UPDATE interventions
		SET client_reference = ?, title = ?, summary = ?, status = 'completed',
		    started_at = COALESCE(started_at, ?), ended_at = ?,
		    duration_minutes = MAX(1, CAST(ROUND((julianday(?) - julianday(COALESCE(started_at, ?))) * 1440) AS INTEGER)),
		    updated_at = ?
		WHERE intervention_id = ? AND license_id = ? AND status IN ('planned', 'client_ready', 'in_progress')
	`, clientReference, title, summary, now, now, now, now, now, interventionID, licenseID)
	if err != nil {
		return nil, err
	}
	rows, rowsErr := result.RowsAffected()
	if rowsErr != nil {
		return nil, rowsErr
	}
	if rows == 0 {
		existing, getErr := GetIntervention(db, licenseID, interventionID, 0)
		if getErr == nil && existing.Status == "completed" {
			return existing, nil
		}
		return nil, errors.New("intervention introuvable ou état incompatible")
	}
	return GetIntervention(db, licenseID, interventionID, 0)
}

// CreateDeviceIntervention creates or reuses an in-progress intervention for a permanent device connection.
func CreateDeviceIntervention(db *sql.DB, licenseID, deviceID, alias, hostname string) (*Intervention, error) {
	licenseID = strings.TrimSpace(licenseID)
	alias = strings.TrimSpace(alias)
	hostname = strings.TrimSpace(hostname)
	ref := alias
	if ref == "" {
		ref = hostname
	}
	if ref == "" {
		ref = deviceID
	}
	title := "Accès permanent : " + ref

	// Deduplication: if an intervention for this device / reference was started in the last 5 minutes and is still in_progress, reuse it.
	var existingID string
	err := db.QueryRow(`
		SELECT intervention_id FROM interventions
		WHERE license_id = ? AND client_reference = ? AND status = 'in_progress'
		  AND datetime(COALESCE(started_at, created_at)) >= datetime('now', '-5 minutes')
		ORDER BY id DESC LIMIT 1
	`, licenseID, ref).Scan(&existingID)
	if err == nil && existingID != "" {
		return GetIntervention(db, licenseID, existingID, 0)
	}

	for attempt := 0; attempt < 5; attempt++ {
		interventionID, err := generateInterventionID()
		if err != nil {
			return nil, err
		}
		now := time.Now().UTC().Format(time.RFC3339)
		summary := fmt.Sprintf("Connexion directe au poste permanent %s", ref)
		result, err := db.Exec(`
			INSERT INTO interventions (intervention_id, license_id, client_reference, title, status, started_at, summary)
			VALUES (?, ?, ?, ?, 'in_progress', ?, ?)
		`, interventionID, licenseID, ref, title, now, summary)
		if err != nil {
			continue
		}
		id, _ := result.LastInsertId()
		return GetIntervention(db, licenseID, interventionID, int(id))
	}
	return nil, errors.New("impossible de générer la référence d'intervention pour le poste")
}

// StartInterventionByViewerCode finds the intervention associated with a viewer code and marks it in_progress.
func StartInterventionByViewerCode(db *sql.DB, licenseID, code string) (*Intervention, error) {
	code = NormalizeViewerCode(code)
	vc, err := getViewerCodeForValidation(db, code)
	if err != nil {
		return nil, errors.New("code viewer introuvable")
	}
	if vc.TechnicianLicenseID != licenseID {
		return nil, errors.New("code non associé à cette licence")
	}
	existing, err := getInterventionByViewerCode(db, licenseID, vc.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			existing, err = EnsureViewerCodeIntervention(db, vc)
			if err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}
	return StartIntervention(db, licenseID, existing.InterventionID)
}

func CancelIntervention(db *sql.DB, licenseID, interventionID string) (*Intervention, error) {
	result, err := db.Exec(`
		UPDATE interventions SET status = 'cancelled', updated_at = CURRENT_TIMESTAMP
		WHERE intervention_id = ? AND license_id = ? AND status != 'completed'
	`, interventionID, licenseID)
	if err != nil {
		return nil, err
	}
	if err := ensureInterventionAffected(result); err != nil {
		return nil, err
	}
	return GetIntervention(db, licenseID, interventionID, 0)
}

func ensureInterventionAffected(result sql.Result) error {
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return errors.New("intervention introuvable ou état incompatible")
	}
	return nil
}

func GetIntervention(db *sql.DB, licenseID, interventionID string, rowID int) (*Intervention, error) {
	where := "intervention_id = ? AND license_id = ?"
	args := []any{interventionID, licenseID}
	if rowID > 0 {
		where = "id = ? AND license_id = ?"
		args = []any{rowID, licenseID}
	}
	return scanIntervention(db.QueryRow(`
		SELECT id, intervention_id, license_id, viewer_code_id, COALESCE(client_reference, ''), title,
		       status, started_at, ended_at, duration_minutes, COALESCE(summary, ''), created_at, updated_at
		FROM interventions WHERE `+where, args...))
}

// GetCustomerIntervention returns one intervention only when it belongs to a
// licence owned by the given customer.
func GetCustomerIntervention(db *sql.DB, customerID int64, interventionID string) (*Intervention, error) {
	return scanIntervention(db.QueryRow(`
		SELECT interventions.id, interventions.intervention_id, interventions.license_id,
		       interventions.viewer_code_id, COALESCE(interventions.client_reference, ''), interventions.title,
		       interventions.status, interventions.started_at, interventions.ended_at,
		       interventions.duration_minutes, COALESCE(interventions.summary, ''),
		       interventions.created_at, interventions.updated_at
		FROM interventions
		JOIN licences l ON l.license_id = interventions.license_id
		WHERE interventions.intervention_id = ? AND l.customer_id = ?
	`, strings.TrimSpace(interventionID), customerID))
}

func getInterventionByViewerCode(db *sql.DB, licenseID string, viewerCodeID int) (*Intervention, error) {
	return scanIntervention(db.QueryRow(`
		SELECT id, intervention_id, license_id, viewer_code_id, COALESCE(client_reference, ''), title,
		       status, started_at, ended_at, duration_minutes, COALESCE(summary, ''), created_at, updated_at
		FROM interventions WHERE license_id = ? AND viewer_code_id = ? LIMIT 1
	`, licenseID, viewerCodeID))
}

func ListInterventionsByLicense(db *sql.DB, licenseID string, limit int) ([]Intervention, error) {
	if limit <= 0 || limit > 1000 {
		limit = 250
	}
	return listInterventions(db, `WHERE license_id = ?`, []any{licenseID}, limit)
}

func ListInterventionsByCustomer(db *sql.DB, email string, limit int) ([]Intervention, error) {
	identity, err := GetCustomerIdentityByEmail(db, email)
	if err != nil {
		return nil, err
	}
	return ListInterventionsByCustomerID(db, identity.ID, limit)
}

func ListInterventionsByCustomerID(db *sql.DB, customerID int64, limit int) ([]Intervention, error) {
	if limit <= 0 || limit > 1000 {
		limit = 250
	}
	return listInterventions(db, `
		JOIN licences l ON l.license_id = interventions.license_id
		WHERE l.customer_id = ?
	`, []any{customerID}, limit)
}

func listInterventions(db *sql.DB, clause string, args []any, limit int) ([]Intervention, error) {
	query := `
		SELECT interventions.id, intervention_id, interventions.license_id, viewer_code_id,
		       COALESCE(client_reference, ''), title, interventions.status, started_at, ended_at,
		       duration_minutes, COALESCE(summary, ''), interventions.created_at, interventions.updated_at
		FROM interventions ` + clause + `
		ORDER BY COALESCE(started_at, interventions.created_at) DESC
		LIMIT ?`
	args = append(args, limit)
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Intervention, 0)
	for rows.Next() {
		item, err := scanInterventionRows(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	return items, rows.Err()
}

type interventionScanner interface {
	Scan(dest ...any) error
}

func scanIntervention(scanner interventionScanner) (*Intervention, error) {
	var item Intervention
	var viewerID sql.NullInt64
	var startedRaw, endedRaw any
	var duration sql.NullInt64
	var createdRaw, updatedRaw any
	if err := scanner.Scan(
		&item.ID, &item.InterventionID, &item.LicenseID, &viewerID, &item.ClientReference,
		&item.Title, &item.Status, &startedRaw, &endedRaw, &duration, &item.Summary,
		&createdRaw, &updatedRaw,
	); err != nil {
		return nil, err
	}
	if viewerID.Valid {
		value := int(viewerID.Int64)
		item.ViewerCodeID = &value
	}
	if duration.Valid {
		value := int(duration.Int64)
		item.DurationMinutes = &value
	}
	if startedRaw != nil {
		if value, err := ParseSQLiteTime(startedRaw); err == nil {
			item.StartedAt = &value
		}
	}
	if endedRaw != nil {
		if value, err := ParseSQLiteTime(endedRaw); err == nil {
			item.EndedAt = &value
		}
	}
	item.CreatedAt, _ = ParseSQLiteTime(createdRaw)
	item.UpdatedAt, _ = ParseSQLiteTime(updatedRaw)
	return &item, nil
}

func scanInterventionRows(rows *sql.Rows) (*Intervention, error) {
	return scanIntervention(rows)
}

func CustomerOwnsLicense(db *sql.DB, email, licenseID string) bool {
	identity, err := GetCustomerIdentityByEmail(db, email)
	if err != nil {
		return false
	}
	return CustomerOwnsLicenseID(db, identity.ID, licenseID)
}

func CustomerOwnsLicenseID(db *sql.DB, customerID int64, licenseID string) bool {
	var owns int
	err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM licences WHERE license_id = ? AND customer_id = ?)`,
		strings.TrimSpace(licenseID), customerID).Scan(&owns)
	return err == nil && owns == 1
}

func InterventionCSV(items []Intervention) string {
	// Kept in the database package only as a compact, deterministic export
	// representation; HTTP content-disposition is handled by the API.
	var builder strings.Builder
	builder.WriteString("reference;licence;client;titre;statut;debut;fin;duree_minutes;compte_rendu\n")
	for _, item := range items {
		fields := []string{item.InterventionID, item.LicenseID, item.ClientReference, item.Title, item.Status, formatOptionalTime(item.StartedAt), formatOptionalTime(item.EndedAt), "", item.Summary}
		if item.DurationMinutes != nil {
			fields[7] = fmt.Sprintf("%d", *item.DurationMinutes)
		}
		for index, field := range fields {
			if index > 0 {
				builder.WriteByte(';')
			}
			builder.WriteString(csvField(field))
		}
		builder.WriteByte('\n')
	}
	return builder.String()
}

func formatOptionalTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

func csvField(value string) string {
	// Prefix potentially executable spreadsheet formulas and quote per RFC 4180.
	trimmed := strings.TrimLeftFunc(value, unicode.IsSpace)
	if strings.HasPrefix(trimmed, "=") || strings.HasPrefix(trimmed, "+") || strings.HasPrefix(trimmed, "-") || strings.HasPrefix(trimmed, "@") {
		value = "'" + value
	}
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}
