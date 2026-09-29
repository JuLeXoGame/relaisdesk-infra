package invoice

import (
	"encoding/xml"
	"fmt"
	"strings"
	"time"

	dbpkg "database"
)

// Factur-X BASIC (EN 16931) profile identifier.
const ciiBasicProfile = "urn:cen.eu:en16931:2017#conformant#urn:factur-x.eu:1p0:BASIC"

// GenerateInvoiceCII builds a Factur-X BASIC Cross Industry Invoice (CII) XML
// document for the given invoice. The issuer is a micro-entreprise under
// article 293 B du CGI, so VAT is reported with category E (exempt) at 0 %.
func GenerateInvoiceCII(inv *dbpkg.Invoice) ([]byte, error) {
	if inv == nil || !invoiceNumberPattern.MatchString(inv.InvoiceNumber) {
		return nil, fmt.Errorf("numéro de facture invalide")
	}

	isCreditNote := strings.HasPrefix(inv.InvoiceNumber, "AV-") || inv.Status == "credit_note"
	typeCode := "380"
	if isCreditNote {
		typeCode = "381"
	}

	issued := inv.CreatedAt
	if issued.IsZero() {
		issued = time.Now().UTC()
	}

	amountHT := fmt.Sprintf("%.2f", inv.AmountHT)
	amountTTC := fmt.Sprintf("%.2f", inv.AmountTTC)
	duePayable := amountTTC
	if strings.EqualFold(strings.TrimSpace(inv.Status), "paid") {
		duePayable = "0.00"
	}

	productName := "Abonnement RelaisDesk " + strings.TrimSpace(inv.Plan)
	if inv.Technicians > 0 {
		label := "technicien"
		if inv.Technicians > 1 {
			label = "techniciens"
		}
		productName += fmt.Sprintf(" — %d %s", inv.Technicians, label)
	}

	tax := ciiTradeTax{
		TypeCode:        "VAT",
		ExemptionReason: IssuerVATExemption,
		CategoryCode:    "E",
		RateApplicable:  "0",
	}

	doc := ciiInvoice{
		XMLNSRSM: "urn:un:unece:uncefact:data:standard:CrossIndustryInvoice:100",
		XMLNSQDT: "urn:un:unece:uncefact:data:standard:QualifiedDataType:100",
		XMLNSRAM: "urn:un:unece:uncefact:data:standard:ReusableAggregateBusinessInformationEntity:100",
		XMLNSUDT: "urn:un:unece:uncefact:data:standard:UnqualifiedDataType:100",
		Context: ciiContext{
			Guideline: ciiGuideline{ID: ciiBasicProfile},
		},
		ExchangedDocument: ciiExchangedDocument{
			ID:       inv.InvoiceNumber,
			TypeCode: typeCode,
			IssueDateTime: ciiDateTime{
				Value: ciiDateValue{Format: "102", Date: issued.Format("20060102")},
			},
		},
		Transaction: ciiTransaction{
			LineItem: ciiLineItem{
				LineDoc:    ciiLineDoc{LineID: "1"},
				Product:    ciiProduct{Name: productName},
				Agreement:  ciiLineAgreement{NetPrice: ciiCharge{Amount: amountHT}},
				Delivery:   ciiLineDelivery{Quantity: ciiQuantity{UnitCode: "H87", Value: "1"}},
				Settlement: ciiLineSettlement{Tax: tax, Summation: ciiLineSummation{LineTotal: amountHT}},
			},
			Agreement: ciiAgreement{
				Seller: ciiParty{
					Name:     IssuerName,
					LegalOrg: &ciiLegalOrg{ID: ciiSchemeID{SchemeID: "0002", Value: IssuerSIRET}},
					Address: ciiAddress{
						Postcode: ciiPostcode("03170"),
						LineOne:  ciiLine(IssuerAddress),
						City:     ciiCity("Bizeneuille"),
						Country:  "FR",
					},
				},
				Buyer: ciiParty{
					Name:     strings.TrimSpace(inv.CustomerName),
					LegalOrg: ciiBuyerLegalOrg(strings.TrimSpace(inv.CustomerSIRET)),
					Address: ciiAddress{
						Postcode: ciiPostcode(strings.TrimSpace(inv.CustomerPostalCode)),
						LineOne:  ciiLine(strings.TrimSpace(inv.CustomerAddress)),
						City:     ciiCity(strings.TrimSpace(inv.CustomerCity)),
						Country:  ciiCountryCode(strings.TrimSpace(inv.CustomerCountry)),
					},
					Contact: &ciiContact{
						PersonName: strings.TrimSpace(inv.CustomerName),
						Email:      ciiEmail(strings.TrimSpace(inv.CustomerEmail)),
					},
				},
			},
			Settlement: ciiSettlement{
				CurrencyCode: "EUR",
				Tax:          tax,
				Summation: ciiSummation{
					LineTotal:  amountHT,
					TaxBasis:   amountHT,
					TaxTotal:   ciiMoney{CurrencyID: "EUR", Value: "0.00"},
					GrandTotal: amountTTC,
					DuePayable: duePayable,
				},
			},
		},
	}

	out, err := xml.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("génération XML CII: %w", err)
	}
	return append([]byte(xml.Header), out...), nil
}

func ciiPostcode(v string) *string { return ciiOpt(v) }
func ciiLine(v string) *string     { return ciiOpt(v) }
func ciiCity(v string) *string     { return ciiOpt(v) }

func ciiOpt(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

func ciiEmail(v string) *ciiEmailValue {
	if v == "" {
		return nil
	}
	return &ciiEmailValue{URIID: v}
}

func ciiBuyerLegalOrg(siret string) *ciiLegalOrg {
	if siret == "" {
		return nil
	}
	return &ciiLegalOrg{ID: ciiSchemeID{SchemeID: "0002", Value: siret}}
}

// ciiCountryCode maps a free-text country to an ISO 3166-1 alpha-2 code,
// defaulting to FR for the domestic customer base.
func ciiCountryCode(country string) string {
	upper := strings.ToUpper(strings.TrimSpace(country))
	switch {
	case upper == "", strings.HasPrefix(upper, "FR"):
		return "FR"
	case len(upper) == 2:
		return upper
	default:
		return "FR"
	}
}

type ciiInvoice struct {
	XMLName           xml.Name             `xml:"rsm:CrossIndustryInvoice"`
	XMLNSRSM          string               `xml:"xmlns:rsm,attr"`
	XMLNSQDT          string               `xml:"xmlns:qdt,attr"`
	XMLNSRAM          string               `xml:"xmlns:ram,attr"`
	XMLNSUDT          string               `xml:"xmlns:udt,attr"`
	Context           ciiContext           `xml:"rsm:ExchangedDocumentContext"`
	ExchangedDocument ciiExchangedDocument `xml:"rsm:ExchangedDocument"`
	Transaction       ciiTransaction       `xml:"rsm:SupplyChainTradeTransaction"`
}

type ciiContext struct {
	Guideline ciiGuideline `xml:"ram:GuidelineSpecifiedDocumentContextParameter"`
}

type ciiGuideline struct {
	ID string `xml:"ram:ID"`
}

type ciiExchangedDocument struct {
	ID            string      `xml:"ram:ID"`
	TypeCode      string      `xml:"ram:TypeCode"`
	IssueDateTime ciiDateTime `xml:"ram:IssueDateTime"`
}

type ciiDateTime struct {
	Value ciiDateValue `xml:"udt:DateTimeString"`
}

type ciiDateValue struct {
	Format string `xml:"format,attr"`
	Date   string `xml:",chardata"`
}

type ciiTransaction struct {
	LineItem   ciiLineItem   `xml:"ram:IncludedSupplyChainTradeLineItem"`
	Agreement  ciiAgreement  `xml:"ram:ApplicableHeaderTradeAgreement"`
	Settlement ciiSettlement `xml:"ram:ApplicableHeaderTradeSettlement"`
}

type ciiLineItem struct {
	LineDoc    ciiLineDoc        `xml:"ram:AssociatedDocumentLineDocument"`
	Product    ciiProduct        `xml:"ram:SpecifiedTradeProduct"`
	Agreement  ciiLineAgreement  `xml:"ram:SpecifiedLineTradeAgreement"`
	Delivery   ciiLineDelivery   `xml:"ram:SpecifiedLineTradeDelivery"`
	Settlement ciiLineSettlement `xml:"ram:SpecifiedLineTradeSettlement"`
}

type ciiLineDoc struct {
	LineID string `xml:"ram:LineID"`
}

type ciiProduct struct {
	Name string `xml:"ram:Name"`
}

type ciiLineAgreement struct {
	NetPrice ciiCharge `xml:"ram:NetPriceProductTradePrice"`
}

type ciiCharge struct {
	Amount string `xml:"ram:ChargeAmount"`
}

type ciiLineDelivery struct {
	Quantity ciiQuantity `xml:"ram:BilledQuantity"`
}

type ciiQuantity struct {
	UnitCode string `xml:"unitCode,attr"`
	Value    string `xml:",chardata"`
}

type ciiLineSettlement struct {
	Tax       ciiTradeTax      `xml:"ram:ApplicableTradeTax"`
	Summation ciiLineSummation `xml:"ram:SpecifiedTradeSettlementLineMonetarySummation"`
}

type ciiLineSummation struct {
	LineTotal string `xml:"ram:LineTotalAmount"`
}

type ciiTradeTax struct {
	TypeCode        string `xml:"ram:TypeCode"`
	ExemptionReason string `xml:"ram:ExemptionReason"`
	CategoryCode    string `xml:"ram:CategoryCode"`
	RateApplicable  string `xml:"ram:RateApplicablePercent"`
}

type ciiAgreement struct {
	Seller ciiParty `xml:"ram:SellerTradeParty"`
	Buyer  ciiParty `xml:"ram:BuyerTradeParty"`
}

type ciiParty struct {
	Name     string       `xml:"ram:Name"`
	LegalOrg *ciiLegalOrg `xml:"ram:SpecifiedLegalOrganization,omitempty"`
	Address  ciiAddress   `xml:"ram:PostalTradeAddress"`
	Contact  *ciiContact  `xml:"ram:DefinedTradeContact,omitempty"`
}

type ciiLegalOrg struct {
	ID ciiSchemeID `xml:"ram:ID"`
}

type ciiSchemeID struct {
	SchemeID string `xml:"schemeID,attr"`
	Value    string `xml:",chardata"`
}

type ciiAddress struct {
	Postcode *string `xml:"ram:PostcodeCode,omitempty"`
	LineOne  *string `xml:"ram:LineOne,omitempty"`
	City     *string `xml:"ram:CityName,omitempty"`
	Country  string  `xml:"ram:CountryID"`
}

type ciiContact struct {
	PersonName string         `xml:"ram:PersonName"`
	Email      *ciiEmailValue `xml:"ram:EmailURIUniversalCommunication,omitempty"`
}

type ciiEmailValue struct {
	URIID string `xml:"ram:URIID"`
}

type ciiSettlement struct {
	CurrencyCode string       `xml:"ram:InvoiceCurrencyCode"`
	Tax          ciiTradeTax  `xml:"ram:ApplicableTradeTax"`
	Summation    ciiSummation `xml:"ram:SpecifiedTradeSettlementHeaderMonetarySummation"`
}

type ciiSummation struct {
	LineTotal  string   `xml:"ram:LineTotalAmount"`
	TaxBasis   string   `xml:"ram:TaxBasisTotalAmount"`
	TaxTotal   ciiMoney `xml:"ram:TaxTotalAmount"`
	GrandTotal string   `xml:"ram:GrandTotalAmount"`
	DuePayable string   `xml:"ram:DuePayableAmount"`
}

type ciiMoney struct {
	CurrencyID string `xml:"currencyID,attr"`
	Value      string `xml:",chardata"`
}
