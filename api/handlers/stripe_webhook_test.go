package handlers

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	dbpkg "database"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"api/config"
)

func TestStripePaymentMustMatchOrderAmountCurrencyAndMode(t *testing.T) {
	order := &dbpkg.Order{
		PaymentMethod:   "stripe",
		StripeSessionID: "cs_test_expected",
		Price:           40,
	}
	session := stripeCheckoutSession{
		ID: "cs_test_expected", PaymentStatus: "paid", AmountTotal: 4000,
		Currency: "eur", Mode: "payment",
	}
	if !stripePaymentMatchesOrder(session, order) {
		t.Fatal("valid Stripe payment was rejected")
	}

	mutations := []func(*stripeCheckoutSession){
		func(s *stripeCheckoutSession) { s.AmountTotal = 1 },
		func(s *stripeCheckoutSession) { s.Currency = "usd" },
		func(s *stripeCheckoutSession) { s.Mode = "subscription" },
		func(s *stripeCheckoutSession) { s.ID = "cs_test_other" },
		func(s *stripeCheckoutSession) { s.PaymentStatus = "unpaid" },
	}
	for index, mutate := range mutations {
		candidate := session
		mutate(&candidate)
		if stripePaymentMatchesOrder(candidate, order) {
			t.Fatalf("mismatched Stripe payment mutation %d was accepted", index)
		}
	}
}

func TestStripeWebhookPersistsBeforeAsynchronousFulfillment(t *testing.T) {
	db, err := dbpkg.InitDatabase(filepath.Join(t.TempDir(), "stripe-queue.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	order, err := dbpkg.CreateOrderWithBilling(db, "client@example.com", "starter", 1, "stripe", "", "", &dbpkg.BillingDetails{
		Name: "Client", CustomerType: "business", TermsVersion: publicTermsVersion, TermsAccepted: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	sessionID := "cs_test_queue"
	if err := dbpkg.UpdateOrderStripeSessionID(db, order.OrderID, sessionID); err != nil {
		t.Fatal(err)
	}
	event := stripeEvent{ID: "evt_queue_1", Type: "checkout.session.completed"}
	event.Data.Object = stripeCheckoutSession{
		ID: sessionID, ClientReferenceID: order.OrderID, PaymentStatus: "paid",
		AmountTotal: int64(order.Price * 100), Currency: "eur", Mode: "payment",
	}
	body, _ := json.Marshal(event)
	secret := "whsec_test"
	timestamp := time.Now().Unix()
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(fmt.Sprintf("%d.%s", timestamp, body)))
	request := httptest.NewRequest(http.MethodPost, "/api/v1/stripe/webhook", bytes.NewReader(body))
	request.Header.Set("Stripe-Signature", fmt.Sprintf("t=%d,v1=%s", timestamp, hex.EncodeToString(mac.Sum(nil))))
	recorder := httptest.NewRecorder()
	cfg := &config.Config{StripeWebhookSecret: secret, InvoicesDir: filepath.Join(t.TempDir(), "invoices")}
	StripeWebhookHandler(db, cfg, nil).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	pending, _ := dbpkg.GetOrderByID(db, order.OrderID)
	if pending.Status != "pending" {
		t.Fatal("webhook performed business work before returning")
	}
	job, err := dbpkg.ClaimNextJob(db, time.Now().UTC())
	if err != nil || job == nil || job.Type != jobStripeEvent {
		t.Fatalf("stripe job=%+v err=%v", job, err)
	}
	if err := processJob(db, cfg, nil, job); err != nil {
		t.Fatal(err)
	}
	if err := dbpkg.CompleteJob(db, job.ID); err != nil {
		t.Fatal(err)
	}
	paid, _ := dbpkg.GetOrderByID(db, order.OrderID)
	if paid.Status != "paid" || paid.LicenseID == "" || paid.InvoiceNumber == "" {
		t.Fatalf("asynchronous fulfillment incomplete: %+v", paid)
	}
	next, err := dbpkg.ClaimNextJob(db, time.Now().UTC())
	if err != nil || next == nil || next.Type != jobOrderDelivery {
		t.Fatalf("delivery job=%+v err=%v", next, err)
	}
}
