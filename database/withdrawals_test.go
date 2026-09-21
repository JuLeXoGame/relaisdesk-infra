package database

import (
	"path/filepath"
	"testing"
	"time"
)

func TestWithdrawalEmailStagesArePersistedIndependently(t *testing.T) {
	db, err := InitDatabase(filepath.Join(t.TempDir(), "withdrawals.db"))
	if err != nil {
		t.Fatalf("InitDatabase failed: %v", err)
	}
	defer db.Close()

	order, err := CreateOrder(db, "client@example.com", "starter", 1, "bank_transfer", "test", "")
	if err != nil {
		t.Fatalf("CreateOrder failed: %v", err)
	}
	request, err := CreateWithdrawalRequest(db, order.OrderID, order.Email, "Client Test")
	if err != nil {
		t.Fatalf("CreateWithdrawalRequest failed: %v", err)
	}

	sentAt := time.Now().UTC().Truncate(time.Second)
	if err := MarkWithdrawalEmailSent(db, request.RequestID, "customer", sentAt); err != nil {
		t.Fatalf("MarkWithdrawalEmailSent(customer) failed: %v", err)
	}
	stored, err := GetWithdrawalRequest(db, request.RequestID)
	if err != nil {
		t.Fatalf("GetWithdrawalRequest failed: %v", err)
	}
	if stored.CustomerEmailSentAt == nil || !stored.CustomerEmailSentAt.Equal(sentAt) {
		t.Fatalf("customer delivery stage was not persisted: %+v", stored)
	}
	if stored.AdminEmailSentAt != nil {
		t.Fatalf("admin delivery must still be pending: %+v", stored)
	}

	if err := MarkWithdrawalEmailSent(db, request.RequestID, "admin", sentAt.Add(time.Second)); err != nil {
		t.Fatalf("MarkWithdrawalEmailSent(admin) failed: %v", err)
	}
	stored, err = GetWithdrawalRequest(db, request.RequestID)
	if err != nil || stored.AdminEmailSentAt == nil {
		t.Fatalf("admin delivery stage was not persisted: item=%+v err=%v", stored, err)
	}
}
