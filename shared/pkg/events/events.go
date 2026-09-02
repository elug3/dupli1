// Package events defines the NATS subject names and payload shapes shared
// between a publishing service and its subscriber(s): order publishes
// order.* (notification subscribes), payment publishes payment.succeeded
// (order subscribes), and product publishes product.* (notification
// subscribes). Each subject has exactly one publisher and one or more
// subscribers that must agree on the exact string and payload fields, so
// both are defined once here rather than redeclared per service.
package events

import "time"

// Subject names published over NATS.
const (
	OrderCreated      = "order.created"
	OrderStatusUpdate = "order.status_updated"
	OrderPaid         = "order.paid"
	PaymentSucceeded  = "payment.succeeded"
	ProductCreated    = "product.created"
	ProductUpdated    = "product.updated"
	ProductDeleted    = "product.deleted"
	ProductImage      = "product.image_uploaded"
)

// OrderItem is one line of an Order event payload.
type OrderItem struct {
	SkuID          string `json:"sku_id,omitempty"`
	SKU            string `json:"sku"`
	Quantity       int    `json:"quantity"`
	UnitPriceCents int64  `json:"unit_price_cents"`
}

// Order is the payload for OrderCreated, OrderStatusUpdate, and OrderPaid —
// published by order, consumed by notification.
type Order struct {
	EventType     string      `json:"event_type"`
	OrderID       string      `json:"order_id"`
	CustomerID    string      `json:"customer_id"`
	Status        string      `json:"status"`
	SubtotalCents int64       `json:"subtotal_cents"`
	DiscountCents int64       `json:"discount_cents"`
	TotalCents    int64       `json:"total_cents"`
	Items         []OrderItem `json:"items"`
	CreatedAt     time.Time   `json:"created_at"`
	Occurred      time.Time   `json:"occurred_at"`
}

// Product is the payload for ProductCreated, ProductUpdated,
// ProductDeleted, and ProductImage — published by product, consumed by
// notification. Also reused (with a service-local subject) for product's
// unconsumed variant_created/updated/deleted events.
type Product struct {
	EventType string    `json:"event_type"`
	ProductID string    `json:"product_id"`
	SKU       string    `json:"sku,omitempty"`
	Name      string    `json:"name"`
	Brand     string    `json:"brand"`
	Category  string    `json:"category"`
	Status    string    `json:"status"`
	Price     float64   `json:"price"`
	ImageURL  string    `json:"image_url,omitempty"`
	Occurred  time.Time `json:"occurred_at"`
}

// PaymentSucceededEvent is the payload for PaymentSucceeded — published by
// payment, consumed by order.
type PaymentSucceededEvent struct {
	EventType   string `json:"event_type"`
	OrderID     string `json:"order_id"`
	PaymentID   string `json:"payment_id"`
	AmountCents int64  `json:"amount_cents"`
}
