package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/elug3/dupli1/notification/pkg/ports"
	"github.com/elug3/dupli1/shared/pkg/events"
	"github.com/elug3/dupli1/shared/pkg/money"
)

// Subject aliases of the shared event contract — see shared/pkg/events.
const (
	SubjectOrderCreated      = events.OrderCreated
	SubjectOrderStatusUpdate = events.OrderStatusUpdate
	SubjectOrderPaid         = events.OrderPaid
	SubjectProductCreated    = events.ProductCreated
	SubjectProductUpdated    = events.ProductUpdated
	SubjectProductDeleted    = events.ProductDeleted
	SubjectProductImage      = events.ProductImage
	SubjectPaymentCanceled   = events.PaymentCanceled
)

type ChatRouting interface {
	OrderChatID(ctx context.Context) string
	ProductChatID(ctx context.Context) string
}

type DispatcherConfig struct {
	Routing       ChatRouting
	OrderChatID   string
	ProductChatID string
	ManageWebURL  string
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
		SubjectOrderPaid,
		SubjectPaymentCanceled,
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
	case SubjectOrderCreated, SubjectOrderStatusUpdate, SubjectOrderPaid:
		return d.handleOrder(ctx, subject, payload)
	case SubjectPaymentCanceled:
		return d.handlePaymentCanceled(ctx, payload)
	case SubjectProductCreated, SubjectProductUpdated, SubjectProductDeleted, SubjectProductImage:
		return d.handleProduct(ctx, subject, payload)
	default:
		return nil
	}
}

func (d *Dispatcher) handleOrder(ctx context.Context, subject string, payload []byte) error {
	var event events.Order
	if err := json.Unmarshal(payload, &event); err != nil {
		return fmt.Errorf("decode order event: %w", err)
	}

	chatID := strings.TrimSpace(d.orderChatID(ctx))
	if chatID == "" {
		log.Printf("order event %s for %s skipped: order telegram chat not configured", subject, event.OrderID)
		return nil
	}

	message := formatOrderMessage(subject, event, d.cfg.ManageWebURL)
	if err := d.notifier.Send(ctx, chatID, message); err != nil {
		return fmt.Errorf("notify order event: %w", err)
	}
	return nil
}

// handlePaymentCanceled alerts ops that money went back to a customer. Refunds
// were previously silent: order.* events covered creation and payment, so a
// cancelled payment produced no message at all and the only trace was a row in
// the payments table.
func (d *Dispatcher) handlePaymentCanceled(ctx context.Context, payload []byte) error {
	var event events.PaymentCanceledEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return fmt.Errorf("decode payment.canceled event: %w", err)
	}

	chatID := strings.TrimSpace(d.orderChatID(ctx))
	if chatID == "" {
		log.Printf("payment.canceled for %s skipped: order telegram chat not configured", event.OrderID)
		return nil
	}

	if err := d.notifier.Send(ctx, chatID, formatPaymentCanceledMessage(event, d.cfg.ManageWebURL)); err != nil {
		return fmt.Errorf("notify payment.canceled: %w", err)
	}
	return nil
}

// formatPaymentCanceledMessage distinguishes a full refund from a partial one:
// a full refund cancels the order automatically, a partial leaves it standing
// with money still owed, and ops need to know which they are looking at.
func formatPaymentCanceledMessage(event events.PaymentCanceledEvent, manageWebURL string) string {
	manageLink := formatManageOrderLink(manageWebURL, event.OrderID)
	reason := strings.TrimSpace(event.Reason)
	reasonLine := ""
	if reason != "" {
		reasonLine = fmt.Sprintf("Reason: %s\n", escapeHTML(reason))
	}
	byLine := ""
	if by := strings.TrimSpace(event.CanceledBy); by != "" {
		byLine = fmt.Sprintf("By: %s\n", escapeHTML(by))
	}

	if event.RemainingCents > 0 {
		return fmt.Sprintf(
			"↩️ <b>Partial refund</b> %s\n%sRefunded: <b>%s</b>\nStill captured: <b>%s</b>\n%s%sOrder is unchanged — review whether it should still ship.",
			escapeHTML(event.OrderID), manageLink,
			formatMoney(event.AmountCents), formatMoney(event.RemainingCents),
			reasonLine, byLine,
		)
	}
	return fmt.Sprintf(
		"↩️ <b>Refunded in full</b> %s\n%sRefunded: <b>%s</b>\n%s%sOrder has been canceled and its stock released.",
		escapeHTML(event.OrderID), manageLink,
		formatMoney(event.AmountCents), reasonLine, byLine,
	)
}

func (d *Dispatcher) handleProduct(ctx context.Context, subject string, payload []byte) error {
	var event events.Product
	if err := json.Unmarshal(payload, &event); err != nil {
		return fmt.Errorf("decode product event: %w", err)
	}

	chatID := strings.TrimSpace(d.productChatID(ctx))
	if chatID == "" {
		log.Printf("product event %s for %s skipped: product telegram chat not configured", subject, event.ProductID)
		return nil
	}

	message := formatProductMessage(subject, event)
	if err := d.notifier.Send(ctx, chatID, message); err != nil {
		return fmt.Errorf("notify product event: %w", err)
	}
	return nil
}

func (d *Dispatcher) orderChatID(ctx context.Context) string {
	if d.cfg.Routing != nil {
		if id := strings.TrimSpace(d.cfg.Routing.OrderChatID(ctx)); id != "" {
			return id
		}
	}
	return strings.TrimSpace(d.cfg.OrderChatID)
}

func (d *Dispatcher) productChatID(ctx context.Context) string {
	if d.cfg.Routing != nil {
		if id := strings.TrimSpace(d.cfg.Routing.ProductChatID(ctx)); id != "" {
			return id
		}
	}
	return strings.TrimSpace(d.cfg.ProductChatID)
}

func formatOrderMessage(subject string, event events.Order, manageWebURL string) string {
	items := make([]string, 0, len(event.Items))
	for _, item := range event.Items {
		items = append(items, fmt.Sprintf("%d× %s", item.Quantity, escapeHTML(item.SKU)))
	}
	itemsLine := strings.Join(items, ", ")
	if itemsLine == "" {
		itemsLine = "no items"
	}

	total := formatMoney(event.TotalCents)
	createdLine := formatOrderCreatedAt(event.CreatedAt, event.Occurred)
	manageLink := formatManageOrderLink(manageWebURL, event.OrderID)

	switch subject {
	case SubjectOrderPaid:
		return fmt.Sprintf(
			"💳 <b>Order paid — action required</b> %s\n%s%sStatus: <b>paid</b>\nCustomer: %s\nItems: %s\nTotal: <b>%s</b>\nShip when ready.",
			escapeHTML(event.OrderID),
			createdLine,
			manageLink,
			escapeHTML(event.CustomerID),
			itemsLine,
			total,
		)
	case SubjectOrderCreated:
		return fmt.Sprintf(
			"🛒 <b>New order</b> %s\n%s%sStatus: <b>%s</b>\nCustomer: %s\nItems: %s\nTotal: <b>%s</b>",
			escapeHTML(event.OrderID),
			createdLine,
			manageLink,
			escapeHTML(event.Status),
			escapeHTML(event.CustomerID),
			itemsLine,
			total,
		)
	default:
		return fmt.Sprintf(
			"📦 <b>Order update</b> %s\n%s%sStatus: <b>%s</b>\nCustomer: %s\nTotal: <b>%s</b>",
			escapeHTML(event.OrderID),
			createdLine,
			manageLink,
			escapeHTML(event.Status),
			escapeHTML(event.CustomerID),
			total,
		)
	}
}

func formatOrderCreatedAt(createdAt, occurredAt time.Time) string {
	t := createdAt
	if t.IsZero() {
		t = occurredAt
	}
	if t.IsZero() {
		return ""
	}
	loc, err := time.LoadLocation("Asia/Seoul")
	if err != nil {
		loc = time.UTC
	}
	return fmt.Sprintf("Created: <b>%s</b>\n", escapeHTML(t.In(loc).Format("2006-01-02 15:04 KST")))
}

func formatManageOrderLink(manageWebURL, orderID string) string {
	manageWebURL = strings.TrimRight(strings.TrimSpace(manageWebURL), "/")
	orderID = strings.TrimSpace(orderID)
	if manageWebURL == "" || orderID == "" {
		return ""
	}
	url := fmt.Sprintf("%s/orders/%s", manageWebURL, escapeHTML(orderID))
	return fmt.Sprintf("<a href=\"%s\">View order in manage-web</a>\n", url)
}

func formatProductMessage(subject string, event events.Product) string {
	price := money.FormatKRW(money.FromProductPrice(event.Price))
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

func formatMoney(won int64) string {
	return money.FormatKRW(won)
}

func escapeHTML(value string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
	)
	return replacer.Replace(strings.TrimSpace(value))
}
