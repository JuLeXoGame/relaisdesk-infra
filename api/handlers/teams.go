package handlers

import (
	dbpkg "database"
	"database/sql"
	"net/http"
	"strings"
)

func CustomerTeamHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		identity, ok := customerIdentity(r)
		if !ok {
			writeJSONError(w, "Session invalide", 401)
			return
		}
		licenses, err := dbpkg.ListTeamLicenses(db, identity.ID)
		if err != nil {
			writeJSONError(w, "Équipe indisponible", 500)
			return
		}
		members, err := dbpkg.ListOwnedTeamMembers(db, identity.ID)
		if err != nil {
			writeJSONError(w, "Équipe indisponible", 500)
			return
		}
		memberships, err := dbpkg.ListMyTeamMemberships(db, identity.Email)
		if err != nil {
			writeJSONError(w, "Équipe indisponible", 500)
			return
		}
		writeJSON(w, 200, map[string]any{"licenses": licenses, "members": members, "memberships": memberships})
	}
}

type teamInvitationRequest struct {
	LicenseID string   `json:"license_id"`
	Email     string   `json:"email"`
	FolderIDs []string `json:"folder_ids"`
}

func CustomerTeamInviteHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		identity, ok := customerIdentity(r)
		if !ok {
			writeJSONError(w, "Session invalide", 401)
			return
		}
		var req teamInvitationRequest
		if err := decodeSingleJSON(r, &req); err != nil {
			writeJSONError(w, "Requête invalide", 400)
			return
		}
		member, err := dbpkg.CreateTeamInvitation(db, identity.ID, req.LicenseID, req.Email, req.FolderIDs)
		if err != nil {
			writeJSONError(w, "Invitation refusée : vérifiez l’offre, les places disponibles, l’adresse et les dossiers.", 409)
			return
		}
		if err = EnqueueTeamInvitationEmail(db, member.MemberID); err != nil {
			writeJSONError(w, "Invitation créée mais envoi indisponible. Utilisez Renvoyer.", 503)
			return
		}
		writeJSON(w, 201, map[string]any{"member": member, "message": "Invitation mise en file d’envoi."})
	}
}

func CustomerTeamMemberHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		identity, ok := customerIdentity(r)
		if !ok {
			writeJSONError(w, "Session invalide", 401)
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/api/v1/customer/team/members/")
		resend := strings.HasSuffix(id, "/resend")
		if resend {
			id = strings.TrimSuffix(id, "/resend")
		}
		if id == "" || len(id) > 80 || strings.Contains(id, "/") {
			writeJSONError(w, "Utilisateur introuvable", 404)
			return
		}
		var err error
		switch {
		case r.Method == http.MethodPost && resend:
			_, _, err = dbpkg.IssueTeamInvitationToken(db, identity.ID, id)
			if err == nil {
				err = EnqueueTeamInvitationEmail(db, id)
			}
		case r.Method == http.MethodPut && !resend:
			var req struct {
				FolderIDs []string `json:"folder_ids"`
			}
			if e := decodeSingleJSON(r, &req); e != nil {
				writeJSONError(w, "Requête invalide", 400)
				return
			}
			err = dbpkg.UpdateTeamFolders(db, identity.ID, id, req.FolderIDs)
		case r.Method == http.MethodDelete && !resend:
			err = dbpkg.RevokeTeamMember(db, identity.ID, id)
		default:
			writeJSONError(w, "Méthode non autorisée", 405)
			return
		}
		if err != nil {
			writeJSONError(w, "Opération refusée ou invitation expirée", 403)
			return
		}
		writeJSON(w, 200, map[string]bool{"success": true})
	}
}

func TeamAcceptInvitationHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Token string `json:"token"`
		}
		if err := decodeSingleJSON(r, &req); err != nil {
			writeJSONError(w, "Invitation invalide", 400)
			return
		}
		token, email, err := dbpkg.AcceptTeamInvitation(db, req.Token)
		if err != nil {
			writeJSONError(w, "Invitation invalide, expirée ou révoquée", 400)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, 200, map[string]string{"token": token, "email": email})
	}
}

func CustomerTeamTechnicianSessionHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		identity, ok := customerIdentity(r)
		if !ok {
			writeJSONError(w, "Session invalide", 401)
			return
		}
		var req struct {
			LicenseID string `json:"license_id"`
		}
		if err := decodeSingleJSON(r, &req); err != nil {
			writeJSONError(w, "Requête invalide", 400)
			return
		}
		token, err := dbpkg.CreatePersonalTechnicianSession(db, identity.Email, req.LicenseID)
		if err != nil {
			writeJSONError(w, "Accès à cette équipe refusé", 403)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, 200, map[string]any{"token": token, "license_id": req.LicenseID, "restricted_to_folders": technicianIsTeamMember(db, token)})
	}
}
