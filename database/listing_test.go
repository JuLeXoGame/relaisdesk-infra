package database

import (
	"fmt"
	"testing"
	"time"
)

func TestListOrdersPaginationAndCount(t *testing.T) {
	db, err := InitDatabase(t.TempDir() + "/listing-orders.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var ids []string
	for i := 0; i < 5; i++ {
		order, err := CreateOrder(db, "page@example.com", "starter", 1, "stripe", "", "")
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, order.OrderID)
	}
	// ids[4] is the newest (id DESC first).

	total, err := CountOrders(db, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if total != 5 {
		t.Fatalf("CountOrders = %d, want 5", total)
	}

	p1, err := ListOrders(db, "", "", 2, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(p1) != 2 || p1[0].OrderID != ids[4] || p1[1].OrderID != ids[3] {
		t.Fatalf("page 1 = %v, want [%s %s]", orderIDs(p1), ids[4], ids[3])
	}

	p2, err := ListOrders(db, "", "", 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(p2) != 2 || p2[0].OrderID != ids[2] || p2[1].OrderID != ids[1] {
		t.Fatalf("page 2 = %v, want [%s %s]", orderIDs(p2), ids[2], ids[1])
	}

	p3, err := ListOrders(db, "", "", 2, 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(p3) != 1 || p3[0].OrderID != ids[0] {
		t.Fatalf("page 3 = %v, want [%s]", orderIDs(p3), ids[0])
	}

	all, err := ListOrders(db, "", "", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 5 {
		t.Fatalf("limit 0 = %d rows, want 5 (no pagination)", len(all))
	}

	pending, err := CountOrders(db, "pending", "")
	if err != nil {
		t.Fatal(err)
	}
	if pending != 5 {
		t.Fatalf("CountOrders(pending) = %d, want 5", pending)
	}
	if _, err := db.Exec(`UPDATE orders SET status = 'paid' WHERE order_id = ?`, ids[0]); err != nil {
		t.Fatal(err)
	}
	paid, err := CountOrders(db, "paid", "")
	if err != nil {
		t.Fatal(err)
	}
	if paid != 1 {
		t.Fatalf("CountOrders(paid) = %d, want 1", paid)
	}
	paidRows, err := ListOrders(db, "paid", "", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(paidRows) != 1 || paidRows[0].OrderID != ids[0] {
		t.Fatalf("ListOrders(paid) = %v, want [%s]", orderIDs(paidRows), ids[0])
	}
}

func orderIDs(orders []Order) []string {
	out := make([]string, 0, len(orders))
	for _, o := range orders {
		out = append(out, o.OrderID)
	}
	return out
}

func TestOrderEmailStatusBatch(t *testing.T) {
	db, err := InitDatabase(t.TempDir() + "/listing-email.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	o1, err := CreateOrder(db, "a@example.com", "starter", 1, "stripe", "", "")
	if err != nil {
		t.Fatal(err)
	}
	o2, err := CreateOrder(db, "b@example.com", "starter", 1, "stripe", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := MarkOrderEmailSent(db, o1.OrderID, "license"); err != nil {
		t.Fatal(err)
	}
	if err := MarkOrderEmailSent(db, o1.OrderID, "invoice"); err != nil {
		t.Fatal(err)
	}
	if err := MarkOrderEmailSent(db, o2.OrderID, "license"); err != nil {
		t.Fatal(err)
	}

	got, err := GetOrderEmailStatusBatch(db, []string{o1.OrderID, o2.OrderID, "CMD-DOES-NOT-EXIST"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("batch size = %d, want 2 (unknown ID skipped)", len(got))
	}
	if !got[o1.OrderID].LicenseSent || !got[o1.OrderID].InvoiceSent {
		t.Errorf("o1 flags = %+v, want both true", got[o1.OrderID])
	}
	if !got[o2.OrderID].LicenseSent || got[o2.OrderID].InvoiceSent {
		t.Errorf("o2 flags = %+v, want license only", got[o2.OrderID])
	}
	// Parity with the single lookup.
	single, err := GetOrderEmailStatus(db, o1.OrderID)
	if err != nil {
		t.Fatal(err)
	}
	if single != got[o1.OrderID] {
		t.Errorf("batch %+v != single %+v", got[o1.OrderID], single)
	}

	empty, err := GetOrderEmailStatusBatch(db, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(empty) != 0 {
		t.Errorf("empty input = %d rows, want 0", len(empty))
	}
}

func TestCryptoQuotesBatchLatest(t *testing.T) {
	db, err := InitDatabase(t.TempDir() + "/listing-crypto.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	order, err := CreateOrder(db, "c@example.com", "starter", 1, "crypto_btc", "", "")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	mk := func(amount string) *CryptoQuote {
		return &CryptoQuote{
			OrderID: order.OrderID, Asset: "BTC", AmountCrypto: amount,
			RateEUR: "50000", QuotedAt: now, ExpiresAt: now.Add(30 * time.Minute),
			PayAddress: "bc1qtest",
		}
	}
	first := mk("0.001")
	if err := CreateCryptoQuote(db, first); err != nil {
		t.Fatal(err)
	}
	second := mk("0.002")
	if err := CreateCryptoQuote(db, second); err != nil {
		t.Fatal(err)
	}

	got, err := GetCryptoQuotesByOrderIDs(db, []string{order.OrderID, "CMD-DOES-NOT-EXIST"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("batch size = %d, want 1", len(got))
	}
	if got[order.OrderID].ID != second.ID || got[order.OrderID].AmountCrypto != "0.002" {
		t.Errorf("latest quote = %+v, want ID %d", got[order.OrderID], second.ID)
	}
	// Parity with the single lookup.
	single, err := GetCryptoQuoteByOrderID(db, order.OrderID)
	if err != nil {
		t.Fatal(err)
	}
	if single.ID != second.ID {
		t.Errorf("single lookup ID = %d, want %d", single.ID, second.ID)
	}

	empty, err := GetCryptoQuotesByOrderIDs(db, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(empty) != 0 {
		t.Errorf("empty input = %d rows, want 0", len(empty))
	}
}

func TestListAbandonedCarts(t *testing.T) {
	db, err := InitDatabase(t.TempDir() + "/listing-carts.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	now := time.Now().UTC().Truncate(time.Second)
	mk := func(email, method, status string, created, expires time.Time) string {
		t.Helper()
		o, err := CreateOrder(db, email, "starter", 1, method, "", "")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`UPDATE orders SET status = ?, created_at = ?, expires_at = ? WHERE order_id = ?`,
			status, created.Format(time.RFC3339), expires.Format(time.RFC3339), o.OrderID); err != nil {
			t.Fatal(err)
		}
		return o.OrderID
	}
	old := mk("old@example.com", "stripe", "pending", now.Add(-30*time.Hour), now.Add(18*time.Hour))
	mk("recent@example.com", "stripe", "pending", now.Add(-2*time.Hour), now.Add(46*time.Hour))
	mk("paid@example.com", "stripe", "paid", now.Add(-30*time.Hour), now.Add(18*time.Hour))
	mk("wire@example.com", "bank_transfer", "pending", now.Add(-30*time.Hour), now.Add(18*time.Hour))
	mk("expired@example.com", "stripe", "pending", now.Add(-50*time.Hour), now.Add(-2*time.Hour))

	got, err := ListAbandonedCarts(db, now.Add(-40*time.Hour), now.Add(-24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].OrderID != old {
		t.Fatalf("R1 = %v, want [%s]", orderIDs(got), old)
	}
	got, err = ListAbandonedCarts(db, now.Add(-48*time.Hour), now.Add(-40*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("R2 = %v, want [] (expired excluded)", orderIDs(got))
	}
}

func TestChunkStrings(t *testing.T) {
	if got := chunkStrings(nil, 500); len(got) != 0 {
		t.Errorf("nil input = %d chunks, want 0", len(got))
	}
	in := make([]string, 1200)
	for i := range in {
		in[i] = "x"
	}
	chunks := chunkStrings(in, 500)
	if len(chunks) != 3 || len(chunks[0]) != 500 || len(chunks[1]) != 500 || len(chunks[2]) != 200 {
		t.Fatalf("chunk sizes = %v, want [500 500 200]", []int{len(chunks[0]), len(chunks[1]), len(chunks[2])})
	}
}

func TestListInvoicesPaginationAndCount(t *testing.T) {
	db, err := InitDatabase(t.TempDir() + "/listing-invoices.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var orderIDs []string
	for i := 0; i < 3; i++ {
		o, err := CreateOrder(db, fmt.Sprintf("inv%d@example.com", i), "starter", 1, "stripe", "", "")
		if err != nil {
			t.Fatal(err)
		}
		orderIDs = append(orderIDs, o.OrderID)
	}
	mk := func(orderID, email string) {
		t.Helper()
		_, err := CreateInvoice(db, &Invoice{
			OrderID: orderID, CustomerEmail: email, CustomerName: "Test Client",
			Plan: "Starter", Technicians: 1,
			AmountHT: 10, AmountTVA: 0, AmountTTC: 10,
			Status: "paid", PDFPath: "/tmp/test.html",
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	mk(orderIDs[0], "one@example.com")
	mk(orderIDs[1], "two@example.com")
	mk(orderIDs[2], "three@example.com")

	total, err := CountInvoices(db, "")
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 {
		t.Fatalf("CountInvoices = %d, want 3", total)
	}

	p1, err := ListInvoices(db, "", 2, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(p1) != 2 {
		t.Fatalf("page 1 = %d rows, want 2", len(p1))
	}
	p2, err := ListInvoices(db, "", 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(p2) != 1 {
		t.Fatalf("page 2 = %d rows, want 1", len(p2))
	}

	filtered, err := ListInvoices(db, orderIDs[1], 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 1 || filtered[0].OrderID != orderIDs[1] {
		t.Fatalf("search order = %+v, want 1 row", filtered)
	}
	filteredTotal, err := CountInvoices(db, "two@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if filteredTotal != 1 {
		t.Fatalf("CountInvoices search = %d, want 1", filteredTotal)
	}
}

func TestListLicensesPaginationAndCount(t *testing.T) {
	db, err := InitDatabase(t.TempDir() + "/listing-licenses.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, err := CreateLicense(db, "a@example.com", 30, 1, "notes alpha"); err != nil {
		t.Fatal(err)
	}
	if _, err := CreateLicense(db, "b@example.com", 30, 1, "notes beta"); err != nil {
		t.Fatal(err)
	}
	if _, err := CreateLicense(db, "c@example.com", 30, 1, "notes gamma"); err != nil {
		t.Fatal(err)
	}

	total, err := CountLicenses(db, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 {
		t.Fatalf("CountLicenses = %d, want 3", total)
	}

	// created_at has second precision: assert sets, not order.
	p1, err := ListLicenses(db, "", "", "", 2, 0)
	if err != nil {
		t.Fatal(err)
	}
	p2, err := ListLicenses(db, "", "", "", 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(p1) != 2 || len(p2) != 1 {
		t.Fatalf("pages = %d/%d rows, want 2/1", len(p1), len(p2))
	}
	seen := map[string]bool{}
	for _, lic := range append(p1, p2...) {
		if seen[lic.LicenseID] {
			t.Fatalf("duplicate %s across pages", lic.LicenseID)
		}
		seen[lic.LicenseID] = true
	}

	bySearch, err := ListLicenses(db, "", "", "beta", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(bySearch) != 1 || bySearch[0].Email != "b@example.com" {
		t.Fatalf("search beta = %+v, want b@example.com", bySearch)
	}
	byStatus, err := CountLicenses(db, "active", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if byStatus != 3 {
		t.Fatalf("CountLicenses(active) = %d, want 3", byStatus)
	}
	if _, err := db.Exec(`UPDATE licences SET status = 'revoked' WHERE email = 'a@example.com'`); err != nil {
		t.Fatal(err)
	}
	byStatus, err = CountLicenses(db, "revoked", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if byStatus != 1 {
		t.Fatalf("CountLicenses(revoked) = %d, want 1", byStatus)
	}
}
