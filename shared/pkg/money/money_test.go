package money_test

import (
	"testing"

	"github.com/elug3/dupli1/shared/pkg/money"
)

func TestFromProductPrice(t *testing.T) {
	if got := money.FromProductPrice(2890000); got != 2890000 {
		t.Fatalf("FromProductPrice(2890000)=%d, want 2890000 (no ×100)", got)
	}
	if got := money.FromProductPrice(1990.4); got != 1990 {
		t.Fatalf("FromProductPrice(1990.4)=%d, want 1990", got)
	}
	if got := money.FromProductPrice(-1); got != 0 {
		t.Fatalf("negative → 0, got %d", got)
	}
}

func TestNormalizeCurrency(t *testing.T) {
	got, err := money.NormalizeCurrency("")
	if err != nil || got != money.Currency {
		t.Fatalf("empty: got %q err %v", got, err)
	}
	got, err = money.NormalizeCurrency("KRW")
	if err != nil || got != money.Currency {
		t.Fatalf("KRW: got %q err %v", got, err)
	}
	if _, err := money.NormalizeCurrency("usd"); err == nil {
		t.Fatal("usd should be rejected")
	}
}

func TestFormatKRW(t *testing.T) {
	if got := money.FormatKRW(2890000); got != "₩2,890,000" {
		t.Fatalf("FormatKRW(2890000)=%q", got)
	}
	if got := money.FormatKRW(0); got != "₩0" {
		t.Fatalf("FormatKRW(0)=%q", got)
	}
	if got := money.FormatKRW(-1500); got != "-₩1,500" {
		t.Fatalf("FormatKRW(-1500)=%q", got)
	}
}
