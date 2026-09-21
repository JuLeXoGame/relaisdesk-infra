package database

import (
	"math"
	"path/filepath"
	"testing"
)

func TestCatalogRenewalUsesCurrentPriceWithoutRepricingOriginalOrder(t *testing.T) {
	for _, tariff := range []struct {
		plan              string
		capacity          int
		previous, current float64
	}{{"pro", 5, 129, 110}, {"ultra", 10, 239, 199}} {
		for _, cycle := range []string{"monthly", "annual"} {
			t.Run(tariff.plan+"/"+cycle, func(t *testing.T) {
				db, err := InitDatabase(filepath.Join(t.TempDir(), "pricing.db"))
				if err != nil {
					t.Fatal(err)
				}
				defer db.Close()
				order, err := CreateOrderWithBilling(db, "pricing@example.test", tariff.plan, tariff.capacity, "stripe", "", "", &BillingDetails{
					Name: "Test", CustomerType: "business", BillingCycle: cycle,
					TermsVersion: "2026-09-07", TermsAccepted: true,
				})
				if err != nil {
					t.Fatal(err)
				}
				// Simulate a pending order accepted before the price change.
				previous, current := tariff.previous, tariff.current
				if cycle == "annual" {
					previous, current = 10*previous, 10*current
				}
				if _, err := db.Exec("UPDATE orders SET price = ? WHERE order_id = ?", previous, order.OrderID); err != nil {
					t.Fatal(err)
				}
				license, err := FulfillPendingOrder(db, order.OrderID, "price regression")
				if err != nil {
					t.Fatal(err)
				}
				renewal, err := CreateRenewalOrder(db, license.Email, license.LicenseID, "stripe", "2026-09-08", true, false)
				if err != nil {
					t.Fatal(err)
				}
				if renewal.Price != current || renewal.Technicians != tariff.capacity || renewal.BillingCycle != cycle {
					t.Fatalf("incorrect renewal: %+v", renewal)
				}
				original, err := GetOrderByID(db, order.OrderID)
				if err != nil {
					t.Fatal(err)
				}
				if original.Price != previous || original.TermsVersion != "2026-09-07" {
					t.Fatal("the historical order was repriced or assigned new terms")
				}
			})
		}
	}
}

func TestAllCustomCapacitiesFollowApprovedGridAndBeatStarterProBundles(t *testing.T) {
	for capacity := 10; capacity <= 500; capacity++ {
		expectedMonthly := 199 + min(capacity-10, 40)*14 + min(max(capacity-50, 0), 50)*10 + max(capacity-100, 0)*7
		for _, cycle := range []string{"monthly", "annual"} {
			price, technicians, plan, err := CalculateServerPrice("ultra", capacity, cycle)
			expected := float64(expectedMonthly)
			if cycle == "annual" {
				expected *= 10
			}
			if err != nil || price != expected || technicians != capacity || plan != "Ultra" {
				t.Fatalf("custom %d/%s: price=%v capacity=%d plan=%s err=%v", capacity, cycle, price, technicians, plan, err)
			}
			starter, _, _, err := CalculateServerPrice("starter", 1, cycle)
			if err != nil {
				t.Fatal(err)
			}
			pro, _, _, err := CalculateServerPrice("pro", 5, cycle)
			if err != nil {
				t.Fatal(err)
			}
			starterCents, proCents := int(math.Round(starter*100)), int(math.Round(pro*100))
			best := capacity * starterCents
			for count := 1; count <= (capacity+4)/5; count++ {
				best = min(best, count*proCents+max(capacity-5*count, 0)*starterCents)
			}
			if int(math.Round(price*100)) >= best {
				t.Fatalf("custom %d/%s is not advantageous: %.2f vs %.2f", capacity, cycle, price, float64(best)/100)
			}
		}
	}
}
