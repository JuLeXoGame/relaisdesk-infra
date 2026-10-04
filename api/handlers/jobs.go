package handlers

import (
	"context"
	"crypto/rand"
	dbpkg "database"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"time"

	"api/config"
	"api/invoice"
	"api/mailer"
)

const (
	jobStripeEvent        = "stripe_event"
	jobCustomerLogin      = "customer_login_email"
	jobTeamInvitation     = "team_invitation_email"
	jobBankInstructions   = "bank_instructions_email"
	jobOrderDelivery      = "order_delivery_email"
	jobWithdrawalEmails   = "withdrawal_emails"
	jobRenewalReminder    = "renewal_reminder_email"
	jobManualInvoiceMail  = "manual_invoice_email"
	jobCryptoEvent        = "crypto_event"
	jobCryptoInstructions = "crypto_instructions_email"
)

type stringPayload struct {
	Value string `json:"value"`
}

type reminderJobPayload struct {
	LicenseID    string `json:"license_id"`
	Email        string `json:"email"`
	ExpiresAt    string `json:"expires_at"`
	ReminderType string `json:"reminder_type"`
}

func randomJobKey(prefix string) (string, error) {
	value := make([]byte, 12)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return prefix + ":" + hex.EncodeToString(value), nil
}

func enqueueJSONJob(db *sql.DB, jobType string, payload any, uniqueKey string, maxAttempts int) (bool, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return false, err
	}
	return dbpkg.EnqueueJob(db, jobType, string(encoded), uniqueKey, maxAttempts, time.Now().UTC())
}

func EnqueueCustomerLoginEmail(db *sql.DB, email string) error {
	key, err := randomJobKey("login")
	if err != nil {
		return err
	}
	_, err = enqueueJSONJob(db, jobCustomerLogin, stringPayload{Value: email}, key, 8)
	return err
}

func EnqueueTeamInvitationEmail(db *sql.DB, memberID string) error {
	key, err := randomJobKey("team-invite")
	if err != nil {
		return err
	}
	_, err = enqueueJSONJob(db, jobTeamInvitation, stringPayload{Value: memberID}, key, 8)
	return err
}

func EnqueueBankInstructionsEmail(db *sql.DB, orderID string) error {
	_, err := enqueueJSONJob(db, jobBankInstructions, stringPayload{Value: orderID}, "bank-instructions:"+orderID, 12)
	return err
}

// EnqueueCryptoInstructionsEmail queues the payment recap email for a crypto
// order. One email per order: refreshed quotes are shown on the page.
func EnqueueCryptoInstructionsEmail(db *sql.DB, orderID string) error {
	_, err := enqueueJSONJob(db, jobCryptoInstructions, stringPayload{Value: orderID}, "crypto-instructions:"+orderID, 12)
	return err
}

// orderDeliveryPayload carries the order plus, for first deliveries, the
// plaintext license key for one-time inclusion in the delivery email. The
// worker scrubs the key from the stored payload after a successful send.
// Renewal and recovery re-deliveries pass an empty key: the email then shows
// the support-safe hint instead of re-sending the secret.
type orderDeliveryPayload struct {
	OrderID    string `json:"order_id"`
	LicenseKey string `json:"license_key,omitempty"`
}

func EnqueueOrderDeliveryEmail(db *sql.DB, orderID, licenseKey string) error {
	_, err := enqueueJSONJob(db, jobOrderDelivery, orderDeliveryPayload{OrderID: orderID, LicenseKey: licenseKey}, "order-delivery:"+orderID, 20)
	return err
}

func EnqueueWithdrawalEmails(db *sql.DB, requestID string) error {
	_, err := enqueueJSONJob(db, jobWithdrawalEmails, stringPayload{Value: requestID}, "withdrawal:"+requestID, 20)
	return err
}

func EnqueueRenewalReminderEmail(db *sql.DB, candidate dbpkg.RenewalReminderCandidate) error {
	payload := reminderJobPayload{
		LicenseID: candidate.LicenseID, Email: candidate.Email,
		ExpiresAt: candidate.ExpiresAt.UTC().Format(time.RFC3339), ReminderType: candidate.ReminderType,
	}
	key := fmt.Sprintf("renewal-reminder:%s:%s:%s", candidate.LicenseID, candidate.ReminderType, candidate.ExpiresAt.UTC().Format(time.RFC3339))
	_, err := enqueueJSONJob(db, jobRenewalReminder, payload, key, 20)
	return err
}

func EnqueueStripeEvent(db *sql.DB, eventID string, body []byte) (bool, error) {
	return dbpkg.EnqueueJob(db, jobStripeEvent, string(body), "stripe:"+eventID, 30, time.Now().UTC())
}

func EnqueueCryptoEvent(db *sql.DB, dedupKey string, payload cryptoEventPayload) (bool, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return false, err
	}
	return dbpkg.EnqueueJob(db, jobCryptoEvent, string(encoded), "crypto:"+dedupKey, 30, time.Now().UTC())
}

func EnqueueManualInvoiceEmail(db *sql.DB, invoiceNumber string) error {
	key, err := randomJobKey("invoice:" + invoiceNumber)
	if err != nil {
		return err
	}
	_, err = enqueueJSONJob(db, jobManualInvoiceMail, stringPayload{Value: invoiceNumber}, key, 12)
	return err
}

// RunJobWorker is intentionally lightweight: one local SQLite worker, no
// broker, no telemetry agent and no external process. Durable leases make a
// restart safe.
func RunJobWorker(ctx context.Context, db *sql.DB, cfg *config.Config, mail *mailer.Mailer) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		for processed := 0; processed < 20; processed++ {
			job, err := dbpkg.ClaimNextJob(db, time.Now().UTC())
			if err != nil {
				log.Printf("[Jobs] claim impossible: %v", err)
				break
			}
			if job == nil {
				break
			}
			if err := processJob(db, cfg, mail, job); err != nil {
				log.Printf("[Jobs] %s #%d échec tentative %d/%d: %v", job.Type, job.ID, job.Attempts, job.MaxAttempts, err)
				if failErr := dbpkg.FailJob(db, job, err, time.Now().UTC()); failErr != nil {
					log.Printf("[Jobs] enregistrement échec #%d impossible: %v", job.ID, failErr)
				}
				continue
			}
			if err := dbpkg.CompleteJob(db, job.ID); err != nil {
				log.Printf("[Jobs] finalisation #%d impossible: %v", job.ID, err)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func processJob(db *sql.DB, cfg *config.Config, mail *mailer.Mailer, job *dbpkg.Job) error {
	if job == nil {
		return errors.New("travail absent")
	}
	if job.Type == jobTrialCancel {
		var payload stringPayload
		if err := json.Unmarshal([]byte(job.Payload), &payload); err != nil {
			return err
		}
		return processTrialCancellation(db, cfg, payload.Value)
	}
	if job.Type == jobTrialWithdrawalProcess {
		var p stringPayload
		if err := json.Unmarshal([]byte(job.Payload), &p); err != nil {
			return err
		}
		return processTrialWithdrawal(db, cfg, p.Value)
	}
	if job.Type != jobStripeEvent && job.Type != jobCryptoEvent && mail == nil {
		return errors.New("service e-mail indisponible")
	}
	switch job.Type {
	case jobTrialWithdrawalEmail:
		return processTrialWithdrawalEmail(db, cfg, mail, job)
	case jobSubscriptionRenewalNotice:
		return processSubscriptionRenewalNotice(db, cfg, mail, job)
	case jobTrialVerify, jobTrialNotice:
		return processTrialEmail(db, cfg, mail, job)
	case jobStripeEvent:
		return processStripeEvent(db, cfg, []byte(job.Payload))
	case jobCryptoEvent:
		return processCryptoEvent(db, cfg, []byte(job.Payload))
	case jobCustomerLogin:
		var payload stringPayload
		if err := json.Unmarshal([]byte(job.Payload), &payload); err != nil {
			return err
		}
		token, eligible, err := dbpkg.CreateCustomerLoginToken(db, payload.Value)
		if err != nil || !eligible {
			return err
		}
		loginURL, err := customerPortalURL(cfg.PublicWebsiteURL, token)
		if err != nil {
			return err
		}
		return mail.SendCustomerLoginLink(payload.Value, loginURL, customerMagicLinkLifetime)
	case jobTeamInvitation:
		var payload stringPayload
		if err := json.Unmarshal([]byte(job.Payload), &payload); err != nil {
			return err
		}
		token, email, err := dbpkg.IssueTeamInvitationToken(db, 0, payload.Value)
		if errors.Is(err, dbpkg.ErrTeamInvite) {
			return nil
		}
		if err != nil {
			return err
		}
		return mail.SendTeamInvitation(email, strings.TrimRight(cfg.PublicWebsiteURL, "/")+"/client/#invite="+url.QueryEscape(token))
	case jobCartReminder:
		return processCartReminderEmail(db, cfg, mail, job)
	case jobBankInstructions:
		var payload stringPayload
		if err := json.Unmarshal([]byte(job.Payload), &payload); err != nil {
			return err
		}
		order, err := dbpkg.GetOrderByID(db, payload.Value)
		if err != nil {
			return err
		}
		return mail.SendBankTransferInstructionsEmail(order.Email, order.OrderID, order.Plan, order.Price, cfg.BankIBAN, cfg.BankBIC, cfg.BankHolder, order.TermsVersion)
	case jobCryptoInstructions:
		var payload stringPayload
		if err := json.Unmarshal([]byte(job.Payload), &payload); err != nil {
			return err
		}
		order, err := dbpkg.GetOrderByID(db, payload.Value)
		if err != nil {
			return err
		}
		quote, err := dbpkg.GetCryptoQuoteByOrderID(db, payload.Value)
		if err != nil || quote == nil {
			return fmt.Errorf("devis crypto introuvable pour %s", payload.Value)
		}
		return mail.SendCryptoInstructionsEmail(order.Email, order.OrderID, order.Plan, order.Price,
			quote.Asset, quote.AmountCrypto, quote.RateEUR, quote.PayAddress, quote.DestTag,
			quote.ExpiresAt, order.TermsVersion)
	case jobOrderDelivery:
		var payload orderDeliveryPayload
		if err := json.Unmarshal([]byte(job.Payload), &payload); err != nil {
			return err
		}
		if payload.OrderID == "" {
			// Jobs queued before the key-carrying payload shape.
			var legacy stringPayload
			if err := json.Unmarshal([]byte(job.Payload), &legacy); err != nil {
				return err
			}
			payload.OrderID = legacy.Value
		}
		return processOrderDeliveryEmail(db, cfg, mail, job.ID, payload.OrderID, payload.LicenseKey)
	case jobWithdrawalEmails:
		var payload stringPayload
		if err := json.Unmarshal([]byte(job.Payload), &payload); err != nil {
			return err
		}
		item, err := dbpkg.GetWithdrawalRequest(db, payload.Value)
		if err != nil {
			return err
		}
		requestedAt := item.RequestedAt.Format(time.RFC3339)
		if item.CustomerEmailSentAt == nil {
			if err := mail.SendWithdrawalAcknowledgementEmail(item.Email, item.RequestID, item.OrderID, requestedAt); err != nil {
				return err
			}
			if err := dbpkg.MarkWithdrawalEmailSent(db, item.RequestID, "customer", time.Now().UTC()); err != nil {
				return err
			}
		}
		if item.AdminEmailSentAt == nil {
			if err := mail.SendWithdrawalNotificationEmail(item.RequestID, item.OrderID, item.Email, item.CustomerName, requestedAt); err != nil {
				return err
			}
			return dbpkg.MarkWithdrawalEmailSent(db, item.RequestID, "admin", time.Now().UTC())
		}
		return nil
	case jobRenewalReminder:
		var payload reminderJobPayload
		if err := json.Unmarshal([]byte(job.Payload), &payload); err != nil {
			return err
		}
		expiresAt, err := time.Parse(time.RFC3339, payload.ExpiresAt)
		if err != nil {
			return err
		}
		candidate := dbpkg.RenewalReminderCandidate{LicenseID: payload.LicenseID, Email: payload.Email, ExpiresAt: expiresAt, ReminderType: payload.ReminderType}
		portalURL, err := customerPortalBaseURL(cfg.PublicWebsiteURL)
		if err == nil {
			err = mail.SendRenewalReminder(payload.Email, payload.LicenseID, expiresAt.Format("02/01/2006"), portalURL, payload.ReminderType)
		}
		if finishErr := dbpkg.FinishRenewalReminder(db, candidate, err); finishErr != nil {
			return finishErr
		}
		return err
	case jobManualInvoiceMail:
		var payload stringPayload
		if err := json.Unmarshal([]byte(job.Payload), &payload); err != nil {
			return err
		}
		return processManualInvoiceEmail(db, cfg, mail, payload.Value)
	default:
		return fmt.Errorf("type de travail inconnu: %s", job.Type)
	}
}

func processOrderDeliveryEmail(db *sql.DB, cfg *config.Config, mail *mailer.Mailer, jobID int64, orderID, licenseKey string) error {
	order, err := dbpkg.GetOrderByID(db, orderID)
	if err != nil || order.Status != "paid" || order.LicenseID == "" {
		return errors.New("commande payée introuvable ou incomplète")
	}
	lic, err := dbpkg.GetLicense(db, order.LicenseID)
	if err != nil {
		return err
	}
	inv, pdfBytes, err := ensureOrderInvoice(db, cfg, order, paymentLabel(order), fmt.Sprintf("Paiement commande %s", order.OrderID))
	if err != nil {
		return err
	}
	if err := deliverPendingOrderEmails(db, mail, order, lic, inv, pdfBytes, licenseKey); err != nil {
		return err
	}
	if licenseKey != "" {
		// The key was delivered: scrub it from the retained job row.
		// A scrub failure must not retry (emails are sent); it is logged.
		redacted, _ := json.Marshal(orderDeliveryPayload{OrderID: orderID})
		if err := dbpkg.ScrubJobPayload(db, jobID, string(redacted)); err != nil {
			log.Printf("[Jobs] purge de la clé du travail #%d impossible: %v", jobID, err)
		}
	}
	return nil
}

func paymentLabel(order *dbpkg.Order) string {
	if order == nil {
		return "Virement Bancaire"
	}
	switch order.PaymentMethod {
	case "stripe":
		return "Carte Bancaire (Stripe)"
	case "crypto_btc":
		return "Bitcoin (BTC)"
	case "crypto_xrp":
		return "XRP"
	default:
		return "Virement Bancaire"
	}
}

func processManualInvoiceEmail(db *sql.DB, cfg *config.Config, mail *mailer.Mailer, invoiceNumber string) error {
	inv, err := dbpkg.GetInvoiceByNumber(db, invoiceNumber)
	if err != nil {
		return err
	}
	var pdfBytes []byte
	if inv.PDFPath != "" && isAllowedInvoicePath(cfg, inv.PDFPath) && fileExists(inv.PDFPath) {
		pdfBytes, _ = os.ReadFile(inv.PDFPath)
	}
	if len(pdfBytes) == 0 {
		pdfPath, generated, err := invoice.GenerateInvoicePDF(cfg.InvoicesDir, inv, "Email Client")
		if err != nil {
			return err
		}
		pdfBytes = generated
		_, _ = db.Exec(`UPDATE invoices SET pdf_path = ? WHERE invoice_number = ?`, pdfPath, inv.InvoiceNumber)
	}
	return mail.SendInvoiceEmailWithPDF(inv.CustomerEmail, inv.InvoiceNumber, inv.Plan, inv.AmountTTC, pdfBytes)
}
