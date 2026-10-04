package cryptopay

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// OKXCredentials authenticates the private Funding endpoints (deposit
// history, deposit address). A read-only API key is enough: it can neither
// trade nor withdraw. Restrict it by IP in the OKX back office.
type OKXCredentials struct {
	Key        string
	Secret     string
	Passphrase string
}

// Complete reports whether the credentials can sign a request.
func (c OKXCredentials) Complete() bool {
	return strings.TrimSpace(c.Key) != "" && strings.TrimSpace(c.Secret) != "" &&
		strings.TrimSpace(c.Passphrase) != ""
}

// OKXTimestamp formats t the way OKX expects in OK-ACCESS-TIMESTAMP
// (UTC ISO 8601 with milliseconds).
func OKXTimestamp(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000Z")
}

// SignOKXRequest builds the OK-ACCESS-SIGN header: base64 of
// HMAC-SHA256(secret, timestamp + method + requestPath + body) where
// requestPath includes the query string.
func SignOKXRequest(secret, timestamp, method, requestPath, body string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp + strings.ToUpper(method) + requestPath + body))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// OKXDeposit is one credited deposit from /api/v5/asset/deposit-history.
type OKXDeposit struct {
	CCY      string // 'BTC' or 'XRP'
	Amt      string // exact decimal amount credited
	From     string // sender on-chain address
	To       string // credited on-chain address (our deposit address)
	TxID     string // on-chain transaction hash
	TsMillis int64  // record creation time (ms epoch)
	State    string // '2' means deposit successful
	DepID    string // OKX deposit id (unique per credit)
}

// DepositHistory fetches the successful (state=2) deposits of a currency
// newer than afterMillis (ms epoch, "" for no bound), following pagination
// internally. Only credited deposits are ever returned by the matcher.
func DepositHistory(ctx context.Context, client *http.Client, baseURL string, creds OKXCredentials, ccy string, afterMillis string) ([]OKXDeposit, error) {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	base := strings.TrimRight(baseURL, "/")
	if base == "" || !creds.Complete() {
		return nil, fmt.Errorf("configuration OKX incomplète")
	}
	if strings.ToUpper(strings.TrimSpace(ccy)) != AssetBTC && strings.ToUpper(strings.TrimSpace(ccy)) != AssetXRP {
		return nil, fmt.Errorf("actif crypto invalide")
	}
	var out []OKXDeposit
	after := strings.TrimSpace(afterMillis)
	// Few deposits a day: cap pagination well above any realistic volume.
	for page := 0; page < 5; page++ {
		query := "ccy=" + strings.ToUpper(strings.TrimSpace(ccy)) + "&state=2&limit=100"
		if after != "" {
			query += "&after=" + after
		}
		path := "/api/v5/asset/deposit-history?" + query
		body, err := signedOKXGet(ctx, client, base, creds, path)
		if err != nil {
			return nil, err
		}
		var decoded struct {
			Code string `json:"code"`
			Msg  string `json:"msg"`
			Data []struct {
				CCY   string `json:"ccy"`
				Amt   string `json:"amt"`
				From  string `json:"from"`
				To    string `json:"to"`
				TxID  string `json:"txId"`
				Ts    string `json:"ts"`
				State string `json:"state"`
				DepID string `json:"depId"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &decoded); err != nil {
			return nil, fmt.Errorf("historique OKX invalide: %w", err)
		}
		if decoded.Code != "0" {
			return nil, fmt.Errorf("historique OKX indisponible (%s %s)", decoded.Code, decoded.Msg)
		}
		if len(decoded.Data) == 0 {
			break
		}
		for _, row := range decoded.Data {
			ts, err := strconv.ParseInt(strings.TrimSpace(row.Ts), 10, 64)
			if err != nil || ts <= 0 {
				continue
			}
			if strings.TrimSpace(row.State) != "2" || strings.TrimSpace(row.DepID) == "" {
				continue
			}
			out = append(out, OKXDeposit{
				CCY: strings.ToUpper(strings.TrimSpace(row.CCY)), Amt: strings.TrimSpace(row.Amt),
				From: strings.TrimSpace(row.From), To: strings.TrimSpace(row.To),
				TxID: strings.TrimSpace(row.TxID), TsMillis: ts,
				State: strings.TrimSpace(row.State), DepID: strings.TrimSpace(row.DepID),
			})
		}
		if len(decoded.Data) < 100 {
			break
		}
		after = strings.TrimSpace(decoded.Data[len(decoded.Data)-1].Ts)
		if after == "" {
			break
		}
	}
	return out, nil
}

// VerifyDepositAddress checks the configured deposit address (and XRP tag)
// against the selected OKX deposit address, so a typo in the configuration
// never shows customers an address outside the operator account.
func VerifyDepositAddress(ctx context.Context, client *http.Client, baseURL string, creds OKXCredentials, ccy, expectedAddress, expectedTag string) error {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	base := strings.TrimRight(baseURL, "/")
	if base == "" || !creds.Complete() {
		return fmt.Errorf("configuration OKX incomplète")
	}
	path := "/api/v5/asset/deposit-address?ccy=" + strings.ToUpper(strings.TrimSpace(ccy))
	body, err := signedOKXGet(ctx, client, base, creds, path)
	if err != nil {
		return err
	}
	var decoded struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
		Data []struct {
			Addr     string `json:"addr"`
			Tag      string `json:"tag"`
			Memo     string `json:"memo"`
			Selected bool   `json:"selected"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return fmt.Errorf("adresse de dépôt OKX invalide: %w", err)
	}
	if decoded.Code != "0" || len(decoded.Data) == 0 {
		return fmt.Errorf("adresse de dépôt OKX indisponible (%s %s)", decoded.Code, decoded.Msg)
	}
	chosen := decoded.Data[0]
	for _, row := range decoded.Data {
		if row.Selected {
			chosen = row
			break
		}
	}
	if strings.TrimSpace(chosen.Addr) != strings.TrimSpace(expectedAddress) {
		return fmt.Errorf("adresse de dépôt %s configurée différente du compte OKX", ccy)
	}
	if strings.ToUpper(strings.TrimSpace(ccy)) == AssetXRP && strings.TrimSpace(expectedTag) != "" {
		got := strings.TrimSpace(chosen.Tag)
		if got == "" {
			got = strings.TrimSpace(chosen.Memo)
		}
		if got != "" && got != strings.TrimSpace(expectedTag) {
			return fmt.Errorf("tag de dépôt XRP configuré différent du compte OKX")
		}
	}
	return nil
}

func signedOKXGet(ctx context.Context, client *http.Client, base string, creds OKXCredentials, requestPath string) ([]byte, error) {
	timestamp := OKXTimestamp(time.Now())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+requestPath, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("OK-ACCESS-KEY", creds.Key)
	req.Header.Set("OK-ACCESS-SIGN", SignOKXRequest(creds.Secret, timestamp, http.MethodGet, requestPath, ""))
	req.Header.Set("OK-ACCESS-TIMESTAMP", timestamp)
	req.Header.Set("OK-ACCESS-PASSPHRASE", creds.Passphrase)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("OKX injoignable: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("réponse OKX illisible: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("OKX indisponible (%d)", resp.StatusCode)
	}
	return body, nil
}

// MatchQuote is the subset of a stored quote the deposit matcher needs.
type MatchQuote struct {
	ID           int64
	OrderID      string
	AmountCrypto string
	PayAddress   string
	QuotedAt     time.Time
	ExpiresAt    time.Time
}

// MatchOKXDeposit returns the unique quote a credited OKX deposit pays and
// the number of eligible quotes. Eligibility: same credited address, amount
// greater than or equal (exact rational comparison, so underpayments never
// pay) but within the overpay tolerance (small overpayments from rounding or
// fee buffers pay; anything beyond needs manual review), deposit recorded
// after the quote (minus clock skew) and within a grace window past expiry
// (a BTC credit can land after the quote expired).
//
// All deposit addresses are shared between orders, so "first match wins"
// would let one customer's deposit pay another customer's older quote.
// Instead, several eligible quotes resolve to the single exact-amount match
// when there is exactly one, and to no match (manual review) when ambiguous.
// Callers pass pending quotes first, oldest first.
func MatchOKXDeposit(quotes []MatchQuote, deposit OKXDeposit, payAddress string) (*MatchQuote, int) {
	depAmount, ok := new(big.Rat).SetString(strings.TrimSpace(deposit.Amt))
	if !ok || depAmount.Sign() <= 0 {
		return nil, 0
	}
	if strings.TrimSpace(deposit.To) == "" || strings.TrimSpace(deposit.To) != strings.TrimSpace(payAddress) {
		return nil, 0
	}
	if deposit.TsMillis <= 0 {
		return nil, 0
	}
	depTime := time.UnixMilli(deposit.TsMillis)
	const (
		clockSkew = 5 * time.Minute
		grace     = 24 * time.Hour
	)
	var eligible []*MatchQuote
	for i := range quotes {
		quote := &quotes[i]
		if strings.TrimSpace(quote.PayAddress) != "" && strings.TrimSpace(quote.PayAddress) != strings.TrimSpace(payAddress) {
			continue
		}
		expected, ok := new(big.Rat).SetString(strings.TrimSpace(quote.AmountCrypto))
		if !ok || expected.Sign() <= 0 || depAmount.Cmp(expected) < 0 {
			continue
		}
		if !withinOverpayTolerance(depAmount, expected) {
			continue
		}
		if depTime.Before(quote.QuotedAt.Add(-clockSkew)) {
			continue
		}
		if depTime.After(quote.ExpiresAt.Add(grace)) {
			continue
		}
		eligible = append(eligible, quote)
	}
	if len(eligible) == 0 {
		return nil, 0
	}
	if len(eligible) == 1 {
		return eligible[0], 1
	}
	var exact []*MatchQuote
	for _, quote := range eligible {
		expected, ok := new(big.Rat).SetString(strings.TrimSpace(quote.AmountCrypto))
		if !ok {
			continue
		}
		if depAmount.Cmp(expected) == 0 {
			exact = append(exact, quote)
		}
	}
	if len(exact) == 1 {
		return exact[0], len(eligible)
	}
	return nil, len(eligible) // ambiguous: manual review, never auto-pay
}

// withinOverpayTolerance reports whether dep pays expected with at most a 2%
// overpayment (dep <= expected * 51/50, exact rational arithmetic). Beyond
// that, the deposit needs manual review instead of auto-paying the oldest
// eligible quote.
func withinOverpayTolerance(dep, expected *big.Rat) bool {
	fiftyOne := big.NewRat(51, 50)
	limit := new(big.Rat).Mul(expected, fiftyOne)
	return dep.Cmp(limit) <= 0
}
