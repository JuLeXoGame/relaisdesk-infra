package handlers

import (
	"database/sql"
	"net/http"
	"net/url"
	"strings"

	"api/config"
	dbpkg "database"
)

// CustomerBillingPortalHandler creates a Stripe billing portal session for one
// of the customer's trials so they can update their card, see invoices or
// manage payment details without operator assistance.
func CustomerBillingPortalHandler(db *sql.DB, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		identity, ok := customerIdentity(r)
		if !ok {
			writeJSONError(w, "Session invalide", http.StatusUnauthorized)
			return
		}
		if cfg.StripeSecretKey == "" {
			writeJSONError(w, "Paiement par carte temporairement indisponible", http.StatusServiceUnavailable)
			return
		}
		var req struct {
			ID string `json:"id"`
		}
		if err := decodeSingleJSON(r, &req); err != nil || strings.TrimSpace(req.ID) == "" {
			writeJSONError(w, "Requête invalide", http.StatusBadRequest)
			return
		}
		t, err := dbpkg.GetTrial(db, req.ID)
		if err != nil || t.CustomerID != identity.ID {
			writeJSONError(w, "Abonnement introuvable", http.StatusNotFound)
			return
		}
		if t.StripeCustomerID == "" {
			writeJSONError(w, "Aucun moyen de paiement enregistré pour cet abonnement", http.StatusConflict)
			return
		}
		returnURL, err := customerPortalBaseURL(cfg.PublicWebsiteURL)
		if err != nil {
			writeJSONError(w, "Configuration du portail invalide", http.StatusInternalServerError)
			return
		}
		form := url.Values{}
		form.Set("customer", t.StripeCustomerID)
		form.Set("return_url", returnURL)
		var out struct {
			URL string `json:"url"`
		}
		if err := stripeCall(cfg, http.MethodPost, "billing_portal/sessions", "", form, &out); err != nil {
			writeJSONError(w, "Portail de paiement indisponible", http.StatusBadGateway)
			return
		}
		if out.URL == "" {
			writeJSONError(w, "Portail de paiement indisponible", http.StatusBadGateway)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"url": out.URL})
	}
}
