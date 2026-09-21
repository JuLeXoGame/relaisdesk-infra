package handlers

import (
	"bytes"
	dbpkg "database"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"api/config"
)

func TestPublicProPricingMatchesChargedPrice(t *testing.T) {
	recorder := httptest.NewRecorder()
	PublicPricingHandler()(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/public/pricing", nil))
	var catalog map[string]struct {
		Price        float64 `json:"price"`
		MonthlyPrice float64 `json:"price_monthly"`
		AnnualPrice  float64 `json:"price_annual"`
		Technicians  int     `json:"technicians"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &catalog); err != nil {
		t.Fatal(err)
	}
	pro := catalog["pro"]
	if recorder.Code != http.StatusOK || pro.Price != 110 || pro.MonthlyPrice != 110 || pro.AnnualPrice != 1100 || pro.Technicians != 5 {
		t.Fatalf("invalid Pro catalog: %+v", pro)
	}
	for _, cycle := range []string{"monthly", "annual"} {
		price, _, _, err := dbpkg.CalculateServerPrice("pro", 5, cycle)
		advertised := pro.MonthlyPrice
		if cycle == "annual" {
			advertised = pro.AnnualPrice
		}
		if err != nil || price != advertised {
			t.Fatalf("catalog/checkout mismatch for %s: %v", cycle, err)
		}
	}
}

func TestCatalogCheckoutUsesNewPriceAndTerms(t *testing.T) {
	for _, tariff := range []struct {
		plan     string
		capacity int
		monthly  float64
	}{{"pro", 5, 110}, {"ultra", 10, 199}, {"ultra", 50, 759}, {"ultra", 500, 4059}} {
		for _, method := range []string{"stripe", "bank_transfer"} {
			for _, cycle := range []string{"monthly", "annual"} {
				t.Run(tariff.plan+"/"+method+"/"+cycle, func(t *testing.T) {
					db, err := dbpkg.InitDatabase(filepath.Join(t.TempDir(), "orders.db"))
					if err != nil {
						t.Fatal(err)
					}
					defer db.Close()
					body, err := json.Marshal(PublicOrderRequest{
						Email: "pricing@example.test", Plan: tariff.plan, Technicians: tariff.capacity,
						PaymentMethod: method, BillingCycle: cycle,
						Name: "Test", Address: "1 rue Test", PostalCode: "75001", City: "Paris",
						CustomerType: "business", TermsVersion: publicTermsVersion, TermsAccepted: true,
					})
					if err != nil {
						t.Fatal(err)
					}
					cfg := &config.Config{DevHTTP: true, StripeMock: true,
						StripeSuccessURL: "https://example.test/success", StripeCancelURL: "https://example.test/cancel",
						BankIBAN: "FR76 3000 6000 0112 3456 7890 189", BankBIC: "AGRIFRPP", BankHolder: "RelaisDesk"}
					recorder := httptest.NewRecorder()
					PublicOrderHandler(db, cfg, nil)(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/public/order", bytes.NewReader(body)))
					if recorder.Code != http.StatusCreated {
						t.Fatalf("checkout: %d %s", recorder.Code, recorder.Body.String())
					}
					var response struct {
						OrderID string  `json:"order_id"`
						Price   float64 `json:"price"`
					}
					if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
						t.Fatal(err)
					}
					expected := tariff.monthly
					if cycle == "annual" {
						expected *= 10
					}
					order, err := dbpkg.GetOrderByID(db, response.OrderID)
					if err != nil {
						t.Fatal(err)
					}
					if response.Price != expected || order.Price != expected || order.TermsVersion != publicTermsVersion || order.Technicians != tariff.capacity {
						t.Fatalf("wrong price/terms/capacity: %+v", order)
					}
				})
			}
		}
	}
}

func TestPublicCustomPricingMatchesChargedPrice(t *testing.T) {
	recorder := httptest.NewRecorder()
	PublicPricingHandler()(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/public/pricing", nil))
	var catalog struct {
		Ultra struct {
			Price     float64 `json:"base_price"`
			Monthly   float64 `json:"base_monthly"`
			Annual    float64 `json:"base_annual"`
			BaseTechs int     `json:"base_techs"`
			MaxTechs  int     `json:"max_techs"`
		} `json:"ultra"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &catalog); err != nil {
		t.Fatal(err)
	}
	custom := catalog.Ultra
	if recorder.Code != http.StatusOK || custom.Price != 199 || custom.Monthly != 199 || custom.Annual != 1990 || custom.BaseTechs != 10 || custom.MaxTechs != 500 {
		t.Fatalf("incorrect custom catalog: %+v", custom)
	}
	for _, cycle := range []string{"monthly", "annual"} {
		price, _, _, err := dbpkg.CalculateServerPrice("ultra", custom.BaseTechs, cycle)
		advertised := custom.Monthly
		if cycle == "annual" {
			advertised = custom.Annual
		}
		if err != nil || price != advertised {
			t.Fatalf("custom catalog/checkout mismatch: %s %v", cycle, err)
		}
	}
}

func TestNewProOrderRejectsPreviousPriceGridAcceptance(t *testing.T) {
	db, err := dbpkg.InitDatabase(filepath.Join(t.TempDir(), "orders.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	body := []byte(`{"email":"pricing@example.test","plan":"pro","technicians":5,"payment_method":"stripe","name":"Test","address":"1 rue Test","postal_code":"75001","city":"Paris","customer_type":"business","terms_version":"2026-09-07","terms_accepted":true}`)
	recorder := httptest.NewRecorder()
	PublicOrderHandler(db, &config.Config{DevHTTP: true, StripeMock: true}, nil)(recorder,
		httptest.NewRequest(http.MethodPost, "/api/v1/public/order", bytes.NewReader(body)))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("previous price grid accepted for a new order: %d", recorder.Code)
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM orders").Scan(&count); err != nil || count != 0 {
		t.Fatalf("unexpected order created: %d, %v", count, err)
	}
}
