package checkout

import (
	"context"
	"fmt"

	"github.com/elug3/dupli1/payment/pkg/ports"
)

// UnavailableProvider rejects credit-card checkout when no PG is configured
// (typical until a PG is contracted; use method=bypass for manager-recorded
// payments in the meantime, including local development).
type UnavailableProvider struct {
	Reason string
}

func NewUnavailableProvider(reason string) *UnavailableProvider {
	if reason == "" {
		reason = "credit card checkout is not configured"
	}
	return &UnavailableProvider{Reason: reason}
}

func (p *UnavailableProvider) CreateSession(_ context.Context, _ ports.CheckoutSessionInput) (*ports.CheckoutSessionResult, error) {
	return nil, fmt.Errorf("%w: %s", ports.ErrMethodUnavailable, p.Reason)
}
