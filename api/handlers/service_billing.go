package handlers

import (
	"api/config"
	"api/middleware"
	"api/servicelegal"
	dbpkg "database"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Serializes provider creation; durable idempotency keys survive API restarts.
var serviceStripeMu sync.Mutex
var serviceHTTPClient = &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

func serviceAvailable(c *config.Config) bool {
	return c.ServicePaymentsEnabled && c.StripeSecretKey != "" && c.StripeConnectWebhookSecret != "" && c.StripeMode() != "" && !c.StripeMock
}
func serviceStripe(c *config.Config, account, method, path, key string, form url.Values, out any) error {
	req, err := http.NewRequest(method, "https://api.stripe.com/v1/"+path, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.StripeSecretKey)
	req.Header.Set("Stripe-Version", subscriptionAPIVersion)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if account != "" {
		req.Header.Set("Stripe-Account", account)
	}
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	res, err := serviceHTTPClient.Do(req)
	if err != nil {
		return errors.New("Stripe temporairement indisponible")
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
		var stripeErr struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(raw, &stripeErr) == nil && stripeErr.Error.Message != "" {
			return fmt.Errorf("Stripe : %s", stripeErr.Error.Message)
		}
		return fmt.Errorf("Stripe indisponible (HTTP %d)", res.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil || len(raw) >= 1<<20 {
		return errors.New("réponse Stripe invalide")
	}
	var mode struct {
		Livemode *bool `json:"livemode"`
	}
	if err = json.Unmarshal(raw, &mode); err != nil {
		return err
	}
	if mode.Livemode != nil && *mode.Livemode != (c.StripeMode() == "live") {
		return errors.New("environnement Stripe incohérent")
	}
	return json.Unmarshal(raw, out)
}
func serviceURL(raw, host string) bool {
	u, e := url.Parse(raw)
	// No fragment check: Stripe appends a #fid... fragment to Checkout URLs
	// and fragments are never sent to servers, so the host check stays sound.
	return e == nil && u.Scheme == "https" && u.Host == host && u.User == nil
}

type serviceAccount struct {
	ID               string
	ChargesEnabled   bool `json:"charges_enabled"`
	PayoutsEnabled   bool `json:"payouts_enabled"`
	DetailsSubmitted bool `json:"details_submitted"`
}

func serviceAccountReady(c *config.Config, id string) bool {
	path, e := stripePath("accounts", id)
	if e != nil {
		return false
	}
	var a serviceAccount
	return serviceStripe(c, "", "GET", path, "", nil, &a) == nil && a.ID == id && a.ChargesEnabled && a.PayoutsEnabled && a.DetailsSubmitted
}
func serviceOwner(db *sql.DB, r *http.Request) (*dbpkg.CustomerIdentity, bool) {
	who, ok := customerIdentity(r)
	if !ok || who.Role != "owner" {
		return nil, false
	}
	return who, true
}
func CustomerServiceBillingHandler(db *sql.DB, c *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		who, ok := serviceOwner(db, r)
		if !ok {
			writeJSONError(w, "Réservé au propriétaire du compte", 403)
			return
		}
		m, err := dbpkg.GetServiceMerchant(db, who.ID)
		if err != nil {
			writeJSONError(w, "Prestations indisponibles", 500)
			return
		}
		action := strings.TrimPrefix(r.URL.Path, "/api/v1/customer/service-billing")
		acceptedAt, err := dbpkg.ServiceTermsAccepted(db, who.ID, servicelegal.Version, servicelegal.Hash())
		if err != nil {
			writeJSONError(w, "Conditions indisponibles", 500)
			return
		}
		var active int
		if err = db.QueryRow(`SELECT COUNT(*) FROM licences WHERE customer_id=? AND status='active' AND julianday(expires_at)>julianday('now')`, who.ID).Scan(&active); err != nil {
			writeJSONError(w, "Compte indisponible", 500)
			return
		}
		if action == "" && r.Method == "GET" {
			rates, e := dbpkg.ListServiceRates(db, who.ID)
			if e != nil {
				writeJSONError(w, "Catalogue indisponible", 500)
				return
			}
			work, e := dbpkg.ListServiceWork(db, who.ID, "", "")
			if e != nil {
				writeJSONError(w, "Historique indisponible", 500)
				return
			}
			writeJSON(w, 200, map[string]any{"available": serviceAvailable(c), "active_license": active > 0, "merchant": m, "rates": rates, "work": work, "terms": map[string]any{"version": servicelegal.Version, "sha256": servicelegal.Hash(), "url": servicelegal.URL, "accepted_at": acceptedAt}})
			return
		}
		if action == "/terms/accept" && r.Method == "POST" {
			var v struct {
				Accepted bool   `json:"accepted"`
				Version  string `json:"version"`
				Hash     string `json:"sha256"`
			}
			if decodeSingleJSON(r, &v) != nil || !v.Accepted || v.Version != servicelegal.Version || v.Hash != servicelegal.Hash() {
				writeJSONError(w, "Acceptez la version courante des conditions de prestations", 409)
				return
			}
			if err = dbpkg.AcceptServiceTerms(db, who.ID, v.Version, v.Hash); err != nil {
				writeJSONError(w, "Acceptation non enregistrée", 500)
				return
			}
			writeJSON(w, 200, map[string]bool{"ok": true})
			return
		}
		if strings.HasPrefix(action, "/work/") && r.Method == "POST" {
			parts := strings.Split(strings.TrimPrefix(action, "/work/"), "/")
			if len(parts) != 2 {
				writeJSONError(w, "Action inconnue", 404)
				return
			}
			work, e := dbpkg.GetServiceWork(db, parts[0])
			if e != nil || work.CustomerID != who.ID {
				writeJSONError(w, "Prestation introuvable", 404)
				return
			}
			switch parts[1] {
			case "finish", "cancel":
				work, e = dbpkg.FinishServiceWork(db, work.ID, parts[1] == "cancel", time.Now())
			case "checkout":
				if !serviceAvailable(c) {
					writeJSONError(w, "Paiements désactivés", 503)
					return
				}
				serviceStripeMu.Lock()
				e = serviceCheckout(db, c, work)
				serviceStripeMu.Unlock()
				if e == nil {
					work, e = dbpkg.GetServiceWork(db, work.ID)
				}
			default:
				writeJSONError(w, "Action inconnue", 404)
				return
			}
			if e != nil {
				writeJSONError(w, e.Error(), 409)
				return
			}
			writeJSON(w, 200, work)
			return
		}
		// Disabling must remain possible even after expiry or a global stop.
		if action == "" && r.Method == "PUT" {
			var v struct {
				Enabled bool `json:"enabled"`
			}
			if decodeSingleJSON(r, &v) != nil {
				writeJSONError(w, "Requête invalide", 400)
				return
			}
			if v.Enabled && (!serviceAvailable(c) || active == 0 || acceptedAt == 0) {
				writeJSONError(w, "Licence active et acceptation des conditions requises pour activer", 409)
				return
			}
			if v.Enabled && !serviceAccountReady(c, m.AccountID) {
				writeJSONError(w, "Terminez l'inscription Stripe et vérifiez les encaissements et versements", 409)
				return
			}
			if _, err = db.Exec(`UPDATE service_merchants SET enabled=? WHERE customer_id=?`, v.Enabled, who.ID); err != nil {
				writeJSONError(w, "Activation indisponible", 500)
				return
			}
			writeJSON(w, 200, map[string]bool{"ok": true})
			return
		}
		if !serviceAvailable(c) {
			writeJSONError(w, "Paiements de prestations non activés sur le serveur", 503)
			return
		}
		if active == 0 || acceptedAt == 0 {
			writeJSONError(w, "Licence active et acceptation des conditions requises", 409)
			return
		}
		if action == "/onboarding" && r.Method == "POST" {
			serviceStripeMu.Lock()
			defer serviceStripeMu.Unlock()
			m, err = dbpkg.EnsureServiceMerchant(db, who.ID)
			if err == nil && m.AccountID == "" {
				if time.Now().Unix()-m.SetupStarted > 23*3600 {
					err = errors.New("création Stripe ancienne à réconcilier avec le support, aucun nouveau compte créé")
				} else {
					var a serviceAccount
					err = serviceStripe(c, "", "POST", "accounts", fmt.Sprintf("rd-service-merchant-%d-%d", who.ID, m.SetupStarted), url.Values{"type": {"standard"}, "metadata[relaisdesk_customer]": {who.PublicID}}, &a)
					if err == nil && (!strings.HasPrefix(a.ID, "acct_") || !stripeObjectID.MatchString(a.ID)) {
						err = errors.New("compte Stripe invalide")
					}
					if err == nil {
						_, err = db.Exec(`UPDATE service_merchants SET account_id=? WHERE customer_id=? AND account_id=''`, a.ID, who.ID)
						m.AccountID = a.ID
					}
				}
			}
			var link struct{ URL string }
			back := strings.TrimRight(c.PublicWebsiteURL, "/") + "/client/#services"
			if err == nil {
				err = serviceStripe(c, "", "POST", "account_links", "", url.Values{"account": {m.AccountID}, "type": {"account_onboarding"}, "return_url": {back}, "refresh_url": {back}}, &link)
			}
			if err != nil {
				writeJSONError(w, err.Error(), 503)
				return
			}
			if !serviceURL(link.URL, "connect.stripe.com") {
				writeJSONError(w, "Lien Stripe invalide", 502)
				return
			}
			writeJSON(w, 200, map[string]string{"url": link.URL})
			return
		}
		if action == "/rates" && r.Method == "POST" {
			var rate dbpkg.ServiceRate
			if decodeSingleJSON(r, &rate) != nil {
				writeJSONError(w, "Tarif invalide", 400)
				return
			}
			var created *dbpkg.ServiceRate
			created, err = dbpkg.CreateServiceRate(db, who.ID, rate)
			if err == nil {
				writeJSON(w, 201, created)
				return
			}
		} else if strings.HasPrefix(action, "/rates/") && r.Method == "DELETE" {
			_, err = db.Exec(`UPDATE service_rates SET active=0 WHERE customer_id=? AND id=?`, who.ID, strings.TrimPrefix(action, "/rates/"))
		} else {
			writeJSONError(w, "Action inconnue", 405)
			return
		}
		if err != nil {
			writeJSONError(w, err.Error(), 409)
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	}
}

func serviceTechnician(db *sql.DB, r *http.Request) (*dbpkg.License, *dbpkg.TeamMember, error) {
	id, _ := r.Context().Value(middleware.TechnicianLicenseContextKey).(string)
	l, e := dbpkg.GetLicense(db, id)
	if e == nil {
		e = db.QueryRow(`SELECT COALESCE(customer_id,0) FROM licences WHERE license_id=?`, id).Scan(&l.CustomerID)
	}
	if e != nil || l.CustomerID < 1 {
		return nil, nil, dbpkg.ErrServiceState
	}
	m, _ := r.Context().Value(middleware.TechnicianTeamMemberContextKey).(*dbpkg.TeamMember)
	return l, m, nil
}
func serviceTarget(db *sql.DB, l *dbpkg.License, m *dbpkg.TeamMember, kind, id string) (string, error) {
	if kind == "device" {
		d, e := dbpkg.GetDeviceByID(db, id)
		if e != nil || d.LicenseID != l.LicenseID || d.EnrollmentState != "enrolled" {
			return "", dbpkg.ErrServiceState
		}
		if m != nil {
			ok, e := dbpkg.TeamMemberCanAccessDevice(db, m, d)
			if e != nil || !ok {
				return "", dbpkg.ErrServiceState
			}
		}
		return d.RustDeskID, nil
	}
	if kind == "code" && m == nil {
		var peer string
		e := db.QueryRow(`SELECT client_rustdesk_id FROM viewer_codes WHERE code=? AND technician_license_id=? AND is_active=1 AND julianday(expires_at)>julianday('now')`, id, l.LicenseID).Scan(&peer)
		return peer, e
	}
	return "", dbpkg.ErrServiceState
}
func TechnicianServiceBillingHandler(db *sql.DB, c *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l, m, err := serviceTechnician(db, r)
		if err != nil {
			writeJSONError(w, "Accès refusé", 403)
			return
		}
		member := ""
		if m != nil {
			member = m.MemberID
		}
		action := strings.TrimPrefix(r.URL.Path, "/api/v1/technician/service-billing")
		if action == "" && r.Method == "GET" {
			merchant, e := dbpkg.GetServiceMerchant(db, l.CustomerID)
			if e != nil {
				writeJSONError(w, "Catalogue indisponible", 500)
				return
			}
			rates, e := dbpkg.ListServiceRates(db, l.CustomerID)
			if e != nil {
				writeJSONError(w, "Catalogue indisponible", 500)
				return
			}
			active := []dbpkg.ServiceRate{}
			for _, rate := range rates {
				if rate.Active {
					active = append(active, rate)
				}
			}
			work, e := dbpkg.ListServiceWork(db, l.CustomerID, l.LicenseID, member)
			if e != nil {
				writeJSONError(w, "Historique indisponible", 500)
				return
			}
			// Previously authorized targets may have moved out of a member's folders.
			visible := []dbpkg.ServiceWork{}
			for _, v := range work {
				if _, e = serviceTarget(db, l, m, v.TargetKind, v.TargetID); e == nil {
					visible = append(visible, v)
				}
			}
			accepted, e := dbpkg.ServiceTermsAccepted(db, l.CustomerID, servicelegal.Version, servicelegal.Hash())
			if e != nil {
				writeJSONError(w, "Conditions indisponibles", 500)
				return
			}
			writeJSON(w, 200, map[string]any{"enabled": serviceAvailable(c) && merchant.Enabled && accepted > 0, "rates": active, "work": visible})
			return
		}
		if !serviceAvailable(c) {
			writeJSONError(w, "Paiements de prestations désactivés", 503)
			return
		}
		if action == "" && r.Method == "POST" {
			accepted, e := dbpkg.ServiceTermsAccepted(db, l.CustomerID, servicelegal.Version, servicelegal.Hash())
			if e != nil || accepted == 0 {
				writeJSONError(w, "Le propriétaire doit accepter les conditions de prestations", 409)
				return
			}
			var v struct {
				RateID       string `json:"rate_id"`
				Kind         string `json:"target_kind"`
				Target       string `json:"target_id"`
				ClientAgreed bool   `json:"client_agreed"`
			}
			if decodeSingleJSON(r, &v) != nil || !v.ClientAgreed {
				writeJSONError(w, "L'accord du client sur le tarif est requis", 400)
				return
			}
			peer, e := serviceTarget(db, l, m, v.Kind, v.Target)
			if e != nil || !servicePeerID(peer) {
				writeJSONError(w, "Poste non autorisé ou non prêt", 403)
				return
			}
			work, e := dbpkg.CreateServiceWork(db, l.CustomerID, l.LicenseID, member, v.Kind, v.Target, peer, v.RateID)
			if e != nil {
				writeJSONError(w, e.Error(), 409)
				return
			}
			writeJSON(w, 201, work)
			return
		}
		parts := strings.Split(strings.TrimPrefix(action, "/"), "/")
		if len(parts) < 1 || len(parts) > 2 {
			writeJSONError(w, "Action inconnue", 404)
			return
		}
		work, e := dbpkg.GetServiceWork(db, parts[0])
		if e != nil || work.LicenseID != l.LicenseID || work.CustomerID != l.CustomerID || (m != nil && work.MemberID != member) {
			writeJSONError(w, "Prestation introuvable", 404)
			return
		}
		peer, e := serviceTarget(db, l, m, work.TargetKind, work.TargetID)
		needsTarget := m != nil || (len(parts) == 2 && parts[1] == "pulse")
		if needsTarget && (e != nil || peer != work.PeerID) {
			writeJSONError(w, "Poste non autorisé ou identité modifiée", 403)
			return
		}
		if len(parts) == 1 && r.Method == "GET" {
			writeJSON(w, 200, work)
			return
		}
		if len(parts) != 2 || r.Method != "POST" {
			writeJSONError(w, "Méthode refusée", 405)
			return
		}
		switch parts[1] {
		case "pulse":
			var p struct {
				Connection string `json:"connection"`
				Sequence   int64  `json:"sequence"`
				Cumulative int64  `json:"cumulative_ms"`
				Closed     bool   `json:"closed"`
			}
			if decodeSingleJSON(r, &p) != nil {
				writeJSONError(w, "Mesure invalide", 400)
				return
			}
			work, err = dbpkg.RecordServicePulse(db, work.ID, p.Connection, p.Sequence, p.Cumulative, p.Closed, time.Now())
		case "finish", "cancel":
			work, err = dbpkg.FinishServiceWork(db, work.ID, parts[1] == "cancel", time.Now())
		case "checkout":
			serviceStripeMu.Lock()
			defer serviceStripeMu.Unlock()
			err = serviceCheckout(db, c, work)
			if err == nil {
				work, err = dbpkg.GetServiceWork(db, work.ID)
			}
		default:
			writeJSONError(w, "Action inconnue", 404)
			return
		}
		if err != nil {
			writeJSONError(w, err.Error(), 409)
			return
		}
		writeJSON(w, 200, work)
	}
}
func servicePeerID(s string) bool {
	if len(s) < 6 || len(s) > 16 {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

type serviceCheckoutResponse struct {
	ID            string `json:"id"`
	URL           string `json:"url"`
	Status        string `json:"status"`
	PaymentStatus string `json:"payment_status"`
	Mode          string `json:"mode"`
	Currency      string `json:"currency"`
	Amount        int64  `json:"amount_total"`
	Reference     string `json:"client_reference_id"`
}

func serviceCheckout(db *sql.DB, c *config.Config, work *dbpkg.ServiceWork) error {
	var err error
	work, err = dbpkg.GetServiceWork(db, work.ID)
	if err != nil {
		return err
	}
	if work.Paid {
		return nil
	}
	if work.State == "cancelled" || (work.Mode == "hourly" && work.State != "finished") || (work.Mode == "prepaid" && work.State != "prepared") {
		return dbpkg.ErrServiceState
	}
	if work.AmountCents < 50 {
		return errors.New("montant inférieur au minimum Stripe de 0,50 € : aucun paiement créé et aucune majoration appliquée")
	}
	if work.CheckoutID != "" {
		var s serviceCheckoutResponse
		err = serviceStripe(c, work.AccountID, "GET", "checkout/sessions/"+work.CheckoutID, "", nil, &s)
		if err != nil {
			return err
		}
		if s.Status == "expired" {
			return errors.New("lien expiré : prestation à réconcilier, aucun second paiement créé automatiquement")
		}
		return nil
	}
	if !serviceAccountReady(c, work.AccountID) {
		return errors.New("compte Stripe prestataire indisponible")
	}
	if work.CheckoutStarted == 0 {
		work.CheckoutStarted = time.Now().Unix()
		var result sql.Result
		result, err = db.Exec(`UPDATE service_work SET checkout_started=? WHERE id=? AND checkout_started=0 AND state=? AND paid=0`, work.CheckoutStarted, work.ID, work.State)
		if err != nil {
			return err
		}
		if changed, e := result.RowsAffected(); e != nil || changed != 1 {
			return dbpkg.ErrServiceState
		}
	}
	if time.Now().Unix()-work.CheckoutStarted > 23*3600 {
		return errors.New("paiement ancien à réconcilier, nouvelle création bloquée")
	}
	var s serviceCheckoutResponse
	form := url.Values{"mode": {"payment"}, "payment_method_types[0]": {"card"}, "client_reference_id": {work.ID}, "line_items[0][price_data][currency]": {"eur"}, "line_items[0][price_data][unit_amount]": {strconv.FormatInt(work.AmountCents, 10)}, "line_items[0][price_data][product_data][name]": {work.Label}, "line_items[0][quantity]": {"1"}, "metadata[relaisdesk_service]": {work.ID}, "success_url": {strings.TrimRight(c.PublicWebsiteURL, "/") + "/paiement-prestation.html"}, "cancel_url": {strings.TrimRight(c.PublicWebsiteURL, "/") + "/paiement-prestation.html?cancel=1"}}
	if work.Mode == "hourly" {
		form.Set("line_items[0][price_data][product_data][description]", fmt.Sprintf("%.2f minutes de connexion confirmée à %.2f EUR/heure, au prorata et arrondi au centime sur le total.", float64(work.ConnectedMS)/60000, float64(work.RateCents)/100))
	}
	err = serviceStripe(c, work.AccountID, "POST", "checkout/sessions", "rd-service-checkout-"+work.ID, form, &s)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(s.ID, "cs_") || !stripeObjectID.MatchString(s.ID) || !serviceURL(s.URL, "checkout.stripe.com") {
		return errors.New("session Stripe invalide")
	}
	_, err = db.Exec(`UPDATE service_work SET checkout_id=?,checkout_url=? WHERE id=? AND checkout_id=''`, s.ID, s.URL, work.ID)
	return err
}
func ServiceStripeWebhookHandler(db *sql.DB, c *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Keep settlements working even when new sales are disabled.
		if c.StripeConnectWebhookSecret == "" || c.StripeSecretKey == "" {
			writeJSONError(w, "Webhook indisponible", 503)
			return
		}
		raw, err := io.ReadAll(io.LimitReader(r.Body, (1<<20)+1))
		if err != nil || len(raw) > 1<<20 || !verifyStripeSignature(raw, r.Header.Get("Stripe-Signature"), c.StripeConnectWebhookSecret) {
			writeJSONError(w, "Signature invalide", 400)
			return
		}
		var event struct {
			Type     string `json:"type"`
			Account  string `json:"account"`
			Livemode *bool  `json:"livemode"`
			Data     struct {
				Object struct {
					ID       string            `json:"id"`
					Metadata map[string]string `json:"metadata"`
				} `json:"object"`
			} `json:"data"`
		}
		if json.Unmarshal(raw, &event) != nil || event.Livemode == nil || *event.Livemode != (c.StripeMode() == "live") {
			writeJSONError(w, "Événement invalide", 400)
			return
		}
		if event.Type != "checkout.session.completed" && event.Type != "checkout.session.async_payment_succeeded" {
			writeJSON(w, 200, map[string]bool{"received": true})
			return
		}
		work, err := scanServiceWebhook(db, event.Account, event.Data.Object.ID)
		if errors.Is(err, sql.ErrNoRows) {
			if reference := event.Data.Object.Metadata["relaisdesk_service"]; reference != "" {
				pending, e := dbpkg.GetServiceWork(db, reference)
				if e == nil && pending.AccountID == event.Account && pending.CheckoutID == "" && pending.CheckoutStarted > 0 {
					writeJSONError(w, "Création en cours, réessayer", 503)
					return
				}
			}
			writeJSON(w, 200, map[string]bool{"received": true})
			return
		}
		if err != nil {
			writeJSONError(w, "Réessayer", 500)
			return
		}
		var s serviceCheckoutResponse
		if err = serviceStripe(c, work.AccountID, "GET", "checkout/sessions/"+work.CheckoutID, "", nil, &s); err != nil {
			writeJSONError(w, "Réessayer", 503)
			return
		}
		if s.ID != work.CheckoutID || s.Reference != work.ID || s.Mode != "payment" || s.Currency != "eur" || s.Amount != work.AmountCents {
			writeJSONError(w, "Paiement incohérent", 400)
			return
		}
		if s.PaymentStatus == "paid" {
			_, err = db.Exec(`UPDATE service_work SET paid=1 WHERE id=? AND checkout_id=? AND account_id=?`, work.ID, s.ID, event.Account)
		}
		if err != nil {
			writeJSONError(w, "Réessayer", 500)
			return
		}
		writeJSON(w, 200, map[string]bool{"received": true})
	}
}
func scanServiceWebhook(db *sql.DB, account, session string) (*dbpkg.ServiceWork, error) {
	var id string
	err := db.QueryRow(`SELECT id FROM service_work WHERE account_id=? AND checkout_id=? AND checkout_id<>''`, account, session).Scan(&id)
	if err != nil {
		return nil, err
	}
	return dbpkg.GetServiceWork(db, id)
}
