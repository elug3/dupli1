package domain_test

import (
	"testing"
	"time"

	"github.com/elug3/dupli1/payment/pkg/domain"
)

func TestNewPaymentRejectsNonKRW(t *testing.T) {
	now := time.Now().UTC()
	_, err := domain.NewPayment("pay-1", "ord-1", "cust-1", 1000, "usd", "stripe", "ref", "https://example", now)
	if err != domain.ErrInvalidPayment {
		t.Fatalf("usd: err=%v, want ErrInvalidPayment", err)
	}
	p, err := domain.NewPayment("pay-1", "ord-1", "cust-1", 1000, "", "stripe", "ref", "https://example", now)
	if err != nil {
		t.Fatal(err)
	}
	if p.Currency != domain.DefaultCurrency {
		t.Fatalf("currency=%q, want %q", p.Currency, domain.DefaultCurrency)
	}
}
