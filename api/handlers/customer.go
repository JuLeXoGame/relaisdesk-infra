package handlers

import (
	dbpkg "database"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	netmail "net/mail"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"api/config"
	"api/invoice"
	"api/mailer"
	"api/middleware"
)

const customerMagicLinkLifetime = 15 * time.Minute

type customerLoginRequest struct {
	Email string `json:"email"`
}

type customerLoginPasswordRequest struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DeviceToken string `json:"device_token,omitempty"`
}

type customerSetPasswordRequest struct {
	CurrentCode   string   `json:"current_2fa_code,omitempty"`
	Token         string   `json:"token"`
	Password      string   `json:"password"`
	TotpSecret    string   `json:"totp_secret,omitempty"`
	TotpCode      string   `json:"totp_code,omitempty"`
	RecoveryCodes []string `json:"recovery_codes,omitempty"`
}

type customerChangePasswordRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

type customerVerifyRequest struct {
	Token string `json:"token"`
	Code  string `json:"code,omitempty"`
}

type customerRenewalRequest struct {
	PaymentMethod                 string `json:"payment_method"`
	TermsVersion                  string `json:"terms_version"`
	TermsAccepted                 bool   `json:"terms_accepted"`
	ImmediatePerformanceRequested bool   `json:"immediate_performance_requested"`
}

type customerPreferencesRequest struct {
	RenewalRemindersEnabled bool `json:"renewal_reminders_enabled"`
}

type customerInterventionRequest struct {
	LicenseID       string `json:"license_id"`
	ClientReference string `json:"client_reference"`
	Title           string `json:"title"`
}

type customerInterventionActionRequest struct {
	ClientReference string `json:"client_reference"`
	Title           string `json:"title"`
	Summary         string `json:"summary"`
}

type customerLicenseView struct {
	LicenseID      string `json:"license_id"`
	Status         string `json:"status"`
	CreatedAt      string `json:"created_at"`
	ExpiresAt      string `json:"expires_at"`
	MaxConnections int    `json:"max_connections"`
	PendingRenewal bool   `json:"pending_renewal"`
	Renewable      bool   `json:"renewable"`
}

type customerOrderView struct {
	OrderID       string  `json:"order_id"`
	Plan          string  `json:"plan"`
	Technicians   int     `json:"technicians"`
	Price         float64 `json:"price"`
	PaymentMethod string  `json:"payment_method"`
	Status        string  `json:"status"`
	OrderKind     string  `json:"order_kind"`
	LicenseID     string  `json:"license_id,omitempty"`
	InvoiceNumber string  `json:"invoice_number,omitempty"`
	CreatedAt     string  `json:"created_at"`
	PaidAt        string  `json:"paid_at,omitempty"`
}

type customerInvoiceView struct {
	InvoiceNumber string  `json:"invoice_number"`
	OrderID       string  `json:"order_id"`
	Plan          string  `json:"plan"`
	Technicians   int     `json:"technicians"`
	AmountTTC     float64 `json:"amount_ttc"`
	Status        string  `json:"status"`
	CreatedAt     string  `json:"created_at"`
}

// CustomerLoginRequestHandler never reveals whether an address exists.
func CustomerLoginRequestHandler(db *sql.DB, cfg *config.Config, mail *mailer.Mailer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var request customerLoginRequest
		if err := decodeSingleJSON(r, &request); err != nil {
			writeJSONError(w, "Requête invalide", http.StatusBadRequest)
			return
		}
		request.Email = strings.ToLower(strings.TrimSpace(request.Email))
		parsed, err := netmail.ParseAddress(request.Email)
		if err != nil || parsed.Address != request.Email || len(request.Email) > 254 || strings.ContainsAny(request.Email, "\r\n\t") {
			writeJSONError(w, "Adresse e-mail invalide", http.StatusBadRequest)
			return
		}
		eligible, err := dbpkg.CustomerLoginEligible(db, request.Email)
		if err != nil {
			log.Printf("[Customer Login] préparation impossible: %v", err)
			writeJSONError(w, "Impossible de préparer la connexion", http.StatusInternalServerError)
			return
		}
		if eligible {
			if err := EnqueueCustomerLoginEmail(db, request.Email); err != nil {
				log.Printf("[Customer Login] mise en file impossible: %v", err)
			}
		}
		writeJSON(w, http.StatusAccepted, map[string]string{
			"message": "Si cette adresse correspond à un compte client, un lien de connexion vient d'être envoyé.",
		})
	}
}

func CustomerLoginVerifyHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var request customerVerifyRequest
		if err := decodeSingleJSON(r, &request); err != nil || len(request.Token) < 32 || len(request.Token) > 256 {
			writeJSONError(w, "Lien invalide ou expiré", http.StatusUnauthorized)
			return
		}
		session, email, err := dbpkg.ConsumeCustomerLoginToken(db, request.Token, request.Code)
		if err != nil {
			if errors.Is(err, dbpkg.ErrMFARequired) {
				// 200 like every other 2FA challenge (password, Google,
				// admin): this is a legitimate next step, not a failure,
				// and 401 here would burn the IP-ban budget on each MFA
				// login (StrictAuthLimiter counts 401/403).
				writeJSON(w, http.StatusOK, map[string]any{"requires_2fa": true})
				return
			}
			writeJSONError(w, "Lien invalide ou expiré", http.StatusUnauthorized)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"token": session, "email": email})
	}
}

// CustomerLoginWithPasswordHandler authenticates a customer using their email and password.
func CustomerLoginWithPasswordHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var request customerLoginPasswordRequest
		if err := decodeSingleJSON(r, &request); err != nil {
			writeJSONError(w, "Requête invalide", http.StatusBadRequest)
			return
		}
		request.Email = strings.ToLower(strings.TrimSpace(request.Email))
		request.Password = strings.TrimSpace(request.Password)
		if request.Email == "" || request.Password == "" {
			writeJSONError(w, "Adresse e-mail et mot de passe requis", http.StatusBadRequest)
			return
		}

		request.DeviceToken = strings.TrimSpace(request.DeviceToken)

		authRes, err := dbpkg.ValidateCustomerPasswordWith2FA(db, request.Email, request.Password, request.DeviceToken)
		if err != nil {
			// Generic message in all cases: revealing "no password set"
			// discloses account existence (enumeration oracle). The login
			// page already links to password recovery.
			time.Sleep(300 * time.Millisecond) // Throttling against distributed brute force
			writeJSONError(w, "Identifiants incorrects", http.StatusUnauthorized)
			return
		}

		if authRes.Requires2FA {
			writeJSON(w, http.StatusOK, map[string]any{
				"requires_2fa":    true,
				"challenge_token": authRes.ChallengeToken,
				"customer_id":     authRes.CustomerID,
				"email":           authRes.Email,
			})
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{
			"token":       authRes.SessionToken,
			"customer_id": authRes.CustomerID,
			"email":       authRes.Email,
		})
	}
}

type customerGoogleLoginRequest struct {
	Credential string `json:"credential"`
}

type googleTokenInfo struct {
	Iss              string `json:"iss"`
	Sub              string `json:"sub"`
	Azp              string `json:"azp"`
	Aud              string `json:"aud"`
	Email            string `json:"email"`
	EmailVerified    any    `json:"email_verified"`
	HostedDomain     string `json:"hd"`
	Name             string `json:"name"`
	Picture          string `json:"picture"`
	Exp              string `json:"exp"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

func verifyGoogleIDToken(credential, expectedClientID string) (*googleTokenInfo, error) {
	if strings.TrimSpace(expectedClientID) == "" {
		return nil, errors.New("connexion Google non configurée")
	}
	credential = strings.TrimSpace(credential)
	if credential == "" || len(credential) > 16384 {
		return nil, errors.New("jeton Google manquant")
	}
	client := &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Get("https://oauth2.googleapis.com/tokeninfo?id_token=" + url.QueryEscape(credential))
	if err != nil {
		// url.Error contains the credential in the query string: never log it.
		return nil, errors.New("impossible de vérifier le jeton auprès de Google")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("jeton Google invalide ou expiré")
	}

	var info googleTokenInfo
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&info); err != nil {
		return nil, fmt.Errorf("réponse Google invalide: %w", err)
	}

	if info.Error != "" {
		return nil, fmt.Errorf("erreur Google: %s", info.Error)
	}

	if err := validateGoogleClaims(&info, expectedClientID, time.Now()); err != nil {
		return nil, err
	}
	return &info, nil
}

func validateGoogleClaims(info *googleTokenInfo, expectedClientID string, now time.Time) error {
	if info == nil || strings.TrimSpace(expectedClientID) == "" || info.Aud != expectedClientID || (info.Azp != "" && info.Azp != expectedClientID) {
		return errors.New("audience du jeton Google non valide")
	}

	// Verify issuer
	if info.Iss != "accounts.google.com" && info.Iss != "https://accounts.google.com" {
		return errors.New("émetteur du jeton Google non valide")
	}
	expires, err := strconv.ParseInt(info.Exp, 10, 64)
	if err != nil || expires <= now.Unix() || strings.TrimSpace(info.Sub) == "" {
		return errors.New("jeton Google expiré ou identité absente")
	}

	// Verify email_verified
	isVerified := false
	switch v := info.EmailVerified.(type) {
	case bool:
		isVerified = v
	case string:
		isVerified = strings.ToLower(v) == "true" || v == "1"
	}
	if !isVerified {
		return errors.New("adresse e-mail Google non vérifiée")
	}
	address, err := netmail.ParseAddress(info.Email)
	if err != nil || address.Address != info.Email || (!strings.HasSuffix(strings.ToLower(info.Email), "@gmail.com") && strings.TrimSpace(info.HostedDomain) == "") {
		// Google is not authoritative for a third-party mailbox without hd.
		return errors.New("utilisez la connexion par mot de passe pour cette adresse e-mail")
	}
	return nil
}

var verifyGoogleToken = verifyGoogleIDToken

// CustomerGoogleLoginHandler authenticates an eligible customer using their verified Google account credential.
func CustomerGoogleLoginHandler(db *sql.DB, expectedClientID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.TrimSpace(expectedClientID) == "" {
			writeJSONError(w, "Connexion Google non configurée", http.StatusServiceUnavailable)
			return
		}
		var request customerGoogleLoginRequest
		if err := decodeSingleJSON(r, &request); err != nil || strings.TrimSpace(request.Credential) == "" {
			writeJSONError(w, "Jeton Google manquant ou invalide", http.StatusBadRequest)
			return
		}

		info, err := verifyGoogleToken(request.Credential, expectedClientID)
		if err != nil {
			log.Printf("[Customer Google Login] échec validation: %v", err)
			writeJSONError(w, "Échec de l'authentification Google", http.StatusUnauthorized)
			return
		}

		result, err := dbpkg.AuthenticateCustomerGoogle(db, info.Email)
		if err != nil {
			log.Printf("[Customer Google Login] aucun compte client éligible pour %s: %v", info.Email, err)
			writeJSON(w, http.StatusNotFound, map[string]any{
				"error": "Aucun compte client ou commande trouvé avec cette adresse e-mail Google.",
				"email": info.Email,
			})
			return
		}

		if result.Requires2FA {
			writeJSON(w, http.StatusOK, map[string]any{"requires_2fa": true, "challenge_token": result.ChallengeToken, "email_code_allowed": false})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{
			"token":       result.SessionToken,
			"customer_id": result.CustomerID,
			"email":       result.Email,
		})
	}
}

// CustomerSetPasswordWithTokenHandler allows setting a new password using a valid email token.
func CustomerSetPasswordWithTokenHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var request customerSetPasswordRequest
		if err := decodeSingleJSON(r, &request); err != nil {
			writeJSONError(w, "Requête invalide", http.StatusBadRequest)
			return
		}
		request.Token = strings.TrimSpace(request.Token)
		request.Password = strings.TrimSpace(request.Password)
		if len(request.Token) < 32 || len(request.Token) > 256 {
			writeJSONError(w, "Lien invalide ou expiré", http.StatusBadRequest)
			return
		}
		if len(request.Password) < 8 {
			writeJSONError(w, "Le mot de passe doit contenir au moins 8 caractères", http.StatusBadRequest)
			return
		}

		sessionToken, email, customerID, err := dbpkg.ResetCustomerPassword(db, request.Token, request.Password, request.CurrentCode, request.TotpSecret, request.TotpCode, request.RecoveryCodes)
		if err != nil {
			writeJSONError(w, err.Error(), http.StatusBadRequest)
			return
		}

		totpActivated := request.TotpSecret != ""

		resp := map[string]string{
			"token":       sessionToken,
			"email":       email,
			"customer_id": customerID,
			"message":     "Mot de passe enregistré avec succès.",
		}
		if totpActivated {
			resp["totp_activated"] = "true"
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

// CustomerChangePasswordHandler allows changing the password when already authenticated.
func CustomerChangePasswordHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		identity, ok := customerIdentity(r)
		if !ok {
			writeJSONError(w, "Session client invalide", http.StatusUnauthorized)
			return
		}
		var request customerChangePasswordRequest
		if err := decodeSingleJSON(r, &request); err != nil {
			writeJSONError(w, "Requête invalide", http.StatusBadRequest)
			return
		}
		if len(strings.TrimSpace(request.NewPassword)) < 8 {
			writeJSONError(w, "Le nouveau mot de passe doit contenir au moins 8 caractères", http.StatusBadRequest)
			return
		}
		if err := dbpkg.ChangeCustomerPassword(db, identity.Email, request.OldPassword, request.NewPassword); err != nil {
			writeJSONError(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"message": "Mot de passe modifié avec succès."})
	}
}

func CustomerLogoutHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Fields(r.Header.Get("Authorization"))
		if len(parts) == 2 && parts[0] == "Bearer" {
			_ = dbpkg.DeleteCustomerSession(db, parts[1])
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "logged_out"})
	}
}

func CustomerDashboardHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		identity, ok := customerIdentity(r)
		if !ok {
			writeJSONError(w, "Session client invalide", http.StatusUnauthorized)
			return
		}
		licenses, err := dbpkg.ListLicensesByCustomerID(db, identity.ID)
		if err != nil {
			writeJSONError(w, "Impossible de charger les licences", http.StatusInternalServerError)
			return
		}
		licenseViews := make([]customerLicenseView, 0, len(licenses))
		for _, lic := range licenses {
			recurring, err := dbpkg.LicenseHasSubscription(db, lic.LicenseID)
			if err != nil {
				writeJSONError(w, "Impossible de charger les abonnements", 500)
				return
			}
			pending, pendingErr := dbpkg.HasPendingRenewal(db, lic.LicenseID)
			if pendingErr != nil {
				writeJSONError(w, "Impossible de charger les renouvellements", http.StatusInternalServerError)
				return
			}
			status := lic.Status
			if status == "active" && !lic.ExpiresAt.After(time.Now().UTC()) {
				status = "expired"
			}
			licenseViews = append(licenseViews, customerLicenseView{
				LicenseID: lic.LicenseID, Status: status, CreatedAt: lic.CreatedAt.UTC().Format(time.RFC3339),
				ExpiresAt: lic.ExpiresAt.UTC().Format(time.RFC3339), MaxConnections: lic.MaxConnections,
				PendingRenewal: pending, Renewable: lic.Status != "revoked" && !recurring && lic.ExpiresAt.Year() != 9999,
			})
		}
		orders, err := dbpkg.ListOrdersByCustomerID(db, identity.ID)
		if err != nil {
			writeJSONError(w, "Impossible de charger les commandes", http.StatusInternalServerError)
			return
		}
		orderViews := make([]customerOrderView, 0, len(orders))
		for _, order := range orders {
			view := customerOrderView{
				OrderID: order.OrderID, Plan: order.Plan, Technicians: order.Technicians, Price: order.Price,
				PaymentMethod: order.PaymentMethod, Status: order.Status, OrderKind: order.OrderKind,
				LicenseID: order.LicenseID, InvoiceNumber: order.InvoiceNumber, CreatedAt: order.CreatedAt.UTC().Format(time.RFC3339),
			}
			if order.PaidAt != nil {
				view.PaidAt = order.PaidAt.UTC().Format(time.RFC3339)
			}
			orderViews = append(orderViews, view)
		}
		invoices, err := dbpkg.ListInvoicesByCustomerID(db, identity.ID)
		if err != nil {
			writeJSONError(w, "Impossible de charger les factures", http.StatusInternalServerError)
			return
		}
		invoiceViews := make([]customerInvoiceView, 0, len(invoices))
		for _, item := range invoices {
			invoiceViews = append(invoiceViews, customerInvoiceView{
				InvoiceNumber: item.InvoiceNumber, OrderID: item.OrderID, Plan: item.Plan,
				Technicians: item.Technicians, AmountTTC: item.AmountTTC, Status: item.Status,
				CreatedAt: item.CreatedAt.UTC().Format(time.RFC3339),
			})
		}
		interventions, err := dbpkg.ListInterventionsByCustomerID(db, identity.ID, 250)
		if err != nil {
			writeJSONError(w, "Impossible de charger les interventions", http.StatusInternalServerError)
			return
		}
		reminders, _ := dbpkg.CustomerRenewalRemindersEnabledByID(db, identity.ID)
		subscriptions, err := dbpkg.ListCustomerTrials(db, identity.ID)
		if err != nil {
			writeJSONError(w, "Impossible de charger les abonnements", 500)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"email": identity.Email, "customer_id": identity.PublicID, "customer_role": identity.Role,
			"display_name": identity.DisplayName, "billing_email": identity.BillingEmail,
			"renewal_reminders_enabled": reminders, "licenses": licenseViews,
			"orders": orderViews, "invoices": invoiceViews, "interventions": interventions,
			"subscriptions": subscriptions,
		})
	}
}

func CustomerPreferencesHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		identity, ok := customerIdentity(r)
		if !ok {
			writeJSONError(w, "Session client invalide", http.StatusUnauthorized)
			return
		}
		var request customerPreferencesRequest
		if err := decodeSingleJSON(r, &request); err != nil {
			writeJSONError(w, "Requête invalide", http.StatusBadRequest)
			return
		}
		if err := dbpkg.SetCustomerRenewalRemindersByID(db, identity.ID, request.RenewalRemindersEnabled); err != nil {
			writeJSONError(w, "Impossible d'enregistrer la préférence", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"renewal_reminders_enabled": request.RenewalRemindersEnabled})
	}
}

func CustomerRenewLicenseHandler(db *sql.DB, cfg *config.Config, mail *mailer.Mailer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		identity, ok := customerIdentity(r)
		if !ok {
			writeJSONError(w, "Session client invalide", http.StatusUnauthorized)
			return
		}
		licenseID := customerPathID(r.URL.Path, "/api/v1/customer/licenses/", "/renew")
		if licenseID == "" || !dbpkg.CustomerOwnsLicenseID(db, identity.ID, licenseID) {
			writeJSONError(w, "Licence introuvable", http.StatusNotFound)
			return
		}
		pending, err := dbpkg.HasPendingRenewal(db, licenseID)
		if err != nil {
			writeJSONError(w, "Impossible de vérifier le renouvellement", http.StatusInternalServerError)
			return
		}
		if pending {
			writeJSONError(w, "Un renouvellement est déjà en attente pour cette licence", http.StatusConflict)
			return
		}
		var request customerRenewalRequest
		if err := decodeSingleJSON(r, &request); err != nil {
			writeJSONError(w, "Requête invalide", http.StatusBadRequest)
			return
		}
		request.PaymentMethod = strings.ToLower(strings.TrimSpace(request.PaymentMethod))
		if !dbpkg.ValidPaymentMethod(request.PaymentMethod) {
			writeJSONError(w, "Méthode de paiement invalide (choix: stripe, bank_transfer, crypto_btc, crypto_xrp)", http.StatusBadRequest)
			return
		}
		if !request.TermsAccepted || request.TermsVersion != publicTermsVersion {
			writeJSONError(w, "Acceptation des CGV en vigueur requise", http.StatusBadRequest)
			return
		}
		if request.PaymentMethod == "stripe" && cfg.StripeSecretKey == "" && !(cfg.DevHTTP && cfg.StripeMock) {
			writeJSONError(w, "Paiement par carte temporairement indisponible", http.StatusServiceUnavailable)
			return
		}
		if request.PaymentMethod == "bank_transfer" && !cfg.BankTransferConfigured() {
			writeJSONError(w, "Paiement par virement temporairement indisponible", http.StatusServiceUnavailable)
			return
		}
		order, err := dbpkg.CreateRenewalOrder(db, identity.Email, licenseID, request.PaymentMethod, request.TermsVersion,
			request.TermsAccepted, request.ImmediatePerformanceRequested)
		if err != nil {
			writeJSONError(w, err.Error(), http.StatusBadRequest)
			return
		}
		if order.CustomerType == "consumer" && !cfg.B2CSalesReady() {
			_, _ = db.Exec(`UPDATE orders SET status = 'cancelled' WHERE order_id = ?`, order.OrderID)
			writeJSONError(w, "Les renouvellements consommateurs ne sont pas encore ouverts", http.StatusServiceUnavailable)
			return
		}
		if request.PaymentMethod == "stripe" {
			checkoutURL, sessionID, err := createStripeCheckoutSession(cfg, order)
			if err != nil {
				_, _ = db.Exec(`UPDATE orders SET status = 'cancelled' WHERE order_id = ?`, order.OrderID)
				writeJSONError(w, "Impossible de préparer le paiement", http.StatusInternalServerError)
				return
			}
			if err := dbpkg.UpdateOrderStripeSessionID(db, order.OrderID, sessionID); err != nil {
				_, _ = db.Exec(`UPDATE orders SET status = 'cancelled' WHERE order_id = ? AND status = 'pending'`, order.OrderID)
				writeJSONError(w, "Impossible d'enregistrer le paiement", http.StatusInternalServerError)
				return
			}
			writeJSON(w, http.StatusCreated, map[string]any{"order_id": order.OrderID, "checkout_url": checkoutURL})
			return
		}
		if request.PaymentMethod == "crypto_btc" || request.PaymentMethod == "crypto_xrp" {
			writeCryptoOrder(w, r, db, cfg, order)
			return
		}
		if err := EnqueueBankInstructionsEmail(db, order.OrderID); err != nil {
			log.Printf("[Customer Renewal] instructions de virement non mises en file pour %s: %v", order.OrderID, err)
		}
		writeJSON(w, http.StatusCreated, map[string]any{
			"order_id":     order.OrderID,
			"instructions": map[string]any{"amount": order.Price, "reference": order.OrderID, "beneficiary": cfg.BankHolder, "iban": cfg.BankIBAN, "bic": cfg.BankBIC},
		})
	}
}

func CustomerInterventionsHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		identity, ok := customerIdentity(r)
		if !ok {
			writeJSONError(w, "Session client invalide", http.StatusUnauthorized)
			return
		}
		if r.Method == http.MethodGet {
			items, err := dbpkg.ListInterventionsByCustomerID(db, identity.ID, 1000)
			if err != nil {
				writeJSONError(w, "Impossible de charger l'historique", http.StatusInternalServerError)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"interventions": items})
			return
		}
		var request customerInterventionRequest
		if err := decodeSingleJSON(r, &request); err != nil || !dbpkg.CustomerOwnsLicenseID(db, identity.ID, request.LicenseID) {
			writeJSONError(w, "Licence ou intervention invalide", http.StatusBadRequest)
			return
		}
		item, err := dbpkg.CreateIntervention(db, request.LicenseID, nil, request.ClientReference, request.Title)
		if err != nil {
			writeJSONError(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusCreated, item)
	}
}

// CustomerInterventionActionHandler drives one owned intervention through
// start/complete/cancel from the unified customer space.
func CustomerInterventionActionHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		identity, ok := customerIdentity(r)
		if !ok {
			writeJSONError(w, "Session client invalide", http.StatusUnauthorized)
			return
		}
		trimmed := strings.TrimPrefix(r.URL.Path, "/api/v1/customer/interventions/")
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
		item, err := dbpkg.GetCustomerIntervention(db, identity.ID, interventionID)
		if err != nil {
			writeJSONError(w, "Intervention introuvable", http.StatusNotFound)
			return
		}
		var result *dbpkg.Intervention
		switch action {
		case "start":
			result, err = dbpkg.StartIntervention(db, item.LicenseID, interventionID)
		case "complete":
			var request customerInterventionActionRequest
			if r.Body != nil {
				_ = decodeSingleJSON(r, &request)
			}
			result, err = dbpkg.CompleteIntervention(db, item.LicenseID, interventionID, request.ClientReference, request.Title, request.Summary)
		case "cancel":
			result, err = dbpkg.CancelIntervention(db, item.LicenseID, interventionID)
		default:
			writeJSONError(w, "Action inconnue", http.StatusNotFound)
			return
		}
		if err != nil {
			writeJSONError(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

func CustomerInterventionsExportHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		identity, ok := customerIdentity(r)
		if !ok {
			writeJSONError(w, "Session client invalide", http.StatusUnauthorized)
			return
		}
		items, err := dbpkg.ListInterventionsByCustomerID(db, identity.ID, 1000)
		if err != nil {
			writeJSONError(w, "Impossible d'exporter l'historique", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="interventions-relaisdesk.csv"`)
		_, _ = w.Write(append([]byte{0xEF, 0xBB, 0xBF}, []byte(dbpkg.InterventionCSV(items))...))
	}
}

func CustomerDownloadInvoiceHandler(db *sql.DB, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		identity, ok := customerIdentity(r)
		if !ok {
			writeJSONError(w, "Session client invalide", http.StatusUnauthorized)
			return
		}
		invoiceNumber := customerPathID(r.URL.Path, "/api/v1/customer/invoices/", "/download")
		if invoiceNumber == "" || !invoiceNumberPattern.MatchString(invoiceNumber) {
			writeJSONError(w, "Facture introuvable", http.StatusNotFound)
			return
		}
		inv, err := dbpkg.GetInvoiceForCustomerID(db, invoiceNumber, identity.ID)
		if err != nil {
			writeJSONError(w, "Facture introuvable", http.StatusNotFound)
			return
		}
		filePath := inv.PDFPath
		// The administrator may have attached an HTML invoice. The customer
		// portal always serves a freshly generated PDF so that a document cannot
		// be interpreted as active HTML in the customer's browser.
		if filePath == "" || strings.ToLower(filepath.Ext(filePath)) != ".pdf" || !isAllowedInvoicePath(cfg, filePath) || !fileExists(filePath) {
			filePath, _, err = invoice.GenerateInvoicePDF(cfg.InvoicesDir, inv, "Téléchargement client")
			if err != nil || !isAllowedInvoicePath(cfg, filePath) {
				writeJSONError(w, "Document de facture indisponible", http.StatusNotFound)
				return
			}
		}
		log.Printf("[AUDIT CUSTOMER INVOICE] Facture %s téléchargée par son titulaire", invoiceNumber)
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filepath.Base(filePath)))
		http.ServeFile(w, r, filePath)
	}
}

// CustomerDownloadInvoiceCIIHandler serves the Factur-X BASIC (CII XML)
// e-invoicing document for one of the customer's invoices.
func CustomerDownloadInvoiceCIIHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		identity, ok := customerIdentity(r)
		if !ok {
			writeJSONError(w, "Session client invalide", http.StatusUnauthorized)
			return
		}
		invoiceNumber := customerPathID(r.URL.Path, "/api/v1/customer/invoices/", "/cii")
		if invoiceNumber == "" || !invoiceNumberPattern.MatchString(invoiceNumber) {
			writeJSONError(w, "Facture introuvable", http.StatusNotFound)
			return
		}
		inv, err := dbpkg.GetInvoiceForCustomerID(db, invoiceNumber, identity.ID)
		if err != nil {
			writeJSONError(w, "Facture introuvable", http.StatusNotFound)
			return
		}
		out, err := invoice.GenerateInvoiceCII(inv)
		if err != nil {
			writeJSONError(w, "Document de facture indisponible", http.StatusNotFound)
			return
		}
		log.Printf("[AUDIT CUSTOMER INVOICE CII] Facture %s (CII) téléchargée par son titulaire", invoiceNumber)
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", invoiceNumber+".xml"))
		_, _ = w.Write(out)
	}
}

func customerPortalURL(base, token string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(base))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return "", fmt.Errorf("URL publique invalide")
	}
	parsed.Path = strings.TrimSuffix(parsed.Path, "/") + "/client/"
	parsed.RawQuery = ""
	parsed.Fragment = "token=" + url.QueryEscape(token)
	return parsed.String(), nil
}

func customerIdentity(r *http.Request) (*dbpkg.CustomerIdentity, bool) {
	identity, ok := r.Context().Value(middleware.CustomerIdentityContextKey).(*dbpkg.CustomerIdentity)
	return identity, ok && identity != nil && identity.ID > 0
}

func customerPathID(path, prefix, suffix string) string {
	value := strings.TrimPrefix(path, prefix)
	value = strings.TrimSuffix(value, suffix)
	value = strings.Trim(value, "/")
	if value == "" || strings.Contains(value, "/") || len(value) > 128 {
		return ""
	}
	return value
}

type customerGenerateViewerCodeRequest struct {
	LicenseID   string `json:"license_id"`
	ClientEmail string `json:"client_email"`
}

type customerViewerCodeView struct {
	ID                  int    `json:"id"`
	Code                string `json:"code"`
	TechnicianLicenseID string `json:"technician_license_id"`
	ClientEmail         string `json:"client_email"`
	ClientRustDeskID    string `json:"client_rustdesk_id,omitempty"`
	CreatedAt           string `json:"created_at"`
	ExpiresAt           string `json:"expires_at"`
	UsedAt              string `json:"used_at,omitempty"`
	IsActive            bool   `json:"is_active"`
	Status              string `json:"status"`
	MaxConnections      int    `json:"max_connections"`
	CurrentConnections  int    `json:"current_connections"`
}

// CustomerGenerateViewerCodeHandler allows an authenticated customer to generate a viewer code using one of their active licenses.
func CustomerGenerateViewerCodeHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		identity, ok := customerIdentity(r)
		if !ok {
			writeJSONError(w, "Session client invalide", http.StatusUnauthorized)
			return
		}

		var req customerGenerateViewerCodeRequest
		if err := decodeSingleJSON(r, &req); err != nil {
			writeJSONError(w, "Requête invalide", http.StatusBadRequest)
			return
		}

		licenses, err := dbpkg.ListLicensesByCustomerID(db, identity.ID)
		if err != nil {
			writeJSONError(w, "Erreur base de données", http.StatusInternalServerError)
			return
		}

		var selectedLic *dbpkg.License
		reqLicID := strings.TrimSpace(req.LicenseID)
		now := time.Now().UTC()

		if reqLicID != "" {
			for i := range licenses {
				if licenses[i].LicenseID == reqLicID {
					selectedLic = &licenses[i]
					break
				}
			}
			if selectedLic == nil {
				writeJSONError(w, "Licence introuvable ou non autorisée", http.StatusForbidden)
				return
			}
		} else {
			// Select first active and non-expired license
			for i := range licenses {
				if licenses[i].Status == "active" && licenses[i].ExpiresAt.After(now) {
					selectedLic = &licenses[i]
					break
				}
			}
			if selectedLic == nil {
				writeJSONError(w, "Aucune licence active disponible", http.StatusBadRequest)
				return
			}
		}

		if selectedLic.Status != "active" || !selectedLic.ExpiresAt.After(now) {
			writeJSONError(w, "La licence sélectionnée n'est pas active ou est expirée", http.StatusBadRequest)
			return
		}

		code, err := dbpkg.CreateViewerCode(db, selectedLic.LicenseID, req.ClientEmail)
		if err != nil {
			writeJSONError(w, err.Error(), http.StatusBadRequest)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"code":                  code.Code,
			"technician_license_id": code.TechnicianLicenseID,
			"client_email":          code.ClientEmail,
			"created_at":            code.CreatedAt.UTC().Format(time.RFC3339),
			"expires_at":            code.ExpiresAt.UTC().Format(time.RFC3339),
		})
	}
}

// CustomerListViewerCodesHandler returns all viewer codes associated with the customer's licenses.
func CustomerListViewerCodesHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		identity, ok := customerIdentity(r)
		if !ok {
			writeJSONError(w, "Session client invalide", http.StatusUnauthorized)
			return
		}

		licenses, err := dbpkg.ListLicensesByCustomerID(db, identity.ID)
		if err != nil {
			writeJSONError(w, "Erreur lors de la récupération des licences", http.StatusInternalServerError)
			return
		}

		now := time.Now().UTC()
		var allCodes []customerViewerCodeView

		for _, lic := range licenses {
			codes, err := dbpkg.ListViewerCodes(db, lic.LicenseID)
			if err != nil {
				continue
			}
			for _, c := range codes {
				var status string
				if !c.IsActive {
					status = "revoked"
				} else if now.After(c.ExpiresAt) {
					status = "expired"
				} else {
					status = "active"
				}

				item := customerViewerCodeView{
					ID:                  c.ID,
					Code:                c.Code,
					TechnicianLicenseID: c.TechnicianLicenseID,
					ClientEmail:         c.ClientEmail,
					ClientRustDeskID:    c.ClientRustDeskID,
					CreatedAt:           c.CreatedAt.UTC().Format(time.RFC3339),
					ExpiresAt:           c.ExpiresAt.UTC().Format(time.RFC3339),
					IsActive:            c.IsActive && c.ExpiresAt.After(now),
					Status:              status,
					MaxConnections:      c.MaxConnections,
					CurrentConnections:  c.CurrentConnections,
				}
				if c.UsedAt != nil {
					item.UsedAt = c.UsedAt.UTC().Format(time.RFC3339)
				}
				allCodes = append(allCodes, item)
			}
		}

		if allCodes == nil {
			allCodes = []customerViewerCodeView{}
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"codes": allCodes,
		})
	}
}

// CustomerRevokeViewerCodeHandler revokes a viewer code belonging to one of the customer's licenses.
func CustomerRevokeViewerCodeHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		identity, ok := customerIdentity(r)
		if !ok {
			writeJSONError(w, "Session client invalide", http.StatusUnauthorized)
			return
		}

		trimmed := strings.TrimPrefix(r.URL.Path, "/api/v1/customer/viewer-codes/")
		trimmed = strings.TrimSuffix(trimmed, "/revoke")
		codeStr := dbpkg.NormalizeViewerCode(strings.Trim(trimmed, "/"))

		if codeStr == "" {
			writeJSONError(w, "Code requis", http.StatusBadRequest)
			return
		}

		// Ensure the code actually belongs to one of this customer's licenses!
		var ownerLicenseID string
		err := db.QueryRow("SELECT technician_license_id FROM viewer_codes WHERE code = ?", codeStr).Scan(&ownerLicenseID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeJSONError(w, "Code introuvable", http.StatusNotFound)
				return
			}
			writeJSONError(w, "Erreur base de données", http.StatusInternalServerError)
			return
		}

		licenses, err := dbpkg.ListLicensesByCustomerID(db, identity.ID)
		if err != nil {
			writeJSONError(w, "Erreur base de données", http.StatusInternalServerError)
			return
		}

		belongs := false
		for _, lic := range licenses {
			if lic.LicenseID == ownerLicenseID {
				belongs = true
				break
			}
		}

		if !belongs {
			writeJSONError(w, "Non autorisé", http.StatusForbidden)
			return
		}

		if err := dbpkg.RevokeViewerCode(db, codeStr); err != nil {
			writeJSONError(w, err.Error(), http.StatusBadRequest)
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{
			"status": "revoked",
			"code":   codeStr,
		})
	}
}
