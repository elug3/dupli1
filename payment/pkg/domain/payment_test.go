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
	if p.Method != domain.MethodCreditCard {
		t.Fatalf("method=%q, want %q", p.Method, domain.MethodCreditCard)
	}
}

func TestNormalizeMethod(t *testing.T) {
	got, err := domain.NormalizeMethod("")
	if err != nil || got != domain.MethodCreditCard {
		t.Fatalf("empty: got=%q err=%v", got, err)
	}
	got, err = domain.NormalizeMethod("BYPASS")
	if err != nil || got != domain.MethodBypass {
		t.Fatalf("bypass: got=%q err=%v", got, err)
	}
	_, err = domain.NormalizeMethod("paypal")
	if err != domain.ErrInvalidPayment {
		t.Fatalf("unknown: err=%v", err)
	}
}
