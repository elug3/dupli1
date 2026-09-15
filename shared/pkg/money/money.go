// Package money defines Dupli1's single storefront currency (KRW) and amount helpers.
//
// Catalog prices (product.price) and cart/order/payment integer fields use the same
// unit: whole Korean won. For KRW (a zero-decimal currency) that is whole won —
// not won×100.
//
// Money fields are named *_won everywhere: JSON, Go identifiers, and Postgres
// columns. Two dead names come before it — *_cents (a Stripe "minor units"
// leftover, renamed 2026-09-09) and *_krw (canonical for the five days between,
// renamed to *_won on 2026-09-14). Nothing emits either one.
//
// They survive only as read-side aliases, and that is the trap: a producer that
// sends an old name is accepted on the few paths that decode it (the
// UnmarshalJSON shims in shared/pkg/events, payment's cancel body and its order
// client, shipping_fee in the storefront checkout) and reads as zero everywhere
// else. Existing databases rename leftover *_krw / *_cents columns on migrate.
// Write *_won.
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

// FormatWon formats an integer won amount for display (e.g. Telegram alerts).
func FormatWon(amount int64) string {
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
