package cryptopay

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// SpotRateEUR fetches the latest OKX spot price (EUR per unit of asset) from
// the public market ticker. No API key is required.
func SpotRateEUR(ctx context.Context, client *http.Client, baseURL, asset string) (rate string, quotedAt time.Time, err error) {
	instID, err := okxInstrument(asset)
	if err != nil {
		return "", time.Time{}, err
	}
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	endpoint := strings.TrimRight(baseURL, "/") + "/api/v5/market/ticker?instId=" + url.QueryEscape(instID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", time.Time{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("cours OKX injoignable: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("cours OKX illisible: %w", err)
	}
	if resp.StatusCode >= 400 {
		return "", time.Time{}, fmt.Errorf("cours OKX indisponible (%d)", resp.StatusCode)
	}
	var decoded struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
		Data []struct {
			Last string `json:"last"`
			Ts   string `json:"ts"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return "", time.Time{}, fmt.Errorf("cours OKX invalide: %w", err)
	}
	if decoded.Code != "0" || len(decoded.Data) == 0 {
		return "", time.Time{}, fmt.Errorf("cours OKX indisponible (%s)", decoded.Msg)
	}
	last := strings.TrimSpace(decoded.Data[0].Last)
	rateFloat, _, err := big.ParseFloat(last, 10, 256, big.ToNearestEven)
	if err != nil || rateFloat.Sign() <= 0 {
		return "", time.Time{}, fmt.Errorf("cours OKX invalide")
	}
	quotedAt = time.Now().UTC()
	if tsMillis, err := strconv.ParseInt(strings.TrimSpace(decoded.Data[0].Ts), 10, 64); err == nil && tsMillis > 0 {
		quotedAt = time.UnixMilli(tsMillis).UTC()
	}
	return last, quotedAt, nil
}

func okxInstrument(asset string) (string, error) {
	switch strings.ToUpper(strings.TrimSpace(asset)) {
	case AssetBTC:
		return "BTC-EUR", nil
	case AssetXRP:
		return "XRP-EUR", nil
	default:
		return "", fmt.Errorf("actif crypto invalide")
	}
}
