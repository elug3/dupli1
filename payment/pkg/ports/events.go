package ports

import "context"

const PaymentSucceededSubject = "payment.succeeded"

type PaymentSucceededEvent struct {
	EventType   string `json:"event_type"`
	OrderID     string `json:"order_id"`
	PaymentID   string `json:"payment_id"`
	AmountCents int64  `json:"amount_cents"`
}

type EventPublisher interface {
	Publish(ctx context.Context, subject string, event any) error
}
