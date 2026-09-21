package handlers

import (
	dbpkg "database"
	"database/sql"
	"net/http"
	"strings"

	"api/middleware"
)

type technicianInterventionRequest struct {
	ClientReference string `json:"client_reference"`
	Title           string `json:"title"`
	Summary         string `json:"summary"`
}

func TechnicianInterventionsHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		licenseID, ok := r.Context().Value(middleware.TechnicianLicenseContextKey).(string)
		if !ok || licenseID == "" {
			writeJSONError(w, "Authentification requise", http.StatusUnauthorized)
			return
		}
		if r.Method == http.MethodGet {
			items, err := dbpkg.ListInterventionsByLicense(db, licenseID, 500)
			if err != nil {
				writeJSONError(w, "Impossible de charger les interventions", http.StatusInternalServerError)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"interventions": items})
			return
		}
		var request technicianInterventionRequest
		if err := decodeSingleJSON(r, &request); err != nil {
			writeJSONError(w, "Requête invalide", http.StatusBadRequest)
			return
		}
		item, err := dbpkg.CreateIntervention(db, licenseID, nil, request.ClientReference, request.Title)
		if err != nil {
			writeJSONError(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusCreated, item)
	}
}

func TechnicianInterventionActionHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		licenseID, ok := r.Context().Value(middleware.TechnicianLicenseContextKey).(string)
		if !ok || licenseID == "" {
			writeJSONError(w, "Authentification requise", http.StatusUnauthorized)
			return
		}
		trimmed := strings.TrimPrefix(r.URL.Path, "/api/v1/technician/interventions/")
		parts := strings.Split(strings.Trim(trimmed, "/"), "/")
		if len(parts) != 2 || parts[0] == "" {
			writeJSONError(w, "Chemin d'intervention invalide", http.StatusBadRequest)
			return
		}
		interventionID, action := parts[0], parts[1]
		if len(interventionID) > 64 {
			writeJSONError(w, "Référence invalide", http.StatusBadRequest)
			return
		}

		var item *dbpkg.Intervention
		var err error
		switch action {
		case "start":
			item, err = dbpkg.StartIntervention(db, licenseID, interventionID)
		case "complete":
			var request technicianInterventionRequest
			if r.Body != nil {
				_ = decodeSingleJSON(r, &request)
			}
			item, err = dbpkg.CompleteIntervention(db, licenseID, interventionID, request.ClientReference, request.Title, request.Summary)
		case "cancel":
			item, err = dbpkg.CancelIntervention(db, licenseID, interventionID)
		default:
			writeJSONError(w, "Action inconnue", http.StatusNotFound)
			return
		}
		if err != nil {
			writeJSONError(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, item)
	}
}
