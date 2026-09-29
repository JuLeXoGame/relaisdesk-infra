package cryptopay

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSignOKXRequestFormat(t *testing.T) {
	// The message layout timestamp+METHOD+path+body comes from the OKX v5
	// documentation; pin it so any drift breaks loudly.
	got := SignOKXRequest("secret", "2020-12-08T09:08:57.715Z", "get", "/api/v5/asset/deposit-history?ccy=BTC", "")
	mac := hmac.New(sha256.New, []byte("secret"))
	mac.Write([]byte("2020-12-08T09:08:57.715ZGET/api/v5/asset/deposit-history?ccy=BTC"))
	want := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	if got != want {
		t.Fatalf("signature = %q, want %q", got, want)
	}
	if decoded, err := base64.StdEncoding.DecodeString(got); err != nil || len(decoded) != 32 {
		t.Fatalf("signature is not 32 raw bytes: %v", err)
	}
	if ts := OKXTimestamp(time.Unix(1607416137, 715*1e6).UTC()); ts != "2020-12-08T08:28:57.715Z" {
		t.Fatalf("timestamp = %q", ts)
	}
	if (OKXCredentials{Key: "k", Secret: "s"}).Complete() {
		t.Fatal("incomplete credentials accepted")
	}
}

func TestDepositHistorySignsAndParses(t *testing.T) {
	var gotKey, gotPassphrase, gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("OK-ACCESS-KEY")
		gotPassphrase = r.Header.Get("OK-ACCESS-PASSPHRASE")
		gotQuery = r.URL.RawQuery
		ts := r.Header.Get("OK-ACCESS-TIMESTAMP")
		// The server side recomputes the signature over the full path WITH
		// query, exactly like OKX does.
		want := SignOKXRequest("test-secret", ts, r.Method, r.URL.RequestURI(), "")
		if r.Header.Get("OK-ACCESS-SIGN") != want {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"code":"50111","msg":"Invalid signature"}`))
			return
		}
		_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[
			{"ccy":"BTC","chain":"BTC-Bitcoin","amt":"0.00024900","from":"bc1qsender","to":"bc1qoperator","txId":"TXHASH","ts":"1758499200000","state":"2","depId":"999"},
			{"ccy":"BTC","amt":"1","from":"x","to":"y","txId":"PENDING","ts":"1758499200001","state":"0","depId":"1000"},
			{"ccy":"BTC","amt":"1","from":"x","to":"y","txId":"NOTS","ts":"abc","state":"2","depId":"1001"},
			{"ccy":"BTC","amt":"1","from":"x","to":"y","txId":"NODEP","ts":"1758499200002","state":"2","depId":""}
		]}`))
	}))
	defer server.Close()
	creds := OKXCredentials{Key: "test-key", Secret: "test-secret", Passphrase: "test-pass"}
	deposits, err := DepositHistory(context.Background(), server.Client(), server.URL, creds, "BTC", "1758499000000")
	if err != nil {
		t.Fatal(err)
	}
	if gotKey != "test-key" || gotPassphrase != "test-pass" {
		t.Fatalf("auth headers = %q %q", gotKey, gotPassphrase)
	}
	if !strings.Contains(gotQuery, "ccy=BTC") || !strings.Contains(gotQuery, "state=2") || !strings.Contains(gotQuery, "after=1758499000000") {
		t.Fatalf("query = %q", gotQuery)
	}
	if len(deposits) != 1 {
		t.Fatalf("deposits = %+v", deposits)
	}
	dep := deposits[0]
	if dep.CCY != "BTC" || dep.Amt != "0.00024900" || dep.From != "bc1qsender" || dep.To != "bc1qoperator" ||
		dep.TxID != "TXHASH" || dep.TsMillis != 1758499200000 || dep.State != "2" || dep.DepID != "999" {
		t.Fatalf("deposit = %+v", dep)
	}
}

func TestDepositHistoryRejectsBadResponses(t *testing.T) {
	creds := OKXCredentials{Key: "k", Secret: "s", Passphrase: "p"}
	for _, body := range []string{
		`{"code":"50111","msg":"Invalid signature","data":[]}`,
		`{"code":"0","msg":"","data":[]}`, // empty is fine actually: tested separately
		`not json`,
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(body))
		}))
		_, err := DepositHistory(context.Background(), server.Client(), server.URL, creds, "BTC", "")
		server.Close()
		if body == `{"code":"0","msg":"","data":[]}` {
			if err != nil {
				t.Fatalf("empty history rejected: %v", err)
			}
			continue
		}
		if err == nil {
			t.Fatalf("bad payload accepted: %s", body)
		}
	}
	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer failing.Close()
	if _, err := DepositHistory(context.Background(), failing.Client(), failing.URL, creds, "BTC", ""); err == nil {
		t.Fatal("server error accepted")
	}
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":"0","data":[]}`))
	}))
	defer ok.Close()
	if _, err := DepositHistory(context.Background(), ok.Client(), ok.URL, OKXCredentials{}, "BTC", ""); err == nil {
		t.Fatal("incomplete credentials accepted")
	}
	if _, err := DepositHistory(context.Background(), ok.Client(), ok.URL, creds, "ETH", ""); err == nil {
		t.Fatal("unknown asset accepted")
	}
}

func TestVerifyDepositAddress(t *testing.T) {
	creds := OKXCredentials{Key: "k", Secret: "s", Passphrase: "p"}
	serve := func(body string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(body))
		}))
	}
	btc := serve(`{"code":"0","msg":"","data":[{"addr":"bc1qoperator","tag":"","memo":"","ccy":"BTC","chain":"BTC-Bitcoin","selected":true}]}`)
	defer btc.Close()
	if err := VerifyDepositAddress(context.Background(), btc.Client(), btc.URL, creds, "BTC", "bc1qoperator", ""); err != nil {
		t.Fatalf("valid address rejected: %v", err)
	}
	if err := VerifyDepositAddress(context.Background(), btc.Client(), btc.URL, creds, "BTC", "bc1qother", ""); err == nil {
		t.Fatal("mismatched address accepted")
	}
	xrp := serve(`{"code":"0","msg":"","data":[{"addr":"rOperator","tag":"123456","memo":"","ccy":"XRP","chain":"XRP-Ripple","selected":true}]}`)
	defer xrp.Close()
	if err := VerifyDepositAddress(context.Background(), xrp.Client(), xrp.URL, creds, "XRP", "rOperator", "123456"); err != nil {
		t.Fatalf("valid xrp deposit rejected: %v", err)
	}
	if err := VerifyDepositAddress(context.Background(), xrp.Client(), xrp.URL, creds, "XRP", "rOperator", "999"); err == nil {
		t.Fatal("mismatched xrp tag accepted")
	}
	empty := serve(`{"code":"0","msg":"","data":[]}`)
	defer empty.Close()
	if err := VerifyDepositAddress(context.Background(), empty.Client(), empty.URL, creds, "BTC", "bc1qoperator", ""); err == nil {
		t.Fatal("empty address list accepted")
	}
}

func TestMatchOKXDeposit(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	quote := func(id int64, order, amount string, quotedAgo, expiresIn time.Duration) MatchQuote {
		return MatchQuote{
			ID: id, OrderID: order, AmountCrypto: amount, PayAddress: "bc1qoperator",
			QuotedAt: now.Add(-quotedAgo), ExpiresAt: now.Add(expiresIn),
		}
	}
	deposit := func(amt, to string, ts time.Time) OKXDeposit {
		return OKXDeposit{CCY: "BTC", Amt: amt, From: "bc1qsender", To: to, TxID: "H", TsMillis: ts.UnixMilli(), State: "2", DepID: "1"}
	}
	open := quote(1, "RD-1", "0.00024900", 10*time.Minute, 20*time.Minute)
	cases := []struct {
		name    string
		quotes  []MatchQuote
		deposit OKXDeposit
		wantID  int64 // 0 = no match
	}{
		{"exact", []MatchQuote{open}, deposit("0.00024900", "bc1qoperator", now), 1},
		{"overpay", []MatchQuote{open}, deposit("0.00025000", "bc1qoperator", now), 1},
		{"underpay", []MatchQuote{open}, deposit("0.00024899", "bc1qoperator", now), 0},
		{"wrong address", []MatchQuote{open}, deposit("0.00024900", "bc1qother", now), 0},
		{"empty to", []MatchQuote{open}, deposit("0.00024900", "", now), 0},
		{"before quote", []MatchQuote{open}, deposit("0.00024900", "bc1qoperator", now.Add(-time.Hour)), 0},
		{"within grace after expiry", []MatchQuote{quote(2, "RD-2", "0.001", 2*time.Hour, -time.Hour)}, deposit("0.001", "bc1qoperator", now.Add(-30*time.Minute)), 2},
		{"past grace", []MatchQuote{quote(3, "RD-3", "0.001", 48*time.Hour, -25*time.Hour)}, deposit("0.001", "bc1qoperator", now), 0},
		{"garbage amount", []MatchQuote{open}, deposit("abc", "bc1qoperator", now), 0},
		{"first wins", []MatchQuote{open, quote(4, "RD-4", "0.00010000", 5*time.Minute, 25*time.Minute)}, deposit("0.00024900", "bc1qoperator", now), 1},
	}
	for _, tc := range cases {
		got := MatchOKXDeposit(tc.quotes, tc.deposit, "bc1qoperator")
		var gotID int64
		if got != nil {
			gotID = got.ID
		}
		if gotID != tc.wantID {
			t.Fatalf("%s: match = %d, want %d", tc.name, gotID, tc.wantID)
		}
	}
}
