package handlers

import (
	"api/middleware"
	"context"
	dbpkg "database"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func ciiInvoiceFixture(t *testing.T) (*sql.DB, *dbpkg.Invoice, int64, int64) {
	t.Helper()
	db, err := dbpkg.InitDatabase(filepath.Join(t.TempDir(), "cii.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	owner, err := dbpkg.EnsureCustomer(db, "cii-owner@example.com", "business", "CII Owner")
	if err != nil {
		t.Fatal(err)
	}
	stranger, err := dbpkg.EnsureCustomer(db, "cii-stranger@example.com", "business", "CII Stranger")
	if err != nil {
		t.Fatal(err)
	}
	inv, err := dbpkg.CreateInvoice(db, &dbpkg.Invoice{
		CustomerEmail: "cii-owner@example.com",
		CustomerName:  "CII Owner",
		Plan:          "Pro",
		Technicians:   3,
		AmountHT:      110.00,
		Status:        "paid",
	})
	if err != nil {
		t.Fatal(err)
	}
	if inv.CustomerID != owner.ID {
		t.Fatalf("invoice customer = %d, want owner %d", inv.CustomerID, owner.ID)
	}
	return db, inv, owner.ID, stranger.ID
}

func TestCustomerDownloadInvoiceCII(t *testing.T) {
	db, inv, ownerID, strangerID := ciiInvoiceFixture(t)
	handler := CustomerDownloadInvoiceCIIHandler(db)
	path := "/api/v1/customer/invoices/" + inv.InvoiceNumber + "/cii"

	withIdentity := func(customerID int64, target string) *http.Request {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		if customerID > 0 {
			req = req.WithContext(context.WithValue(req.Context(), middleware.CustomerIdentityContextKey, &dbpkg.CustomerIdentity{ID: customerID}))
		}
		return req
	}

	// 401 without identity.
	rec := httptest.NewRecorder()
	handler(rec, withIdentity(0, path))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no identity status = %d, want 401", rec.Code)
	}

	// 404 for a stranger and for an unknown number.
	rec = httptest.NewRecorder()
	handler(rec, withIdentity(strangerID, path))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("stranger status = %d, want 404", rec.Code)
	}
	rec = httptest.NewRecorder()
	handler(rec, withIdentity(ownerID, "/api/v1/customer/invoices/FAC-2099-9999/cii"))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown number status = %d, want 404", rec.Code)
	}

	// 200 with XML payload for the owner.
	rec = httptest.NewRecorder()
	handler(rec, withIdentity(ownerID, path))
	if rec.Code != http.StatusOK {
		t.Fatalf("owner status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/xml; charset=utf-8" {
		t.Errorf("content-type = %q", ct)
	}
	if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, inv.InvoiceNumber+".xml") {
		t.Errorf("content-disposition = %q", cd)
	}
	body := rec.Body.String()
	for _, want := range []string{"rsm:CrossIndustryInvoice", inv.InvoiceNumber, "<ram:TypeCode>380</ram:TypeCode>"} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q", want)
		}
	}
}

func TestAdminDownloadInvoiceCII(t *testing.T) {
	db, inv, _, _ := ciiInvoiceFixture(t)
	handler := AdminDownloadInvoiceCIIHandler(db)

	rec := httptest.NewRecorder()
	handler(rec, httptest.NewRequest(http.MethodGet, "/api/v1/admin/invoices/FAC-2099-9999/cii", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown number status = %d, want 404", rec.Code)
	}

	rec = httptest.NewRecorder()
	handler(rec, httptest.NewRequest(http.MethodGet, "/api/v1/admin/invoices/"+inv.InvoiceNumber+"/cii", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/xml; charset=utf-8" {
		t.Errorf("content-type = %q", ct)
	}
	if body := rec.Body.String(); !strings.Contains(body, "rsm:CrossIndustryInvoice") || !strings.Contains(body, inv.InvoiceNumber) {
		t.Errorf("body is not the CII document: %.120s", body)
	}
}
