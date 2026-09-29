package database

import (
	"path/filepath"
	"testing"
	"time"
)

func TestCryptoQuoteLifecycle(t *testing.T) {
	db, err := InitDatabase(filepath.Join(t.TempDir(), "crypto-quote-test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	order, err := CreateOrder(db, "crypto@example.com", "starter", 1, "stripe", "", "")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	quote := &CryptoQuote{
		OrderID:      order.OrderID,
		Asset:        "BTC",
		AmountCrypto: "0.00024900",
		RateEUR:      "100000",
		QuotedAt:     now,
		ExpiresAt:    now.Add(30 * time.Minute),
		PayAddress:   "bc1qtest",
	}
	if err := CreateCryptoQuote(db, quote); err != nil {
		t.Fatal(err)
	}
	if quote.Status != "pending" || quote.RateSource != "OKX" {
		t.Fatalf("unexpected stored quote: %+v", quote)
	}
	got, err := GetCryptoQuoteByOrderID(db, order.OrderID)
	if err != nil || got.AmountCrypto != "0.00024900" || got.DestTag != nil {
		t.Fatalf("quote mismatch: %+v %v", got, err)
	}
	gotByID, err := GetCryptoQuoteByID(db, quote.ID)
	if err != nil || gotByID.OrderID != order.OrderID {
		t.Fatalf("quote by id: %+v %v", gotByID, err)
	}
	updated, err := MarkCryptoQuotePaidByID(db, quote.ID, "txid-1", "sender-1", 2)
	if err != nil || !updated {
		t.Fatalf("mark paid = %v, %v", updated, err)
	}
	// Second call is a no-op: safe to process a deposit twice.
	updated, err = MarkCryptoQuotePaidByID(db, quote.ID, "txid-1", "sender-1", 2)
	if err != nil || updated {
		t.Fatalf("re-mark paid = %v, %v", updated, err)
	}
	if _, err := MarkCryptoQuotePaidByID(db, 0, "tx", "s", 1); err == nil {
		t.Fatal("zero quote id accepted")
	}
	if err := RecordCryptoPayment(db, &CryptoPayment{
		OrderID: order.OrderID, InvoiceNumber: "FAC-1", Asset: "BTC",
		AmountCrypto: "0.00024900", RateEUR: "100000", RateAt: now,
		TxID: "txid-1", Sender: "sender-1",
	}); err != nil {
		t.Fatalf("record payment: %v", err)
	}
	payments, err := ListCryptoPaymentsByOrder(db, order.OrderID)
	if err != nil || len(payments) != 1 || payments[0].TxID != "txid-1" {
		t.Fatalf("registry mismatch: %+v %v", payments, err)
	}
}

func TestCryptoQuoteValidation(t *testing.T) {
	db, err := InitDatabase(filepath.Join(t.TempDir(), "crypto-quote-invalid-test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := CreateCryptoQuote(db, nil); err == nil {
		t.Fatal("nil quote accepted")
	}
	base := CryptoQuote{
		OrderID: "RD-TEST", Asset: "ETH", AmountCrypto: "1",
		RateEUR: "1", QuotedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour),
		PayAddress: "0x0",
	}
	if err := CreateCryptoQuote(db, &base); err == nil {
		t.Fatal("ETH quote accepted")
	}
	base.Asset = "XRP"
	base.PayAddress = "rDest"
	if err := CreateCryptoQuote(db, &base); err == nil {
		t.Fatal("XRP quote without tag accepted")
	}
	tag := uint32(42)
	base.DestTag = &tag
	// No matching order row: FK or insert must fail loudly, never silently.
	if err := CreateCryptoQuote(db, &base); err == nil {
		t.Fatal("quote without order accepted")
	}
}

func TestExpireCryptoQuotes(t *testing.T) {
	db, err := InitDatabase(filepath.Join(t.TempDir(), "crypto-quote-expire-test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	old, err := CreateOrder(db, "old@example.com", "starter", 1, "stripe", "", "")
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := CreateOrder(db, "fresh@example.com", "starter", 1, "stripe", "", "")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	for _, tc := range []struct {
		order   string
		expires time.Time
	}{
		{old.OrderID, now.Add(-time.Minute)},
		{fresh.OrderID, now.Add(time.Hour)},
	} {
		if err := CreateCryptoQuote(db, &CryptoQuote{
			OrderID: tc.order, Asset: "BTC", AmountCrypto: "0.001",
			RateEUR: "50000", QuotedAt: now.Add(-time.Hour), ExpiresAt: tc.expires,
			PayAddress: "bc1qtest",
		}); err != nil {
			t.Fatal(err)
		}
	}
	count, err := ExpireCryptoQuotes(db, now)
	if err != nil || count != 1 {
		t.Fatalf("expired = %d, %v", count, err)
	}
	got, err := GetCryptoQuoteByOrderID(db, old.OrderID)
	if err != nil || got.Status != "expired" {
		t.Fatalf("old quote = %+v %v", got, err)
	}
	got, err = GetCryptoQuoteByOrderID(db, fresh.OrderID)
	if err != nil || got.Status != "pending" {
		t.Fatalf("fresh quote = %+v %v", got, err)
	}
}

func TestListOpenCryptoQuotes(t *testing.T) {
	db, err := InitDatabase(filepath.Join(t.TempDir(), "crypto-open-test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	now := time.Now().UTC()
	mkOrder := func(email string) string {
		order, err := CreateOrder(db, email, "starter", 1, "stripe", "", "")
		if err != nil {
			t.Fatal(err)
		}
		return order.OrderID
	}
	pendingID := mkOrder("pending@example.com")
	recentID := mkOrder("recent@example.com")
	oldID := mkOrder("old@example.com")
	paidID := mkOrder("paid@example.com")
	add := func(orderID string, quotedAt, expiresAt time.Time) int64 {
		quote := &CryptoQuote{
			OrderID: orderID, Asset: "BTC", AmountCrypto: "0.001",
			RateEUR: "50000", QuotedAt: quotedAt, ExpiresAt: expiresAt,
			PayAddress: "bc1qtest",
		}
		if err := CreateCryptoQuote(db, quote); err != nil {
			t.Fatal(err)
		}
		return quote.ID
	}
	add(pendingID, now.Add(-5*time.Minute), now.Add(25*time.Minute))
	add(recentID, now.Add(-2*time.Hour), now.Add(-time.Hour))
	add(oldID, now.Add(-48*time.Hour), now.Add(-47*time.Hour))
	paidQuote := add(paidID, now.Add(-5*time.Minute), now.Add(25*time.Minute))
	if _, err := db.Exec(`UPDATE crypto_quotes SET status = 'expired' WHERE order_id IN (?, ?)`, recentID, oldID); err != nil {
		t.Fatal(err)
	}
	if _, err := MarkCryptoQuotePaidByID(db, paidQuote, "tx", "s", 1); err != nil {
		t.Fatal(err)
	}
	open, err := ListOpenCryptoQuotes(db, "BTC", now.Add(-24*time.Hour), 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(open) != 2 || open[0].OrderID != pendingID || open[1].OrderID != recentID {
		t.Fatalf("open quotes = %+v", open)
	}
}
