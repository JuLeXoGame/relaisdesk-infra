package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"time"

	dbpkg "database"

	"api/config"
	"api/cryptopay"
)

type cryptoEventPayload struct {
	Kind      string `json:"kind"` // 'okx'
	Reference string `json:"reference"`
	QuoteID   int64  `json:"quote_id,omitempty"`
	TxID      string `json:"txid,omitempty"`
	Sender    string `json:"sender,omitempty"`
}

func processCryptoEvent(db *sql.DB, cfg *config.Config, payload []byte) error {
	var event cryptoEventPayload
	if err := json.Unmarshal(payload, &event); err != nil {
		return err
	}
	switch event.Kind {
	case "okx":
		return completeCryptoPayment(db, cfg, event.Reference, event.QuoteID, event.TxID, event.Sender, 1)
	default:
		return fmt.Errorf("événement crypto inconnu: %s", event.Kind)
	}
}

// completeCryptoPayment settles the quote an OKX deposit paid: quote,
// fulfillment, invoice with crypto mentions, payments registry, delivery
// email. It is idempotent: replays and second deposits converge without
// duplicating licenses, invoices or registry rows.
func completeCryptoPayment(db *sql.DB, cfg *config.Config, orderID string, quoteID int64, txid, sender string, confirmations int) error {
	order, err := dbpkg.GetOrderByID(db, orderID)
	if err != nil || order == nil {
		return fmt.Errorf("commande crypto introuvable (%s): %w", orderID, err)
	}
	asset, err := cryptopay.AssetForPaymentMethod(order.PaymentMethod)
	if err != nil {
		return fmt.Errorf("commande %s non crypto", orderID)
	}
	quote, err := dbpkg.GetCryptoQuoteByID(db, quoteID)
	if err != nil || quote == nil || quote.OrderID != order.OrderID || quote.Asset != asset {
		return fmt.Errorf("devis crypto introuvable pour %s", orderID)
	}
	label := cryptopay.AssetLabel(asset)
	notes := cryptoInvoiceNotes(asset, quote.AmountCrypto, quote.RateEUR, quote.QuotedAt, txid)
	claimed, err := dbpkg.MarkCryptoQuotePaidByID(db, quote.ID, txid, sender, confirmations)
	if err != nil {
		return err
	}
	if !claimed && order.Status != "pending" {
		// Replay, or a second deposit for an already settled quote:
		// converge. The registry keeps the first payment only.
		log.Printf("[Crypto] dépôt déjà traité pour %s (devis %d, %s), ignoré", orderID, quote.ID, txid)
		if order.Status == "paid" {
			// A previous attempt may have died after fulfillment but
			// before invoicing or delivery: reissue those idempotent
			// stages (existing invoice and queued delivery are no-ops).
			if _, _, err := ensureOrderInvoice(db, cfg, order, label, notes); err != nil {
				return err
			}
			return EnqueueOrderDeliveryEmail(db, order.OrderID, "")
		}
		return nil
	}
	if !claimed {
		// A previous attempt claimed this quote but died before (or
		// during) fulfillment: resume instead of converging, or the paid
		// order would stay pending forever (the watcher never re-queues
		// a consumed deposit key).
		log.Printf("[Crypto] reprise du règlement %s après interruption (devis %d déjà acquis)", orderID, quote.ID)
	}
	// A late deposit can settle an expired quote while a refreshed quote is
	// pending: the refreshed one must not stay live on a paid order.
	if err := dbpkg.ExpireCryptoQuotesForOrder(db, order.OrderID); err != nil {
		log.Printf("[Crypto] expiration des devis résiduels impossible pour %s: %v", order.OrderID, err)
	}
	pendingKey := ""
	if order.Status == "pending" {
		if order.OrderKind == "renewal" {
			_, err = dbpkg.FulfillPendingRenewalOrder(db, order.OrderID, fmt.Sprintf("Crypto %s %s", asset, txid))
		} else {
			var lic *dbpkg.License
			lic, err = dbpkg.FulfillPendingOrder(db, order.OrderID, fmt.Sprintf("Crypto %s %s", asset, txid))
			if err == nil && lic != nil {
				pendingKey = lic.LicenseKey // one-time plaintext
			}
		}
		if err != nil {
			return err
		}
		order, err = dbpkg.GetOrderByID(db, order.OrderID)
		if err != nil {
			return err
		}
	} else if order.Status != "paid" || order.LicenseID == "" {
		return fmt.Errorf("état de commande crypto incompatible: %s", order.Status)
	}
	inv, _, err := ensureOrderInvoice(db, cfg, order, label, notes)
	if err != nil {
		return err
	}
	if err := dbpkg.RecordCryptoPayment(db, &dbpkg.CryptoPayment{
		OrderID: order.OrderID, InvoiceNumber: inv.InvoiceNumber, Asset: asset,
		AmountCrypto: quote.AmountCrypto, RateEUR: quote.RateEUR, RateAt: quote.QuotedAt,
		TxID: txid, Sender: sender,
	}); err != nil {
		log.Printf("[Crypto] registre incomplet pour %s: %v", order.OrderID, err)
	}
	return EnqueueOrderDeliveryEmail(db, order.OrderID, deliveryKeyFor(order, pendingKey))
}

// OKXWatcherInterval is the deposit polling cadence.
const OKXWatcherInterval = 60 * time.Second

// okxWatcherState carries per-process watcher memory (unmatched deposits
// are logged once).
type okxWatcherState struct {
	unmatchedLogged map[string]bool
}

// RunOKXDepositWatcher polls the OKX deposit history for open crypto quotes
// and expires stale quotes. It stops with ctx.
func RunOKXDepositWatcher(ctx context.Context, db *sql.DB, cfg *config.Config) {
	state := &okxWatcherState{unmatchedLogged: map[string]bool{}}
	ticker := time.NewTicker(OKXWatcherInterval)
	defer ticker.Stop()
	pollOKXDepositsOnce(ctx, db, cfg, state)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pollOKXDepositsOnce(ctx, db, cfg, state)
		}
	}
}

func pollOKXDepositsOnce(ctx context.Context, db *sql.DB, cfg *config.Config, state *okxWatcherState) {
	if state == nil {
		state = &okxWatcherState{unmatchedLogged: map[string]bool{}}
	}
	now := time.Now().UTC()
	if count, err := dbpkg.ExpireCryptoQuotes(db, now); err != nil {
		log.Printf("[Crypto] expiration des devis impossible: %v", err)
	} else if count > 0 {
		log.Printf("[Crypto] %d devis expirés", count)
	}
	if cfg == nil || !cfg.CryptoEnabled {
		return
	}
	creds := cryptopay.OKXCredentials{Key: cfg.OKXAPIKey, Secret: cfg.OKXAPISecret, Passphrase: cfg.OKXAPIPassphrase}
	for _, asset := range []string{cryptopay.AssetBTC, cryptopay.AssetXRP} {
		if !cfg.CryptoAssetConfigured(asset) {
			continue
		}
		pollOKXAssetDeposits(ctx, db, cfg, state, creds, asset, now)
	}
}

func pollOKXAssetDeposits(ctx context.Context, db *sql.DB, cfg *config.Config, state *okxWatcherState, creds cryptopay.OKXCredentials, asset string, now time.Time) {
	payAddress := cfg.OKXBTCDepositAddress
	expectedTag := ""
	if asset == cryptopay.AssetXRP {
		payAddress = cfg.OKXXRPDepositAddress
		expectedTag = cfg.OKXXRPDepositTag
	}
	quotes, err := dbpkg.ListOpenCryptoQuotes(db, asset, now.Add(-24*time.Hour), 200)
	if err != nil {
		log.Printf("[Crypto] lecture des devis %s impossible: %v", asset, err)
		return
	}
	if len(quotes) == 0 {
		return
	}
	// Safety rail: never match against an address outside the operator OKX
	// account. A configuration typo pauses matching instead of paying
	// strangers' quotes.
	pollCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := cryptopay.VerifyDepositAddress(pollCtx, nil, cfg.OKXBaseURL, creds, asset, payAddress, expectedTag); err != nil {
		log.Printf("[Crypto] vérification de l'adresse de dépôt %s impossible: %v", asset, err)
		return
	}
	oldest := quotes[0].QuotedAt
	for _, quote := range quotes[1:] {
		if quote.QuotedAt.Before(oldest) {
			oldest = quote.QuotedAt
		}
	}
	deposits, err := cryptopay.DepositHistory(pollCtx, nil, cfg.OKXBaseURL, creds, asset, fmt.Sprintf("%d", oldest.Add(-5*time.Minute).UnixMilli()))
	if err != nil {
		log.Printf("[Crypto] historique des dépôts %s injoignable: %v", asset, err)
		return
	}
	candidates := make([]cryptopay.MatchQuote, 0, len(quotes))
	for _, quote := range quotes {
		candidates = append(candidates, cryptopay.MatchQuote{
			ID: quote.ID, OrderID: quote.OrderID, AmountCrypto: quote.AmountCrypto,
			PayAddress: quote.PayAddress, QuotedAt: quote.QuotedAt, ExpiresAt: quote.ExpiresAt,
		})
	}
	for _, deposit := range deposits {
		if deposit.CCY != asset || deposit.TxID == "" || deposit.DepID == "" {
			continue
		}
		match, eligible := cryptopay.MatchOKXDeposit(candidates, deposit, payAddress)
		if match == nil {
			if !state.unmatchedLogged["okx:"+deposit.DepID] {
				state.unmatchedLogged["okx:"+deposit.DepID] = true
				if eligible > 1 {
					log.Printf("[Crypto] dépôt %s %s ambigu (%d devis éligibles, %s, traitement manuel requis)", asset, deposit.Amt, eligible, deposit.TxID)
				} else {
					log.Printf("[Crypto] dépôt %s %s sans devis (%s, traitement manuel si commande en attente)", asset, deposit.Amt, deposit.TxID)
				}
			}
			continue
		}
		queued, err := EnqueueCryptoEvent(db, "okx:"+deposit.DepID, cryptoEventPayload{
			Kind: "okx", Reference: match.OrderID, QuoteID: match.ID,
			TxID: deposit.TxID, Sender: deposit.From,
		})
		if err != nil {
			log.Printf("[Crypto] mise en file impossible pour %s: %v", match.OrderID, err)
			continue
		}
		if queued {
			log.Printf("[Crypto] dépôt %s détecté pour %s (%s)", asset, match.OrderID, deposit.TxID)
		}
	}
}
