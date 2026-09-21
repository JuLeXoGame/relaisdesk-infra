package handlers

import (
	"bytes"
	dbpkg "database"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestPublicWithdrawalRecordsTimestampedNotice(t *testing.T) {
	db, err := dbpkg.InitDatabase(filepath.Join(t.TempDir(), "withdrawals.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	order, err := dbpkg.CreateOrderWithBilling(db, "client@example.com", "starter", 1, "stripe", "", "", &dbpkg.BillingDetails{
		Name: "Client Test", CustomerType: "consumer", TermsVersion: publicTermsVersion,
		TermsAccepted: true, ImmediatePerformanceRequested: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	body, _ := json.Marshal(map[string]string{
		"order_id": order.OrderID,
		"email":    "client@example.com",
		"name":     "Client Test",
	})
	recorder := httptest.NewRecorder()
	PublicWithdrawalHandler(db, nil).ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/public/withdrawal", bytes.NewReader(body)))

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %q", recorder.Code, http.StatusCreated, recorder.Body.String())
	}
	var response struct {
		RequestID   string `json:"request_id"`
		RequestedAt string `json:"requested_at"`
		Status      string `json:"status"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.RequestID == "" || response.RequestedAt == "" || response.Status != "received" {
		t.Fatalf("accusé incomplet: %+v", response)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM withdrawal_requests WHERE request_id = ? AND order_id = ?`, response.RequestID, order.OrderID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("demandes enregistrées = %d, want 1", count)
	}
}

func TestPublicWithdrawalDoesNotRevealOrderToWrongEmail(t *testing.T) {
	db, err := dbpkg.InitDatabase(filepath.Join(t.TempDir(), "withdrawals.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	order, err := dbpkg.CreateOrder(db, "client@example.com", "starter", 1, "stripe", "", "")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]string{"order_id": order.OrderID, "email": "intrus@example.com"})
	recorder := httptest.NewRecorder()
	PublicWithdrawalHandler(db, nil).ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/public/withdrawal", bytes.NewReader(body)))

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM withdrawal_requests").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("demandes enregistrées = %d, want 0", count)
	}
}
