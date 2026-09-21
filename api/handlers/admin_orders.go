package handlers

import (
	dbpkg "database"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"api/config"
	"api/mailer"
)

type AdminOrderView struct {
	dbpkg.Order
	PaymentComplete  bool `json:"payment_complete"`
	LicenseCreated   bool `json:"license_created"`
	InvoiceCreated   bool `json:"invoice_created"`
	LicenseEmailSent bool `json:"license_email_sent"`
	InvoiceEmailSent bool `json:"invoice_email_sent"`
	FulfillmentDone  bool `json:"fulfillment_done"`
}

// AdminListOrdersHandler lists orders with filters.
func AdminListOrdersHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := r.URL.Query().Get("status")
		email := r.URL.Query().Get("email")

		orders, err := dbpkg.ListOrders(db, status, email)
		if err != nil {
			writeJSONError(w, err.Error(), http.StatusInternalServerError)
			return
		}

		views := make([]AdminOrderView, 0, len(orders))
		for _, order := range orders {
			emailStatus, err := dbpkg.GetOrderEmailStatus(db, order.OrderID)
			if err != nil {
				writeJSONError(w, "Impossible de lire l'état de livraison des commandes", http.StatusInternalServerError)
				return
			}
			view := AdminOrderView{
				Order:            order,
				PaymentComplete:  order.Status == "paid",
				LicenseCreated:   order.LicenseID != "",
				InvoiceCreated:   order.InvoiceNumber != "",
				LicenseEmailSent: emailStatus.LicenseSent,
				InvoiceEmailSent: emailStatus.InvoiceSent,
			}
			view.FulfillmentDone = view.PaymentComplete && view.LicenseCreated && view.InvoiceCreated &&
				view.LicenseEmailSent && view.InvoiceEmailSent
			views = append(views, view)
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"orders": views,
			"total":  len(views),
		})
	}
}

// AdminMarkOrderPaidHandler marks a bank transfer as paid or retries any
// missing fulfillment stage of an already paid order. Every stage is idempotent.
func AdminMarkOrderPaidHandler(db *sql.DB, cfg *config.Config, mail *mailer.Mailer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		pathParts := strings.Split(r.URL.Path, "/")
		// /api/v1/admin/orders/{order_id}/mark-paid
		if len(pathParts) < 6 {
			writeJSONError(w, "Chemin invalide", http.StatusBadRequest)
			return
		}

		orderID := pathParts[5]
		order, err := dbpkg.GetOrderByID(db, orderID)
		if err != nil {
			writeJSONError(w, "Commande introuvable", http.StatusNotFound)
			return
		}

		alreadyPaid := order.Status == "paid"
		if !alreadyPaid && (order.Status != "pending" || order.PaymentMethod != "bank_transfer") {
			writeJSONError(w, "Seule une commande par virement en attente peut être validée", http.StatusConflict)
			return
		}

		var lic *dbpkg.License
		if alreadyPaid {
			if order.LicenseID == "" {
				writeJSONError(w, "Commande payée sans licence associée", http.StatusConflict)
				return
			}
			lic, err = dbpkg.GetLicense(db, order.LicenseID)
		} else {
			if order.OrderKind == "renewal" {
				lic, err = dbpkg.FulfillPendingRenewalOrder(db, order.OrderID, fmt.Sprintf("Renouvellement Virement %s (%s)", order.OrderID, order.Plan))
			} else {
				lic, err = dbpkg.FulfillPendingOrder(db, order.OrderID, fmt.Sprintf("Validation Virement %s (%s)", order.OrderID, order.Plan))
			}
		}
		if err != nil || lic == nil {
			writeJSONError(w, "Impossible de finaliser cette commande", http.StatusConflict)
			return
		}

		paymentLabel := "Virement Bancaire"
		if order.PaymentMethod == "stripe" {
			paymentLabel = "Carte Bancaire (Stripe)"
		}
		createdInv, _, err := ensureOrderInvoice(
			db, cfg, order, paymentLabel, fmt.Sprintf("Paiement commande %s", order.OrderID),
		)
		if err != nil {
			log.Printf("[Admin Orders] Erreur facture pour commande %s: %v", order.OrderID, err)
			writeJSONError(w, "Commande payée, mais la facture n'a pas pu être générée; réessayez", http.StatusInternalServerError)
			return
		}
		if err := EnqueueOrderDeliveryEmail(db, order.OrderID); err != nil {
			log.Printf("[Admin Orders] Notifications non mises en file pour commande %s: %v", order.OrderID, err)
			writeJSONError(w, "Commande payée, mais les notifications n'ont pas pu être mises en file", http.StatusInternalServerError)
			return
		}

		status := "paid"
		if alreadyPaid {
			status = "already_paid"
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status":         status,
			"order_id":       order.OrderID,
			"license_id":     lic.LicenseID,
			"license_key":    lic.LicenseKey,
			"invoice_number": createdInv.InvoiceNumber,
			"email":          lic.Email,
			"expires_at":     lic.ExpiresAt.Format(time.RFC3339),
			"technicians":    lic.MaxConnections,
		})
	}
}

// AdminDeleteOrderHandler permanently deletes an order by its ID.
func AdminDeleteOrderHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orderID := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/api/v1/admin/orders/"))
		if orderID == "" || strings.Contains(orderID, "/") {
			writeJSONError(w, "Identifiant de commande invalide", http.StatusBadRequest)
			return
		}

		if err := dbpkg.DeleteOrder(db, orderID); err != nil {
			writeJSONError(w, err.Error(), http.StatusBadRequest)
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"deleted":  true,
			"order_id": orderID,
		})
	}
}

