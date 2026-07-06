package checkout

import (
	"context"
	"fmt"
	"strings"

	"github.com/elug3/dupli1/payment/pkg/ports"
	"github.com/stripe/stripe-go/v81"
	"github.com/stripe/stripe-go/v81/checkout/session"
)

type StripeProvider struct {
	successURL string
	cancelURL  string
}

func NewStripeProvider(secretKey, successURL, cancelURL string) *StripeProvider {
	stripe.Key = secretKey
	return &StripeProvider{successURL: successURL, cancelURL: cancelURL}
}

func (p *StripeProvider) CreateSession(ctx context.Context, input ports.CheckoutSessionInput) (*ports.CheckoutSessionResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	params := &stripe.CheckoutSessionParams{
		Mode: stripe.String(string(stripe.CheckoutSessionModePayment)),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				PriceData: &stripe.CheckoutSessionLineItemPriceDataParams{
					Currency: stripe.String(input.Currency),
					ProductData: &stripe.CheckoutSessionLineItemPriceDataProductDataParams{
						Name: stripe.String("Order " + input.OrderID),
					},
					UnitAmount: stripe.Int64(input.AmountCents),
				},
				Quantity: stripe.Int64(1),
			},
		},
		SuccessURL: stripe.String(appendQuery(p.successURL, "order_id", input.OrderID, "payment_id", input.PaymentID)),
		CancelURL:  stripe.String(appendQuery(p.cancelURL, "order_id", input.OrderID, "payment_id", input.PaymentID)),
		Metadata: map[string]string{
			"order_id":   input.OrderID,
			"payment_id": input.PaymentID,
		},
	}
	params.Context = ctx

	sess, err := session.New(params)
	if err != nil {
		return nil, fmt.Errorf("stripe checkout session: %w", err)
	}
	return &ports.CheckoutSessionResult{
		ProviderRef: sess.ID,
		CheckoutURL: sess.URL,
	}, nil
}

func appendQuery(base, k1, v1, k2, v2 string) string {
	sep := "?"
	if strings.Contains(base, "?") {
		sep = "&"
	}
	return fmt.Sprintf("%s%s%s=%s&%s=%s", base, sep, k1, v1, k2, v2)
}
