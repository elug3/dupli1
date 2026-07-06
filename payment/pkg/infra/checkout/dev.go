package checkout

import (
	"context"
	"fmt"
	"strings"

	"github.com/elug3/dupli1/payment/pkg/ports"
)

// DevProvider returns a local simulate URL when Stripe is not configured.
type DevProvider struct {
	publicBaseURL string
}

func NewDevProvider(publicBaseURL string) *DevProvider {
	return &DevProvider{publicBaseURL: strings.TrimRight(publicBaseURL, "/")}
}

func (p *DevProvider) CreateSession(_ context.Context, input ports.CheckoutSessionInput) (*ports.CheckoutSessionResult, error) {
	ref := "dev_cs_" + input.PaymentID
	url := fmt.Sprintf("%s/api/v1/payments/%s/simulate-success", p.publicBaseURL, input.PaymentID)
	return &ports.CheckoutSessionResult{
		ProviderRef: ref,
		CheckoutURL: url,
	}, nil
}
