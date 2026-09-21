package database

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestInvoicesCRUDAndNumbering(t *testing.T) {
	db, err := InitDatabase(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("InitDatabase failed: %v", err)
	}
	defer db.Close()

	// 1. Test Number Generation
	num1, err := GenerateNextInvoiceNumber(db)
	if err != nil {
		t.Fatalf("GenerateNextInvoiceNumber failed: %v", err)
	}
	expectedNum1 := "FAC-" + time.Now().Format("2006") + "-0001"
	if num1 != expectedNum1 {
		t.Errorf("Expected %s, got %s", expectedNum1, num1)
	}

	// 2. Create Invoice
	inv := &Invoice{
		OrderID:            "",
		CustomerEmail:      "pro@domaine.fr",
		CustomerName:       "SARL Technologique",
		CustomerAddress:    "45 Avenue de la République",
		CustomerPostalCode: "69002",
		CustomerCity:       "Lyon",
		CustomerCountry:    "France",
		CustomerSIRET:      "98765432100099",
		Plan:               "Starter",
		Technicians:        1,
		AmountHT:           10.00,
		AmountTVA:          0.00,
		AmountTTC:          10.00,
		Status:             "paid",
		PDFPath:            "/data/relaisdesk/invoices/" + num1 + ".html",
		IsManual:           false,
		Notes:              "Test automatique",
	}

	created, err := CreateInvoice(db, inv)
	if err != nil {
		t.Fatalf("CreateInvoice failed: %v", err)
	}
	if created.InvoiceNumber != expectedNum1 {
		t.Errorf("Expected invoice number %s, got %s", expectedNum1, created.InvoiceNumber)
	}

	// 3. Test next sequence number
	num2, err := GenerateNextInvoiceNumber(db)
	if err != nil {
		t.Fatalf("GenerateNextInvoiceNumber second call failed: %v", err)
	}
	expectedNum2 := "FAC-" + time.Now().Format("2006") + "-0002"
	if num2 != expectedNum2 {
		t.Errorf("Expected %s, got %s", expectedNum2, num2)
	}

	// Deleting an older invoice must never recycle or collide with a later number.
	inv2 := *inv
	inv2.ID = 0
	inv2.InvoiceNumber = num2
	inv2.PDFPath = "/data/relaisdesk/invoices/" + num2 + ".html"
	if _, err := CreateInvoice(db, &inv2); err != nil {
		t.Fatalf("CreateInvoice second invoice failed: %v", err)
	}
	if _, err := db.Exec(`DELETE FROM invoices WHERE invoice_number = ?`, expectedNum1); err != nil {
		t.Fatal(err)
	}
	num3, err := GenerateNextInvoiceNumber(db)
	if err != nil {
		t.Fatal(err)
	}
	expectedNum3 := "FAC-" + time.Now().Format("2006") + "-0003"
	if num3 != expectedNum3 {
		t.Fatalf("Expected %s after a gap, got %s", expectedNum3, num3)
	}

	// 4. Retrieve the remaining invoice by number
	fetched, err := GetInvoiceByNumber(db, expectedNum2)
	if err != nil {
		t.Fatalf("GetInvoiceByNumber failed: %v", err)
	}
	if fetched.CustomerName != "SARL Technologique" || fetched.CustomerSIRET != "98765432100099" {
		t.Errorf("Fetched invoice data mismatch: %+v", fetched)
	}

	// 5. List invoices
	list, err := ListInvoices(db, "")
	if err != nil {
		t.Fatalf("ListInvoices failed: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("Expected 1 remaining invoice, got %d", len(list))
	}
}

func TestConcurrentInvoiceNumbering(t *testing.T) {
	db, err := InitDatabase(t.TempDir() + "/concurrent.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	const workers = 8
	numbers := make(chan string, workers)
	errors := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			created, err := CreateInvoice(db, &Invoice{
				CustomerEmail: fmt.Sprintf("client-%d@example.com", index),
				CustomerName:  fmt.Sprintf("Client %d", index),
				Plan:          "Starter",
				Technicians:   1,
				AmountHT:      10,
				AmountTTC:     10,
				PDFPath:       fmt.Sprintf("invoice-%d.pdf", index),
			})
			if err != nil {
				errors <- err
				return
			}
			numbers <- created.InvoiceNumber
		}(i)
	}
	wg.Wait()
	close(numbers)
	close(errors)
	for err := range errors {
		t.Errorf("concurrent creation failed: %v", err)
	}

	seen := make(map[string]bool, workers)
	for number := range numbers {
		if seen[number] {
			t.Errorf("duplicate invoice number: %s", number)
		}
		seen[number] = true
	}
	if len(seen) != workers {
		t.Fatalf("created %d unique invoices, want %d", len(seen), workers)
	}
}

func TestCreateInvoiceIsIdempotentPerOrder(t *testing.T) {
	db, err := InitDatabase(t.TempDir() + "/idempotent.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	order, err := CreateOrder(db, "invoice@example.com", "starter", 1, "bank_transfer", "", "")
	if err != nil {
		t.Fatal(err)
	}

	input := &Invoice{
		OrderID: order.OrderID, CustomerEmail: order.Email, CustomerName: "Client",
		Plan: order.Plan, Technicians: order.Technicians, AmountHT: order.Price, AmountTTC: order.Price,
	}
	first, err := CreateInvoice(db, input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := CreateInvoice(db, &Invoice{
		OrderID: order.OrderID, CustomerEmail: order.Email, CustomerName: "Client",
		Plan: order.Plan, Technicians: order.Technicians, AmountHT: order.Price, AmountTTC: order.Price,
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.InvoiceNumber != second.InvoiceNumber {
		t.Fatalf("duplicate order produced %q and %q", first.InvoiceNumber, second.InvoiceNumber)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM invoices WHERE order_id = ?`, order.OrderID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("order has %d invoices, want 1", count)
	}
}

func TestDeleteInvoiceAndDecoupling(t *testing.T) {
	db, err := InitDatabase(t.TempDir() + "/del_inv.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	order, err := CreateOrder(db, "test@example.com", "starter", 1, "bank_transfer", "", "")
	if err != nil {
		t.Fatal(err)
	}

	inv, err := CreateInvoice(db, &Invoice{
		OrderID:       order.OrderID,
		CustomerEmail: order.Email,
		CustomerName:  "Test Client",
		Plan:          order.Plan,
		Technicians:   order.Technicians,
		AmountHT:      order.Price,
		AmountTTC:     order.Price,
		PDFPath:       "/some/path/FAC.pdf",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Verify order has invoice_number
	ord, err := GetOrderByID(db, order.OrderID)
	if err != nil {
		t.Fatal(err)
	}
	if ord.InvoiceNumber != inv.InvoiceNumber {
		t.Fatalf("expected order invoice_number %s, got %s", inv.InvoiceNumber, ord.InvoiceNumber)
	}

	// Delete invoice
	pdfPath, err := DeleteInvoice(db, inv.InvoiceNumber)
	if err != nil {
		t.Fatalf("DeleteInvoice failed: %v", err)
	}
	if pdfPath != "/some/path/FAC.pdf" {
		t.Fatalf("expected pdfPath /some/path/FAC.pdf, got %s", pdfPath)
	}

	// Verify invoice is gone
	_, err = GetInvoiceByNumber(db, inv.InvoiceNumber)
	if err == nil {
		t.Fatal("expected invoice to be deleted")
	}

	// Verify order still exists and invoice_number is cleared
	ordAfter, err := GetOrderByID(db, order.OrderID)
	if err != nil {
		t.Fatalf("expected order to still exist: %v", err)
	}
	if ordAfter.InvoiceNumber != "" {
		t.Fatalf("expected order invoice_number to be cleared, got %s", ordAfter.InvoiceNumber)
	}
}

func TestCreditNoteCreation(t *testing.T) {
	db, err := InitDatabase(t.TempDir() + "/credit_note.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	inv, err := CreateInvoice(db, &Invoice{
		CustomerEmail: "credit@example.com",
		CustomerName:  "Entreprise Avoir",
		Plan:          "Starter",
		Technicians:   1,
		AmountHT:      50.0,
		AmountTTC:     50.0,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Create credit note
	creditNote, err := CreateCreditNote(db, inv.InvoiceNumber, "Erreur de facturation")
	if err != nil {
		t.Fatalf("CreateCreditNote failed: %v", err)
	}

	expectedPrefix := "AV-" + time.Now().Format("2006") + "-0001"
	if creditNote.InvoiceNumber != expectedPrefix {
		t.Fatalf("expected credit note number %s, got %s", expectedPrefix, creditNote.InvoiceNumber)
	}
	if creditNote.Status != "credit_note" {
		t.Fatalf("expected status credit_note, got %s", creditNote.Status)
	}
	if creditNote.AmountHT != 50.0 || creditNote.AmountTTC != 50.0 {
		t.Fatalf("expected amounts 50.0, got %f / %f", creditNote.AmountHT, creditNote.AmountTTC)
	}

	// Verify original invoice has note
	origAfter, err := GetInvoiceByNumber(db, inv.InvoiceNumber)
	if err != nil {
		t.Fatal(err)
	}
	if origAfter.Notes == "" {
		t.Fatal("expected notes on original invoice")
	}

	// Cannot create credit note on a credit note
	_, err = CreateCreditNote(db, creditNote.InvoiceNumber, "test invalid")
	if err == nil {
		t.Fatal("expected error when creating credit note on credit note")
	}
}

