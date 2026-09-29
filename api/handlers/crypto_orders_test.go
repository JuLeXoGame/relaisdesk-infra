package handlers

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	dbpkg "database"

	"api/config"
	"api/cryptopay"
)

// mockRateServer stubs the OKX ticker for every asset.
func mockRateServer(t *testing.T, last string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{"last":"` + last + `","ts":"1758499200000"}]}`))
	}))
}

func cryptoTestConfig(okxURL string) *config.Config {
	return &config.Config{
		CryptoEnabled:         true,
		CryptoQuoteTTLMinutes: 30,
		OKXBaseURL:            okxURL,
		OKXAPIKey:             "test-key",
		OKXAPISecret:          "test-secret",
		OKXAPIPassphrase:      "test-pass",
		OKXBTCDepositAddress:  "bc1qoperator",
		OKXXRPDepositAddress:  "rOperator",
		OKXXRPDepositTag:      "123456",
		InvoicesDir:           "",
	}
}

func cryptoOrderBody(method string) []byte {
	return []byte(`{"email":"crypto@example.com","plan":"starter","technicians":1,"payment_method":"` + method + `","name":"Client Test","address":"1 rue du Test","postal_code":"75001","city":"Paris","customer_type":"business","terms_version":"` + publicTermsVersion + `","terms_accepted":true}`)
}

func TestPublicOrderCryptoBTCIssuesQuote(t *testing.T) {
	okx := mockRateServer(t, "100000")
	defer okx.Close()

	db, err := dbpkg.InitDatabase(filepath.Join(t.TempDir(), "crypto-order.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	cfg := cryptoTestConfig(okx.URL)
	cfg.InvoicesDir = t.TempDir()
	recorder := httptest.NewRecorder()
	PublicOrderHandler(db, cfg, nil)(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/public/order", bytes.NewReader(cryptoOrderBody("crypto_btc"))))
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %q", recorder.Code, recorder.Body.String())
	}
	var recap map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &recap); err != nil {
		t.Fatal(err)
	}
	if recap["asset"] != "BTC" || recap["amount_crypto"] != "0.00024900" || recap["rate_eur"] != "100000" {
		t.Fatalf("recap = %v", recap)
	}
	if recap["pay_address"] != "bc1qoperator" {
		t.Fatalf("recap = %v", recap)
	}
	if _, ok := recap["dest_tag"]; ok {
		t.Fatalf("unexpected dest_tag: %v", recap)
	}
	orderID, _ := recap["order_id"].(string)
	quote, err := dbpkg.GetCryptoQuoteByOrderID(db, orderID)
	if err != nil || quote.Status != "pending" || quote.PayAddress != "bc1qoperator" {
		t.Fatalf("stored quote = %+v, %v", quote, err)
	}
}

func TestPublicOrderCryptoXRPShowsFixedTag(t *testing.T) {
	okx := mockRateServer(t, "2.50")
	defer okx.Close()

	db, err := dbpkg.InitDatabase(filepath.Join(t.TempDir(), "crypto-order-xrp.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	cfg := cryptoTestConfig(okx.URL)
	cfg.InvoicesDir = t.TempDir()
	recorder := httptest.NewRecorder()
	PublicOrderHandler(db, cfg, nil)(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/public/order", bytes.NewReader(cryptoOrderBody("crypto_xrp"))))
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %q", recorder.Code, recorder.Body.String())
	}
	var recap map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &recap); err != nil {
		t.Fatal(err)
	}
	if recap["asset"] != "XRP" || recap["amount_crypto"] != "9.960000" {
		t.Fatalf("recap = %v", recap)
	}
	if recap["pay_address"] != "rOperator" {
		t.Fatalf("recap = %v", recap)
	}
	// The OKX deposit tag is fixed: every order shows the same one and
	// deposits are told apart by amount and time window.
	if tag, ok := recap["dest_tag"].(float64); !ok || tag != 123456 {
		t.Fatalf("dest_tag = %v", recap["dest_tag"])
	}
	recorder2 := httptest.NewRecorder()
	PublicOrderHandler(db, cfg, nil)(recorder2, httptest.NewRequest(http.MethodPost, "/api/v1/public/order", bytes.NewReader(cryptoOrderBody("crypto_xrp"))))
	if recorder2.Code != http.StatusCreated {
		t.Fatalf("second status = %d", recorder2.Code)
	}
	var recap2 map[string]any
	if err := json.Unmarshal(recorder2.Body.Bytes(), &recap2); err != nil {
		t.Fatal(err)
	}
	if recap2["dest_tag"] != recap["dest_tag"] {
		t.Fatal("fixed deposit tag changed between orders")
	}
}

func TestPublicOrderCryptoDisabled(t *testing.T) {
	db, err := dbpkg.InitDatabase(filepath.Join(t.TempDir(), "crypto-disabled.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	recorder := httptest.NewRecorder()
	PublicOrderHandler(db, &config.Config{}, nil)(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/public/order", bytes.NewReader(cryptoOrderBody("crypto_btc"))))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, body = %q", recorder.Code, recorder.Body.String())
	}
}

// mockOKXServer stubs ticker, deposit address and deposit history. Deposits
// are served per currency and every private call must carry a valid OKX
// signature.
func mockOKXServer(t *testing.T, deposits map[string]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/v5/market/ticker"):
			last := "100000"
			if strings.Contains(r.URL.RawQuery, "XRP") {
				last = "2.50"
			}
			_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{"last":"` + last + `","ts":"1758499200000"}]}`))
			return
		case strings.HasPrefix(r.URL.Path, "/api/v5/asset/deposit-address"):
			ccy := r.URL.Query().Get("ccy")
			addr, tag := "bc1qoperator", ""
			if ccy == "XRP" {
				addr, tag = "rOperator", "123456"
			}
			_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{"addr":"` + addr + `","tag":"` + tag + `","ccy":"` + ccy + `","selected":true}]}`))
			return
		case strings.HasPrefix(r.URL.Path, "/api/v5/asset/deposit-history"):
			ts := r.Header.Get("OK-ACCESS-TIMESTAMP")
			want := cryptopay.SignOKXRequest("test-secret", ts, r.Method, r.URL.RequestURI(), "")
			if r.Header.Get("OK-ACCESS-SIGN") != want || r.Header.Get("OK-ACCESS-KEY") != "test-key" {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"code":"50111","msg":"Invalid signature"}`))
				return
			}
			rows := deposits[r.URL.Query().Get("ccy")]
			_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[` + rows + `]}`))
			return
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func okxDepositRow(ccy, amt, to, txID, depID string, tsMillis int64) string {
	return fmt.Sprintf(`{"ccy":%q,"amt":%q,"from":"sender-%s","to":%q,"txId":%q,"ts":"%d","state":"2","depId":%q}`,
		ccy, amt, depID, to, txID, tsMillis, depID)
}

func TestOKXWatcherSettlesBothAssets(t *testing.T) {
	now := time.Now().UTC()
	okx := mockOKXServer(t, map[string]string{
		"BTC": okxDepositRow("BTC", "0.00024900", "bc1qoperator", "BTCHASH", "dep-btc-1", now.UnixMilli()),
		"XRP": okxDepositRow("XRP", "9.960000", "rOperator", "XRPHASH", "dep-xrp-1", now.UnixMilli()),
	})
	defer okx.Close()

	db, err := dbpkg.InitDatabase(filepath.Join(t.TempDir(), "crypto-okx-watch.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	cfg := cryptoTestConfig(okx.URL)
	cfg.InvoicesDir = t.TempDir()
	orderIDs := map[string]string{}
	for _, method := range []string{"crypto_btc", "crypto_xrp"} {
		recorder := httptest.NewRecorder()
		PublicOrderHandler(db, cfg, nil)(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/public/order", bytes.NewReader(cryptoOrderBody(method))))
		if recorder.Code != http.StatusCreated {
			t.Fatalf("%s status = %d, body = %q", method, recorder.Code, recorder.Body.String())
		}
		var recap map[string]any
		if err := json.Unmarshal(recorder.Body.Bytes(), &recap); err != nil {
			t.Fatal(err)
		}
		orderIDs[method], _ = recap["order_id"].(string)
	}
	pollOKXDepositsOnce(context.Background(), db, cfg, nil)
	// Drain the queued jobs like the worker does (mail stays nil: deposit
	// events settle without a mailer, instructions and delivery emails are
	// completed without sending in this test).
	settled := 0
	for i := 0; i < 10; i++ {
		job, err := dbpkg.ClaimNextJob(db, time.Now().UTC())
		if err != nil {
			t.Fatalf("claim job: %v", err)
		}
		if job == nil {
			break
		}
		if job.Type != jobCryptoEvent {
			if err := dbpkg.CompleteJob(db, job.ID); err != nil {
				t.Fatalf("complete job %d: %v", job.ID, err)
			}
			continue
		}
		if err := processJob(db, cfg, nil, job); err != nil {
			t.Fatalf("process job %d: %v", job.ID, err)
		}
		settled++
	}
	if settled != 2 {
		t.Fatalf("settled %d deposit events, want 2", settled)
	}
	for method, orderID := range orderIDs {
		order, err := dbpkg.GetOrderByID(db, orderID)
		if err != nil || order.Status != "paid" || order.LicenseID == "" {
			t.Fatalf("%s order = %+v, %v", method, order, err)
		}
		quote, err := dbpkg.GetCryptoQuoteByOrderID(db, orderID)
		if err != nil || quote.Status != "paid" || quote.TxID == "" {
			t.Fatalf("%s quote = %+v, %v", method, quote, err)
		}
		payments, err := dbpkg.ListCryptoPaymentsByOrder(db, orderID)
		if err != nil || len(payments) != 1 || payments[0].InvoiceNumber == "" {
			t.Fatalf("%s registry = %+v, %v", method, payments, err)
		}
	}
	// A second poll re-enqueues nothing (job dedup on the OKX deposit id).
	pollOKXDepositsOnce(context.Background(), db, cfg, nil)
	if job, err := dbpkg.ClaimNextJob(db, time.Now().UTC()); err != nil || job != nil {
		t.Fatalf("duplicate job queued: %+v %v", job, err)
	}
}

func TestOKXWatcherIgnoresUnmatchedDeposits(t *testing.T) {
	now := time.Now().UTC()
	okx := mockOKXServer(t, map[string]string{
		// Underpaid: never matches, the order stays pending.
		"BTC": okxDepositRow("BTC", "0.00000001", "bc1qoperator", "DUST", "dep-dust-1", now.UnixMilli()),
	})
	defer okx.Close()

	db, err := dbpkg.InitDatabase(filepath.Join(t.TempDir(), "crypto-okx-unmatched.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	cfg := cryptoTestConfig(okx.URL)
	cfg.InvoicesDir = t.TempDir()
	recorder := httptest.NewRecorder()
	PublicOrderHandler(db, cfg, nil)(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/public/order", bytes.NewReader(cryptoOrderBody("crypto_btc"))))
	if recorder.Code != http.StatusCreated {
		t.Fatalf("order status = %d", recorder.Code)
	}
	var recap map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &recap); err != nil {
		t.Fatal(err)
	}
	orderID, _ := recap["order_id"].(string)
	drainCryptoInstructions(t, db, 1)
	pollOKXDepositsOnce(context.Background(), db, cfg, nil)
	if job, err := dbpkg.ClaimNextJob(db, time.Now().UTC()); err != nil || job != nil {
		t.Fatalf("unexpected job: %+v %v", job, err)
	}
	order, err := dbpkg.GetOrderByID(db, orderID)
	if err != nil || order.Status != "pending" {
		t.Fatalf("order = %+v, %v", order, err)
	}
}

func TestOKXWatcherPausesOnAddressMismatch(t *testing.T) {
	now := time.Now().UTC()
	okx := mockOKXServer(t, map[string]string{
		"BTC": okxDepositRow("BTC", "0.00024900", "bc1qoperator", "BTCHASH", "dep-btc-2", now.UnixMilli()),
	})
	defer okx.Close()

	db, err := dbpkg.InitDatabase(filepath.Join(t.TempDir(), "crypto-okx-mismatch.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	cfg := cryptoTestConfig(okx.URL)
	cfg.InvoicesDir = t.TempDir()
	recorder := httptest.NewRecorder()
	PublicOrderHandler(db, cfg, nil)(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/public/order", bytes.NewReader(cryptoOrderBody("crypto_btc"))))
	if recorder.Code != http.StatusCreated {
		t.Fatalf("order status = %d", recorder.Code)
	}
	drainCryptoInstructions(t, db, 1)
	// Operator typo: the configured address is not the OKX account address.
	cfg.OKXBTCDepositAddress = "bc1qTYPO"
	pollOKXDepositsOnce(context.Background(), db, cfg, nil)
	if job, err := dbpkg.ClaimNextJob(db, time.Now().UTC()); err != nil || job != nil {
		t.Fatalf("job queued despite address mismatch: %+v %v", job, err)
	}
}

func TestCryptoQuoteRefresh(t *testing.T) {
	okx := mockRateServer(t, "100000")
	defer okx.Close()

	db, err := dbpkg.InitDatabase(filepath.Join(t.TempDir(), "crypto-refresh.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	cfg := cryptoTestConfig(okx.URL)
	cfg.InvoicesDir = t.TempDir()
	orderReq := httptest.NewRecorder()
	PublicOrderHandler(db, cfg, nil)(orderReq, httptest.NewRequest(http.MethodPost, "/api/v1/public/order", bytes.NewReader(cryptoOrderBody("crypto_btc"))))
	if orderReq.Code != http.StatusCreated {
		t.Fatalf("order status = %d", orderReq.Code)
	}
	var recap map[string]any
	if err := json.Unmarshal(orderReq.Body.Bytes(), &recap); err != nil {
		t.Fatal(err)
	}
	orderID, _ := recap["order_id"].(string)

	refresh := func(order, email string) int {
		recorder := httptest.NewRecorder()
		CryptoQuoteRefreshHandler(db, cfg)(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/public/crypto/quote",
			strings.NewReader(`{"order_id":"`+order+`","email":"`+email+`"}`)))
		return recorder.Code
	}
	// Live quote: refresh refused.
	if got := refresh(orderID, "crypto@example.com"); got != http.StatusConflict {
		t.Fatalf("live refresh status = %d", got)
	}
	if got := refresh(orderID, "other@example.com"); got != http.StatusNotFound {
		t.Fatalf("wrong email status = %d", got)
	}
	// Expire the quote, then refresh succeeds with a new live quote.
	if _, err := db.Exec(`UPDATE crypto_quotes SET status = 'expired' WHERE order_id = ?`, orderID); err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	CryptoQuoteRefreshHandler(db, cfg)(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/public/crypto/quote",
		strings.NewReader(`{"order_id":"`+orderID+`","email":"crypto@example.com"}`)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("refresh status = %d, body = %q", recorder.Code, recorder.Body.String())
	}
	quote, err := dbpkg.GetCryptoQuoteByOrderID(db, orderID)
	if err != nil || quote.Status != "pending" {
		t.Fatalf("refreshed quote = %+v, %v", quote, err)
	}
}

func TestCryptoEventReplayConverges(t *testing.T) {
	okx := mockRateServer(t, "100000")
	defer okx.Close()

	db, err := dbpkg.InitDatabase(filepath.Join(t.TempDir(), "crypto-replay.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	cfg := cryptoTestConfig(okx.URL)
	cfg.InvoicesDir = t.TempDir()
	orderReq := httptest.NewRecorder()
	PublicOrderHandler(db, cfg, nil)(orderReq, httptest.NewRequest(http.MethodPost, "/api/v1/public/order", bytes.NewReader(cryptoOrderBody("crypto_btc"))))
	if orderReq.Code != http.StatusCreated {
		t.Fatalf("order status = %d", orderReq.Code)
	}
	var recap map[string]any
	if err := json.Unmarshal(orderReq.Body.Bytes(), &recap); err != nil {
		t.Fatal(err)
	}
	orderID, _ := recap["order_id"].(string)
	quote, err := dbpkg.GetCryptoQuoteByOrderID(db, orderID)
	if err != nil {
		t.Fatal(err)
	}
	payload := fmt.Sprintf(`{"kind":"okx","reference":%q,"quote_id":%d,"txid":"BTCHASH","sender":"bc1qsender"}`, orderID, quote.ID)
	if err := processCryptoEvent(db, cfg, []byte(payload)); err != nil {
		t.Fatalf("settle: %v", err)
	}
	if err := processCryptoEvent(db, cfg, []byte(payload)); err != nil {
		t.Fatalf("replay: %v", err)
	}
	payments, _ := dbpkg.ListCryptoPaymentsByOrder(db, orderID)
	if len(payments) != 1 {
		t.Fatalf("registry duplicated on replay: %d rows", len(payments))
	}
	if err := processCryptoEvent(db, cfg, []byte(`{"kind":"nope","reference":"x"}`)); err == nil {
		t.Fatal("unknown event kind accepted")
	}
}

// drainCryptoInstructions completes the instructions emails queued by order
// creation so deposit-driven assertions only see watcher jobs.
func drainCryptoInstructions(t *testing.T, db *sql.DB, want int) {
	t.Helper()
	for i := 0; i < want; i++ {
		job, err := dbpkg.ClaimNextJob(db, time.Now().UTC())
		if err != nil || job == nil || job.Type != jobCryptoInstructions {
			t.Fatalf("instructions job %d = %+v, %v", i, job, err)
		}
		if err := dbpkg.CompleteJob(db, job.ID); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCryptoOrderEnqueuesInstructionsEmail(t *testing.T) {
	okx := mockRateServer(t, "100000")
	defer okx.Close()

	db, err := dbpkg.InitDatabase(filepath.Join(t.TempDir(), "crypto-instructions.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	cfg := cryptoTestConfig(okx.URL)
	cfg.InvoicesDir = t.TempDir()
	recorder := httptest.NewRecorder()
	PublicOrderHandler(db, cfg, nil)(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/public/order", bytes.NewReader(cryptoOrderBody("crypto_btc"))))
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d", recorder.Code)
	}
	var recap map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &recap); err != nil {
		t.Fatal(err)
	}
	orderID, _ := recap["order_id"].(string)
	job, err := dbpkg.ClaimNextJob(db, time.Now().UTC())
	if err != nil || job == nil || job.Type != jobCryptoInstructions {
		t.Fatalf("instructions job = %+v, %v", job, err)
	}
	if !strings.Contains(job.Payload, orderID) {
		t.Fatalf("instructions payload = %q, want order %s", job.Payload, orderID)
	}
}

func TestCryptoPaymentResumesAfterInterruption(t *testing.T) {
	okx := mockRateServer(t, "100000")
	defer okx.Close()

	db, err := dbpkg.InitDatabase(filepath.Join(t.TempDir(), "crypto-resume.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	cfg := cryptoTestConfig(okx.URL)
	cfg.InvoicesDir = t.TempDir()
	recorder := httptest.NewRecorder()
	PublicOrderHandler(db, cfg, nil)(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/public/order", bytes.NewReader(cryptoOrderBody("crypto_btc"))))
	if recorder.Code != http.StatusCreated {
		t.Fatalf("order status = %d", recorder.Code)
	}
	var recap map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &recap); err != nil {
		t.Fatal(err)
	}
	orderID, _ := recap["order_id"].(string)
	drainCryptoInstructions(t, db, 1)

	quote, err := dbpkg.GetCryptoQuoteByOrderID(db, orderID)
	if err != nil || quote == nil {
		t.Fatal(err)
	}
	// Simulate an interrupted first attempt: quote claimed, order still pending.
	claimed, err := dbpkg.MarkCryptoQuotePaidByID(db, quote.ID, "tx-resume-1", "r-sender", 1)
	if err != nil || !claimed {
		t.Fatalf("claim = %v, %v", claimed, err)
	}
	// The retry must resume fulfillment instead of converging silently.
	if err := completeCryptoPayment(db, cfg, orderID, quote.ID, "tx-resume-1", "r-sender", 1); err != nil {
		t.Fatalf("resume: %v", err)
	}
	order, err := dbpkg.GetOrderByID(db, orderID)
	if err != nil || order.Status != "paid" || order.LicenseID == "" {
		t.Fatalf("resumed order = %+v, %v", order, err)
	}
	payments, err := dbpkg.ListCryptoPaymentsByOrder(db, orderID)
	if err != nil || len(payments) != 1 || payments[0].InvoiceNumber == "" {
		t.Fatalf("registry = %+v, %v", payments, err)
	}
	// A further replay on the settled order converges without duplicating.
	if err := completeCryptoPayment(db, cfg, orderID, quote.ID, "tx-resume-1", "r-sender", 1); err != nil {
		t.Fatalf("replay: %v", err)
	}
	payments, err = dbpkg.ListCryptoPaymentsByOrder(db, orderID)
	if err != nil || len(payments) != 1 {
		t.Fatalf("registry after replay = %+v, %v", payments, err)
	}
}

func TestPublicPaymentMethods(t *testing.T) {
	methods := func(cfg *config.Config) map[string]bool {
		t.Helper()
		recorder := httptest.NewRecorder()
		PublicPaymentMethodsHandler(cfg)(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/public/payment-methods", nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d", recorder.Code)
		}
		var got map[string]bool
		if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		return got
	}
	full := methods(cryptoTestConfig("https://www.okx.com"))
	if !full["crypto_btc"] || !full["crypto_xrp"] {
		t.Fatalf("methods = %v", full)
	}
	if full["bank_transfer"] {
		t.Fatalf("bank without configuration offered: %v", full)
	}
	off := methods(&config.Config{})
	if off["crypto_btc"] || off["crypto_xrp"] || off["bank_transfer"] {
		t.Fatalf("disabled methods offered: %v", off)
	}
	btcDown := cryptoTestConfig("https://www.okx.com")
	btcDown.OKXBTCDepositAddress = ""
	partial := methods(btcDown)
	if partial["crypto_btc"] || !partial["crypto_xrp"] {
		t.Fatalf("partial methods = %v", partial)
	}
}

func TestAdminListOrdersShowsCryptoQuote(t *testing.T) {
	okx := mockRateServer(t, "100000")
	defer okx.Close()

	db, err := dbpkg.InitDatabase(filepath.Join(t.TempDir(), "crypto-admin.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	cfg := cryptoTestConfig(okx.URL)
	cfg.InvoicesDir = t.TempDir()
	orderReq := httptest.NewRecorder()
	PublicOrderHandler(db, cfg, nil)(orderReq, httptest.NewRequest(http.MethodPost, "/api/v1/public/order", bytes.NewReader(cryptoOrderBody("crypto_btc"))))
	if orderReq.Code != http.StatusCreated {
		t.Fatalf("order status = %d", orderReq.Code)
	}
	var recap map[string]any
	if err := json.Unmarshal(orderReq.Body.Bytes(), &recap); err != nil {
		t.Fatal(err)
	}
	orderID, _ := recap["order_id"].(string)

	recorder := httptest.NewRecorder()
	AdminListOrdersHandler(db)(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/admin/orders", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	var listed struct {
		Orders []struct {
			OrderID string `json:"order_id"`
			Crypto  *struct {
				Asset        string `json:"asset"`
				AmountCrypto string `json:"amount_crypto"`
				PayAddress   string `json:"pay_address"`
				Status       string `json:"status"`
			} `json:"crypto"`
		} `json:"orders"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Orders) != 1 || listed.Orders[0].OrderID != orderID {
		t.Fatalf("orders = %+v", listed.Orders)
	}
	quote := listed.Orders[0].Crypto
	if quote == nil || quote.Asset != "BTC" || quote.Status != "pending" ||
		quote.AmountCrypto != "0.00024900" || quote.PayAddress != "bc1qoperator" {
		t.Fatalf("crypto = %+v", quote)
	}
}

func TestPaymentLabel(t *testing.T) {
	cases := []struct {
		method string
		want   string
	}{
		{"stripe", "Carte Bancaire (Stripe)"},
		{"bank_transfer", "Virement Bancaire"},
		{"crypto_btc", "Bitcoin (BTC)"},
		{"crypto_xrp", "XRP"},
		{"", "Virement Bancaire"},
	}
	for _, tc := range cases {
		if got := paymentLabel(&dbpkg.Order{PaymentMethod: tc.method}); got != tc.want {
			t.Fatalf("paymentLabel(%q) = %q, want %q", tc.method, got, tc.want)
		}
	}
	if got := paymentLabel(nil); got != "Virement Bancaire" {
		t.Fatalf("paymentLabel(nil) = %q", got)
	}
}
