package handlers

import (
	"bytes"
	dbpkg "database"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"api/config"
	"api/mailer"
)

func TestNewOrderRejectsPreviousContractVersion(t *testing.T) {
	if dbpkg.TrialTermsVersion != mailer.CurrentTrialTermsVersion {
		t.Fatal("trial and mailer versions differ")
	}
	if publicTermsVersion != mailer.CurrentTermsVersion {
		t.Fatal("checkout and mailer versions differ")
	}
	db, err := dbpkg.InitDatabase(filepath.Join(t.TempDir(), "orders.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	handler := PublicOrderHandler(db, &config.Config{DevHTTP: true, StripeMock: true}, nil)
	for _, version := range []string{"2026-08-25", "2026-09-09", "2026-09-10", "2026-09-11", "2026-09-21", "2026-09-24"} {
		body := []byte(fmt.Sprintf(`{"email":"client@example.com","plan":"starter","technicians":1,"payment_method":"stripe","name":"Client Test","address":"1 rue du Test","postal_code":"75001","city":"Paris","customer_type":"business","terms_version":%q,"terms_accepted":true}`, version))
		recorder := httptest.NewRecorder()
		handler(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/public/order", bytes.NewReader(body)))
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("stale contract %s accepted: %d", version, recorder.Code)
		}
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM orders").Scan(&count); err != nil || count != 0 {
		t.Fatalf("order created with outdated acceptance: %d, %v", count, err)
	}
}
