package handlers

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	dbpkg "database"

	"api/config"
	"api/cryptopay"
)

// createCryptoQuote prices a crypto order at the OKX spot rate, stores the
// quote and returns the recap payload shown to the customer. Previous pending
// quotes of the order are expired first so only one quote is ever live.
func createCryptoQuote(ctx context.Context, db *sql.DB, cfg *config.Config, order *dbpkg.Order) (map[string]any, error) {
	asset, err := cryptopay.AssetForPaymentMethod(order.PaymentMethod)
	if err != nil {
		return nil, err
	}
	if !cfg.CryptoAssetConfigured(asset) {
		return nil, fmt.Errorf("paiement %s temporairement indisponible", asset)
	}
	decimals, err := cryptopay.AssetDecimals(asset)
	if err != nil {
		return nil, err
	}
	rate, quotedAt, err := cryptopay.SpotRateEUR(ctx, nil, cfg.OKXBaseURL, asset)
	if err != nil {
		return nil, err
	}
	amount, err := cryptopay.CryptoAmount(order.Price, rate, decimals)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	expiresAt := now.Add(cfg.CryptoQuoteTTL())
	quote := &dbpkg.CryptoQuote{
		OrderID:      order.OrderID,
		Asset:        asset,
		AmountCrypto: amount,
		RateEUR:      rate,
		QuotedAt:     quotedAt,
		ExpiresAt:    expiresAt,
	}
	recap := map[string]any{
		"order_id":       order.OrderID,
		"email":          order.Email,
		"plan":           order.Plan,
		"billing_cycle":  order.BillingCycle,
		"technicians":    order.Technicians,
		"price":          order.Price,
		"payment_method": order.PaymentMethod,
		"asset":          asset,
		"amount_crypto":  amount,
		"rate_eur":       rate,
		"rate_source":    "OKX",
		"quoted_at":      quotedAt.Format(time.RFC3339),
		"expires_at":     expiresAt.Format(time.RFC3339),
	}
	switch asset {
	case cryptopay.AssetBTC:
		quote.PayAddress = cfg.OKXBTCDepositAddress
		recap["pay_address"] = cfg.OKXBTCDepositAddress
		recap["notice"] = fmt.Sprintf(
			"Payez exactement %s BTC à l'adresse indiquée avant le %s. Le montant en euros (%0.2f €) fait foi ; les frais réseau sont à votre charge. La commande est validée à réception du dépôt (généralement quelques dizaines de minutes).",
			amount, expiresAt.Format("02/01/2006 15:04"), order.Price)
	case cryptopay.AssetXRP:
		tag, err := cfg.OKXXRPDepositTagValue()
		if err != nil {
			return nil, err
		}
		quote.PayAddress = cfg.OKXXRPDepositAddress
		quote.DestTag = &tag
		recap["pay_address"] = cfg.OKXXRPDepositAddress
		recap["dest_tag"] = tag
		recap["notice"] = fmt.Sprintf(
			"Payez exactement %s XRP à l'adresse indiquée avec le Destination Tag %d avant le %s, en le reprenant exactement (sans lui, le dépôt ne peut pas être crédité). Le montant en euros (%0.2f €) fait foi ; les frais réseau sont à votre charge. La commande est validée à réception du dépôt (généralement quelques minutes).",
			amount, tag, expiresAt.Format("02/01/2006 15:04"), order.Price)
	default:
		return nil, fmt.Errorf("actif crypto invalide")
	}
	if err := dbpkg.ExpireCryptoQuotesForOrder(db, order.OrderID); err != nil {
		return nil, err
	}
	if err := dbpkg.CreateCryptoQuote(db, quote); err != nil {
		return nil, err
	}
	return recap, nil
}

// writeCryptoOrder branches order creation for crypto payment methods. On any
// pricing failure the pending order is cancelled, mirroring the Stripe branch.
func writeCryptoOrder(w http.ResponseWriter, r *http.Request, db *sql.DB, cfg *config.Config, order *dbpkg.Order) {
	asset, err := cryptopay.AssetForPaymentMethod(order.PaymentMethod)
	if err != nil {
		_, _ = db.Exec(`UPDATE orders SET status = 'cancelled' WHERE order_id = ? AND status = 'pending'`, order.OrderID)
		writeJSONError(w, "Moyen de paiement crypto invalide", http.StatusBadRequest)
		return
	}
	if !cfg.CryptoAssetConfigured(asset) {
		_, _ = db.Exec(`UPDATE orders SET status = 'cancelled' WHERE order_id = ? AND status = 'pending'`, order.OrderID)
		writeJSONError(w, fmt.Sprintf("Paiement %s temporairement indisponible", asset), http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	recap, err := createCryptoQuote(ctx, db, cfg, order)
	if err != nil {
		log.Printf("[Crypto] devis impossible pour %s: %v", order.OrderID, err)
		_, _ = db.Exec(`UPDATE orders SET status = 'cancelled' WHERE order_id = ? AND status = 'pending'`, order.OrderID)
		writeJSONError(w, "Devis crypto indisponible pour le moment", http.StatusServiceUnavailable)
		return
	}
	if err := EnqueueCryptoInstructionsEmail(db, order.OrderID); err != nil {
		log.Printf("[Crypto] instructions non mises en file pour %s: %v", order.OrderID, err)
	}
	writeJSON(w, http.StatusCreated, recap)
}

type cryptoQuoteRefreshRequest struct {
	OrderID string `json:"order_id"`
	Email   string `json:"email"`
}

// CryptoQuoteRefreshHandler issues a fresh quote for a pending crypto order
// whose previous quote expired. The order email must match.
func CryptoQuoteRefreshHandler(db *sql.DB, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req cryptoQuoteRefreshRequest
		if err := decodeSingleJSON(r, &req); err != nil {
			writeJSONError(w, "Requête JSON invalide", http.StatusBadRequest)
			return
		}
		orderID := strings.TrimSpace(req.OrderID)
		email := strings.ToLower(strings.TrimSpace(req.Email))
		if orderID == "" || email == "" {
			writeJSONError(w, "Commande et email requis", http.StatusBadRequest)
			return
		}
		order, err := dbpkg.GetOrderByID(db, orderID)
		if err != nil || order == nil || strings.ToLower(order.Email) != email {
			writeJSONError(w, "Commande introuvable", http.StatusNotFound)
			return
		}
		if order.Status != "pending" {
			writeJSONError(w, "Commande déjà traitée", http.StatusConflict)
			return
		}
		asset, err := cryptopay.AssetForPaymentMethod(order.PaymentMethod)
		if err != nil || !cfg.CryptoAssetConfigured(asset) {
			writeJSONError(w, "Devis crypto indisponible pour le moment", http.StatusServiceUnavailable)
			return
		}
		if existing, err := dbpkg.GetCryptoQuoteByOrderID(db, orderID); err == nil && existing != nil &&
			existing.Status == "pending" && time.Now().UTC().Before(existing.ExpiresAt) {
			writeJSONError(w, "Un devis est déjà en cours pour cette commande", http.StatusConflict)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		recap, err := createCryptoQuote(ctx, db, cfg, order)
		if err != nil {
			log.Printf("[Crypto] renouvellement de devis impossible pour %s: %v", orderID, err)
			writeJSONError(w, "Devis crypto indisponible pour le moment", http.StatusServiceUnavailable)
			return
		}
		writeJSON(w, http.StatusOK, recap)
	}
}

// cryptoInvoiceNotes builds the invoice mentions required by the cahier:
// crypto amount, applied rate with timestamp and source, transaction id.
func cryptoInvoiceNotes(asset, amountCrypto, rateEUR string, quotedAt time.Time, txid string) string {
	return fmt.Sprintf("Montant crypto : %s %s – Cours : %s EUR/%s (OKX, %s UTC) – TXID : %s",
		amountCrypto, asset, rateEUR, asset, quotedAt.Format("02/01/2006 15:04"), txid)
}
