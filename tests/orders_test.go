package tests

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	dbpkg "database"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"api/config"
	"api/handlers"
	"api/mailer"
)

func TestPublicOrdersAndWebhookFlow(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test_orders_api.db")
	db, err := dbpkg.InitDatabase(dbPath)
	if err != nil {
		t.Fatalf("InitDatabase failed: %v", err)
	}
	defer db.Close()

	cfg := &config.Config{
		BankIBAN:            "FR76 3000 6000 0112 3456 7890 189",
		BankBIC:             "AGRIFRPP",
		BankHolder:          "RelaisDesk Support",
		StripeSecretKey:     "", // Mock mode in tests
		StripeWebhookSecret: "whsec_test_orders",
		StripeMock:          true,
		DevHTTP:             true,
		StripeSuccessURL:    "https://relaisdesk.example/commande.html",
		StripeCancelURL:     "https://relaisdesk.example/commande.html",
		DownloadsDir:        t.TempDir(),
		InvoicesDir:         t.TempDir(),
	}

	mail := mailer.NewMailer("", 0, "", "", "test@relaisdesk.fr")

	// 1. Test POST /api/v1/public/order with bank transfer
	reqBody, _ := json.Marshal(map[string]interface{}{
		"email":          "tech_virement@example.com",
		"plan":           "starter",
		"technicians":    1,
		"payment_method": "bank_transfer",
		"name":           "Entreprise Test",
		"address":        "1 rue du Test",
		"postal_code":    "75001",
		"city":           "Paris",
		"customer_type":  "business",
		"terms_version":  "2026-09-21",
		"terms_accepted": true,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/public/order", bytes.NewReader(reqBody))
	w := httptest.NewRecorder()
	handlers.PublicOrderHandler(db, cfg, mail)(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("PublicOrderHandler bank_transfer status %d, body: %s", w.Code, w.Body.String())
	}

	var bankResp struct {
		OrderID       string  `json:"order_id"`
		Price         float64 `json:"price"`
		PaymentMethod string  `json:"payment_method"`
		Instructions  struct {
			IBAN      string  `json:"iban"`
			Reference string  `json:"reference"`
			Amount    float64 `json:"amount"`
		} `json:"instructions"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &bankResp); err != nil {
		t.Fatalf("Unmarshal bankResp failed: %v", err)
	}

	if bankResp.Price != 24.90 || bankResp.Instructions.Reference != bankResp.OrderID {
		t.Errorf("Unexpected bank order response: %+v", bankResp)
	}

	// 2. Test Admin Mark Paid for this bank transfer order
	markReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/orders/"+bankResp.OrderID+"/mark-paid", nil)
	markW := httptest.NewRecorder()
	handlers.AdminMarkOrderPaidHandler(db, cfg, mail)(markW, markReq)

	if markW.Code != http.StatusOK {
		t.Fatalf("AdminMarkOrderPaidHandler status %d, body: %s", markW.Code, markW.Body.String())
	}

	var markResp struct {
		Status     string `json:"status"`
		LicenseID  string `json:"license_id"`
		LicenseKey string `json:"license_key"`
	}
	if err := json.Unmarshal(markW.Body.Bytes(), &markResp); err != nil {
		t.Fatalf("Unmarshal markResp failed: %v", err)
	}
	if markResp.Status != "paid" || markResp.LicenseID == "" {
		t.Errorf("Unexpected markResp: %+v", markResp)
	}

	// 3. Test Public License Status Check
	statusReqBody, _ := json.Marshal(map[string]string{
		"license_id": markResp.LicenseID,
	})
	statusReq := httptest.NewRequest(http.MethodPost, "/api/v1/public/license/status", bytes.NewReader(statusReqBody))
	statusW := httptest.NewRecorder()
	handlers.PublicLicenseStatusHandler(db)(statusW, statusReq)

	if statusW.Code != http.StatusOK {
		t.Fatalf("PublicLicenseStatusHandler status %d, body: %s", statusW.Code, statusW.Body.String())
	}

	var statusResp struct {
		Valid          bool   `json:"valid"`
		Status         string `json:"status"`
		MaxConnections int    `json:"max_connections"`
	}
	if err := json.Unmarshal(statusW.Body.Bytes(), &statusResp); err != nil {
		t.Fatalf("Unmarshal statusResp failed: %v", err)
	}
	if !statusResp.Valid || statusResp.Status != "active" || statusResp.MaxConnections != 1 {
		t.Errorf("Unexpected statusResp: %+v", statusResp)
	}

	// 4. Test Stripe Order & Webhook Flow
	stripeReqBody, _ := json.Marshal(map[string]interface{}{
		"email":          "tech_stripe@example.com",
		"plan":           "ultra",
		"technicians":    30,
		"payment_method": "stripe",
		"name":           "Entreprise Test",
		"address":        "1 rue du Test",
		"postal_code":    "75001",
		"city":           "Paris",
		"customer_type":  "business",
		"terms_version":  "2026-09-21",
		"terms_accepted": true,
	})
	stripeReq := httptest.NewRequest(http.MethodPost, "/api/v1/public/order", bytes.NewReader(stripeReqBody))
	stripeW := httptest.NewRecorder()
	handlers.PublicOrderHandler(db, cfg, mail)(stripeW, stripeReq)

	if stripeW.Code != http.StatusCreated {
		t.Fatalf("PublicOrderHandler stripe status %d, body: %s", stripeW.Code, stripeW.Body.String())
	}

	var stripeOrderResp struct {
		OrderID     string  `json:"order_id"`
		Price       float64 `json:"price"`
		CheckoutURL string  `json:"checkout_url"`
		SessionID   string  `json:"session_id"`
	}
	if err := json.Unmarshal(stripeW.Body.Bytes(), &stripeOrderResp); err != nil {
		t.Fatalf("Unmarshal stripeOrderResp failed: %v", err)
	}

	// Ultra 30 techs = 199.00 + 20*14 = 479.00€
	if stripeOrderResp.Price != 479.00 || stripeOrderResp.SessionID == "" {
		t.Errorf("Unexpected stripe order resp: %+v", stripeOrderResp)
	}

	// Trigger Stripe Webhook
	webhookPayload := map[string]interface{}{
		"id":   "evt_test_123",
		"type": "checkout.session.completed",
		"data": map[string]interface{}{
			"object": map[string]interface{}{
				"id":                  stripeOrderResp.SessionID,
				"client_reference_id": stripeOrderResp.OrderID,
				"customer_email":      "tech_stripe@example.com",
				"payment_status":      "paid",
				"amount_total":        47900,
				"currency":            "eur",
				"mode":                "payment",
			},
		},
	}
	wbBytes, _ := json.Marshal(webhookPayload)
	wbReq := httptest.NewRequest(http.MethodPost, "/api/v1/stripe/webhook", bytes.NewReader(wbBytes))
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	mac := hmac.New(sha256.New, []byte(cfg.StripeWebhookSecret))
	mac.Write([]byte(timestamp + "." + string(wbBytes)))
	wbReq.Header.Set("Stripe-Signature", "t="+timestamp+",v1="+fmt.Sprintf("%x", mac.Sum(nil)))
	wbW := httptest.NewRecorder()
	handlers.StripeWebhookHandler(db, cfg, mail)(wbW, wbReq)

	if wbW.Code != http.StatusOK {
		t.Fatalf("StripeWebhookHandler status %d, body: %s", wbW.Code, wbW.Body.String())
	}

	// The signed webhook only persists the event. The lightweight local worker
	// then fulfills it, which keeps Stripe retries fast and restart-safe.
	workerCtx, stopWorker := context.WithCancel(context.Background())
	defer stopWorker()
	go handlers.RunJobWorker(workerCtx, db, cfg, mail)
	deadline := time.Now().Add(3 * time.Second)
	var paidOrder *dbpkg.Order
	for time.Now().Before(deadline) {
		paidOrder, err = dbpkg.GetOrderByID(db, stripeOrderResp.OrderID)
		if err == nil && paidOrder.Status == "paid" && paidOrder.LicenseID != "" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("GetOrderByID failed: %v", err)
	}
	if paidOrder.Status != "paid" || paidOrder.LicenseID == "" {
		t.Fatalf("Order was not updated by the persistent worker: %+v", paidOrder)
	}

	// Verify license created with 30 connections
	lic, err := dbpkg.GetLicense(db, paidOrder.LicenseID)
	if err != nil {
		t.Fatalf("GetLicense failed: %v", err)
	}
	if lic.MaxConnections != 30 || lic.Email != "tech_stripe@example.com" {
		t.Errorf("License mismatch: %+v", lic)
	}
}
