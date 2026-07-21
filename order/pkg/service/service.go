package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/elug3/dupli1/order/pkg/domain"
	"github.com/elug3/dupli1/order/pkg/ports"
)

const (
	orderCreatedSubject     = "order.created"
	orderUpdatedSubject     = "order.status_updated"
	orderPaidSubject        = "order.paid"
	paymentSucceededSubject = "payment.succeeded"
)

type Service struct {
	repo           ports.Repository
	stock          ports.StockClient
	product        ports.ProductClient
	eventPublisher ports.EventPublisher
	couponClient   ports.CouponClient
	checkoutTTL    time.Duration
	now            func() time.Time
}

type CreateOrderInput struct {
	CustomerID     string
	Items          []domain.OrderItem
	CouponCode     string
	DiscountCents  int64
	IdempotencyKey string
}

type orderItemEvent struct {
	SkuID          string `json:"sku_id,omitempty"`
	SKU            string `json:"sku"`
	Quantity       int    `json:"quantity"`
	UnitPriceCents int64  `json:"unit_price_cents"`
}

type orderEvent struct {
	EventType     string             `json:"event_type"`
	OrderID       string             `json:"order_id"`
	CustomerID    string             `json:"customer_id"`
	Status        domain.OrderStatus `json:"status"`
	SubtotalCents int64              `json:"subtotal_cents"`
	DiscountCents int64              `json:"discount_cents"`
	TotalCents    int64              `json:"total_cents"`
	Items         []orderItemEvent   `json:"items"`
	Occurred      time.Time          `json:"occurred_at"`
}

type idempotencyFingerprint struct {
	CustomerID    string `json:"customer_id"`
	CouponCode    string `json:"coupon_code,omitempty"`
	DiscountCents int64  `json:"discount_cents,omitempty"`
	Items         []struct {
		SkuID    string `json:"sku_id,omitempty"`
		SKU      string `json:"sku,omitempty"`
		Quantity int    `json:"quantity"`
	} `json:"items"`
}

func New(repo ports.Repository, stock ports.StockClient, eventPublisher ...ports.EventPublisher) *Service {
	return NewWithCheckout(repo, stock, nil, 0, eventPublisher...)
}

func NewWithCheckout(
	repo ports.Repository,
	stock ports.StockClient,
	couponClient ports.CouponClient,
	checkoutTTL time.Duration,
	eventPublisher ...ports.EventPublisher,
) *Service {
	s := &Service{
		repo:         repo,
		stock:        stock,
		couponClient: couponClient,
		checkoutTTL:  checkoutTTL,
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
	if len(eventPublisher) > 0 {
		s.eventPublisher = eventPublisher[0]
	}
	if s.checkoutTTL <= 0 {
		s.checkoutTTL = domain.DefaultCheckoutTTL
	}
	return s
}

// WithProduct sets the catalog client used to resolve server-side line prices.
func (s *Service) WithProduct(product ports.ProductClient) *Service {
	s.product = product
	return s
}

func (s *Service) CreateOrder(ctx context.Context, input CreateOrderInput) (*domain.Order, error) {
	idemKey := strings.TrimSpace(input.IdempotencyKey)
	reqHash := hashCreateOrderInput(input)

	if idemKey != "" {
		if existing, err := s.loadIdempotentOrder(ctx, input.CustomerID, idemKey, reqHash); err == nil {
			return existing, nil
		} else if !errors.Is(err, ports.ErrNotFound) {
			return nil, err
		}
	}

	pricedItems, err := s.priceItems(ctx, input.Items)
	if err != nil {
		return nil, err
	}

	orderID, err := s.repo.NextOrderID(ctx)
	if err != nil {
		return nil, err
	}

	stockItems := make([]ports.StockItem, len(pricedItems))
	for i, item := range pricedItems {
		stockItems[i] = ports.StockItem{
			SkuID:    item.SkuID,
			SKU:      item.SKU,
			Quantity: item.Quantity,
		}
	}

	reservationID, err := s.stock.Reserve(ctx, orderID, stockItems)
	if err != nil {
		return nil, err
	}

	order, err := domain.NewOrder(orderID, input.CustomerID, reservationID, pricedItems, input.CouponCode, input.DiscountCents, s.now())
	if err != nil {
		_ = s.stock.ReleaseReservation(ctx, reservationID)
		return nil, err
	}

	var idem *ports.IdempotencyRecord
	if idemKey != "" {
		idem = &ports.IdempotencyRecord{
			Key:         idemKey,
			CustomerID:  input.CustomerID,
			OrderID:     order.ID,
			RequestHash: reqHash,
		}
	}
	events, err := s.outboxEvents(order, orderCreatedSubject)
	if err != nil {
		_ = s.stock.ReleaseReservation(ctx, reservationID)
		return nil, err
	}

	if err := s.repo.SaveWithOutbox(ctx, order, idem, events); err != nil {
		_ = s.stock.ReleaseReservation(ctx, reservationID)
		if idemKey != "" {
			if existing, replayErr := s.loadIdempotentOrder(ctx, input.CustomerID, idemKey, reqHash); replayErr == nil {
				return existing, nil
			} else if errors.Is(replayErr, ports.ErrIdempotencyConflict) {
				return nil, replayErr
			}
		}
		return nil, err
	}

	// Soft-success: order is source of truth; outbox worker retries publish.
	s.tryDrainOutbox(ctx)
	return cloneOrder(order), nil
}

func (s *Service) GetOrder(ctx context.Context, id string) (*domain.Order, error) {
	order, err := s.repo.Get(ctx, strings.TrimSpace(id))
	if err != nil {
		return nil, err
	}
	return cloneOrder(order), nil
}

func (s *Service) ListCustomerOrders(ctx context.Context, customerID string) ([]domain.Order, error) {
	orders, err := s.repo.ListByCustomer(ctx, strings.TrimSpace(customerID))
	if err != nil {
		return nil, err
	}
	return cloneOrders(orders), nil
}

func (s *Service) MarkOrderPaid(ctx context.Context, orderID, paymentID string, amountCents int64) (*domain.Order, error) {
	order, err := s.repo.Get(ctx, strings.TrimSpace(orderID))
	if err != nil {
		return nil, err
	}
	if order.Status == domain.StatusPaid && order.PaymentID == paymentID {
		return cloneOrder(order), nil
	}
	if err := order.MarkPaid(paymentID, amountCents, s.now()); err != nil {
		return nil, err
	}
	events, err := s.outboxEvents(order, orderPaidSubject, orderUpdatedSubject)
	if err != nil {
		return nil, err
	}
	if err := s.repo.SaveWithOutbox(ctx, order, nil, events); err != nil {
		return nil, err
	}
	s.tryDrainOutbox(ctx)
	return cloneOrder(order), nil
}

func (s *Service) ShipOrder(ctx context.Context, orderID, shippedBy string) (*domain.Order, error) {
	order, err := s.repo.Get(ctx, strings.TrimSpace(orderID))
	if err != nil {
		return nil, err
	}
	if err := s.stock.CommitReservation(ctx, order.ReservationID); err != nil {
		return nil, err
	}
	if err := order.Ship(shippedBy, s.now()); err != nil {
		return nil, err
	}
	return s.saveStatusChange(ctx, order)
}

func (s *Service) CancelOrder(ctx context.Context, id string) (*domain.Order, error) {
	order, err := s.repo.Get(ctx, strings.TrimSpace(id))
	if err != nil {
		return nil, err
	}
	if order.Status != domain.StatusPending && order.Status != domain.StatusPaid {
		return nil, domain.ErrInvalidTransition
	}
	if err := s.stock.ReleaseReservation(ctx, order.ReservationID); err != nil {
		return nil, err
	}
	if err := order.Cancel(s.now()); err != nil {
		return nil, err
	}
	return s.saveStatusChange(ctx, order)
}

func (s *Service) FulfillOrder(ctx context.Context, id string) (*domain.Order, error) {
	order, err := s.repo.Get(ctx, strings.TrimSpace(id))
	if err != nil {
		return nil, err
	}
	if err := order.Fulfill(s.now()); err != nil {
		return nil, err
	}
	return s.saveStatusChange(ctx, order)
}

func (s *Service) saveStatusChange(ctx context.Context, order *domain.Order) (*domain.Order, error) {
	events, err := s.outboxEvents(order, orderUpdatedSubject)
	if err != nil {
		return nil, err
	}
	if err := s.repo.SaveWithOutbox(ctx, order, nil, events); err != nil {
		return nil, err
	}
	s.tryDrainOutbox(ctx)
	return cloneOrder(order), nil
}

func (s *Service) loadIdempotentOrder(ctx context.Context, customerID, key, reqHash string) (*domain.Order, error) {
	rec, err := s.repo.FindByIdempotencyKey(ctx, customerID, key)
	if err != nil {
		return nil, err
	}
	if rec.RequestHash != reqHash {
		return nil, ports.ErrIdempotencyConflict
	}
	order, err := s.repo.Get(ctx, rec.OrderID)
	if err != nil {
		return nil, err
	}
	return cloneOrder(order), nil
}

func hashCreateOrderInput(input CreateOrderInput) string {
	fp := idempotencyFingerprint{
		CustomerID:    strings.TrimSpace(input.CustomerID),
		CouponCode:    strings.TrimSpace(input.CouponCode),
		DiscountCents: input.DiscountCents,
	}
	fp.Items = make([]struct {
		SkuID    string `json:"sku_id,omitempty"`
		SKU      string `json:"sku,omitempty"`
		Quantity int    `json:"quantity"`
	}, len(input.Items))
	for i, item := range input.Items {
		fp.Items[i].SkuID = strings.TrimSpace(item.SkuID)
		fp.Items[i].SKU = strings.TrimSpace(item.SKU)
		fp.Items[i].Quantity = item.Quantity
	}
	sort.Slice(fp.Items, func(i, j int) bool {
		if fp.Items[i].SkuID != fp.Items[j].SkuID {
			return fp.Items[i].SkuID < fp.Items[j].SkuID
		}
		if fp.Items[i].SKU != fp.Items[j].SKU {
			return fp.Items[i].SKU < fp.Items[j].SKU
		}
		return fp.Items[i].Quantity < fp.Items[j].Quantity
	})
	raw, err := json.Marshal(fp)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func (s *Service) outboxEvents(order *domain.Order, subjects ...string) ([]ports.OutboxEvent, error) {
	events := make([]ports.OutboxEvent, 0, len(subjects))
	for _, subject := range subjects {
		payload, err := s.marshalOrderEvent(subject, order)
		if err != nil {
			return nil, err
		}
		events = append(events, ports.OutboxEvent{
			AggregateID: order.ID,
			Subject:     subject,
			Payload:     payload,
		})
	}
	return events, nil
}

func (s *Service) marshalOrderEvent(subject string, order *domain.Order) ([]byte, error) {
	items := make([]orderItemEvent, len(order.Items))
	for i, item := range order.Items {
		items[i] = orderItemEvent{
			SkuID:          item.SkuID,
			SKU:            item.SKU,
			Quantity:       item.Quantity,
			UnitPriceCents: item.UnitPriceCents,
		}
	}
	payload, err := json.Marshal(orderEvent{
		EventType:     subject,
		OrderID:       order.ID,
		CustomerID:    order.CustomerID,
		Status:        order.Status,
		SubtotalCents: order.SubtotalCents,
		DiscountCents: order.DiscountCents,
		TotalCents:    order.TotalCents,
		Items:         items,
		Occurred:      s.now(),
	})
	if err != nil {
		return nil, fmt.Errorf("marshal %s event: %w", subject, err)
	}
	return payload, nil
}

// StartOutboxWorker periodically publishes pending outbox rows.
func (s *Service) StartOutboxWorker(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 2 * time.Second
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := s.DrainOutbox(ctx); err != nil {
					log.Printf("order outbox drain: %v", err)
				}
			}
		}
	}()
}

func (s *Service) tryDrainOutbox(ctx context.Context) {
	if err := s.DrainOutbox(ctx); err != nil {
		log.Printf("order outbox drain: %v", err)
	}
}

// DrainOutbox publishes pending outbox messages. Failures are recorded and retried later.
func (s *Service) DrainOutbox(ctx context.Context) error {
	if s.eventPublisher == nil {
		// No broker configured: mark pending rows published so they do not accumulate in tests.
		msgs, err := s.repo.ListPendingOutbox(ctx, 100)
		if err != nil {
			return err
		}
		for _, msg := range msgs {
			if err := s.repo.MarkOutboxPublished(ctx, msg.ID); err != nil {
				return err
			}
		}
		return nil
	}

	msgs, err := s.repo.ListPendingOutbox(ctx, 50)
	if err != nil {
		return err
	}
	var firstErr error
	for _, msg := range msgs {
		if err := s.eventPublisher.Publish(ctx, msg.Subject, json.RawMessage(msg.Payload)); err != nil {
			_ = s.repo.RecordOutboxAttempt(ctx, msg.ID, err.Error())
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if err := s.repo.MarkOutboxPublished(ctx, msg.ID); err != nil {
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// priceItems resolves each line from the product catalog and ignores any client unit_price_cents.
func (s *Service) priceItems(ctx context.Context, items []domain.OrderItem) ([]domain.OrderItem, error) {
	if s.product == nil {
		return nil, ports.ErrProductUnavailable
	}
	if len(items) == 0 {
		return nil, domain.ErrInvalidOrder
	}
	out := make([]domain.OrderItem, len(items))
	for i, item := range items {
		info, err := s.resolveVariant(ctx, item)
		if err != nil {
			return nil, err
		}
		if info.UnitPriceCents <= 0 {
			return nil, domain.ErrInvalidOrder
		}
		out[i] = domain.OrderItem{
			SkuID:          info.SkuID,
			SKU:            info.SKU,
			Quantity:       item.Quantity,
			UnitPriceCents: info.UnitPriceCents,
		}
	}
	return out, nil
}

func (s *Service) resolveVariant(ctx context.Context, item domain.OrderItem) (*ports.VariantInfo, error) {
	skuID := strings.TrimSpace(item.SkuID)
	sku := strings.TrimSpace(item.SKU)
	var (
		info *ports.VariantInfo
		err  error
	)
	switch {
	case skuID != "":
		info, err = s.product.GetVariantBySkuID(ctx, skuID)
	case sku != "":
		info, err = s.product.GetVariant(ctx, sku)
	default:
		return nil, domain.ErrInvalidOrder
	}
	if err != nil {
		if errors.Is(err, ports.ErrVariantNotFound) {
			return nil, err
		}
		return nil, ports.ErrProductUnavailable
	}
	return info, nil
}

func cloneOrder(order *domain.Order) *domain.Order {
	if order == nil {
		return nil
	}
	copied := *order
	copied.Items = make([]domain.OrderItem, len(order.Items))
	copy(copied.Items, order.Items)
	return &copied
}

func cloneOrders(orders []domain.Order) []domain.Order {
	copied := make([]domain.Order, len(orders))
	for i := range orders {
		copied[i] = *cloneOrder(&orders[i])
	}
	return copied
}
