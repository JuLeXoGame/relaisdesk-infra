package database

import (
	"path/filepath"
	"testing"
	"time"
)

func TestOrderLifecycle(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test_orders.db")
	db, err := InitDatabase(dbPath)
	if err != nil {
		t.Fatalf("InitDatabase failed: %v", err)
	}
	defer db.Close()

	// 1. Test Server Price Calculation (Monthly)
	price, techs, plan, err := CalculateServerPrice("starter", 1)
	if err != nil || price != 24.90 || techs != 1 || plan != "Starter" {
		t.Fatalf("Starter monthly failed: price=%.2f, techs=%d, plan=%s, err=%v", price, techs, plan, err)
	}

	price, techs, plan, err = CalculateServerPrice("pro", 5)
	if err != nil || price != 110.00 || techs != 5 || plan != "Pro" {
		t.Fatalf("Pro monthly failed: price=%.2f, techs=%d, plan=%s, err=%v", price, techs, plan, err)
	}

	price, techs, plan, err = CalculateServerPrice("ultra", 10)
	if err != nil || price != 199.00 || techs != 10 || plan != "Ultra" {
		t.Fatalf("Ultra 10 monthly failed: price=%.2f, techs=%d, plan=%s, err=%v", price, techs, plan, err)
	}

	price, techs, plan, err = CalculateServerPrice("ultra", 20)
	if err != nil || price != 339.00 || techs != 20 || plan != "Ultra" {
		t.Fatalf("Ultra 20 monthly failed: price=%.2f, techs=%d, plan=%s, err=%v", price, techs, plan, err)
	}

	price, techs, plan, err = CalculateServerPrice("ultra", 50)
	if err != nil || price != 759.00 || techs != 50 || plan != "Ultra" {
		t.Fatalf("Ultra 50 monthly failed: price=%.2f, techs=%d, plan=%s, err=%v", price, techs, plan, err)
	}

	price, techs, plan, err = CalculateServerPrice("ultra", 100)
	if err != nil || price != 1259.00 || techs != 100 || plan != "Ultra" {
		t.Fatalf("Ultra 100 monthly failed: price=%.2f, techs=%d, plan=%s, err=%v", price, techs, plan, err)
	}

	price, techs, plan, err = CalculateServerPrice("ultra", 200)
	if err != nil || price != 1959.00 || techs != 200 || plan != "Ultra" {
		t.Fatalf("Ultra 200 monthly failed: price=%.2f, techs=%d, plan=%s, err=%v", price, techs, plan, err)
	}

	price, techs, plan, err = CalculateServerPrice("ultra", 500)
	if err != nil || price != 4059.00 || techs != 500 || plan != "Ultra" {
		t.Fatalf("Ultra 500 monthly failed: price=%.2f, techs=%d, plan=%s, err=%v", price, techs, plan, err)
	}

	if _, _, _, err = CalculateServerPrice("ultra", 501); err == nil {
		t.Fatalf("Expected error for 501 techs")
	}

	// 1b. Test Server Price Calculation (Annual)
	price, techs, plan, err = CalculateServerPrice("starter", 1, "annual")
	if err != nil || price != 239.00 || techs != 1 || plan != "Starter" {
		t.Fatalf("Starter annual failed: price=%.2f, techs=%d, plan=%s, err=%v", price, techs, plan, err)
	}

	price, techs, plan, err = CalculateServerPrice("pro", 5, "annual")
	if err != nil || price != 1100.00 || techs != 5 || plan != "Pro" {
		t.Fatalf("Pro annual failed: price=%.2f, techs=%d, plan=%s, err=%v", price, techs, plan, err)
	}

	price, techs, plan, err = CalculateServerPrice("ultra", 10, "annual")
	if err != nil || price != 1990.00 || techs != 10 || plan != "Ultra" {
		t.Fatalf("Ultra 10 annual failed: price=%.2f, techs=%d, plan=%s, err=%v", price, techs, plan, err)
	}

	// 2. Create Order
	order, err := CreateOrder(db, "tech@example.com", "pro", 5, "bank_transfer", "Test order", "")
	if err != nil {
		t.Fatalf("CreateOrder failed: %v", err)
	}

	if order.Price != 110.00 || order.Status != "pending" || order.Technicians != 5 || order.BillingCycle != "monthly" {
		t.Errorf("Order properties invalid: %+v", order)
	}

	// 3. Get Order by ID
	fetched, err := GetOrderByID(db, order.OrderID)
	if err != nil {
		t.Fatalf("GetOrderByID failed: %v", err)
	}
	if fetched.OrderID != order.OrderID || fetched.Email != "tech@example.com" {
		t.Errorf("Fetched order mismatch: %+v", fetched)
	}

	// 4. Create license and mark order paid
	lic, err := CreateLicense(db, order.Email, 30, order.Technicians, "Paid via test")
	if err != nil {
		t.Fatalf("CreateLicense failed: %v", err)
	}

	if err := MarkOrderPaid(db, order.OrderID, lic.LicenseID); err != nil {
		t.Fatalf("MarkOrderPaid failed: %v", err)
	}

	paidOrder, err := GetOrderByID(db, order.OrderID)
	if err != nil {
		t.Fatalf("GetOrderByID paid failed: %v", err)
	}
	if paidOrder.Status != "paid" || paidOrder.LicenseID != lic.LicenseID || paidOrder.PaidAt == nil {
		t.Errorf("Order not properly marked paid: %+v", paidOrder)
	}

	// 5. Test Reaper for expired orders
	expiredOrder, err := CreateOrder(db, "expired@example.com", "starter", 1, "stripe", "Expired", "sess_123")
	if err != nil {
		t.Fatalf("CreateOrder expired failed: %v", err)
	}

	// Force expiration in DB
	pastTime := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339)
	_, err = db.Exec("UPDATE orders SET expires_at = ? WHERE order_id = ?", pastTime, expiredOrder.OrderID)
	if err != nil {
		t.Fatalf("Force past expiration failed: %v", err)
	}

	cancelledCount, err := CancelExpiredOrders(db)
	if err != nil {
		t.Fatalf("CancelExpiredOrders failed: %v", err)
	}
	if cancelledCount != 1 {
		t.Errorf("Expected 1 cancelled order, got %d", cancelledCount)
	}

	recheck, err := GetOrderByID(db, expiredOrder.OrderID)
	if err != nil {
		t.Fatalf("GetOrderByID recheck failed: %v", err)
	}
	if recheck.Status != "cancelled" {
		t.Errorf("Expected status cancelled, got %s", recheck.Status)
	}

	// 6. Test DeleteOrder
	if err := DeleteOrder(db, paidOrder.OrderID); err == nil {
		t.Errorf("Expected refusal when deleting a paid order")
	}
	if err := DeleteOrder(db, expiredOrder.OrderID); err != nil {
		t.Fatalf("DeleteOrder failed: %v", err)
	}
	if _, err := GetOrderByID(db, expiredOrder.OrderID); err == nil {
		t.Errorf("Expected deleted order to not be found")
	}
	if err := DeleteOrder(db, "ORD-NON-EXISTENT"); err == nil {
		t.Errorf("Expected error when deleting non-existent order")
	}
}

func TestGenerateOrderID(t *testing.T) {
	seen := make(map[string]bool, 1000)
	for i := 0; i < 1000; i++ {
		id, err := GenerateOrderID()
		if err != nil {
			t.Fatalf("GenerateOrderID failed: %v", err)
		}
		if len(id) != 21 || id[:4] != "ORD-" || id[12] != '-' {
			t.Fatalf("bad order ID format: %q", id)
		}
		for _, c := range id[4:12] + id[13:] {
			if (c < '0' || c > '9') && (c < 'A' || c > 'F') {
				t.Fatalf("non-hex order ID: %q", id)
			}
		}
		if seen[id] {
			t.Fatalf("duplicate order ID: %q", id)
		}
		seen[id] = true
	}
}

func TestAnnualOrderFulfillment(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test_annual_orders.db")
	db, err := InitDatabase(dbPath)
	if err != nil {
		t.Fatalf("InitDatabase failed: %v", err)
	}
	defer db.Close()

	// 1. Create Annual Order
	billing := &BillingDetails{
		Name:         "Entreprise Test",
		Address:      "1 rue du Test",
		PostalCode:   "75001",
		City:         "Paris",
		Country:      "France",
		CustomerType: "business",
		BillingCycle: "annual",
	}
	order, err := CreateOrderWithBilling(db, "annual@example.com", "starter", 1, "stripe", "Annual order", "sess_annual", billing)
	if err != nil {
		t.Fatalf("CreateOrderWithBilling failed: %v", err)
	}

	if order.Price != 239.00 || order.BillingCycle != "annual" {
		t.Fatalf("Expected annual starter price 239.00 and cycle annual, got price=%.2f cycle=%s", order.Price, order.BillingCycle)
	}

	// 2. Fulfill Annual Order
	lic, err := FulfillPendingOrder(db, order.OrderID, "Fulfillment annual")
	if err != nil {
		t.Fatalf("FulfillPendingOrder failed: %v", err)
	}

	duration := lic.ExpiresAt.Sub(lic.CreatedAt)
	if duration < 364*24*time.Hour || duration > 366*24*time.Hour {
		t.Errorf("Expected ~365 days validity, got %v", duration)
	}

	// 3. Test Renewal of Annual Order
	renewal, err := CreateRenewalOrder(db, "annual@example.com", lic.LicenseID, "stripe", "2026-08-25", true, false)
	if err != nil {
		t.Fatalf("CreateRenewalOrder failed: %v", err)
	}
	if renewal.BillingCycle != "annual" || renewal.Price != 239.00 {
		t.Fatalf("Expected renewal billing_cycle annual and price 239.00, got cycle=%s price=%.2f", renewal.BillingCycle, renewal.Price)
	}

	renewedLic, err := FulfillPendingRenewalOrder(db, renewal.OrderID, "Renewal annual")
	if err != nil {
		t.Fatalf("FulfillPendingRenewalOrder failed: %v", err)
	}
	totalDuration := renewedLic.ExpiresAt.Sub(lic.CreatedAt)
	if totalDuration < 729*24*time.Hour || totalDuration > 731*24*time.Hour {
		t.Errorf("Expected ~730 days total validity after renewal, got %v", totalDuration)
	}
}

func TestCreateOrderWithBillingReturnsTermsAcceptedAt(t *testing.T) {
	db, err := InitDatabase(filepath.Join(t.TempDir(), "orders-terms.db"))
	if err != nil {
		t.Fatalf("InitDatabase failed: %v", err)
	}
	defer db.Close()
	order, err := CreateOrderWithBilling(db, "terms@example.com", "pro", 5, "bank_transfer", "", "", &BillingDetails{
		Name: "Entreprise", City: "Paris", Country: "France", CustomerType: "business",
		TermsVersion: "2026-09", TermsAccepted: true,
	})
	if err != nil {
		t.Fatalf("CreateOrderWithBilling failed: %v", err)
	}
	if order.TermsAcceptedAt == nil {
		t.Fatal("expected TermsAcceptedAt on returned order")
	}
	fetched, err := GetOrderByID(db, order.OrderID)
	if err != nil {
		t.Fatalf("GetOrderByID failed: %v", err)
	}
	if fetched.TermsAcceptedAt == nil || fetched.TermsAcceptedAt.Unix() != order.TermsAcceptedAt.Unix() {
		t.Fatalf("terms mismatch: returned=%v fetched=%v", order.TermsAcceptedAt, fetched.TermsAcceptedAt)
	}
}
