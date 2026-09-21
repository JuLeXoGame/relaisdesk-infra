package invoice

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"

	dbpkg "database"
)

func TestGenerateInvoicePDF(t *testing.T) {
	tempDir := t.TempDir()

	inv := &dbpkg.Invoice{
		InvoiceNumber:      "FAC-2026-0001",
		OrderID:            "ORD-TEST-0001",
		CustomerEmail:      "client@entreprise.fr",
		CustomerName:       "Société ABC",
		CustomerAddress:    "10 Rue de la Paix",
		CustomerPostalCode: "75001",
		CustomerCity:       "Paris",
		CustomerCountry:    "France",
		CustomerSIRET:      "12345678900012",
		Plan:               "Pro",
		Technicians:        10,
		AmountHT:           20.00,
		AmountTVA:          0.00,
		AmountTTC:          20.00,
		Status:             "paid",
		CreatedAt:          time.Date(2026, 8, 23, 14, 0, 0, 0, time.UTC),
	}

	pdfPath, pdfBytes, err := GenerateInvoicePDF(tempDir, inv, "Carte Bancaire (Stripe)")
	if err != nil {
		t.Fatalf("GenerateInvoicePDF returned error: %v", err)
	}

	if len(pdfBytes) < 500 {
		t.Fatalf("Generated PDF bytes too small: %d", len(pdfBytes))
	}

	// Verify PDF magic header
	if !bytes.HasPrefix(pdfBytes, []byte("%PDF-")) {
		t.Errorf("PDF does not start with %%PDF- magic bytes")
	}

	// Check file on disk
	fileInfo, err := os.Stat(pdfPath)
	if err != nil {
		t.Fatalf("Failed to stat generated PDF file on disk: %v", err)
	}

	if fileInfo.Size() == 0 {
		t.Errorf("Generated PDF file is empty")
	}
}

func TestGenerateInvoiceDocument(t *testing.T) {
	tempDir := t.TempDir()

	inv := &dbpkg.Invoice{
		InvoiceNumber:      "FAC-2026-0001",
		OrderID:            "ORD-TEST-0001",
		CustomerEmail:      "client@entreprise.fr",
		CustomerName:       "Société ABC",
		CustomerAddress:    "10 Rue de la Paix",
		CustomerPostalCode: "75001",
		CustomerCity:       "Paris",
		CustomerCountry:    "France",
		CustomerSIRET:      "12345678900012",
		Plan:               "Pro",
		Technicians:        10,
		AmountHT:           20.00,
		AmountTVA:          0.00,
		AmountTTC:          20.00,
		Status:             "paid",
		CreatedAt:          time.Date(2026, 8, 23, 14, 0, 0, 0, time.UTC),
	}

	filePath, err := GenerateInvoiceDocument(tempDir, inv, "Carte Bancaire (Stripe)")
	if err != nil {
		t.Fatalf("GenerateInvoiceDocument returned error: %v", err)
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read generated invoice file: %v", err)
	}

	contentStr := string(content)

	// Verify required legal mentions
	requiredStrings := []string{
		"FAC-2026-0001",
		"Informatique A Domicile 03",
		"94074710800014",
		"9511Z",
		"TVA non applicable, art. 293 B du CGI",
		"2 Chemin de Lonzais",
		"03170 Bizeneuille",
		"Société ABC",
		"client@entreprise.fr",
		"10 Rue de la Paix",
		"75001 Paris",
		"12345678900012",
		"Plan Pro",
		"20.00 €",
	}

	for _, s := range requiredStrings {
		if !strings.Contains(contentStr, s) {
			t.Errorf("Missing expected string in invoice document: %q", s)
		}
	}
}

func TestGenerateCreditNote(t *testing.T) {
	tempDir := t.TempDir()

	creditNote := &dbpkg.Invoice{
		InvoiceNumber:      "AV-2026-0001",
		OrderID:            "ORD-TEST-0001",
		CustomerEmail:      "client@entreprise.fr",
		CustomerName:       "Société ABC",
		CustomerAddress:    "10 Rue de la Paix",
		CustomerPostalCode: "75001",
		CustomerCity:       "Paris",
		CustomerCountry:    "France",
		CustomerSIRET:      "12345678900012",
		Plan:               "Pro",
		Technicians:        10,
		AmountHT:           20.00,
		AmountTVA:          0.00,
		AmountTTC:          20.00,
		Status:             "credit_note",
		Notes:              "Avoir sur facture FAC-2026-0001 : Remboursement",
		CreatedAt:          time.Date(2026, 8, 24, 10, 0, 0, 0, time.UTC),
	}

	pdfPath, pdfBytes, err := GenerateInvoicePDF(tempDir, creditNote, "Avoir / Crédit")
	if err != nil {
		t.Fatalf("GenerateInvoicePDF returned error: %v", err)
	}
	if len(pdfBytes) < 500 {
		t.Fatalf("Generated PDF bytes too small: %d", len(pdfBytes))
	}
	if _, err := os.Stat(pdfPath); err != nil {
		t.Fatalf("Failed to stat generated PDF file: %v", err)
	}

	docPath, err := GenerateInvoiceDocument(tempDir, creditNote, "Avoir / Crédit")
	if err != nil {
		t.Fatalf("GenerateInvoiceDocument returned error: %v", err)
	}
	content, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatalf("Failed to read HTML document: %v", err)
	}
	contentStr := string(content)
	if !strings.Contains(contentStr, "FACTURE D&#39;AVOIR") && !strings.Contains(contentStr, "FACTURE D'AVOIR") {
		t.Errorf("HTML document does not contain FACTURE D'AVOIR: %s", contentStr)
	}
	if !strings.Contains(contentStr, "AV-2026-0001") {
		t.Errorf("HTML document does not contain AV-2026-0001")
	}
}

