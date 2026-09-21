package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	dbpkg "database"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"api/config"
	"api/mailer"
)

type stripeCustomerDetails struct {
	Name    string `json:"name"`
	Email   string `json:"email"`
	Address struct {
		City       string `json:"city"`
		Country    string `json:"country"`
		Line1      string `json:"line1"`
		Line2      string `json:"line2"`
		PostalCode string `json:"postal_code"`
		State      string `json:"state"`
	} `json:"address"`
	TaxIDs []struct {
		Type  string `json:"type"`
		Value string `json:"value"`
	} `json:"tax_ids"`
}

type stripeCheckoutSession struct {
	ID                string                `json:"id"`
	ClientReferenceID string                `json:"client_reference_id"`
	CustomerEmail     string                `json:"customer_email"`
	CustomerDetails   stripeCustomerDetails `json:"customer_details"`
	PaymentStatus     string                `json:"payment_status"`
	AmountTotal       int64                 `json:"amount_total"`
	Currency          string                `json:"currency"`
	Mode              string                `json:"mode"`
	Metadata          map[string]string     `json:"metadata"`
}

type stripeEvent struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Livemode *bool  `json:"livemode"`
	Data     struct {
		Object stripeCheckoutSession `json:"object"`
	} `json:"data"`
}

func stripePaymentMatchesOrder(session stripeCheckoutSession, order *dbpkg.Order) bool {
	if order == nil || order.PaymentMethod != "stripe" || session.ID == "" ||
		order.StripeSessionID == "" || order.StripeSessionID != session.ID {
		return false
	}
	expectedAmount := int64(mathRound(order.Price * 100))
	return session.PaymentStatus == "paid" && session.Mode == "payment" &&
		strings.EqualFold(session.Currency, "eur") && session.AmountTotal == expectedAmount
}

// StripeWebhookHandler handles Stripe events and activates licenses upon payment.
func StripeWebhookHandler(db *sql.DB, cfg *config.Config, _ *mailer.Mailer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cfg.StripeWebhookSecret == "" {
			log.Printf("[Stripe Webhook] Refused because STRIPE_WEBHOOK_SECRET is not configured")
			http.Error(w, "Webhook unavailable", http.StatusServiceUnavailable)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Cannot read body", http.StatusBadRequest)
			return
		}

		sigHeader := r.Header.Get("Stripe-Signature")
		if !verifyStripeSignature(body, sigHeader, cfg.StripeWebhookSecret) {
			log.Printf("[Stripe Webhook] Invalid signature")
			http.Error(w, "Invalid signature", http.StatusBadRequest)
			return
		}

		var event stripeEvent
		if err := json.Unmarshal(body, &event); err != nil {
			http.Error(w, "Invalid event JSON", http.StatusBadRequest)
			return
		}
		if cfg.StripeMode() != "" && (event.Livemode == nil || *event.Livemode != (cfg.StripeMode() == "live")) {
			http.Error(w, "Stripe mode mismatch", http.StatusBadRequest)
			return
		}

		log.Printf("[Stripe Webhook] Event received: %s (ID: %s)", event.Type, event.ID)
		if event.ID == "" || len(event.ID) > 255 {
			http.Error(w, "Invalid event ID", http.StatusBadRequest)
			return
		}
		if event.Type != "checkout.session.completed" && event.Type != "invoice.paid" && event.Type != "invoice.payment_failed" && event.Type != "customer.subscription.updated" && event.Type != "customer.subscription.deleted" {
			writeJSON(w, http.StatusOK, map[string]any{"received": true, "queued": false})
			return
		}
		queued, err := EnqueueStripeEvent(db, event.ID, body)
		if err != nil {
			http.Error(w, "Cannot persist event", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"received": true, "queued": queued})
	}
}

func processStripeEvent(db *sql.DB, cfg *config.Config, body []byte) error {
	var event stripeEvent
	if err := json.Unmarshal(body, &event); err != nil {
		return err
	}
	if cfg.StripeMode() != "" && (event.Livemode == nil || *event.Livemode != (cfg.StripeMode() == "live")) {
		return errors.New("événement Stripe d'un autre environnement")
	}
	if event.Type == "invoice.paid" {
		return processSubscriptionInvoice(db, cfg, event.Data.Object.ID)
	}
	if event.Type == "invoice.payment_failed" {
		return processSubscriptionFailure(db, cfg, event.Data.Object.ID)
	}
	if event.Type == "customer.subscription.updated" || event.Type == "customer.subscription.deleted" {
		t, err := dbpkg.GetTrialBySubscription(db, event.Data.Object.ID)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		sub, err := getSubscription(cfg, t.SubscriptionID)
		if err != nil {
			return err
		}
		return syncTrialSubscription(db, cfg, t, sub)
	}
	if event.Type != "checkout.session.completed" {
		return nil
	}
	session := event.Data.Object
	if session.Mode == "setup" && session.Metadata["relaisdesk_trial"] != "" {
		return completeTrialSetup(db, cfg, session.ID)
	}
	if session.PaymentStatus != "paid" {
		return nil
	}
	orderID := session.ClientReferenceID
	if orderID == "" && session.Metadata != nil {
		orderID = session.Metadata["order_id"]
	}
	var order *dbpkg.Order
	var err error
	if orderID != "" {
		order, err = dbpkg.GetOrderByID(db, orderID)
	}
	if order == nil && session.ID != "" {
		order, err = dbpkg.GetOrderByStripeSessionID(db, session.ID)
	}
	if err != nil || order == nil {
		return fmt.Errorf("commande Stripe introuvable pour %s: %w", session.ID, err)
	}
	if !stripePaymentMatchesOrder(session, order) {
		log.Printf("[Stripe Jobs] paiement incohérent ignoré pour la commande %s", order.OrderID)
		return nil
	}

	if order.Status == "pending" {
		bName := session.CustomerDetails.Name
		bAddr := session.CustomerDetails.Address.Line1
		if session.CustomerDetails.Address.Line2 != "" {
			bAddr += ", " + session.CustomerDetails.Address.Line2
		}
		bCountry := session.CustomerDetails.Address.Country
		if bCountry == "" {
			bCountry = "France"
		}
		var bSiret string
		if len(session.CustomerDetails.TaxIDs) > 0 {
			bSiret = session.CustomerDetails.TaxIDs[0].Value
		}
		if order.BillingName == "" && bName != "" {
			_ = dbpkg.UpdateOrderBillingDetails(db, order.OrderID, &dbpkg.BillingDetails{
				Name: bName, Address: bAddr, PostalCode: session.CustomerDetails.Address.PostalCode,
				City: session.CustomerDetails.Address.City, Country: bCountry, SIRET: bSiret,
			})
			order.BillingName, order.BillingAddress = bName, bAddr
			order.BillingPostalCode, order.BillingCity = session.CustomerDetails.Address.PostalCode, session.CustomerDetails.Address.City
			order.BillingCountry, order.BillingSIRET = bCountry, bSiret
		}
		if order.OrderKind == "renewal" {
			_, err = dbpkg.FulfillPendingRenewalOrder(db, order.OrderID, "Stripe "+event.ID)
		} else {
			_, err = dbpkg.FulfillPendingOrder(db, order.OrderID, "Stripe "+event.ID)
		}
		if err != nil {
			return err
		}
		order, err = dbpkg.GetOrderByID(db, order.OrderID)
		if err != nil {
			return err
		}
	} else if order.Status != "paid" || order.LicenseID == "" {
		return fmt.Errorf("état de commande Stripe incompatible: %s", order.Status)
	}
	if _, _, err := ensureOrderInvoice(db, cfg, order, "Carte Bancaire (Stripe)", "Paiement Stripe "+session.ID); err != nil {
		return err
	}
	return EnqueueOrderDeliveryEmail(db, order.OrderID)
}

func verifyStripeSignature(payload []byte, sigHeader, secret string) bool {
	if sigHeader == "" || secret == "" {
		return false
	}

	var timestamp string
	var signatures []string

	for _, part := range strings.Split(sigHeader, ",") {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) == 2 {
			if kv[0] == "t" {
				timestamp = kv[1]
			} else if kv[0] == "v1" {
				signatures = append(signatures, kv[1])
			}
		}
	}

	if timestamp == "" || len(signatures) == 0 {
		return false
	}

	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return false
	}

	// Stripe's recommended replay tolerance is five minutes.
	if time.Since(time.Unix(ts, 0)).Abs() > 5*time.Minute {
		return false
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(fmt.Sprintf("%s.%s", timestamp, string(payload))))
	expectedSig := hex.EncodeToString(mac.Sum(nil))

	for _, sig := range signatures {
		if hmac.Equal([]byte(sig), []byte(expectedSig)) {
			return true
		}
	}
	return false
}
