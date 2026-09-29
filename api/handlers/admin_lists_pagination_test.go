package handlers

import (
	dbpkg "database"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAdminListOrdersPaginationAndBatch(t *testing.T) {
	db, err := dbpkg.InitDatabase(t.TempDir() + "/admin-list-orders.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var ids []string
	for i := 0; i < 3; i++ {
		o, err := dbpkg.CreateOrder(db, fmt.Sprintf("page%d@example.com", i), "starter", 1, "stripe", "", "")
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, o.OrderID)
	}
	// Newest order first (id DESC): mark its license email as sent.
	if err := dbpkg.MarkOrderEmailSent(db, ids[2], "license"); err != nil {
		t.Fatal(err)
	}
	// One crypto order with a quote exercises the batch crypto lookup.
	cryptoOrder, err := dbpkg.CreateOrder(db, "crypto@example.com", "starter", 1, "crypto_btc", "", "")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := dbpkg.CreateCryptoQuote(db, &dbpkg.CryptoQuote{
		OrderID: cryptoOrder.OrderID, Asset: "BTC", AmountCrypto: "0.001",
		RateEUR: "50000", QuotedAt: now, ExpiresAt: now.Add(30 * time.Minute),
		PayAddress: "bc1qtest",
	}); err != nil {
		t.Fatal(err)
	}

	get := func(target string) (int, map[string]any) {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		rec := httptest.NewRecorder()
		AdminListOrdersHandler(db)(rec, req)
		var payload map[string]any
		if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
			t.Fatalf("decode %s: %v", target, err)
		}
		return rec.Code, payload
	}

	code, p1 := get("/api/v1/admin/orders?limit=2&page=1")
	if code != http.StatusOK {
		t.Fatalf("page 1 status = %d", code)
	}
	if p1["total"] != float64(4) {
		t.Errorf("total = %v, want 4", p1["total"])
	}
	items1, _ := p1["orders"].([]any)
	if len(items1) != 2 {
		t.Fatalf("page 1 items = %d, want 2", len(items1))
	}
	first := items1[0].(map[string]any)
	if first["order_id"] != cryptoOrder.OrderID {
		t.Errorf("first item = %v, want newest %s", first["order_id"], cryptoOrder.OrderID)
	}
	crypto, ok := first["crypto"].(map[string]any)
	if !ok || crypto["asset"] != "BTC" {
		t.Errorf("crypto view = %v, want BTC quote", first["crypto"])
	}
	second := items1[1].(map[string]any)
	if second["order_id"] != ids[2] || second["license_email_sent"] != true || second["invoice_email_sent"] != false {
		t.Errorf("email flags = %v, want license=true invoice=false", second)
	}
	if _, present := second["crypto"]; present {
		t.Errorf("stripe order should have no crypto view, got %v", second["crypto"])
	}

	code, p2 := get("/api/v1/admin/orders?limit=2&page=2")
	if code != http.StatusOK {
		t.Fatalf("page 2 status = %d", code)
	}
	if p2["total"] != float64(4) || p2["page"] != float64(2) {
		t.Errorf("page 2 meta = total %v page %v, want 4/2", p2["total"], p2["page"])
	}
	if items2, _ := p2["orders"].([]any); len(items2) != 2 {
		t.Fatalf("page 2 items = %d, want 2", len(items2))
	}

	// No params: full list (backward compatible for ledger/unpaid/export).
	code, all := get("/api/v1/admin/orders")
	if code != http.StatusOK {
		t.Fatalf("full list status = %d", code)
	}
	if all["total"] != float64(4) {
		t.Errorf("full list total = %v, want 4", all["total"])
	}
	if items, _ := all["orders"].([]any); len(items) != 4 {
		t.Fatalf("full list items = %d, want 4", len(items))
	}
	if all["limit"] != float64(0) {
		t.Errorf("full list limit = %v, want 0", all["limit"])
	}
}

func TestAdminListInvoicesPagination(t *testing.T) {
	db, err := dbpkg.InitDatabase(t.TempDir() + "/admin-list-invoices.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var orderIDs []string
	for i := 0; i < 2; i++ {
		o, err := dbpkg.CreateOrder(db, fmt.Sprintf("hinv%d@example.com", i), "starter", 1, "stripe", "", "")
		if err != nil {
			t.Fatal(err)
		}
		orderIDs = append(orderIDs, o.OrderID)
	}
	mk := func(orderID, email string) {
		t.Helper()
		_, err := dbpkg.CreateInvoice(db, &dbpkg.Invoice{
			OrderID: orderID, CustomerEmail: email, CustomerName: "Test Client",
			Plan: "Starter", Technicians: 1,
			AmountHT: 10, AmountTVA: 0, AmountTTC: 10,
			Status: "paid", PDFPath: "/tmp/test.html",
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	mk(orderIDs[0], "a@example.com")
	mk(orderIDs[1], "b@example.com")

	get := func(target string) (int, map[string]any) {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		rec := httptest.NewRecorder()
		AdminListInvoicesHandler(db)(rec, req)
		var payload map[string]any
		if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
			t.Fatalf("decode %s: %v", target, err)
		}
		return rec.Code, payload
	}

	code, p := get("/api/v1/admin/invoices?limit=1&page=2")
	if code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	if p["total"] != float64(2) {
		t.Errorf("total = %v, want 2", p["total"])
	}
	items, _ := p["invoices"].([]any)
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}

	code, q := get("/api/v1/admin/invoices?q=" + orderIDs[1])
	if code != http.StatusOK {
		t.Fatalf("search status = %d", code)
	}
	if q["total"] != float64(1) {
		t.Errorf("search total = %v, want 1", q["total"])
	}
}

func TestAdminListLicensesSQLPagination(t *testing.T) {
	db, err := dbpkg.InitDatabase(t.TempDir() + "/admin-list-licenses.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, err := dbpkg.CreateLicense(db, "a@example.com", 30, 1, "notes alpha"); err != nil {
		t.Fatal(err)
	}
	if _, err := dbpkg.CreateLicense(db, "b@example.com", 30, 1, "notes beta"); err != nil {
		t.Fatal(err)
	}
	if _, err := dbpkg.CreateLicense(db, "c@example.com", 30, 1, "notes gamma"); err != nil {
		t.Fatal(err)
	}

	get := func(target string) (int, map[string]any) {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		rec := httptest.NewRecorder()
		AdminListLicensesHandler(db)(rec, req)
		var payload map[string]any
		if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
			t.Fatalf("decode %s: %v", target, err)
		}
		return rec.Code, payload
	}

	code, p := get("/api/v1/admin/licences?limit=2&page=1")
	if code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	if p["total"] != float64(3) {
		t.Errorf("total = %v, want 3", p["total"])
	}
	items, _ := p["licences"].([]any)
	if len(items) != 2 {
		t.Fatalf("items = %d, want 2", len(items))
	}

	code, q := get("/api/v1/admin/licences?q=beta")
	if code != http.StatusOK {
		t.Fatalf("search status = %d", code)
	}
	if q["total"] != float64(1) {
		t.Errorf("search total = %v, want 1", q["total"])
	}
}
