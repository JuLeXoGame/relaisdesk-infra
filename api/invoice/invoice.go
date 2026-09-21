package invoice

import (
	"bytes"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	dbpkg "database"
	"github.com/jung-kurt/gofpdf"
)

var invoiceNumberPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

const (
	IssuerName           = "Julien BELLOT — EI"
	IssuerLegalStatus    = "Informatique A Domicile 03 / RelaisDesk — Micro-entreprise"
	IssuerAddress        = "2 Chemin de Lonzais"
	IssuerCityPostalCode = "03170 Bizeneuille"
	IssuerCountry        = "France"
	IssuerSIRET          = "94074710800014"
	IssuerAPE            = "9511Z"
	IssuerVATExemption   = "TVA non applicable, art. 293 B du CGI"
	IssuerEmail          = "contact@relaisdesk.fr"
	IssuerPhone          = "06 62 85 59 30"
	IssuerWebsite        = "https://relaisdesk.fr"
)

type InvoiceTemplateData struct {
	IsCreditNote         bool
	DocumentTitle        string
	DescriptionTitle     string
	DescriptionText      string
	TotalTitle           string
	PaymentStatusText    string
	InvoiceNumber        string
	OrderID              string
	DateIssued           string
	PaymentDate          string
	PaymentMethod        string
	IssuerName           string
	IssuerLegalStatus    string
	IssuerAddress        string
	IssuerCityPostalCode string
	IssuerCountry        string
	IssuerSIRET          string
	IssuerAPE            string
	IssuerVATExemption   string
	IssuerEmail          string
	IssuerPhone          string
	IssuerWebsite        string
	CustomerName         string
	CustomerEmail        string
	CustomerAddress      string
	CustomerPostalCode   string
	CustomerCity         string
	CustomerCountry      string
	CustomerSIRET        string
	Plan                 string
	Technicians          int
	PeriodStart          string
	PeriodEnd            string
	AmountHT             string
	AmountTTC            string
}

// GenerateInvoicePDF generates an official A4 PDF document with complete legal micro-enterprise mentions,
// UTF-8 accents support, saves it securely to outDir/FAC-YYYY-NNNN.pdf (0600) and returns its path and raw bytes.
func GenerateInvoicePDF(outDir string, inv *dbpkg.Invoice, paymentMethod string) (string, []byte, error) {
	if inv == nil || !invoiceNumberPattern.MatchString(inv.InvoiceNumber) {
		return "", nil, fmt.Errorf("numéro de facture invalide")
	}
	if err := os.MkdirAll(outDir, 0700); err != nil {
		return "", nil, fmt.Errorf("création dossier factures: %w", err)
	}

	isCreditNote := strings.HasPrefix(inv.InvoiceNumber, "AV-") || inv.Status == "credit_note"

	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(15, 15, 15)
	pdf.SetAutoPageBreak(true, 15)
	pdf.AddPage()

	tr := pdf.UnicodeTranslatorFromDescriptor("")

	if paymentMethod == "" {
		if isCreditNote {
			paymentMethod = "Avoir / Crédit"
		} else {
			paymentMethod = "Carte Bancaire"
		}
	}

	// 1. Header (Logo/Title & Invoice Number)
	pdf.SetFont("Arial", "B", 18)
	pdf.SetTextColor(15, 23, 42) // Dark Slate
	pdf.CellFormat(100, 8, tr("RELAISDESK"), "", 0, "L", false, 0, "")

	if isCreditNote {
		pdf.SetFont("Arial", "B", 18)
		pdf.SetTextColor(220, 38, 38) // Crimson
		pdf.CellFormat(80, 8, tr("FACTURE D'AVOIR"), "", 1, "R", false, 0, "")
	} else {
		pdf.SetFont("Arial", "B", 20)
		pdf.SetTextColor(2, 132, 199) // Sky Blue
		pdf.CellFormat(80, 8, tr("FACTURE"), "", 1, "R", false, 0, "")
	}

	pdf.SetFont("Arial", "", 9)
	pdf.SetTextColor(100, 116, 139)
	pdf.CellFormat(100, 5, tr("Plateforme de Support & Télé-assistance Sécurisée"), "", 0, "L", false, 0, "")

	pdf.SetFont("Arial", "B", 10)
	pdf.SetTextColor(15, 23, 42)
	pdf.CellFormat(80, 5, tr(fmt.Sprintf("N° %s", inv.InvoiceNumber)), "", 1, "R", false, 0, "")

	pdf.Ln(4)
	// Horizontal separator
	if isCreditNote {
		pdf.SetDrawColor(220, 38, 38)
	} else {
		pdf.SetDrawColor(2, 132, 199)
	}
	pdf.SetLineWidth(0.8)
	pdf.Line(15, pdf.GetY(), 195, pdf.GetY())
	pdf.Ln(6)

	// 2. Issuer Block (Left) & Customer Block (Right)
	yTop := pdf.GetY()

	// Left: Émetteur (Micro-entreprise)
	pdf.SetXY(15, yTop)
	pdf.SetFont("Arial", "B", 10)
	pdf.SetTextColor(15, 23, 42)
	pdf.CellFormat(85, 5, tr("ÉMETTEUR :"), "", 1, "L", false, 0, "")
	pdf.SetFont("Arial", "B", 9)
	pdf.CellFormat(85, 4.5, tr(IssuerName), "", 1, "L", false, 0, "")
	pdf.SetFont("Arial", "", 8.5)
	pdf.SetTextColor(71, 85, 105)
	pdf.CellFormat(85, 4.2, tr(IssuerLegalStatus), "", 1, "L", false, 0, "")
	pdf.CellFormat(85, 4.2, tr(IssuerAddress), "", 1, "L", false, 0, "")
	pdf.CellFormat(85, 4.2, tr(IssuerCityPostalCode+", "+IssuerCountry), "", 1, "L", false, 0, "")
	pdf.CellFormat(85, 4.2, tr(fmt.Sprintf("SIRET : %s — Code APE : %s", IssuerSIRET, IssuerAPE)), "", 1, "L", false, 0, "")
	pdf.CellFormat(85, 4.2, tr("Email : "+IssuerEmail), "", 1, "L", false, 0, "")
	pdf.CellFormat(85, 4.2, tr("Téléphone : "+IssuerPhone), "", 1, "L", false, 0, "")
	pdf.CellFormat(85, 4.2, tr("Site : "+IssuerWebsite), "", 1, "L", false, 0, "")

	// Right: Client (Facturé à)
	pdf.SetXY(110, yTop)
	pdf.SetFont("Arial", "B", 10)
	pdf.SetTextColor(15, 23, 42)
	pdf.CellFormat(85, 5, tr("FACTURÉ À :"), "", 1, "L", false, 0, "")
	pdf.SetX(110)
	pdf.SetFont("Arial", "B", 9)
	custName := inv.CustomerName
	if custName == "" {
		custName = inv.CustomerEmail
	}
	pdf.CellFormat(85, 4.5, tr(custName), "", 1, "L", false, 0, "")
	pdf.SetFont("Arial", "", 8.5)
	pdf.SetTextColor(71, 85, 105)
	if inv.CustomerAddress != "" {
		pdf.SetX(110)
		pdf.CellFormat(85, 4.2, tr(inv.CustomerAddress), "", 1, "L", false, 0, "")
	}
	if inv.CustomerPostalCode != "" || inv.CustomerCity != "" {
		pdf.SetX(110)
		pdf.CellFormat(85, 4.2, tr(fmt.Sprintf("%s %s, %s", inv.CustomerPostalCode, inv.CustomerCity, inv.CustomerCountry)), "", 1, "L", false, 0, "")
	}
	pdf.SetX(110)
	pdf.CellFormat(85, 4.2, tr("Email : "+inv.CustomerEmail), "", 1, "L", false, 0, "")
	if inv.CustomerSIRET != "" {
		pdf.SetX(110)
		pdf.CellFormat(85, 4.2, tr("SIRET / N° TVA : "+inv.CustomerSIRET), "", 1, "L", false, 0, "")
	}

	pdf.SetY(yTop + 42)
	pdf.Ln(4)

	// 3. Meta info box (Date, Mode de règlement, Commande)
	pdf.SetFillColor(241, 245, 249)
	pdf.SetDrawColor(226, 232, 240)
	pdf.SetFont("Arial", "B", 8.5)
	pdf.SetTextColor(15, 23, 42)
	pdf.CellFormat(60, 7, tr(fmt.Sprintf("Date d'émission : %s", inv.CreatedAt.Format("02/01/2006"))), "1", 0, "L", true, 0, "")
	pdf.CellFormat(60, 7, tr(fmt.Sprintf("Règlement : %s", paymentMethod)), "1", 0, "L", true, 0, "")
	orderRef := inv.OrderID
	if orderRef == "" {
		orderRef = "—"
	}
	pdf.CellFormat(60, 7, tr(fmt.Sprintf("Réf. Commande : %s", orderRef)), "1", 1, "L", true, 0, "")

	pdf.Ln(6)

	// 4. Line Items Table
	if isCreditNote {
		pdf.SetFillColor(220, 38, 38)
	} else {
		pdf.SetFillColor(2, 132, 199)
	}
	pdf.SetTextColor(255, 255, 255)
	pdf.SetFont("Arial", "B", 9)
	tableHeader := "Désignation / Prestation"
	if isCreditNote {
		tableHeader = "Désignation / Motif de l'avoir"
	}
	pdf.CellFormat(105, 8, tr(tableHeader), "1", 0, "L", true, 0, "")
	pdf.CellFormat(20, 8, tr("Qté"), "1", 0, "C", true, 0, "")
	pdf.CellFormat(25, 8, tr("Prix Unit. HT"), "1", 0, "R", true, 0, "")
	pdf.CellFormat(30, 8, tr("Total Net HT"), "1", 1, "R", true, 0, "")

	pdf.SetTextColor(15, 23, 42)
	pdf.SetFont("Arial", "", 8.5)
	pdf.SetFillColor(255, 255, 255)
	periodLabel := "Mensuel"
	if inv.Notes != "" && strings.Contains(strings.ToLower(inv.Notes), "annual") {
		periodLabel = "Annuel"
	}
	lineDesc := fmt.Sprintf("Abonnement RelaisDesk (%s) — Plan %s (%d technicien(s) simultanés)", periodLabel, inv.Plan, inv.Technicians)
	if isCreditNote {
		if inv.Notes != "" {
			lineDesc = inv.Notes
		} else {
			lineDesc = fmt.Sprintf("Avoir sur abonnement RelaisDesk — Plan %s", inv.Plan)
		}
	}
	pdf.CellFormat(105, 10, tr(lineDesc), "1", 0, "L", false, 0, "")
	pdf.CellFormat(20, 10, "1", "1", 0, "C", false, 0, "")
	pdf.CellFormat(25, 10, tr(fmt.Sprintf("%.2f €", inv.AmountHT)), "1", 0, "R", false, 0, "")
	pdf.CellFormat(30, 10, tr(fmt.Sprintf("%.2f €", inv.AmountTTC)), "1", 1, "R", false, 0, "")

	pdf.Ln(4)

	// 5. Totals Block (Right aligned)
	totX := 115.0
	pdf.SetX(totX)
	pdf.SetFont("Arial", "", 9)
	totHTLabel := "Total HT :"
	if isCreditNote {
		totHTLabel = "Total HT Crédité :"
	}
	pdf.CellFormat(35, 6, tr(totHTLabel), "1", 0, "L", false, 0, "")
	pdf.CellFormat(30, 6, tr(fmt.Sprintf("%.2f €", inv.AmountHT)), "1", 1, "R", false, 0, "")

	pdf.SetX(totX)
	pdf.CellFormat(35, 6, tr("TVA (0%) :"), "1", 0, "L", false, 0, "")
	pdf.CellFormat(30, 6, "0.00 €", "1", 1, "R", false, 0, "")

	pdf.SetX(totX)
	pdf.SetFont("Arial", "B", 10)
	pdf.SetFillColor(241, 245, 249)
	totFinalLabel := "Total Net à Payer :"
	if isCreditNote {
		totFinalLabel = "Net Crédité Client :"
	}
	pdf.CellFormat(35, 8, tr(totFinalLabel), "1", 0, "L", true, 0, "")
	pdf.CellFormat(30, 8, tr(fmt.Sprintf("%.2f €", inv.AmountTTC)), "1", 1, "R", true, 0, "")

	pdf.Ln(8)

	// 6. Highlight Box: Mentions légales obligatoires Micro-entreprise
	pdf.SetFillColor(248, 250, 252)
	pdf.SetDrawColor(203, 213, 225)
	pdf.SetFont("Arial", "B", 9)
	pdf.SetTextColor(15, 23, 42)
	pdf.CellFormat(180, 7, tr(fmt.Sprintf("  • %s", IssuerVATExemption)), "LTR", 1, "L", true, 0, "")
	pdf.SetFont("Arial", "", 8.5)
	pdf.SetTextColor(71, 85, 105)
	if isCreditNote {
		pdf.CellFormat(180, 6, tr(fmt.Sprintf("  • Émission de l'avoir : %s — Montant crédité / remboursé au client", inv.CreatedAt.Format("02/01/2006"))), "LR", 1, "L", true, 0, "")
	} else {
		pdf.CellFormat(180, 6, tr(fmt.Sprintf("  • Échéance et acquittement : %s — Paiement validé (%s)", inv.CreatedAt.Format("02/01/2006"), paymentMethod)), "LR", 1, "L", true, 0, "")
	}
	pdf.CellFormat(180, 6, tr("  • Escompte : néant. Retard professionnel : 3 fois le taux légal + indemnité forfaitaire de 40 €."), "LBR", 1, "L", true, 0, "")

	// 7. Save to disk securely (0600)
	outPath := filepath.Join(outDir, inv.InvoiceNumber+".pdf")
	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return "", nil, fmt.Errorf("génération PDF: %w", err)
	}

	pdfBytes := buf.Bytes()
	if err := os.WriteFile(outPath, pdfBytes, 0600); err != nil {
		return "", nil, fmt.Errorf("écriture fichier PDF: %w", err)
	}

	// Also generate HTML view alongside
	_, _ = GenerateInvoiceDocument(outDir, inv, paymentMethod)

	return outPath, pdfBytes, nil
}

// GenerateInvoiceDocument creates a printable, beautiful HTML invoice and saves it to outDir.
func GenerateInvoiceDocument(outDir string, inv *dbpkg.Invoice, paymentMethod string) (string, error) {
	if inv == nil || !invoiceNumberPattern.MatchString(inv.InvoiceNumber) {
		return "", fmt.Errorf("numéro de facture invalide")
	}
	if err := os.MkdirAll(outDir, 0700); err != nil {
		return "", fmt.Errorf("création dossier factures: %w", err)
	}

	custName := inv.CustomerName
	if custName == "" {
		custName = inv.CustomerEmail
	}

	isCreditNote := strings.HasPrefix(inv.InvoiceNumber, "AV-") || inv.Status == "credit_note"
	docTitle := "FACTURE"
	descTitle := "Désignation de la prestation"
	descText := fmt.Sprintf("Abonnement RelaisDesk — Plan %s", inv.Plan)
	totalTitle := "Total Net à Payer :"
	paymentStatus := fmt.Sprintf("✅ Facture acquittée : Règlement de %.2f € reçu le %s via %s.", inv.AmountTTC, inv.CreatedAt.Format("02/01/2006"), paymentMethod)

	if isCreditNote {
		docTitle = "FACTURE D'AVOIR"
		descTitle = "Désignation / Motif de l'avoir"
		if inv.Notes != "" {
			descText = inv.Notes
		} else {
			descText = fmt.Sprintf("Avoir sur abonnement RelaisDesk — Plan %s", inv.Plan)
		}
		totalTitle = "Net Crédité au Client :"
		paymentStatus = fmt.Sprintf("↩️ Avoir émis le %s : Montant de %.2f € crédité / remboursé au client.", inv.CreatedAt.Format("02/01/2006"), inv.AmountTTC)
	}

	data := InvoiceTemplateData{
		IsCreditNote:         isCreditNote,
		DocumentTitle:        docTitle,
		DescriptionTitle:     descTitle,
		DescriptionText:      descText,
		TotalTitle:           totalTitle,
		PaymentStatusText:    paymentStatus,
		InvoiceNumber:        inv.InvoiceNumber,
		OrderID:              inv.OrderID,
		DateIssued:           inv.CreatedAt.Format("02/01/2006"),
		PaymentDate:          inv.CreatedAt.Format("02/01/2006"),
		PaymentMethod:        paymentMethod,
		IssuerName:           IssuerName,
		IssuerLegalStatus:    IssuerLegalStatus,
		IssuerAddress:        IssuerAddress,
		IssuerCityPostalCode: IssuerCityPostalCode,
		IssuerCountry:        IssuerCountry,
		IssuerSIRET:          IssuerSIRET,
		IssuerAPE:            IssuerAPE,
		IssuerVATExemption:   IssuerVATExemption,
		IssuerEmail:          IssuerEmail,
		IssuerPhone:          IssuerPhone,
		IssuerWebsite:        IssuerWebsite,
		CustomerName:         custName,
		CustomerEmail:        inv.CustomerEmail,
		CustomerAddress:      inv.CustomerAddress,
		CustomerPostalCode:   inv.CustomerPostalCode,
		CustomerCity:         inv.CustomerCity,
		CustomerCountry:      inv.CustomerCountry,
		CustomerSIRET:        inv.CustomerSIRET,
		Plan:                 inv.Plan,
		Technicians:          inv.Technicians,
		PeriodStart:          inv.CreatedAt.Format("02/01/2006"),
		PeriodEnd:            inv.CreatedAt.AddDate(0, 1, 0).Format("02/01/2006"),
		AmountHT:             fmt.Sprintf("%.2f", inv.AmountHT),
		AmountTTC:            fmt.Sprintf("%.2f", inv.AmountTTC),
	}

	tmpl, err := template.New("invoice").Parse(invoiceHTMLTemplate)
	if err != nil {
		return "", fmt.Errorf("parse template facture: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("exécution template facture: %w", err)
	}

	fileName := fmt.Sprintf("%s.html", inv.InvoiceNumber)
	filePath := filepath.Join(outDir, fileName)

	if err := os.WriteFile(filePath, buf.Bytes(), 0600); err != nil {
		return "", fmt.Errorf("écriture fichier facture: %w", err)
	}

	return filePath, nil
}

const invoiceHTMLTemplate = `<!DOCTYPE html>
<html lang="fr">
<head>
  <meta charset="UTF-8">
  <title>Facture {{.InvoiceNumber}} — RelaisDesk</title>
  <style>
    @page { size: A4; margin: 18mm; }
    body {
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
      color: #1e293b;
      background: #ffffff;
      margin: 0;
      padding: 24px;
      font-size: 13px;
      line-height: 1.5;
    }
    .invoice-card {
      max-width: 800px;
      margin: 0 auto;
      background: #ffffff;
    }
    .header-row {
      display: flex;
      justify-content: space-between;
      align-items: flex-start;
      border-bottom: 2px solid #3b82f6;
      padding-bottom: 16px;
      margin-bottom: 24px;
    }
    .logo-box h1 {
      margin: 0;
      font-size: 26px;
      font-weight: 800;
      color: #0f172a;
      letter-spacing: -0.5px;
    }
    .logo-box h1 span { color: #3b82f6; }
    .logo-box p { margin: 2px 0 0 0; color: #64748b; font-size: 12px; }
    .invoice-title-box {
      text-align: right;
    }
    .invoice-title-box h2 {
      margin: 0;
      font-size: 24px;
      color: #3b82f6;
      text-transform: uppercase;
      letter-spacing: 1px;
    }
    .invoice-title-box .invoice-number {
      font-size: 14px;
      font-weight: 700;
      color: #0f172a;
      margin-top: 4px;
    }
    .grid-2 {
      display: grid;
      grid-template-columns: 1fr 1fr;
      gap: 32px;
      margin-bottom: 24px;
    }
    .block-title {
      font-size: 11px;
      font-weight: 700;
      text-transform: uppercase;
      color: #64748b;
      letter-spacing: 0.5px;
      margin-bottom: 8px;
      border-bottom: 1px solid #e2e8f0;
      padding-bottom: 4px;
    }
    .info-block p {
      margin: 0 0 3px 0;
    }
    .meta-box {
      background: #f8fafc;
      border: 1px solid #e2e8f0;
      border-radius: 6px;
      padding: 12px 16px;
      margin-bottom: 24px;
      display: grid;
      grid-template-columns: repeat(4, 1fr);
      gap: 12px;
    }
    .meta-item .meta-label {
      font-size: 11px;
      color: #64748b;
      display: block;
    }
    .meta-item .meta-value {
      font-weight: 600;
      color: #0f172a;
    }
    table.invoice-table {
      width: 100%;
      border-collapse: collapse;
      margin-bottom: 24px;
    }
    table.invoice-table th {
      background: #0f172a;
      color: #ffffff;
      font-weight: 600;
      text-align: left;
      padding: 10px 12px;
      font-size: 12px;
    }
    table.invoice-table th.text-right, table.invoice-table td.text-right {
      text-align: right;
    }
    table.invoice-table td {
      padding: 12px;
      border-bottom: 1px solid #e2e8f0;
    }
    .totals-area {
      display: flex;
      justify-content: flex-end;
      margin-bottom: 28px;
    }
    .totals-box {
      width: 280px;
    }
    .totals-row {
      display: flex;
      justify-content: space-between;
      padding: 6px 0;
      font-size: 13px;
    }
    .totals-row.total-final {
      border-top: 2px solid #0f172a;
      margin-top: 6px;
      padding-top: 10px;
      font-size: 16px;
      font-weight: 800;
      color: #0f172a;
    }
    .vat-exemption-badge {
      background: #f1f5f9;
      border-left: 4px solid #3b82f6;
      padding: 12px 16px;
      font-size: 12px;
      color: #334155;
      margin-bottom: 24px;
      border-radius: 0 4px 4px 0;
    }
    .footer-mentions {
      border-top: 1px solid #e2e8f0;
      padding-top: 16px;
      font-size: 11px;
      color: #94a3b8;
      text-align: center;
      line-height: 1.6;
    }
    .print-btn-bar {
      margin-bottom: 20px;
      text-align: right;
    }
    .print-btn {
      background: #3b82f6;
      color: white;
      border: none;
      padding: 8px 16px;
      border-radius: 6px;
      cursor: pointer;
      font-size: 13px;
      font-weight: 600;
    }
    @media print {
      .print-btn-bar { display: none; }
      body { padding: 0; }
    }
  </style>
</head>
<body>
  <div class="invoice-card">
    <div class="print-btn-bar">
      <button class="print-btn" onclick="window.print()">🖨️ Imprimer / Télécharger en PDF</button>
    </div>

    <!-- Header -->
    <div class="header-row">
      <div class="logo-box">
        <h1>RELAIS<span>DESK</span></h1>
        <p>Plateforme de Support & Prise en Main Sécurisée</p>
      </div>
      <div class="invoice-title-box">
        <h2{{if .IsCreditNote}} style="color: #dc2626;"{{end}}>{{.DocumentTitle}}</h2>
        <div class="invoice-number">N° {{.InvoiceNumber}}</div>
      </div>
    </div>

    <!-- Metadata Row -->
    <div class="meta-box">
      <div class="meta-item">
        <span class="meta-label">Date d'émission :</span>
        <span class="meta-value">{{.DateIssued}}</span>
      </div>
      <div class="meta-item">
        <span class="meta-label">Date de règlement :</span>
        <span class="meta-value">{{.PaymentDate}}</span>
      </div>
      <div class="meta-item">
        <span class="meta-label">Mode de règlement :</span>
        <span class="meta-value">{{.PaymentMethod}}</span>
      </div>
      <div class="meta-item">
        <span class="meta-label">Réf. Commande :</span>
        <span class="meta-value">{{if .OrderID}}{{.OrderID}}{{else}}—{{end}}</span>
      </div>
    </div>

    <!-- Addresses Grid -->
    <div class="grid-2">
      <!-- Émetteur -->
      <div class="info-block">
        <div class="block-title">ÉMETTEUR (PRESTATAIRE)</div>
        <p><strong>{{.IssuerName}}</strong></p>
        <p>{{.IssuerLegalStatus}}</p>
        <p>{{.IssuerAddress}}</p>
        <p>{{.IssuerCityPostalCode}}, {{.IssuerCountry}}</p>
        <p><strong>SIRET :</strong> {{.IssuerSIRET}} — <strong>Code APE :</strong> {{.IssuerAPE}}</p>
        <p><strong>Téléphone :</strong> {{.IssuerPhone}}</p>
        <p><strong>Email :</strong> {{.IssuerEmail}} | <strong>Web :</strong> {{.IssuerWebsite}}</p>
      </div>

      <!-- Client -->
      <div class="info-block">
        <div class="block-title">FACTURÉ À (CLIENT)</div>
        <p><strong>{{.CustomerName}}</strong></p>
        {{if .CustomerAddress}}<p>{{.CustomerAddress}}</p>{{end}}
        {{if .CustomerPostalCode}}<p>{{.CustomerPostalCode}} {{.CustomerCity}}, {{.CustomerCountry}}</p>{{end}}
        <p><strong>Email :</strong> {{.CustomerEmail}}</p>
        {{if .CustomerSIRET}}<p><strong>SIRET / N° TVA :</strong> {{.CustomerSIRET}}</p>{{end}}
      </div>
    </div>

    <!-- Table of Lines -->
    <table class="invoice-table">
      <thead>
        <tr>
          <th>{{.DescriptionTitle}}</th>
          <th style="width: 80px; text-align: center;">Qté</th>
          <th style="width: 120px;" class="text-right">Prix Unit. HT</th>
          <th style="width: 120px;" class="text-right">Total Net HT</th>
        </tr>
      </thead>
      <tbody>
        <tr>
          <td>
            <strong>{{.DescriptionText}}</strong><br>
            <span style="font-size: 11px; color: #64748b;">
              {{if .IsCreditNote}}
                Avoir émis en régularisation comptable.<br>
              {{else}}
                Accès logiciel technicien ({{.Technicians}} technicien(s) simultanés) et relais distant sécurisé.<br>
                Période de souscription : du {{.PeriodStart}} au {{.PeriodEnd}}.
              {{end}}
            </span>
          </td>
          <td style="text-align: center;">1</td>
          <td class="text-right">{{.AmountHT}} €</td>
          <td class="text-right">{{.AmountHT}} €</td>
        </tr>
      </tbody>
    </table>

    <!-- Totals Area -->
    <div class="totals-area">
      <div class="totals-box">
        <div class="totals-row">
          <span>{{if .IsCreditNote}}Total HT Crédité :{{else}}Total HT :{{end}}</span>
          <span>{{.AmountHT}} €</span>
        </div>
        <div class="totals-row">
          <span>Taux TVA :</span>
          <span>0,00 %</span>
        </div>
        <div class="totals-row">
          <span>Montant TVA :</span>
          <span>0,00 €</span>
        </div>
        <div class="totals-row total-final">
          <span>{{.TotalTitle}}</span>
          <span>{{.AmountTTC}} €</span>
        </div>
      </div>
    </div>

    <!-- Legal Exemption Badge -->
    <div class="vat-exemption-badge">
      <strong>Mention Légale d'Exonération de TVA :</strong><br>
      « {{.IssuerVATExemption}} »
    </div>

    <!-- Payment Status Badge -->
    <div style="margin-bottom: 24px; padding: 10px 14px; background: {{if .IsCreditNote}}#fef2f2; border: 1px solid #fecaca; color: #991b1b;{{else}}#ecfdf5; border: 1px solid #a7f3d0; color: #065f46;{{end}} border-radius: 6px; font-size: 12px;">
      {{.PaymentStatusText}}
    </div>

    <div style="margin-bottom: 18px; font-size: 11px; color: #475569;">
      {{if not .IsCreditNote}}
      Date d'échéance : {{.PaymentDate}} — Escompte pour paiement anticipé : néant.<br>
      Clients professionnels : en cas de retard sur un délai exceptionnellement accordé, pénalités au taux de trois fois le taux d'intérêt légal et indemnité forfaitaire de 40 € pour frais de recouvrement.
      {{end}}
    </div>

    <!-- Footer -->
    <div class="footer-mentions">
      {{.IssuerName}} — {{.IssuerLegalStatus}} — N° SIRET {{.IssuerSIRET}} — Code APE {{.IssuerAPE}}<br>
      Siège social : {{.IssuerAddress}}, {{.IssuerCityPostalCode}}, {{.IssuerCountry}} — Contact : {{.IssuerPhone}} — {{.IssuerEmail}}<br>
      Document généré conformément aux dispositions du Code de Commerce et du Code Général des Impôts (Art. 293 B).
    </div>
  </div>
</body>
</html>
`
