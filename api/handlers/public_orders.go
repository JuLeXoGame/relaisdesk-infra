package handlers

import (
	dbpkg "database"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	netmail "net/mail"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"api/config"
	"api/mailer"
	"api/releasemanifest"
)

const publicTermsVersion = mailer.CurrentTermsVersion

type PublicOrderRequest struct {
	Email                         string `json:"email"`
	Plan                          string `json:"plan"`
	Technicians                   int    `json:"technicians"`
	BillingCycle                  string `json:"billing_cycle"`  // 'monthly' or 'annual'
	PaymentMethod                 string `json:"payment_method"` // 'stripe' or 'bank_transfer'
	Name                          string `json:"name,omitempty"`
	Address                       string `json:"address,omitempty"`
	PostalCode                    string `json:"postal_code,omitempty"`
	City                          string `json:"city,omitempty"`
	Country                       string `json:"country,omitempty"`
	SIRET                         string `json:"siret,omitempty"`
	CustomerType                  string `json:"customer_type"`
	TermsVersion                  string `json:"terms_version"`
	TermsAccepted                 bool   `json:"terms_accepted"`
	ImmediatePerformanceRequested bool   `json:"immediate_performance_requested"`
}

type PublicLicenseStatusRequest struct {
	LicenseID string `json:"license_id"`
}

// PublicOrderHandler creates an order and returns Stripe Checkout URL or bank transfer details.
func PublicOrderHandler(db *sql.DB, cfg *config.Config, mail *mailer.Mailer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req PublicOrderRequest
		if err := decodeSingleJSON(r, &req); err != nil {
			writeJSONError(w, "Requête JSON invalide", http.StatusBadRequest)
			return
		}

		req.Email = strings.TrimSpace(strings.ToLower(req.Email))
		parsedEmail, err := netmail.ParseAddress(req.Email)
		if err != nil || parsedEmail.Address != req.Email || len(req.Email) > 254 || strings.ContainsAny(req.Email, "\r\n\t") {
			writeJSONError(w, "Adresse email valide requise", http.StatusBadRequest)
			return
		}

		billingCycle := strings.ToLower(strings.TrimSpace(req.BillingCycle))
		if billingCycle != "annual" {
			billingCycle = "monthly"
		}

		// Recalculate price on the server side
		_, techs, planName, err := dbpkg.CalculateServerPrice(req.Plan, req.Technicians, billingCycle)
		if err != nil {
			writeJSONError(w, err.Error(), http.StatusBadRequest)
			return
		}

		paymentMethod := strings.ToLower(strings.TrimSpace(req.PaymentMethod))
		if !dbpkg.ValidPaymentMethod(paymentMethod) {
			writeJSONError(w, "Méthode de paiement invalide (choix: stripe, bank_transfer, crypto_btc, crypto_xrp)", http.StatusBadRequest)
			return
		}
		if paymentMethod == "stripe" && cfg.StripeSecretKey == "" && !(cfg.DevHTTP && cfg.StripeMock) {
			writeJSONError(w, "Paiement par carte temporairement indisponible", http.StatusServiceUnavailable)
			return
		}
		if paymentMethod == "bank_transfer" && !cfg.BankTransferConfigured() {
			writeJSONError(w, "Paiement par virement temporairement indisponible", http.StatusServiceUnavailable)
			return
		}

		billing := &dbpkg.BillingDetails{
			Name:                          strings.TrimSpace(req.Name),
			Address:                       strings.TrimSpace(req.Address),
			PostalCode:                    strings.TrimSpace(req.PostalCode),
			City:                          strings.TrimSpace(req.City),
			Country:                       strings.TrimSpace(req.Country),
			SIRET:                         strings.TrimSpace(req.SIRET),
			CustomerType:                  strings.ToLower(strings.TrimSpace(req.CustomerType)),
			TermsVersion:                  strings.TrimSpace(req.TermsVersion),
			TermsAccepted:                 req.TermsAccepted,
			ImmediatePerformanceRequested: req.ImmediatePerformanceRequested,
			BillingCycle:                  billingCycle,
		}
		if len(billing.Name) > 200 || len(billing.Address) > 500 || len(billing.PostalCode) > 32 ||
			len(billing.City) > 100 || len(billing.Country) > 100 || len(billing.SIRET) > 32 {
			writeJSONError(w, "Informations de facturation trop longues", http.StatusBadRequest)
			return
		}
		if billing.Name == "" || billing.Address == "" || billing.PostalCode == "" || billing.City == "" {
			writeJSONError(w, "Nom et adresse de facturation complets requis", http.StatusBadRequest)
			return
		}
		if billing.CustomerType != "business" && billing.CustomerType != "consumer" {
			writeJSONError(w, "Statut client invalide (choix: business, consumer)", http.StatusBadRequest)
			return
		}
		if billing.CustomerType == "consumer" && !cfg.B2CSalesReady() {
			writeJSONError(w, "Les ventes aux consommateurs ne sont pas encore ouvertes", http.StatusServiceUnavailable)
			return
		}
		if !billing.TermsAccepted || billing.TermsVersion != publicTermsVersion {
			writeJSONError(w, "Acceptation de la version en vigueur des CGV/CGU requise", http.StatusBadRequest)
			return
		}
		if billing.CustomerType == "consumer" && !billing.ImmediatePerformanceRequested {
			writeJSONError(w, "Demande expresse d'activation immédiate requise pour commencer le service avant la fin du délai de rétractation", http.StatusBadRequest)
			return
		}

		order, err := dbpkg.CreateOrderWithBilling(db, req.Email, planName, techs, paymentMethod, fmt.Sprintf("Achat plan %s (%d techs - %s)", planName, techs, billingCycle), "", billing)
		if err != nil {
			writeJSONError(w, "Erreur création commande", http.StatusInternalServerError)
			return
		}

		if paymentMethod == "stripe" {
			checkoutURL, sessionID, err := createStripeCheckoutSession(cfg, order)
			if err != nil {
				log.Printf("[Stripe Error] %v", err)
				_, _ = db.Exec(`UPDATE orders SET status = 'cancelled' WHERE order_id = ? AND status = 'pending'`, order.OrderID)
				writeJSONError(w, "Erreur initialisation paiement Stripe", http.StatusInternalServerError)
				return
			}

			if err := dbpkg.UpdateOrderStripeSessionID(db, order.OrderID, sessionID); err != nil {
				log.Printf("[Stripe Error] session non enregistrée pour %s: %v", order.OrderID, err)
				_, _ = db.Exec(`UPDATE orders SET status = 'cancelled' WHERE order_id = ? AND status = 'pending'`, order.OrderID)
				writeJSONError(w, "Erreur enregistrement paiement Stripe", http.StatusInternalServerError)
				return
			}

			writeJSON(w, http.StatusCreated, map[string]interface{}{
				"order_id":       order.OrderID,
				"email":          order.Email,
				"plan":           order.Plan,
				"billing_cycle":  order.BillingCycle,
				"technicians":    order.Technicians,
				"price":          order.Price,
				"payment_method": "stripe",
				"checkout_url":   checkoutURL,
				"session_id":     sessionID,
			})
			return
		}

		if paymentMethod == "crypto_btc" || paymentMethod == "crypto_xrp" {
			writeCryptoOrder(w, r, db, cfg, order)
			return
		}

		// Bank transfer
		if err := EnqueueBankInstructionsEmail(db, order.OrderID); err != nil {
			log.Printf("[Order] instructions de virement non mises en file pour %s: %v", order.OrderID, err)
		}

		writeJSON(w, http.StatusCreated, map[string]interface{}{
			"order_id":       order.OrderID,
			"email":          order.Email,
			"plan":           order.Plan,
			"billing_cycle":  order.BillingCycle,
			"technicians":    order.Technicians,
			"price":          order.Price,
			"payment_method": "bank_transfer",
			"instructions": map[string]interface{}{
				"amount":         order.Price,
				"reference":      order.OrderID,
				"beneficiary":    cfg.BankHolder,
				"iban":           cfg.BankIBAN,
				"bic":            cfg.BankBIC,
				"mandatory_note": fmt.Sprintf("Veuillez indiquer la référence %s dans le libellé de votre virement", order.OrderID),
			},
		})
	}
}

// PublicLicenseStatusHandler allows a technician to check their license validity without exposing secret keys.
func PublicLicenseStatusHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req PublicLicenseStatusRequest
		if err := decodeSingleJSON(r, &req); err != nil {
			writeJSONError(w, "Requête JSON invalide", http.StatusBadRequest)
			return
		}

		licID := strings.TrimSpace(req.LicenseID)
		if licID == "" {
			writeJSONError(w, "license_id requis", http.StatusBadRequest)
			return
		}

		lic, err := dbpkg.GetLicenseByID(db, licID)
		if err != nil {
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"valid":   false,
				"status":  "not_found",
				"message": "Licence introuvable",
			})
			return
		}

		now := time.Now().UTC()
		status := lic.Status
		if status == "active" && now.After(lic.ExpiresAt) {
			status = "expired"
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"valid":           status == "active",
			"license_id":      lic.LicenseID,
			"email":           maskEmail(lic.Email),
			"status":          status,
			"expires_at":      lic.ExpiresAt.Format(time.RFC3339),
			"max_connections": lic.MaxConnections,
		})
	}
}

// PublicPricingHandler returns official pricing structure in JSON.
func PublicPricingHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"starter": map[string]interface{}{
				"managed_devices": dbpkg.StarterManagedDevices,
				"name":            "Starter",
				"price":           24.90,
				"price_monthly":   24.90,
				"price_annual":    239.00,
				"period":          "mois",
				"technicians":     1,
				"viewers":         "illimités (0 €)",
				"description":     "Idéal pour technicien indépendant",
			},
			"pro": map[string]interface{}{
				"managed_devices": dbpkg.ProManagedDevices,
				"name":            "Pro",
				"price":           dbpkg.ProMonthlyPrice,
				"price_monthly":   dbpkg.ProMonthlyPrice,
				"price_annual":    dbpkg.ProAnnualPrice,
				"period":          "mois",
				"technicians":     5,
				"viewers":         "illimités (0 €)",
				"description":     "Pour équipes jusqu'à 5 techniciens simultanés",
			},
			"ultra": map[string]interface{}{
				"managed_devices":                      dbpkg.UltraBaseManagedDevices,
				"managed_devices_per_extra_technician": dbpkg.UltraDevicesPerExtraTechnician,
				"max_managed_devices":                  dbpkg.UltraMaxManagedDevices,
				"managed_devices_max_tier_bonus":       dbpkg.UltraMaxManagedDevices - dbpkg.UltraBaseManagedDevices - (500-10)*dbpkg.UltraDevicesPerExtraTechnician,
				"name":                                 "Personnalisé",
				"base_price":                           dbpkg.UltraBaseMonthlyPrice,
				"base_monthly":                         dbpkg.UltraBaseMonthlyPrice,
				"base_annual":                          dbpkg.UltraBaseAnnualPrice,
				"base_techs":                           10,
				"max_techs":                            500,
				"period":                               "mois",
				"viewers":                              "illimités (0 €)",
				"description":                          "Solution sur-mesure de 10 à 500 techniciens simultanés",
			},
		})
	}
}

// PublicPaymentMethodsHandler reports which prepaid payment methods the site
// may offer. A crypto asset appears only when globally enabled AND its OKX
// deposit configuration is complete, mirroring the CGV clause
// "lorsqu'ils sont proposés au récapitulatif".
func PublicPaymentMethodsHandler(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"bank_transfer": cfg.BankTransferConfigured(),
			"crypto_btc":    cfg.CryptoAssetConfigured("BTC"),
			"crypto_xrp":    cfg.CryptoAssetConfigured("XRP"),
		})
	}
}

// DownloadHandler serves installation binaries directly from the API server.
func DownloadHandler(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !cfg.DevHTTP && !cfg.SourceDistributionReady() {
			writeJSONError(w, "Téléchargements suspendus jusqu'à la publication du code source correspondant AGPLv3", http.StatusServiceUnavailable)
			return
		}
		target := strings.TrimPrefix(r.URL.Path, "/api/v1/downloads/")
		var fileName string
		var legacyNames []string

		switch strings.ToLower(target) {
		case "configurator-setup", "configurator-setup.exe", "setup-technicien", "relaisdesk_technicien_setup.exe", "relaisdesk_technicien_setup_1.0.0.exe", "relaisdesk_technicien_setup_1.0.5.exe":
			fileName = "RelaisDesk_Technicien_Setup_1.0.0.exe"
			legacyNames = []string{"RelaisDesk_Technicien_Setup_1.0.5.exe", "RelaisDesk_Technicien_Setup.exe"}
		case "configurator", "configurator.exe", "windows-technician", "relaisdesk_technicien_portable.exe", "technicien-portable":
			fileName = "RelaisDesk_Technicien_Portable.exe"
			legacyNames = []string{"configurator.exe"}
		case "configurator-deb", "linux-technician", "relaisdesk_technicien.deb", "relaisdesk-configurator.deb":
			fileName = "RelaisDesk_Technicien.deb"
			legacyNames = []string{"relaisdesk-configurator.deb"}
		case "configurator-linux", "technicien-linux", "relaisdesk_technicien_linux", "linux-technician-bin":
			fileName = "RelaisDesk_Technicien_Linux"
		case "configurator-mac", "technicien-mac", "mac-technician", "relaisdesk_technicien_mac.dmg", "relaisdesk_technicien.dmg":
			fileName = "RelaisDesk_Technicien_Mac.dmg"
			legacyNames = []string{"RelaisDesk_Technicien.dmg"}
		case "setup", "setup.exe", "viewer-setup", "relaisdesk_setup.exe", "relaisdesk_setup_1.0.0.exe":
			fileName = "RelaisDesk_Setup.exe"
			legacyNames = []string{"RelaisDesk_Setup_1.0.0.exe"}
		case "viewer", "viewer.exe", "windows-viewer", "relaisdesk_portable.exe", "viewer-portable":
			fileName = "RelaisDesk_Portable.exe"
			legacyNames = []string{"viewer.exe"}
		case "viewer-deb", "linux-viewer", "relaisdesk_viewer.deb", "relaisdesk-viewer.deb":
			fileName = "RelaisDesk_viewer.deb"
			legacyNames = []string{"relaisdesk-viewer.deb"}
		case "viewer-linux", "linux-viewer-bin", "relaisdesk_viewer_linux":
			fileName = "RelaisDesk_Viewer_Linux"
		case "viewer-mac", "viewer-dmg", "mac-viewer", "relaisdesk_mac.dmg", "relaisdesk.dmg":
			fileName = "RelaisDesk_Mac.dmg"
			legacyNames = []string{"RelaisDesk.dmg"}
		case "sha256sums.txt", "sha256sums":
			fileName = "SHA256SUMS.txt"
		case "release-manifest.json", "manifest.json", "manifest":
			fileName = "release-manifest.json"
		default:
			http.NotFound(w, r)
			return
		}

		names := append([]string{fileName}, legacyNames...)
		directories := make([]string, 0, 6)
		if configured := strings.TrimSpace(cfg.DownloadsDir); configured != "" {
			directories = append(directories, configured)
		}
		// Developer fallbacks are intentionally unavailable in production: a
		// stale artifact in the working directory must never replace the
		// explicitly configured release directory.
		if cfg.DevHTTP {
			directories = append(directories, "relaisdesk/downloads", "installer/build", "installer/viewer", "bin")
		}
		filePath := ""
		for _, directory := range directories {
			for _, name := range names {
				candidate := filepath.Join(directory, name)
				if info, err := os.Lstat(candidate); err == nil && info.Mode().IsRegular() {
					filePath = candidate
					break
				}
			}
			if filePath != "" {
				break
			}
		}
		if filePath == "" {
			writeJSONError(w, fmt.Sprintf("Binaire %s non disponible pour le moment", fileName), http.StatusNotFound)
			return
		}
		// In production the published binary must be present in the signed
		// release manifest and still match both its size and SHA-256. This turns
		// an accidental or malicious replacement in DOWNLOADS_DIR into a 503,
		// never into a client update.
		if !cfg.DevHTTP && strings.TrimSpace(cfg.ReleaseManifestPath) != "" && strings.TrimSpace(cfg.ReleasePublicKey) != "" {
			if filepath.Base(filePath) != "release-manifest.json" {
				manifest, err := releasemanifest.LoadVerified(cfg.ReleaseManifestPath, cfg.ReleasePublicKey)
				if err != nil {
					writeJSONError(w, "Téléchargement suspendu: manifeste de version non vérifiable", http.StatusServiceUnavailable)
					return
				}
				artifact, found := manifest.Artifact(filepath.Base(filePath))
				if !found || releasemanifest.VerifyArtifact(filePath, artifact) != nil {
					writeJSONError(w, "Téléchargement suspendu: artefact non conforme à la version signée", http.StatusServiceUnavailable)
					return
				}
			}
		}

		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", fileName))
		w.Header().Set("Content-Type", "application/octet-stream")
		rc := http.NewResponseController(w)
		_ = rc.SetWriteDeadline(time.Now().Add(30 * time.Minute))
		http.ServeFile(w, r, filePath)
	}
}

func createStripeCheckoutSession(cfg *config.Config, order *dbpkg.Order) (checkoutURL, sessionID string, err error) {
	if cfg.StripeSecretKey == "" {
		if !cfg.DevHTTP || !cfg.StripeMock {
			return "", "", fmt.Errorf("stripe secret key is not configured")
		}
		mockSessionID := fmt.Sprintf("cs_test_%s", order.OrderID)
		mockCheckoutURL, err := stripeReturnURL(cfg.StripeSuccessURL, order.OrderID, mockSessionID)
		if err != nil {
			return "", "", err
		}
		return mockCheckoutURL, mockSessionID, nil
	}

	data := url.Values{}
	successURL, err := stripeReturnURL(cfg.StripeSuccessURL, order.OrderID, "{CHECKOUT_SESSION_ID}")
	if err != nil {
		return "", "", err
	}
	data.Set("success_url", successURL)
	data.Set("cancel_url", cfg.StripeCancelURL)
	data.Set("payment_method_types[0]", "card")
	data.Set("mode", "payment")
	data.Set("customer_email", order.Email)
	data.Set("billing_address_collection", "required")
	data.Set("tax_id_collection[enabled]", "true")
	data.Set("client_reference_id", order.OrderID) // Requirement #3: Order ID in client_reference_id
	data.Set("metadata[order_id]", order.OrderID)

	// Line items
	amountCents := int64(mathRound(order.Price * 100))
	data.Set("line_items[0][price_data][currency]", "eur")
	data.Set("line_items[0][price_data][unit_amount]", strconv.FormatInt(amountCents, 10))
	productName := fmt.Sprintf("RelaisDesk %s (%d technicien(s))", order.Plan, order.Technicians)
	if order.BillingCycle == "annual" {
		productName = fmt.Sprintf("RelaisDesk %s - Annuel 1 an (%d technicien(s))", order.Plan, order.Technicians)
	}
	data.Set("line_items[0][price_data][product_data][name]", productName)
	stripeDesc := "Abonnement mensuel RelaisDesk Support à distance"
	if order.BillingCycle == "annual" {
		stripeDesc = "Abonnement annuel (1 an - 2 mois offerts) RelaisDesk Support à distance"
	}
	data.Set("line_items[0][price_data][product_data][description]", stripeDesc)
	data.Set("line_items[0][quantity]", "1")

	req, err := http.NewRequest(http.MethodPost, "https://api.stripe.com/v1/checkout/sessions", strings.NewReader(data.Encode()))
	if err != nil {
		return "", "", err
	}

	req.Header.Set("Authorization", "Bearer "+cfg.StripeSecretKey)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("stripe request: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if readErr != nil {
		return "", "", fmt.Errorf("read stripe response: %w", readErr)
	}
	if resp.StatusCode >= 400 {
		return "", "", fmt.Errorf("stripe api error (%d): %s", resp.StatusCode, string(bodyBytes))
	}

	var stripeResp struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	}
	if err := json.Unmarshal(bodyBytes, &stripeResp); err != nil {
		return "", "", fmt.Errorf("decode stripe response: %w", err)
	}
	checkout, err := url.Parse(stripeResp.URL)
	if stripeResp.ID == "" || err != nil || checkout.Scheme != "https" ||
		!(checkout.Hostname() == "checkout.stripe.com" || strings.HasSuffix(checkout.Hostname(), ".checkout.stripe.com")) {
		return "", "", fmt.Errorf("stripe returned an invalid checkout session")
	}

	return stripeResp.URL, stripeResp.ID, nil
}

func stripeReturnURL(baseURL, orderID, sessionID string) (string, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return "", fmt.Errorf("invalid Stripe return URL")
	}
	query := parsed.Query()
	query.Set("order", "stripe_success")
	query.Set("order_id", orderID)
	query.Set("session_id", sessionID)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func maskEmail(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return email
	}
	name := parts[0]
	if len(name) <= 2 {
		return name + "@" + parts[1]
	}
	return name[:2] + strings.Repeat("*", len(name)-2) + "@" + parts[1]
}

func mathRound(val float64) float64 {
	if val < 0 {
		return float64(int64(val - 0.5))
	}
	return float64(int64(val + 0.5))
}
