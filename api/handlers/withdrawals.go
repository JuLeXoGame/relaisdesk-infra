package handlers

import (
	"api/mailer"
	dbpkg "database"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	netmail "net/mail"
	"strings"
)

type publicWithdrawalRequest struct {
	OrderID string `json:"order_id"`
	Email   string `json:"email"`
	Name    string `json:"name,omitempty"`
}

// PublicWithdrawalHandler records an online withdrawal notice and returns a timestamped receipt.
func PublicWithdrawalHandler(db *sql.DB, mail *mailer.Mailer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req publicWithdrawalRequest
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&req); err != nil {
			writeJSONError(w, "Requête JSON invalide", http.StatusBadRequest)
			return
		}

		req.Email = strings.ToLower(strings.TrimSpace(req.Email))
		parsedEmail, err := netmail.ParseAddress(req.Email)
		if err != nil || parsedEmail.Address != req.Email || len(req.Email) > 254 || strings.ContainsAny(req.Email, "\r\n\t") {
			writeJSONError(w, "Adresse e-mail valide requise", http.StatusBadRequest)
			return
		}

		item, err := dbpkg.CreateWithdrawalRequest(db, req.OrderID, req.Email, req.Name)
		if err != nil {
			writeJSONError(w, err.Error(), http.StatusBadRequest)
			return
		}

		requestedAt := item.RequestedAt.Format("2006-01-02T15:04:05Z07:00")
		if err := EnqueueWithdrawalEmails(db, item.RequestID); err != nil {
			log.Printf("[Withdrawal] Notifications non mises en file pour %s: %v", item.RequestID, err)
		}

		writeJSON(w, http.StatusCreated, map[string]interface{}{
			"request_id":   item.RequestID,
			"order_id":     item.OrderID,
			"requested_at": requestedAt,
			"status":       item.Status,
		})
	}
}

// AdminListWithdrawalsHandler lists recorded notices for processing.
func AdminListWithdrawalsHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		items, err := dbpkg.ListWithdrawalRequests(db, r.URL.Query().Get("status"))
		if err != nil {
			writeJSONError(w, "Erreur lecture demandes de rétractation", http.StatusInternalServerError)
			return
		}
		trials, err := dbpkg.ListTrialWithdrawals(db)
		if err != nil {
			writeJSONError(w, "Erreur lecture rétractations d'essai", 500)
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"withdrawals": items, "count": len(items), "trial_withdrawals": trials})
	}
}
