package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/elug3/dupli1/notification/pkg/ports"
)

const (
	SubjectOrderCreated      = "order.created"
	SubjectOrderStatusUpdate = "order.status_updated"
	SubjectProductCreated    = "product.created"
	SubjectProductUpdated    = "product.updated"
	SubjectProductDeleted    = "product.deleted"
	SubjectProductImage      = "product.image_uploaded"
)

type DispatcherConfig struct {
	OrderChatID   string
	ProductChatID string
}

type Dispatcher struct {
	notifier ports.Notifier
	cfg      DispatcherConfig
}

func NewDispatcher(notifier ports.Notifier, cfg DispatcherConfig) *Dispatcher {
	return &Dispatcher{notifier: notifier, cfg: cfg}
}

func (d *Dispatcher) Register(subscriber ports.EventSubscriber, ctx context.Context) error {
	subjects := []string{
		SubjectOrderCreated,
		SubjectOrderStatusUpdate,
		SubjectProductCreated,
		SubjectProductUpdated,
		SubjectProductDeleted,
		SubjectProductImage,
	}
	for _, subject := range subjects {
		if err := subscriber.Subscribe(ctx, subject, d.handle); err != nil {
			return err
		}
	}
	return nil
}

// HandleForTest exposes event handling for unit tests.
func (d *Dispatcher) HandleForTest(ctx context.Context, subject string, payload []byte) error {
	return d.handle(ctx, subject, payload)
}

func (d *Dispatcher) handle(ctx context.Context, subject string, payload []byte) error {
	switch subject {
	case SubjectOrderCreated, SubjectOrderStatusUpdate:
		return d.handleOrder(ctx, subject, payload)
	case SubjectProductCreated, SubjectProductUpdated, SubjectProductDeleted, SubjectProductImage:
		return d.handleProduct(ctx, subject, payload)
	default:
		return nil
	}
}

type orderEvent struct {
	EventType     string          `json:"event_type"`
	OrderID       string          `json:"order_id"`
	CustomerID    string          `json:"customer_id"`
	Status        string          `json:"status"`
	SubtotalCents int64           `json:"subtotal_cents"`
	DiscountCents int64           `json:"discount_cents"`
	TotalCents    int64           `json:"total_cents"`
	Items         []orderItemView `json:"items"`
	Occurred      time.Time       `json:"occurred_at"`
}

type orderItemView struct {
	SKU            string `json:"sku"`
	Quantity       int    `json:"quantity"`
	UnitPriceCents int64  `json:"unit_price_cents"`
}

func (d *Dispatcher) handleOrder(ctx context.Context, subject string, payload []byte) error {
	var event orderEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return fmt.Errorf("decode order event: %w", err)
	}

	chatID := strings.TrimSpace(d.cfg.OrderChatID)
	if chatID == "" {
		log.Printf("order event %s for %s skipped: TELEGRAM_ORDER_CHAT_ID not set", subject, event.OrderID)
		return nil
	}

	message := formatOrderMessage(subject, event)
	if err := d.notifier.Send(ctx, chatID, message); err != nil {
		return fmt.Errorf("notify order event: %w", err)
	}
	return nil
}

type productEvent struct {
	EventType string    `json:"event_type"`
	ProductID string    `json:"product_id"`
	Name      string    `json:"name"`
	Brand     string    `json:"brand"`
	Category  string    `json:"category"`
	Status    string    `json:"status"`
	Price     float64   `json:"price"`
	ImageURL  string    `json:"image_url,omitempty"`
	Occurred  time.Time `json:"occurred_at"`
}

func (d *Dispatcher) handleProduct(ctx context.Context, subject string, payload []byte) error {
	var event productEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return fmt.Errorf("decode product event: %w", err)
	}

	chatID := strings.TrimSpace(d.cfg.ProductChatID)
	if chatID == "" {
		log.Printf("product event %s for %s skipped: TELEGRAM_PRODUCT_CHAT_ID not set", subject, event.ProductID)
		return nil
	}

	message := formatProductMessage(subject, event)
	if err := d.notifier.Send(ctx, chatID, message); err != nil {
		return fmt.Errorf("notify product event: %w", err)
	}
	return nil
}

func formatOrderMessage(subject string, event orderEvent) string {
	items := make([]string, 0, len(event.Items))
	for _, item := range event.Items {
		items = append(items, fmt.Sprintf("%d× %s", item.Quantity, escapeHTML(item.SKU)))
	}
	itemsLine := strings.Join(items, ", ")
	if itemsLine == "" {
		itemsLine = "no items"
	}

	total := formatMoney(event.TotalCents)
	switch subject {
	case SubjectOrderCreated:
		return fmt.Sprintf(
			"🛒 <b>New order</b> %s\nStatus: <b>%s</b>\nCustomer: %s\nItems: %s\nTotal: <b>%s</b>",
			escapeHTML(event.OrderID),
			escapeHTML(event.Status),
			escapeHTML(event.CustomerID),
			itemsLine,
			total,
		)
	default:
		return fmt.Sprintf(
			"📦 <b>Order update</b> %s\nStatus: <b>%s</b>\nCustomer: %s\nTotal: <b>%s</b>",
			escapeHTML(event.OrderID),
			escapeHTML(event.Status),
			escapeHTML(event.CustomerID),
			total,
		)
	}
}

func formatProductMessage(subject string, event productEvent) string {
	price := fmt.Sprintf("€%.2f", event.Price)
	name := escapeHTML(event.Name)
	brand := escapeHTML(event.Brand)
	id := escapeHTML(event.ProductID)

	switch subject {
	case SubjectProductCreated:
		return fmt.Sprintf("📦 <b>Product created</b>\n%s — %s (%s)\nCategory: %s\nStatus: %s\nPrice: %s",
			id, name, brand, escapeHTML(event.Category), escapeHTML(event.Status), price)
	case SubjectProductUpdated:
		return fmt.Sprintf("✏️ <b>Product updated</b>\n%s — %s (%s)\nStatus: %s\nPrice: %s",
			id, name, brand, escapeHTML(event.Status), price)
	case SubjectProductDeleted:
		return fmt.Sprintf("🗑️ <b>Product deleted</b>\n%s — %s", id, name)
	case SubjectProductImage:
		return fmt.Sprintf("🖼️ <b>Product image uploaded</b>\n%s — %s\n%s",
			id, name, escapeHTML(event.ImageURL))
	default:
		return fmt.Sprintf("Product event %s for %s", escapeHTML(subject), id)
	}
}

func formatMoney(cents int64) string {
	sign := ""
	if cents < 0 {
		sign = "-"
		cents = -cents
	}
	return fmt.Sprintf("%s€%d.%02d", sign, cents/100, cents%100)
}

func escapeHTML(value string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
	)
	return replacer.Replace(strings.TrimSpace(value))
}
