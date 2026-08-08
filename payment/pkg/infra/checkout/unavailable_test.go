package checkout

import (
	"context"
	"errors"
	"testing"

	"github.com/elug3/dupli1/payment/pkg/ports"
)

func TestUnavailableProviderRejectsSession(t *testing.T) {
	p := NewUnavailableProvider("no pg")
	_, err := p.CreateSession(context.Background(), ports.CheckoutSessionInput{PaymentID: "p1"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ports.ErrMethodUnavailable) {
		t.Fatalf("err = %v, want ErrMethodUnavailable", err)
	}
}
