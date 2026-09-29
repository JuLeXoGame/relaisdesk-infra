package cryptopay

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCryptoAmountRoundsUp(t *testing.T) {
	// 24.90 EUR at 100000 EUR/BTC = 0.000249 exactly -> stays exact.
	got, err := CryptoAmount(24.90, "100000", 8)
	if err != nil || got != "0.00024900" {
		t.Fatalf("exact amount = %q, %v", got, err)
	}
	// Repeating decimal must round UP to the last unit, never down.
	got, err = CryptoAmount(10, "3", 8)
	if err != nil || got != "3.33333334" {
		t.Fatalf("rounded amount = %q, %v", got, err)
	}
	got, err = CryptoAmount(24.90, "2.50", 6)
	if err != nil || got != "9.960000" {
		t.Fatalf("xrp amount = %q, %v", got, err)
	}
	for _, bad := range []struct {
		price float64
		rate  string
		dec   int
	}{
		{0, "100", 8},
		{-5, "100", 8},
		{10, "0", 8},
		{10, "-3", 8},
		{10, "abc", 8},
		{10, "100", -1},
		{10, "100", 19},
	} {
		if _, err := CryptoAmount(bad.price, bad.rate, bad.dec); err == nil {
			t.Fatalf("invalid input accepted: %+v", bad)
		}
	}
}

func TestAssetMappings(t *testing.T) {
	if method, _ := PaymentMethodForAsset("btc"); method != "crypto_btc" {
		t.Fatalf("btc method = %q", method)
	}
	if asset, _ := AssetForPaymentMethod("crypto_xrp"); asset != "XRP" {
		t.Fatalf("xrp asset = %q", asset)
	}
	if _, err := PaymentMethodForAsset("ETH"); err == nil {
		t.Fatal("ETH accepted")
	}
	if _, err := AssetForPaymentMethod("stripe"); err == nil {
		t.Fatal("stripe accepted as crypto")
	}
	if AssetLabel("BTC") != "Bitcoin (BTC)" || AssetLabel("XRP") != "XRP" {
		t.Fatal("labels broken")
	}
}

func TestSpotRateEUR(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.RawQuery, "instId=BTC-EUR") {
			t.Errorf("unexpected instrument: %s", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{"last":"97312.4","ts":"1758499200000"}]}`))
	}))
	defer server.Close()
	rate, at, err := SpotRateEUR(context.Background(), server.Client(), server.URL, "BTC")
	if err != nil {
		t.Fatal(err)
	}
	if rate != "97312.4" {
		t.Fatalf("rate = %q", rate)
	}
	if at.UnixMilli() != 1758499200000 {
		t.Fatalf("quoted at = %v", at)
	}
}

func TestSpotRateEURRejectsBadPayloads(t *testing.T) {
	for _, body := range []string{
		`{"code":"1","msg":"boom","data":[]}`,
		`{"code":"0","msg":"","data":[]}`,
		`{"code":"0","msg":"","data":[{"last":"0","ts":"1"}]}`,
		`{"code":"0","msg":"","data":[{"last":"abc","ts":"1"}]}`,
		`not json`,
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(body))
		}))
		if _, _, err := SpotRateEUR(context.Background(), server.Client(), server.URL, "XRP"); err == nil {
			server.Close()
			t.Fatalf("bad payload accepted: %s", body)
		}
		server.Close()
	}
	if _, _, err := SpotRateEUR(context.Background(), http.DefaultClient, "http://127.0.0.1:1", "ETH"); err == nil {
		t.Fatal("unknown asset accepted")
	}
}


