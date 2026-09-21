package handlers

import (
	dbpkg "database"
	"database/sql"
	"fmt"
	"os"
	"strings"

	"api/config"
	"api/invoice"
	"api/mailer"
)

func ensureOrderInvoice(db *sql.DB, cfg *config.Config, order *dbpkg.Order, paymentLabel, notes string) (*dbpkg.Invoice, []byte, error) {
	if order == nil || cfg == nil {
		return nil, nil, fmt.Errorf("commande ou configuration manquante")
	}
	name := strings.TrimSpace(order.BillingName)
	if name == "" {
		name = order.Email
	}
	created, err := dbpkg.CreateInvoice(db, &dbpkg.Invoice{
		OrderID:            order.OrderID,
		CustomerEmail:      order.Email,
		CustomerName:       name,
		CustomerAddress:    order.BillingAddress,
		CustomerPostalCode: order.BillingPostalCode,
		CustomerCity:       order.BillingCity,
		CustomerCountry:    order.BillingCountry,
		CustomerSIRET:      order.BillingSIRET,
		Plan:               order.Plan,
		Technicians:        order.Technicians,
		AmountHT:           order.Price,
		AmountTVA:          0,
		AmountTTC:          order.Price,
		Status:             "paid",
		Notes:              notes,
	})
	if err != nil {
		return nil, nil, err
	}

	var pdfBytes []byte
	if created.PDFPath != "" && isAllowedInvoicePath(cfg, created.PDFPath) {
		pdfBytes, _ = os.ReadFile(created.PDFPath)
	}
	if len(pdfBytes) == 0 {
		pdfPath, generated, generateErr := invoice.GenerateInvoicePDF(cfg.InvoicesDir, created, paymentLabel)
		if generateErr != nil {
			return nil, nil, generateErr
		}
		created.PDFPath = pdfPath
		pdfBytes = generated
		if _, err := db.Exec(`UPDATE invoices SET pdf_path = ? WHERE invoice_number = ?`, pdfPath, created.InvoiceNumber); err != nil {
			return nil, nil, err
		}
	}
	return created, pdfBytes, nil
}

func deliverPendingOrderEmails(db *sql.DB, mail *mailer.Mailer, order *dbpkg.Order, lic *dbpkg.License, inv *dbpkg.Invoice, pdfBytes []byte) error {
	if mail == nil {
		return nil
	}
	status, err := dbpkg.GetOrderEmailStatus(db, order.OrderID)
	if err != nil {
		return err
	}
	if !status.LicenseSent {
		var sendErr error
		if order.OrderKind == "renewal" {
			sendErr = mail.SendRenewalConfirmation(order.Email, order.Plan, lic.LicenseID, lic.ExpiresAt.Format("2006-01-02"), order.Technicians, order.TermsVersion)
		} else {
			sendErr = mail.SendLicenseEmail(order.Email, order.Plan, lic.LicenseID, lic.LicenseKey, lic.ExpiresAt.Format("2006-01-02"), order.Technicians, order.TermsVersion)
		}
		if sendErr != nil {
			return fmt.Errorf("envoi de la confirmation de licence: %w", sendErr)
		}
		if err := dbpkg.MarkOrderEmailSent(db, order.OrderID, "license"); err != nil {
			return err
		}
	}
	if !status.InvoiceSent {
		if inv == nil || len(pdfBytes) == 0 {
			return fmt.Errorf("facture PDF indisponible")
		}
		if err := mail.SendInvoiceEmailWithPDF(order.Email, inv.InvoiceNumber, order.Plan, order.Price, pdfBytes); err != nil {
			return fmt.Errorf("envoi de la facture: %w", err)
		}
		if err := dbpkg.MarkOrderEmailSent(db, order.OrderID, "invoice"); err != nil {
			return err
		}
	}
	return nil
}
