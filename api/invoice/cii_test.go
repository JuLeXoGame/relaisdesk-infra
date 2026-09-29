package invoice

import (
	"encoding/xml"
	"strings"
	"testing"
	"time"

	dbpkg "database"
)

func ciiSampleInvoice() *dbpkg.Invoice {
	return &dbpkg.Invoice{
		InvoiceNumber:      "FAC-2026-0042",
		CustomerEmail:      "client@example.fr",
		CustomerName:       "Entreprise Exemple",
		CustomerAddress:    "10 rue des Tests",
		CustomerPostalCode: "63000",
		CustomerCity:       "Clermont-Ferrand",
		CustomerCountry:    "France",
		CustomerSIRET:      "12345678900012",
		Plan:               "Pro",
		Technicians:        3,
		AmountHT:           110.00,
		AmountTVA:          0,
		AmountTTC:          110.00,
		Status:             "paid",
		CreatedAt:          time.Date(2026, 9, 27, 10, 0, 0, 0, time.UTC),
	}
}

func TestGenerateInvoiceCII(t *testing.T) {
	out, err := GenerateInvoiceCII(ciiSampleInvoice())
	if err != nil {
		t.Fatalf("GenerateInvoiceCII: %v", err)
	}
	// Well-formed XML.
	var v any
	dec := xml.NewDecoder(strings.NewReader(string(out)))
	dec.Strict = true
	if err := dec.Decode(&v); err != nil {
		t.Fatalf("XML malformed: %v\n%s", err, out)
	}
	doc := string(out)
	for _, want := range []string{
		"rsm:CrossIndustryInvoice",
		"urn:cen.eu:en16931:2017#conformant#urn:factur-x.eu:1p0:BASIC",
		"<ram:ID>FAC-2026-0042</ram:ID>",
		"<ram:TypeCode>380</ram:TypeCode>",
		`<udt:DateTimeString format="102">20260927</udt:DateTimeString>`,
		"<ram:InvoiceCurrencyCode>EUR</ram:InvoiceCurrencyCode>",
		"<ram:LineTotalAmount>110.00</ram:LineTotalAmount>",
		"<ram:GrandTotalAmount>110.00</ram:GrandTotalAmount>",
		"<ram:DuePayableAmount>0.00</ram:DuePayableAmount>",
		"<ram:CategoryCode>E</ram:CategoryCode>",
		"TVA non applicable, art. 293 B du CGI",
		`<ram:ID schemeID="0002">94074710800014</ram:ID>`,
		"Entreprise Exemple",
		"client@example.fr",
		`<ram:ID schemeID="0002">12345678900012</ram:ID>`,
		"Abonnement RelaisDesk Pro — 3 techniciens",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("XML missing %q\n%s", want, doc)
		}
	}
	// No VAT registration for the micro-entreprise issuer.
	if strings.Contains(doc, "SpecifiedTaxRegistration") {
		t.Errorf("issuer must not carry a VAT registration element")
	}
}

func TestGenerateInvoiceCIICreditNote(t *testing.T) {
	inv := ciiSampleInvoice()
	inv.InvoiceNumber = "AV-2026-0007"
	inv.Status = "credit_note"
	out, err := GenerateInvoiceCII(inv)
	if err != nil {
		t.Fatalf("GenerateInvoiceCII: %v", err)
	}
	if !strings.Contains(string(out), "<ram:TypeCode>381</ram:TypeCode>") {
		t.Errorf("credit note must use TypeCode 381\n%s", out)
	}
}

func TestGenerateInvoiceCIIValidation(t *testing.T) {
	if _, err := GenerateInvoiceCII(nil); err == nil {
		t.Errorf("nil invoice must fail")
	}
	bad := ciiSampleInvoice()
	bad.InvoiceNumber = "../evil"
	if _, err := GenerateInvoiceCII(bad); err == nil {
		t.Errorf("invalid number must fail")
	}
	// Minimal buyer still produces valid XML.
	min := ciiSampleInvoice()
	min.CustomerAddress = ""
	min.CustomerPostalCode = ""
	min.CustomerCity = ""
	min.CustomerCountry = ""
	min.CustomerSIRET = ""
	min.CustomerEmail = ""
	out, err := GenerateInvoiceCII(min)
	if err != nil {
		t.Fatalf("minimal buyer: %v", err)
	}
	var v any
	if err := xml.Unmarshal(out, &v); err != nil {
		t.Fatalf("minimal buyer XML malformed: %v", err)
	}
	if !strings.Contains(string(out), "<ram:CountryID>FR</ram:CountryID>") {
		t.Errorf("country must default to FR\n%s", out)
	}
}
