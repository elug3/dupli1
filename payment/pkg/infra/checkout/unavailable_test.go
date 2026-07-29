package checkout

import (
	"context"
	"testing"

	"github.com/elug3/dupli1/payment/pkg/ports"
)

func TestUnavailableProviderRejectsSession(t *testing.T) {
	p := NewUnavailableProvider("no pg")
	_, err := p.CreateSession(context.Background(), ports.CheckoutSessionInput{PaymentID: "p1"})
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "no pg" {
		t.Fatalf("err = %v", err)
	}
}
