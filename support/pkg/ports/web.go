package ports

import (
	"context"
	"errors"
	"time"
)

// ErrReferenceNotFound means a product or order a message referred to does not
// exist — or, for an order, does not belong to whoever asked. The two are one
// error on purpose: telling a shopper "that order exists but is not yours"
// would confirm someone else's order id.
var ErrReferenceNotFound = errors.New("reference not found")

// ProductRef is what a product reference card shows.
type ProductRef struct {
	ProductID string `json:"product_id"`
	SkuID     string `json:"sku_id"`
	SKU       string `json:"sku"`
	Name      string `json:"name"`
	Color     string `json:"color,omitempty"`
	PriceWon  int64  `json:"price_won"`
	ImageURL  string `json:"image_url,omitempty"`
}

// OrderRef is what an order reference card shows.
type OrderRef struct {
	OrderID       string    `json:"order_id"`
	Status        string    `json:"status"`
	TotalWon      int64     `json:"total_won"`
	FirstItemName string    `json:"first_item_name,omitempty"`
	ItemCount     int       `json:"item_count"`
	CreatedAt     time.Time `json:"created_at,omitempty"`
}

// ProductReader looks a sellable variant up in the catalog.
type ProductReader interface {
	// Variant returns ErrReferenceNotFound when no variant has that SKU id.
	Variant(ctx context.Context, skuID string) (*ProductRef, error)
}

// OrderReader reads one order under the caller's own authority.
//
// The bearer token is forwarded as is, so order's own ABAC decides: a shopper
// reads only their own orders, a manager with order.read.all reads any. Support
// gains no privilege of its own here.
type OrderReader interface {
	// Order returns ErrReferenceNotFound for a missing order and for one the
	// token may not read.
	Order(ctx context.Context, bearer, orderID string) (*OrderRef, error)
}

// ShopperNotifier tells a shopper a manager has replied.
type ShopperNotifier interface {
	// NotifyReply sends one notice. subject is what the consultation is about
	// (a product or order name, or empty), link opens the chat. The reply
	// text itself is never passed: it may hold order and address details and
	// would then live in a mailbox indefinitely.
	NotifyReply(ctx context.Context, to, subject, link string) error
}

// LiveEvent is one change an open chat stream should hear about.
type LiveEvent struct {
	Type           string // LiveMessage or LiveInquiry
	InquiryID      string
	ConversationID string
	CustomerID     string
	Channel        string
	MessageID      string
	Status         string
}

// Live event types.
const (
	LiveMessage = "message"
	LiveInquiry = "inquiry"
)

// LivePublisher announces changes to open chat streams. Implementations may
// fan out across replicas; a lost event costs only freshness, since every
// client reloads through the REST API on reconnect.
type LivePublisher interface {
	PublishLive(ctx context.Context, event LiveEvent)
}
