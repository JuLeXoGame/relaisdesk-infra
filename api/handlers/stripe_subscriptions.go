package handlers

import (
	"api/config"
	dbpkg "database"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Pin responses we retrieve, independently of the account/webhook API version.
const subscriptionAPIVersion = "2024-06-20"
const trialProductID = "relaisdesk_subscription_v1"

var stripeObjectID = regexp.MustCompile(`^[A-Za-z0-9_]+$`)
var subscriptionHTTPClient = &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

func stripeCall(cfg *config.Config, method, path, key string, form url.Values, out any) error {
	if cfg.StripeSecretKey == "" {
		return errors.New("Stripe non configuré")
	}
	if method == http.MethodDelete && len(form) > 0 {
		path += "?" + form.Encode()
		form = nil
	}
	req, err := http.NewRequest(method, "https://api.stripe.com/v1/"+path, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.StripeSecretKey)
	req.Header.Set("Stripe-Version", subscriptionAPIVersion)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	resp, err := subscriptionHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("Stripe indisponible: %w", err)
	}
	defer resp.Body.Close()
	// Never log provider bodies: they may contain billing details or secrets.
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Stripe HTTP %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return err
	}
	if len(raw) >= 2<<20 {
		return errors.New("réponse Stripe trop longue")
	}
	var mode struct {
		Livemode *bool `json:"livemode"`
	}
	if err = json.Unmarshal(raw, &mode); err != nil {
		return err
	}
	if cfg.StripeMode() != "" && mode.Livemode != nil && *mode.Livemode != (cfg.StripeMode() == "live") {
		return errors.New("réponse Stripe d'un autre environnement")
	}
	return json.Unmarshal(raw, out)
}
func stripePath(kind, id string) (string, error) {
	if id == "" || len(id) > 255 || !stripeObjectID.MatchString(id) {
		return "", errors.New("identifiant Stripe invalide")
	}
	return kind + "/" + id, nil
}

type billingPaymentMethod struct {
	ID, Type, Customer string
	Card               struct{ Fingerprint string } `json:"card"`
}
type billingSetupIntent struct {
	ID, Status, Customer string
	PaymentMethod        billingPaymentMethod `json:"payment_method"`
	Metadata             map[string]string    `json:"metadata"`
}
type billingSession struct {
	ID, Mode, Status, URL, Customer string
	ClientReferenceID               string             `json:"client_reference_id"`
	SetupIntent                     billingSetupIntent `json:"setup_intent"`
	Metadata                        map[string]string  `json:"metadata"`
}
type billingSubscription struct {
	ID, Customer, Status string
	TrialStart           int64             `json:"trial_start"`
	TrialEnd             int64             `json:"trial_end"`
	CurrentPeriodEnd     int64             `json:"current_period_end"`
	CancelAtPeriodEnd    bool              `json:"cancel_at_period_end"`
	Metadata             map[string]string `json:"metadata"`
	Items                struct {
		Data []struct {
			Quantity int
			Price    struct {
				ID, Currency string
				UnitAmount   int64 `json:"unit_amount"`
				Recurring    struct {
					Interval      string
					IntervalCount int `json:"interval_count"`
				}
			}
		}
	} `json:"items"`
}
type billingInvoice struct {
	ID, Subscription, Customer, Status, Currency string
	Paid                                         bool
	AmountPaid                                   int64 `json:"amount_paid"`
	Total                                        int64
	BillingReason                                string `json:"billing_reason"`
	Lines                                        struct {
		HasMore bool `json:"has_more"`
		Data    []struct {
			Amount                       int64
			Currency, Subscription, Type string
			Proration                    bool
			Quantity                     int
			Period                       struct{ Start, End int64 }
			Price                        struct{ ID string }
		}
	} `json:"lines"`
}

func trialCheckout(cfg *config.Config, db *sql.DB, t *dbpkg.Trial) (string, error) {
	if t.State != "verified" || t.ExpiresAt <= time.Now().Unix() {
		return "", dbpkg.ErrTrialUnavailable
	}
	customerID := t.StripeCustomerID
	if customerID == "" {
		var customer struct{ ID string }
		form := url.Values{"email": {t.Email}, "name": {t.Billing.Name}, "metadata[relaisdesk_trial]": {t.ID}}
		if err := stripeCall(cfg, "POST", "customers", "trial-customer:"+t.ID, form, &customer); err != nil {
			return "", err
		}
		if _, err := stripePath("customers", customer.ID); err != nil {
			return "", err
		}
		customerID = customer.ID
		if _, err := db.Exec(`UPDATE trial_applications SET stripe_customer_id=? WHERE id=?`, customerID, t.ID); err != nil {
			return "", err
		}
	}
	base := strings.TrimRight(cfg.PublicWebsiteURL, "/") + "/essai/"
	interval := "mois"
	if t.BillingCycle == "annual" {
		interval = "an"
	}
	summary := fmt.Sprintf("RelaisDesk %s, %d technicien(s) simultané(s). Essai gratuit de 30 jours sous réserve d'éligibilité, puis %.2f EUR/%s prélevés automatiquement. Annulable dans l'espace client avant la fin de l'essai, puis à chaque échéance. Aucun paiement aujourd'hui.", t.Plan, t.Technicians, float64(t.PriceCents)/100, interval)
	form := url.Values{"mode": {"setup"}, "customer": {customerID}, "payment_method_types[0]": {"card"}, "client_reference_id": {t.ID},
		"metadata[relaisdesk_trial]": {t.ID}, "setup_intent_data[metadata][relaisdesk_trial]": {t.ID},
		"success_url": {base + "?result=setup"}, "cancel_url": {base + "?result=cancel"}, "custom_text[submit][message]": {summary},
		"expires_at": {strconv.FormatInt(t.ExpiresAt+1800, 10)}}
	var session billingSession
	if err := stripeCall(cfg, "POST", "checkout/sessions", "trial-checkout:"+t.ID, form, &session); err != nil {
		return "", err
	}
	u, err := url.Parse(session.URL)
	if err != nil || u.Scheme != "https" || u.Host != "checkout.stripe.com" || u.User != nil {
		return "", errors.New("URL Stripe invalide")
	}
	if _, err = stripePath("checkout/sessions", session.ID); err != nil {
		return "", err
	}
	_, err = db.Exec(`UPDATE trial_applications SET checkout_session_id=? WHERE id=?`, session.ID, t.ID)
	return session.URL, err
}

func getSubscription(cfg *config.Config, id string) (*billingSubscription, error) {
	path, err := stripePath("subscriptions", id)
	if err != nil {
		return nil, err
	}
	var sub billingSubscription
	err = stripeCall(cfg, "GET", path, "", nil, &sub)
	return &sub, err
}
func subscriptionMatchesTrial(sub *billingSubscription, t *dbpkg.Trial) bool {
	if sub == nil || sub.ID == "" || sub.Customer != t.StripeCustomerID || sub.Metadata["relaisdesk_trial"] != t.ID || sub.TrialEnd != t.TrialEnd || len(sub.Items.Data) != 1 {
		return false
	}
	item := sub.Items.Data[0]
	interval := "month"
	if t.BillingCycle == "annual" {
		interval = "year"
	}
	return item.Quantity == 1 && item.Price.UnitAmount == t.PriceCents && item.Price.Currency == "eur" && item.Price.Recurring.Interval == interval && item.Price.Recurring.IntervalCount == 1
}

func provisionTrial(cfg *config.Config, db *sql.DB, t *dbpkg.Trial, pm string) (*dbpkg.Trial, error) {
	// Recovery beyond Stripe's 24h idempotency retention: search the dedicated
	// customer first, including canceled subscriptions. Never create a second one.
	var list struct {
		Data    []billingSubscription
		HasMore bool `json:"has_more"`
	}
	query := url.Values{"customer": {t.StripeCustomerID}, "status": {"all"}, "limit": {"100"}}
	if err := stripeCall(cfg, "GET", "subscriptions?"+query.Encode(), "", nil, &list); err != nil {
		return nil, err
	}
	if list.HasMore {
		return nil, errors.New("abonnements trop nombreux pour une reprise sûre")
	}
	var sub *billingSubscription
	for i := range list.Data {
		if list.Data[i].Metadata["relaisdesk_trial"] == t.ID {
			if sub != nil {
				return nil, errors.New("abonnement dupliqué à contrôler")
			}
			sub = &list.Data[i]
		}
	}
	if sub == nil {
		if time.Now().Unix() > t.TrialStart+1800 {
			return nil, errors.New("création Stripe différée : intervention requise, aucun débit déclenché")
		}
		var product struct{ ID string }
		// The explicit ID prevents duplication even after the idempotency cache expires.
		if err := stripeCall(cfg, "GET", "products/"+trialProductID, "", nil, &product); err != nil {
			if err := stripeCall(cfg, "POST", "products", "trial-product-v1", url.Values{"id": {trialProductID}, "name": {"RelaisDesk — abonnement"}}, &product); err != nil {
				return nil, err
			}
		}
		interval := "month"
		if t.BillingCycle == "annual" {
			interval = "year"
		}
		form := url.Values{"customer": {t.StripeCustomerID}, "default_payment_method": {pm}, "metadata[relaisdesk_trial]": {t.ID},
			"trial_end": {strconv.FormatInt(t.TrialEnd, 10)}, "trial_settings[end_behavior][missing_payment_method]": {"cancel"},
			"items[0][price_data][currency]": {"eur"}, "items[0][price_data][product]": {trialProductID},
			"items[0][price_data][unit_amount]": {strconv.FormatInt(t.PriceCents, 10)}, "items[0][price_data][recurring][interval]": {interval},
			"items[0][quantity]": {"1"}, "collection_method": {"charge_automatically"}, "payment_settings[payment_method_types][0]": {"card"}}
		sub = &billingSubscription{}
		if err := stripeCall(cfg, "POST", "subscriptions", "trial-subscription:"+t.ID, form, sub); err != nil {
			return nil, err
		}
	}
	if !subscriptionMatchesTrial(sub, t) {
		return nil, errors.New("abonnement Stripe hors contrat")
	}
	active, err := dbpkg.ActivateTrial(db, t.ID, sub.ID, sub.TrialEnd)
	if err != nil {
		return nil, err
	}
	if err = syncTrialSubscription(db, cfg, active, sub); err != nil {
		return nil, err
	}
	if _, err = enqueueJSONJob(db, jobTrialNotice, trialNotice{ID: t.ID, Kind: "activated"}, "trial-activated:"+t.ID, 20); err != nil {
		return nil, err
	}
	return active, nil
}

func completeTrialSetup(db *sql.DB, cfg *config.Config, sessionID string) error {
	path, err := stripePath("checkout/sessions", sessionID)
	if err != nil {
		return err
	}
	var session billingSession
	if err = stripeCall(cfg, "GET", path+"?expand[]=setup_intent.payment_method", "", nil, &session); err != nil {
		return err
	}
	id := session.Metadata["relaisdesk_trial"]
	if id == "" {
		return nil
	}
	t, err := dbpkg.GetTrial(db, id)
	if err != nil {
		return err
	}
	if t.State == "rejected" {
		return nil
	}
	si := session.SetupIntent
	pm := si.PaymentMethod
	if session.ID != t.CheckoutSessionID || session.Customer != t.StripeCustomerID || session.ClientReferenceID != t.ID || session.Mode != "setup" || session.Status != "complete" ||
		si.Status != "succeeded" || si.Customer != t.StripeCustomerID || si.Metadata["relaisdesk_trial"] != t.ID || pm.Customer != t.StripeCustomerID || pm.Type != "card" || pm.Card.Fingerprint == "" {
		return errors.New("vérification de carte incomplète ou incohérente")
	}
	if t.CancelRequestedAt != 0 && t.State == "verified" {
		_, err = db.Exec(`UPDATE trial_applications SET state='rejected' WHERE id=?`, id)
		return err
	}
	if t.State == "verified" && !cfg.TrialsReady() {
		return errors.New("nouveaux essais désactivés")
	}
	// A consumed offer remains consumed even if new signups are disabled or
	// their HMAC configuration is unavailable. Recover the existing contract;
	// never send a false "no subscription created" rejection for that case.
	if t.State == "verified" {
		t, err = dbpkg.ReserveTrial(db, id, cfg.TrialKey(), pm.Card.Fingerprint, time.Now().UTC())
		if errors.Is(err, dbpkg.ErrTrialUsed) || errors.Is(err, dbpkg.ErrTrialUnavailable) {
			if _, e := db.Exec(`UPDATE trial_applications SET state='rejected' WHERE id=? AND state='verified'`, id); e != nil {
				return e
			}
			_, e := enqueueJSONJob(db, jobTrialNotice, trialNotice{ID: id, Kind: "rejected"}, "trial-rejected:"+id, 12)
			return e
		}
		if err != nil {
			return err
		}
	}
	_, err = provisionTrial(cfg, db, t, pm.ID)
	return err
}

func syncTrialSubscription(db *sql.DB, cfg *config.Config, t *dbpkg.Trial, sub *billingSubscription) error {
	if sub.ID != t.SubscriptionID || sub.Customer != t.StripeCustomerID || sub.Metadata["relaisdesk_trial"] != t.ID {
		return errors.New("abonnement incohérent")
	}
	// Reload consent changes made while a Stripe request was in flight.
	latest, err := dbpkg.GetTrial(db, t.ID)
	if err != nil {
		return err
	}
	t = latest
	if t.WithdrawalImmediate {
		if t.LicenseID != "" {
			if err = dbpkg.RevokeLicense(db, t.LicenseID, "Rétractation pendant essai gratuit"); err != nil {
				return err
			}
		}
		if sub.Status != "canceled" && sub.Status != "incomplete_expired" {
			path, _ := stripePath("subscriptions", sub.ID)
			if err = stripeCall(cfg, "DELETE", path, "", url.Values{"invoice_now": {"false"}, "prorate": {"false"}}, sub); err != nil {
				return err
			}
			if sub.Status != "canceled" {
				return errors.New("rétractation : arrêt Stripe non confirmé")
			}
		}
	} else if t.CancelRequestedAt != 0 && !sub.CancelAtPeriodEnd && sub.Status != "canceled" && sub.Status != "incomplete_expired" {
		path, _ := stripePath("subscriptions", sub.ID)
		// Setting a boolean is naturally idempotent. Do not reuse a cached
		// Stripe response if an out-of-band action has re-enabled renewal.
		if err := stripeCall(cfg, "POST", path, "", url.Values{"cancel_at_period_end": {"true"}}, sub); err != nil {
			return err
		}
	}
	var ended any
	if sub.Status == "canceled" || sub.Status == "incomplete_expired" {
		ended = time.Now().Unix()
	}
	_, err = db.Exec(`UPDATE trial_applications SET stripe_status=?,cancel_at_period_end=?,ended_at=COALESCE(ended_at,?) WHERE id=?`, sub.Status, sub.CancelAtPeriodEnd, ended, t.ID)
	// No renewal on subscription.updated or payment_failed: access naturally
	// stops at the trial/last paid end, including when Stripe is unreachable.
	return err
}

func processSubscriptionInvoice(db *sql.DB, cfg *config.Config, invoiceID string) error {
	path, err := stripePath("invoices", invoiceID)
	if err != nil {
		return err
	}
	var inv billingInvoice
	if err = stripeCall(cfg, "GET", path, "", nil, &inv); err != nil {
		return err
	}
	if inv.Subscription == "" {
		return nil
	}
	t, err := dbpkg.GetTrialBySubscription(db, inv.Subscription)
	if errors.Is(err, sql.ErrNoRows) {
		// The invoice event may precede checkout completion. Retry only our subs.
		sub, e := getSubscription(cfg, inv.Subscription)
		if e != nil {
			return e
		}
		if sub.Metadata["relaisdesk_trial"] != "" {
			return errors.New("activation d'essai encore en cours")
		}
		return nil
	}
	if err != nil {
		return err
	}
	if !inv.Paid || inv.Status != "paid" || inv.AmountPaid == 0 {
		return nil
	}
	sub, err := getSubscription(cfg, inv.Subscription)
	if err != nil {
		return err
	}
	if !subscriptionMatchesTrial(sub, t) || inv.Customer != t.StripeCustomerID || inv.Currency != "eur" || inv.AmountPaid != t.PriceCents || inv.Total != t.PriceCents || inv.Lines.HasMore || len(inv.Lines.Data) != 1 {
		return errors.New("facture récurrente hors contrat")
	}
	line := inv.Lines.Data[0]
	if line.Type != "subscription" || line.Subscription != sub.ID || line.Proration || line.Quantity != 1 || line.Price.ID != sub.Items.Data[0].Price.ID || line.Amount != t.PriceCents || line.Currency != "eur" {
		return errors.New("ligne de facture incohérente")
	}
	duration := line.Period.End - line.Period.Start
	if (t.BillingCycle == "monthly" && (duration < 28*86400 || duration > 31*86400)) || (t.BillingCycle == "annual" && (duration < 365*86400 || duration > 366*86400)) {
		return errors.New("durée de facturation incohérente")
	}
	order, err := dbpkg.RecordSubscriptionPayment(db, t.ID, inv.ID, inv.AmountPaid, line.Period.Start, line.Period.End)
	if err != nil {
		return err
	}
	if err = syncTrialSubscription(db, cfg, t, sub); err != nil {
		return err
	}
	if _, _, err = ensureOrderInvoice(db, cfg, order, "Carte Bancaire (Stripe)", "Abonnement / "+inv.ID); err != nil {
		return err
	}
	if t.WithdrawalImmediate {
		_, err = enqueueJSONJob(db, jobTrialNotice, trialNotice{ID: t.ID, Kind: "withdrawal_payment"}, "withdrawal-payment:"+inv.ID, 30)
		return err
	}
	return EnqueueOrderDeliveryEmail(db, order.OrderID)
}

func processSubscriptionFailure(db *sql.DB, cfg *config.Config, invoiceID string) error {
	path, err := stripePath("invoices", invoiceID)
	if err != nil {
		return err
	}
	var inv billingInvoice
	if err = stripeCall(cfg, "GET", path, "", nil, &inv); err != nil {
		return err
	}
	if inv.Subscription == "" || inv.Paid || inv.Status == "paid" {
		return nil
	}
	t, err := dbpkg.GetTrialBySubscription(db, inv.Subscription)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if inv.Customer != t.StripeCustomerID {
		return errors.New("client de facture incohérent")
	}
	sub, err := getSubscription(cfg, t.SubscriptionID)
	if err != nil {
		return err
	}
	if err = syncTrialSubscription(db, cfg, t, sub); err != nil {
		return err
	}
	_, err = enqueueJSONJob(db, jobTrialNotice, trialNotice{ID: t.ID, Kind: "payment_failed"}, "trial-payment-failed:"+inv.ID, 20)
	return err
}
