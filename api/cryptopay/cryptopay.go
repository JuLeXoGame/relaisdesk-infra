// Package cryptopay implements the BTC/XRP collection building blocks of the
// crypto payment specification (docs/CAHIER_TECHNIQUE_PAIEMENT_CRYPTO_2026-09-21.md):
// OKX spot rates, exact crypto amounts and OKX deposit matching.
package cryptopay

import (
	"fmt"
	"math/big"
	"strconv"
	"strings"
)

// Supported assets.
const (
	AssetBTC = "BTC"
	AssetXRP = "XRP"
)

// AssetDecimals returns the on-chain decimal precision of an asset.
func AssetDecimals(asset string) (int, error) {
	switch strings.ToUpper(strings.TrimSpace(asset)) {
	case AssetBTC:
		return 8, nil
	case AssetXRP:
		return 6, nil
	default:
		return 0, fmt.Errorf("actif crypto invalide")
	}
}

// PaymentMethodForAsset maps an asset to its order payment method.
func PaymentMethodForAsset(asset string) (string, error) {
	switch strings.ToUpper(strings.TrimSpace(asset)) {
	case AssetBTC:
		return "crypto_btc", nil
	case AssetXRP:
		return "crypto_xrp", nil
	default:
		return "", fmt.Errorf("actif crypto invalide")
	}
}

// AssetForPaymentMethod maps an order payment method back to its asset.
func AssetForPaymentMethod(method string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(method)) {
	case "crypto_btc":
		return AssetBTC, nil
	case "crypto_xrp":
		return AssetXRP, nil
	default:
		return "", fmt.Errorf("moyen de paiement crypto invalide")
	}
}

// AssetLabel is the human label used on invoices and emails.
func AssetLabel(asset string) string {
	switch strings.ToUpper(strings.TrimSpace(asset)) {
	case AssetBTC:
		return "Bitcoin (BTC)"
	case AssetXRP:
		return "XRP"
	default:
		return strings.ToUpper(strings.TrimSpace(asset))
	}
}

// CryptoAmount converts a EUR price to crypto units at the given spot rate,
// rounded UP to decimals so rounding never underpays the merchant. Exact
// rational arithmetic keeps exact quotients exact (no float drift).
func CryptoAmount(priceEUR float64, rateEUR string, decimals int) (string, error) {
	if priceEUR <= 0 {
		return "", fmt.Errorf("montant euros invalide")
	}
	if decimals < 0 || decimals > 18 {
		return "", fmt.Errorf("précision invalide")
	}
	rate, ok := new(big.Rat).SetString(strings.TrimSpace(rateEUR))
	if !ok || rate.Sign() <= 0 {
		return "", fmt.Errorf("cours invalide")
	}
	price, ok := new(big.Rat).SetString(strconv.FormatFloat(priceEUR, 'f', 2, 64))
	if !ok || price.Sign() <= 0 {
		return "", fmt.Errorf("montant euros invalide")
	}
	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
	scaled := new(big.Rat).Quo(price, rate)
	scaled.Mul(scaled, new(big.Rat).SetInt(scale))
	units := new(big.Int).Quo(scaled.Num(), scaled.Denom())
	if new(big.Int).Rem(scaled.Num(), scaled.Denom()).Sign() != 0 {
		units.Add(units, big.NewInt(1)) // ceil: never underpay on rounding
	}
	if units.Sign() <= 0 {
		return "", fmt.Errorf("montant crypto nul")
	}
	whole := new(big.Int).Quo(units, scale)
	frac := new(big.Int).Rem(units, scale)
	fracStr := frac.String()
	if len(fracStr) < decimals {
		fracStr = strings.Repeat("0", decimals-len(fracStr)) + fracStr
	}
	return whole.String() + "." + fracStr, nil
}

