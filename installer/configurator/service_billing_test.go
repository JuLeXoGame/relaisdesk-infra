package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestServiceBridgeAuthenticationAndSequence(t *testing.T) {
	oldURL, oldTransport := APIURL, http.DefaultTransport
	t.Cleanup(func() { APIURL = oldURL; http.DefaultTransport = oldTransport })
	APIURL = "https://service.invalid"
	calls := 0
	http.DefaultTransport = fleetAPITransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Host != "service.invalid" || r.Header.Get("Authorization") != "Bearer session-fixture" || r.URL.Path != "/api/v1/technician/service-billing/SVC-fixture/pulse" {
			t.Fatal("wrong API destination or credential")
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if len(body) != 4 || body["kind"] != nil || body["secret"] != nil {
			t.Fatalf("unexpected payload: %v", body)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"id":"SVC-fixture"}`))}, nil
	})
	b := &serviceBridge{secret: "local-fixture", token: "session-fixture", workID: "SVC-fixture"}
	call := func(secret, origin, body string) int {
		r := httptest.NewRequest("POST", "http://127.0.0.1:12345/meter", strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+secret)
		r.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		b.handle(w, r)
		return w.Code
	}
	if call("wrong", "", `{"kind":"ready"}`) != 403 || call("local-fixture", "https://evil.test", `{"kind":"ready"}`) != 403 {
		t.Fatal("unauthenticated/browser access")
	}
	if call("local-fixture", "", `{"kind":"ready"}`) != 200 || calls != 0 {
		t.Fatal("readiness billed")
	}
	if call("local-fixture", "", `{"kind":"ready"}{}`) != 400 {
		t.Fatal("trailing JSON accepted")
	}
	if call("local-fixture", "", `{"kind":"pulse","connection":"a","sequence":1}`) != 200 {
		t.Fatal("first pulse failed")
	}
	if call("local-fixture", "", `{"kind":"pulse","connection":"b","sequence":1}`) != 409 || calls != 1 {
		t.Fatal("simultaneous connection accepted")
	}
	if call("local-fixture", "", `{"kind":"pulse","connection":"a","sequence":2,"closed":true}`) != 200 {
		t.Fatal("close failed")
	}
	if call("local-fixture", "", `{"kind":"pulse","connection":"b","sequence":1}`) != 200 {
		t.Fatal("reconnect failed")
	}
	b.lastPulse = time.Now().Add(-11 * time.Second)
	if call("local-fixture", "", `{"kind":"pulse","connection":"c","sequence":1}`) != 200 {
		t.Fatal("stale connection prevented crash recovery")
	}
	b.stopped.Store(true)
	if call("local-fixture", "", `{"kind":"ready"}`) != 403 {
		t.Fatal("stopped bridge accepted")
	}
}

func TestServiceCheckoutURLIsolation(t *testing.T) {
	for _, url := range []string{"http://checkout.stripe.com/pay", "https://checkout.stripe.com.evil.test/pay", "https://checkout.stripe.com@evil.test/pay", "https://user@checkout.stripe.com/pay", "https://checkout.stripe.com:444/pay", "https://checkout.stripe.com/pay#anything"} {
		if validServiceCheckout(url) {
			t.Fatalf("unsafe link: %s", url)
		}
	}
	if !validServiceCheckout("https://checkout.stripe.com/c/pay/cs_fixture") {
		t.Fatal("Stripe URL refused")
	}
}
