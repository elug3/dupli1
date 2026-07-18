// Package money defines Dupli1's single storefront currency (KRW) and amount helpers.
//
// Catalog prices (product.price) and cart/order/payment integer fields use the same
// unit: whole Korean won. JSON fields historically named *_cents are Stripe "minor
// units"; for KRW (a zero-decimal currency) that is whole won — not won×100.
package money

import (
	"fmt"
	"math"
	"strings"
)

// Currency is the only supported storefront / payment currency.
const Currency = "krw"

// FromProductPrice converts a product catalog price (KRW won) to the integer
// amount used by cart, order, and payment. Product prices are already in won;
// do not multiply by 100 (that would treat them as USD-style major units).
func FromProductPrice(price float64) int64 {
	if price <= 0 || math.IsNaN(price) || math.IsInf(price, 0) {
		return 0
	}
	return int64(math.Round(price))
}

// NormalizeCurrency returns Currency when empty or already krw; otherwise an error.
func NormalizeCurrency(raw string) (string, error) {
	c := strings.ToLower(strings.TrimSpace(raw))
	if c == "" || c == Currency {
		return Currency, nil
	}
	return "", fmt.Errorf("unsupported currency %q: only %s is allowed", raw, Currency)
}

// FormatKRW formats an integer won amount for display (e.g. Telegram alerts).
func FormatKRW(amount int64) string {
	sign := ""
	if amount < 0 {
		sign = "-"
		amount = -amount
	}
	return sign + "₩" + formatGrouped(amount)
}

func formatGrouped(n int64) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	s := fmt.Sprintf("%d", n)
	var b strings.Builder
	lead := len(s) % 3
	if lead == 0 {
		lead = 3
	}
	b.WriteString(s[:lead])
	for i := lead; i < len(s); i += 3 {
		b.WriteByte(',')
		b.WriteString(s[i : i+3])
	}
	return b.String()
}
