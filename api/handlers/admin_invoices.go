package handlers

import (
	"bytes"
	dbpkg "database"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	netmail "net/mail"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"api/config"
	"api/invoice"
	"api/mailer"
	"api/middleware"
)

var invoiceNumberPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// AdminListInvoicesHandler lists invoices with optional search query.
func AdminListInvoicesHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("q")
		if query == "" {
			query = r.URL.Query().Get("email")
		}
		if len(query) > 200 {
			writeJSONError(w, "Filtre de recherche trop long", http.StatusBadRequest)
			return
		}

		invoices, err := dbpkg.ListInvoices(db, query)
		if err != nil {
			writeJSONError(w, err.Error(), http.StatusInternalServerError)
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"invoices": invoices,
			"total":    len(invoices),
		})
	}
}

// AdminDownloadInvoiceHandler serves an invoice document and logs audit trail.
func AdminDownloadInvoiceHandler(db *sql.DB, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		pathParts := strings.Split(r.URL.Path, "/")
		// /api/v1/admin/invoices/{invoice_number}/download
		if len(pathParts) < 6 {
			writeJSONError(w, "Chemin invalide", http.StatusBadRequest)
			return
		}

		invoiceNumber := pathParts[5]
		inv, err := dbpkg.GetInvoiceByNumber(db, invoiceNumber)
		if err != nil {
			writeJSONError(w, "Facture introuvable", http.StatusNotFound)
			return
		}

		// Security & Audit Logging
		clientIP := middleware.GetClientIP(r)
		log.Printf("[AUDIT INVOICE DOWNLOAD] Facture %s téléchargée depuis IP %s à %s", invoiceNumber, clientIP, time.Now().UTC().Format(time.RFC3339))

		filePath := inv.PDFPath
		if filePath == "" || !isAllowedInvoicePath(cfg, filePath) || !fileExists(filePath) {
			// Check candidates in invoices dir
			candidates := []string{
				filepath.Join(cfg.InvoicesDir, invoiceNumber+".pdf"),
				filepath.Join(cfg.InvoicesDir, invoiceNumber+".html"),
				filepath.Join("data", "invoices", invoiceNumber+".pdf"),
				filepath.Join("data", "invoices", invoiceNumber+".html"),
			}
			found := false
			for _, c := range candidates {
				if fileExists(c) {
					filePath = c
					found = true
					break
				}
			}
			if !found {
				// Re-generate on the fly if needed
				pdfPath, _, genErr := invoice.GenerateInvoicePDF(cfg.InvoicesDir, inv, "Téléchargement Admin")
				if genErr == nil {
					filePath = pdfPath
					found = true
				} else {
					writeJSONError(w, "Fichier de facture introuvable sur le serveur", http.StatusNotFound)
					return
				}
			}
		}

		ext := strings.ToLower(filepath.Ext(filePath))
		contentType := "text/html; charset=utf-8"
		if ext == ".pdf" {
			contentType = "application/pdf"
		}
		if inv.IsManual {
			w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'; style-src 'unsafe-inline'")
			w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filepath.Base(filePath)))
		}

		w.Header().Set("Content-Type", contentType)
		if w.Header().Get("Content-Disposition") == "" {
			w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", filepath.Base(filePath)))
		}
		http.ServeFile(w, r, filePath)
	}
}

// AdminUploadInvoiceHandler allows admin to upload an existing invoice PDF/HTML.
func AdminUploadInvoiceHandler(db *sql.DB, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Limit upload size to 10MB
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			writeJSONError(w, "Formulaire trop volumineux ou invalide", http.StatusBadRequest)
			return
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			writeJSONError(w, "Fichier manquant (champ 'file')", http.StatusBadRequest)
			return
		}
		defer file.Close()

		customerName := strings.TrimSpace(r.FormValue("customer_name"))
		customerEmail := strings.TrimSpace(r.FormValue("customer_email"))
		parsedEmail, emailErr := netmail.ParseAddress(customerEmail)
		if emailErr != nil || parsedEmail.Address != customerEmail || len(customerEmail) > 254 || strings.ContainsAny(customerEmail, "\r\n\t") {
			writeJSONError(w, "L'adresse email client est obligatoire", http.StatusBadRequest)
			return
		}
		if len(customerName) == 0 || len(customerName) > 200 {
			writeJSONError(w, "Nom client invalide", http.StatusBadRequest)
			return
		}

		amountStr := strings.TrimSpace(r.FormValue("amount"))
		amount, err := strconv.ParseFloat(amountStr, 64)
		if err != nil || amount < 0 || amount > 1000000 || math.IsNaN(amount) || math.IsInf(amount, 0) {
			writeJSONError(w, "Montant invalide", http.StatusBadRequest)
			return
		}

		plan := strings.TrimSpace(r.FormValue("plan"))
		if plan == "" {
			plan = "Pro"
		}

		orderID := strings.TrimSpace(r.FormValue("order_id"))
		customerAddress := strings.TrimSpace(r.FormValue("customer_address"))
		customerCity := strings.TrimSpace(r.FormValue("customer_city"))
		customerPostalCode := strings.TrimSpace(r.FormValue("customer_postal_code"))
		customerSiret := strings.TrimSpace(r.FormValue("customer_siret"))
		notes := strings.TrimSpace(r.FormValue("notes"))
		invoiceNumber := strings.TrimSpace(r.FormValue("invoice_number"))
		if len(plan) > 100 || len(orderID) > 64 || len(customerAddress) > 500 || len(customerCity) > 100 ||
			len(customerPostalCode) > 32 || len(customerSiret) > 32 || len(notes) > 2000 {
			writeJSONError(w, "Un ou plusieurs champs sont trop longs", http.StatusBadRequest)
			return
		}
		if orderID != "" && !invoiceNumberPattern.MatchString(orderID) {
			writeJSONError(w, "Référence de commande invalide", http.StatusBadRequest)
			return
		}

		// Ensure invoices directory exists (0700)
		_ = os.MkdirAll(cfg.InvoicesDir, 0700)

		// Generate invoice number if not provided
		if invoiceNumber == "" {
			invoiceNumber, err = dbpkg.GenerateNextInvoiceNumber(db)
			if err != nil {
				writeJSONError(w, "Erreur génération numéro facture: "+err.Error(), http.StatusInternalServerError)
				return
			}
		}
		if !invoiceNumberPattern.MatchString(invoiceNumber) {
			writeJSONError(w, "Numéro de facture invalide", http.StatusBadRequest)
			return
		}

		ext := strings.ToLower(filepath.Ext(header.Filename))
		if ext != ".pdf" && ext != ".html" && ext != ".htm" {
			writeJSONError(w, "Seuls les fichiers PDF ou HTML sont acceptés", http.StatusBadRequest)
			return
		}

		// Do not trust the client-provided filename alone. Manual HTML invoices are
		// always downloaded under a restrictive CSP; PDFs must carry the PDF magic.
		headerBytes := make([]byte, 4096)
		n, readErr := file.Read(headerBytes)
		if readErr != nil && readErr != io.EOF {
			writeJSONError(w, "Fichier illisible", http.StatusBadRequest)
			return
		}
		headerBytes = headerBytes[:n]
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			writeJSONError(w, "Fichier illisible", http.StatusBadRequest)
			return
		}
		if ext == ".pdf" && !bytes.HasPrefix(headerBytes, []byte("%PDF-")) {
			writeJSONError(w, "Le contenu du fichier n'est pas un PDF valide", http.StatusBadRequest)
			return
		}
		if ext != ".pdf" && (!utf8.Valid(headerBytes) || bytes.IndexByte(headerBytes, 0) >= 0) {
			writeJSONError(w, "Le contenu HTML est invalide", http.StatusBadRequest)
			return
		}

		destFilename := fmt.Sprintf("%s%s", sanitizeFilename(invoiceNumber), ext)
		destPath := filepath.Join(cfg.InvoicesDir, destFilename)

		destFile, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			writeJSONError(w, "Une facture portant ce numéro existe déjà", http.StatusConflict)
			return
		}
		keepFile := false
		defer func() {
			_ = destFile.Close()
			if !keepFile {
				_ = os.Remove(destPath)
			}
		}()

		written, copyErr := io.Copy(destFile, io.LimitReader(file, (10<<20)+1))
		if copyErr != nil || written > 10<<20 {
			writeJSONError(w, "Fichier trop volumineux ou illisible", http.StatusBadRequest)
			return
		}
		if err := destFile.Close(); err != nil {
			writeJSONError(w, "Erreur finalisation du fichier", http.StatusInternalServerError)
			return
		}

		techs := 10
		if strings.EqualFold(plan, "Starter") {
			techs = 1
		}

		inv := &dbpkg.Invoice{
			InvoiceNumber:      invoiceNumber,
			OrderID:            orderID,
			CustomerEmail:      customerEmail,
			CustomerName:       customerName,
			CustomerAddress:    customerAddress,
			CustomerPostalCode: customerPostalCode,
			CustomerCity:       customerCity,
			CustomerCountry:    "France",
			CustomerSIRET:      customerSiret,
			Plan:               plan,
			Technicians:        techs,
			AmountHT:           amount,
			AmountTVA:          0.0,
			AmountTTC:          amount,
			Status:             "paid",
			PDFPath:            destPath,
			IsManual:           true,
			CreatedAt:          time.Now().UTC(),
			Notes:              notes,
		}

		createdInv, err := dbpkg.CreateInvoice(db, inv)
		if err != nil {
			writeJSONError(w, "Erreur enregistrement facture: "+err.Error(), http.StatusInternalServerError)
			return
		}
		keepFile = true

		writeJSON(w, http.StatusCreated, map[string]interface{}{
			"status":         "created",
			"invoice_number": createdInv.InvoiceNumber,
			"invoice":        createdInv,
		})
	}
}

// AdminSendInvoiceEmailHandler resends or sends an invoice with PDF attachment by email to customer.
func AdminSendInvoiceEmailHandler(db *sql.DB, _ *config.Config, mail *mailer.Mailer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if mail == nil || !mail.IsConfigured() {
			writeJSONError(w, "Service SMTP non configuré", http.StatusServiceUnavailable)
			return
		}
		pathParts := strings.Split(r.URL.Path, "/")
		// /api/v1/admin/invoices/{invoice_number}/send-email
		if len(pathParts) < 6 {
			writeJSONError(w, "Chemin invalide", http.StatusBadRequest)
			return
		}

		invoiceNumber := pathParts[5]
		inv, err := dbpkg.GetInvoiceByNumber(db, invoiceNumber)
		if err != nil {
			writeJSONError(w, "Facture introuvable", http.StatusNotFound)
			return
		}

		if inv.CustomerEmail == "" {
			writeJSONError(w, "Aucune adresse email associée à cette facture", http.StatusBadRequest)
			return
		}

		if err := EnqueueManualInvoiceEmail(db, inv.InvoiceNumber); err != nil {
			writeJSONError(w, "Impossible de mettre l'envoi en file", http.StatusInternalServerError)
			return
		}

		writeJSON(w, http.StatusAccepted, map[string]interface{}{
			"status":         "queued",
			"invoice_number": inv.InvoiceNumber,
			"email":          inv.CustomerEmail,
		})
	}
}

// AdminTestEmailHandler sends a test email to verify SMTP credentials and network connectivity.
func AdminTestEmailHandler(mail *mailer.Mailer, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			To string `json:"to"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)

		target := strings.TrimSpace(req.To)
		if target == "" {
			target = cfg.SMTPFrom
		}

		if mail == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
				"valid": false,
				"error": "Module Mailer non initialisé",
			})
			return
		}

		if err := mail.SendTestEmail(target); err != nil {
			log.Printf("[Admin Test Email] Échec de l'envoi vers %s: %v", target, err)
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
				"valid": false,
				"error": err.Error(),
				"to":    target,
				"host":  cfg.SMTPHost,
				"port":  cfg.SMTPPort,
			})
			return
		}

		log.Printf("[Admin Test Email] Test d'email réussi vers %s", target)
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"valid":   true,
			"message": fmt.Sprintf("Email de test envoyé avec succès à %s", target),
			"to":      target,
			"host":    cfg.SMTPHost,
			"port":    cfg.SMTPPort,
		})
	}
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func isAllowedInvoicePath(cfg *config.Config, path string) bool {
	for _, root := range []string{cfg.InvoicesDir, filepath.Join("data", "invoices")} {
		rootAbs, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		pathAbs, err := filepath.Abs(path)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(rootAbs, pathAbs)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && !filepath.IsAbs(rel) {
			return true
		}
	}
	return false
}

func sanitizeFilename(name string) string {
	clean := strings.ReplaceAll(name, "/", "-")
	clean = strings.ReplaceAll(clean, "\\", "-")
	clean = strings.ReplaceAll(clean, "..", "")
	clean = strings.ReplaceAll(clean, " ", "_")
	return clean
}

// AdminDeleteInvoiceHandler permanently deletes an invoice and its PDF document.
func AdminDeleteInvoiceHandler(db *sql.DB, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		invoiceNumber := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/api/v1/admin/invoices/"))
		if invoiceNumber == "" || strings.Contains(invoiceNumber, "/") {
			writeJSONError(w, "Numéro de facture invalide", http.StatusBadRequest)
			return
		}

		pdfPath, err := dbpkg.DeleteInvoice(db, invoiceNumber)
		if err != nil {
			if strings.Contains(err.Error(), "introuvable") {
				writeJSONError(w, "Facture introuvable", http.StatusNotFound)
				return
			}
			writeJSONError(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Cleanup files on disk
		if pdfPath != "" && fileExists(pdfPath) && isAllowedInvoicePath(cfg, pdfPath) {
			_ = os.Remove(pdfPath)
		}
		if cfg != nil && cfg.InvoicesDir != "" {
			_ = os.Remove(filepath.Join(cfg.InvoicesDir, invoiceNumber+".pdf"))
			_ = os.Remove(filepath.Join(cfg.InvoicesDir, invoiceNumber+".html"))
		}

		clientIP := middleware.GetClientIP(r)
		log.Printf("[AUDIT INVOICE DELETE] Facture %s supprimée par admin depuis IP %s", invoiceNumber, clientIP)

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"deleted":        true,
			"invoice_number": invoiceNumber,
		})
	}
}

// AdminCreateCreditNoteHandler creates an official credit note (avoir) referencing an original invoice.
func AdminCreateCreditNoteHandler(db *sql.DB, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		trimmed := strings.TrimPrefix(r.URL.Path, "/api/v1/admin/invoices/")
		invoiceNumber := strings.TrimSuffix(trimmed, "/credit-note")
		invoiceNumber = strings.Trim(invoiceNumber, "/")
		if invoiceNumber == "" || strings.Contains(invoiceNumber, "/") {
			writeJSONError(w, "Numéro de facture invalide", http.StatusBadRequest)
			return
		}

		var req struct {
			Reason string `json:"reason"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)

		creditNote, err := dbpkg.CreateCreditNote(db, invoiceNumber, req.Reason)
		if err != nil {
			writeJSONError(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Generate PDF for the credit note
		if cfg != nil && cfg.InvoicesDir != "" {
			pdfPath, _, genErr := invoice.GenerateInvoicePDF(cfg.InvoicesDir, creditNote, "Avoir / Crédit")
			if genErr == nil {
				_ = dbpkg.UpdateInvoicePDFPath(db, creditNote.InvoiceNumber, pdfPath)
				creditNote.PDFPath = pdfPath
			} else {
				log.Printf("[Admin Credit Note] Erreur génération PDF avoir %s: %v", creditNote.InvoiceNumber, genErr)
			}
		}

		clientIP := middleware.GetClientIP(r)
		log.Printf("[AUDIT CREDIT NOTE] Avoir %s généré pour facture %s par admin depuis IP %s", creditNote.InvoiceNumber, invoiceNumber, clientIP)

		writeJSON(w, http.StatusCreated, map[string]interface{}{
			"status":         "created",
			"credit_note":    creditNote,
			"invoice_number": creditNote.InvoiceNumber,
		})
	}
}

