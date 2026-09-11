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
	"github.com/elug3/dupli1/shared/pkg/events"
	"github.com/elug3/dupli1/shared/pkg/outbox"
)

// Subject aliases of the shared event contract — see shared/pkg/events.
const (
	orderCreatedSubject     = events.OrderCreated
	orderUpdatedSubject     = events.OrderStatusUpdate
	orderPaidSubject        = events.OrderPaid
	paymentSucceededSubject = events.PaymentSucceeded
	paymentCanceledSubject  = events.PaymentCanceled
)

type Service struct {
	repo           ports.Repository
	stock          ports.StockClient
	product        ports.ProductClient
	payment        ports.PaymentClient
	eventPublisher ports.EventPublisher
	outboxDrainer  *outbox.Drainer
	couponClient   ports.CouponClient
	checkoutTTL    time.Duration
	// shippingFeeKRW is the flat delivery charge applied to every order, in
	// whole KRW. Zero (the default) means delivery is free, which keeps the
	// service safe to run unconfigured.
	shippingFeeKRW int64
	now            func() time.Time
}

type CreateOrderInput struct {
	CustomerID      string
	Items           []domain.OrderItem
	CouponCode      string
	DiscountKRW     int64
	IdempotencyKey  string
	RecipientName   string
	RecipientPhone  string
	ShippingAddress domain.ShippingAddress
	SourceAddressID string
	// ShippingFeeKRW, when non-nil, is the delivery charge to snapshot on
	// this order (checkout complete uses the session quote). Nil uses the
	// service-configured fee so a direct POST /orders still charges the
	// current amount.
	ShippingFeeKRW *int64
}

type CompleteCheckoutInput struct {
	RecipientName   string
	RecipientPhone  string
	ShippingAddress domain.ShippingAddress
	SourceAddressID string
}

type idempotencyFingerprint struct {
	CustomerID      string                 `json:"customer_id"`
	CouponCode      string                 `json:"coupon_code,omitempty"`
	DiscountKRW     int64                  `json:"discount_krw,omitempty"`
	RecipientName   string                 `json:"recipient_name,omitempty"`
	RecipientPhone  string                 `json:"recipient_phone,omitempty"`
	ShippingAddress domain.ShippingAddress `json:"shipping_address,omitempty"`
	SourceAddressID string                 `json:"source_address_id,omitempty"`
	Items           []struct {
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
	s.outboxDrainer = outbox.NewDrainer(s.repo, s.eventPublisher, "order outbox drain")
	if s.checkoutTTL <= 0 {
		s.checkoutTTL = domain.DefaultCheckoutTTL
	}
	return s
}

// WithClock overrides the service clock. Tests use it to exercise the 2-hour SLA.
func (s *Service) WithClock(now func() time.Time) *Service {
	if now != nil {
		s.now = now
	}
	return s
}

// WithProduct sets the catalog client used to resolve server-side line prices.
func (s *Service) WithProduct(product ports.ProductClient) *Service {
	s.product = product
	return s
}

// WithShippingFee sets the flat delivery charge added to every order, in whole
// KRW. A negative value is ignored so a misconfigured fee cannot make orders
// cheaper than their goods.
func (s *Service) WithShippingFee(krw int64) *Service {
	if krw < 0 {
		log.Printf("order: ignoring negative shipping fee %d", krw)
		return s
	}
	s.shippingFeeKRW = krw
	return s
}

// WithPayment sets the client used to refund a captured payment when a paid
// order is canceled. Without it, paid cancel fails closed so the card capture
// is not dropped locally while NANO still holds the money.
func (s *Service) WithPayment(payment ports.PaymentClient) *Service {
	s.payment = payment
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

	shippingFee := s.shippingFeeKRW
	if input.ShippingFeeKRW != nil && *input.ShippingFeeKRW >= 0 {
		shippingFee = *input.ShippingFeeKRW
	}

	order, err := domain.NewOrder(orderID, input.CustomerID, reservationID, pricedItems, input.CouponCode, input.DiscountKRW, shippingFee, s.now())
	if err != nil {
		_ = s.stock.ReleaseReservation(ctx, reservationID)
		return nil, err
	}
	if err := applyFulfillmentToOrder(order, input); err != nil {
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
	return s.present(order), nil
}

func (s *Service) GetOrder(ctx context.Context, id string) (*domain.Order, error) {
	order, err := s.repo.Get(ctx, strings.TrimSpace(id))
	if err != nil {
		return nil, err
	}
	return s.present(order), nil
}

func (s *Service) ListCustomerOrders(ctx context.Context, customerID string) ([]domain.Order, error) {
	orders, err := s.repo.ListByCustomer(ctx, strings.TrimSpace(customerID))
	if err != nil {
		return nil, err
	}
	return s.presentOrders(orders), nil
}

func (s *Service) ListAllOrders(ctx context.Context) ([]domain.Order, error) {
	orders, err := s.repo.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	return s.presentOrders(orders), nil
}

func (s *Service) MarkOrderPaid(ctx context.Context, orderID, paymentID string, amountKRW int64) (*domain.Order, error) {
	order, err := s.repo.Get(ctx, strings.TrimSpace(orderID))
	if err != nil {
		return nil, err
	}
	// Payment's reconcile worker republishes payment.succeeded for two hours as a
	// guard against lost Core NATS deliveries, so replays arrive long after the order
	// has shipped. Any order already carrying this payment id is done, whatever its
	// current status — re-running the transition would only fail.
	if order.PaymentID == paymentID && order.Status != domain.StatusPending {
		return s.present(order), nil
	}
	startedPending := order.Status == domain.StatusPending
	var reinstatedReservation string
	if order.Status == domain.StatusCanceled {
		reservationID, err := s.reinstateCanceledOrder(ctx, order)
		if err != nil {
			return nil, err
		}
		reinstatedReservation = reservationID
	}
	if err := order.MarkPaid(paymentID, amountKRW, s.now()); err != nil {
		if reinstatedReservation != "" {
			_ = s.stock.ReleaseReservation(ctx, reinstatedReservation)
		}
		return nil, err
	}
	// The expiry worker can cancel a still-pending order after our initial read.
	// Re-read before persisting so we reinstate instead of saving paid with a
	// released reservation_id.
	if startedPending {
		fresh, err := s.repo.Get(ctx, order.ID)
		if err != nil {
			if reinstatedReservation != "" {
				_ = s.stock.ReleaseReservation(ctx, reinstatedReservation)
			}
			return nil, err
		}
		if fresh.PaymentID == paymentID && fresh.Status != domain.StatusPending {
			if reinstatedReservation != "" {
				_ = s.stock.ReleaseReservation(ctx, reinstatedReservation)
			}
			return s.present(fresh), nil
		}
		if fresh.Status == domain.StatusCanceled {
			order = fresh
			reservationID, err := s.reinstateCanceledOrder(ctx, order)
			if err != nil {
				return nil, err
			}
			reinstatedReservation = reservationID
			if err := order.MarkPaid(paymentID, amountKRW, s.now()); err != nil {
				_ = s.stock.ReleaseReservation(ctx, reservationID)
				return nil, err
			}
		}
	}
	events, err := s.outboxEvents(order, orderPaidSubject, orderUpdatedSubject)
	if err != nil {
		if reinstatedReservation != "" {
			_ = s.stock.ReleaseReservation(ctx, reinstatedReservation)
		}
		return nil, err
	}
	saved, err := s.persistPaid(ctx, order, events, reinstatedReservation != "")
	if err != nil {
		if reinstatedReservation != "" {
			_ = s.stock.ReleaseReservation(ctx, reinstatedReservation)
		}
		return nil, err
	}
	if saved {
		s.tryDrainOutbox(ctx)
		return s.present(order), nil
	}

	// Concurrent expiry or duplicate payment event — reconcile once.
	fresh, err := s.repo.Get(ctx, order.ID)
	if err != nil {
		if reinstatedReservation != "" {
			_ = s.stock.ReleaseReservation(ctx, reinstatedReservation)
		}
		return nil, err
	}
	if fresh.PaymentID == paymentID && fresh.Status != domain.StatusPending {
		if reinstatedReservation != "" {
			_ = s.stock.ReleaseReservation(ctx, reinstatedReservation)
		}
		return s.present(fresh), nil
	}
	if fresh.Status == domain.StatusCanceled {
		reservationID, err := s.reinstateCanceledOrder(ctx, fresh)
		if err != nil {
			return nil, err
		}
		if err := fresh.MarkPaid(paymentID, amountKRW, s.now()); err != nil {
			_ = s.stock.ReleaseReservation(ctx, reservationID)
			return nil, err
		}
		events, err = s.outboxEvents(fresh, orderPaidSubject, orderUpdatedSubject)
		if err != nil {
			_ = s.stock.ReleaseReservation(ctx, reservationID)
			return nil, err
		}
		saved, err = s.persistPaid(ctx, fresh, events, true)
		if err != nil {
			_ = s.stock.ReleaseReservation(ctx, reservationID)
			return nil, err
		}
		if !saved {
			_ = s.stock.ReleaseReservation(ctx, reservationID)
			return nil, fmt.Errorf("mark order paid order_id=%s: concurrent status change", order.ID)
		}
		s.tryDrainOutbox(ctx)
		return s.present(fresh), nil
	}
	if reinstatedReservation != "" {
		_ = s.stock.ReleaseReservation(ctx, reinstatedReservation)
	}
	return nil, fmt.Errorf("mark order paid order_id=%s: order not pending (status=%s)", order.ID, fresh.Status)
}

func (s *Service) persistPaid(ctx context.Context, order *domain.Order, events []ports.OutboxEvent, fromCanceled bool) (bool, error) {
	if fromCanceled {
		return s.repo.SavePaidIfCanceled(ctx, order, events)
	}
	return s.repo.SavePaidIfPending(ctx, order, events)
}

func (s *Service) ShipOrder(ctx context.Context, orderID, shippedBy string, tracking domain.ShipmentTracking) (*domain.Order, error) {
	order, err := s.repo.Get(ctx, strings.TrimSpace(orderID))
	if err != nil {
		return nil, err
	}
	shippedBy = strings.TrimSpace(shippedBy)
	if shippedBy == "" {
		return nil, domain.ErrInvalidOrder
	}
	normalized, err := domain.NormalizeShipmentTracking(tracking.Carrier, tracking.TrackingNumber, tracking.CarrierNote)
	if err != nil {
		return nil, err
	}
	// Validate before touching stock — CommitReservation is irreversible.
	if order.Status != domain.StatusPaid {
		return nil, domain.ErrInvalidTransition
	}
	if err := s.commitReservationForShip(ctx, order.ReservationID); err != nil {
		return nil, err
	}
	if err := order.Ship(shippedBy, normalized, s.now()); err != nil {
		return nil, err
	}
	events, err := s.outboxEvents(order, orderUpdatedSubject)
	if err != nil {
		return nil, err
	}
	ok, err := s.repo.ShipIfPaid(ctx, order, events)
	if err != nil {
		return nil, err
	}
	if !ok {
		fresh, getErr := s.repo.Get(ctx, order.ID)
		if getErr != nil {
			return nil, fmt.Errorf("ship order %s: concurrent status change", order.ID)
		}
		if fresh.Status == domain.StatusInTransit {
			return s.present(fresh), nil
		}
		log.Printf(
			"ship order %s: stock committed but order is %s (reservation %s); needs ops review",
			order.ID, fresh.Status, order.ReservationID,
		)
		return nil, domain.ErrInvalidTransition
	}
	s.tryDrainOutbox(ctx)
	return s.present(order), nil
}

func (s *Service) CancelOrder(ctx context.Context, id string) (*domain.Order, error) {
	order, err := s.repo.Get(ctx, strings.TrimSpace(id))
	if err != nil {
		return nil, err
	}
	if order.Status != domain.StatusPending && order.Status != domain.StatusPaid && order.Status != domain.StatusInTransit {
		return nil, domain.ErrInvalidTransition
	}
	wasInTransit := order.Status == domain.StatusInTransit
	if order.Status == domain.StatusPaid || wasInTransit {
		if err := s.refundCapturedPayment(ctx, order); err != nil {
			return nil, err
		}
		// payment.canceled may have already moved the order; don't fail the
		// operator after the card refund has gone through.
		fresh, err := s.repo.Get(ctx, order.ID)
		if err != nil {
			return nil, err
		}
		if fresh.Status == domain.StatusCanceled {
			return s.present(fresh), nil
		}
		order = fresh
		if order.Status != domain.StatusPaid && order.Status != domain.StatusPending && order.Status != domain.StatusInTransit {
			return nil, domain.ErrInvalidTransition
		}
		wasInTransit = order.Status == domain.StatusInTransit
	}
	if err := order.Cancel(s.now()); err != nil {
		return nil, err
	}
	saved, err := s.saveStatusChange(ctx, order)
	if err != nil {
		return nil, err
	}
	// In-transit stock was committed on ship; a refund here does not restock.
	if !wasInTransit {
		if err := s.releaseReservationForCancel(ctx, saved.ReservationID); err != nil {
			log.Printf("cancel order %s: release reservation %s: %v", saved.ID, saved.ReservationID, err)
		}
	}
	return saved, nil
}

// ConfirmOrder is the manager acceptance of a paid order. After this (or ship),
// customer cancel becomes a request instead of an immediate refund.
func (s *Service) ConfirmOrder(ctx context.Context, id string) (*domain.Order, error) {
	order, err := s.repo.Get(ctx, strings.TrimSpace(id))
	if err != nil {
		return nil, err
	}
	if err := order.Confirm(s.now()); err != nil {
		return nil, err
	}
	return s.saveStatusChange(ctx, order)
}

// CustomerCancel immediately refunds before manager confirmation, and otherwise
// opens a cancel request the manager must confirm within 2 hours.
func (s *Service) CustomerCancel(ctx context.Context, id, reason string) (*domain.Order, error) {
	order, err := s.repo.Get(ctx, strings.TrimSpace(id))
	if err != nil {
		return nil, err
	}
	if order.AllowsImmediateCancel() {
		return s.CancelOrder(ctx, order.ID)
	}
	if order.CancelRequestedAt != nil {
		return s.present(order), nil
	}
	if err := order.RequestCancel(reason, s.now()); err != nil {
		return nil, err
	}
	return s.saveStatusChange(ctx, order)
}

// ApproveCancelRequest refunds and cancels after a customer request (or the
// 2-hour auto-approve sweep).
func (s *Service) ApproveCancelRequest(ctx context.Context, id string) (*domain.Order, error) {
	order, err := s.repo.Get(ctx, strings.TrimSpace(id))
	if err != nil {
		return nil, err
	}
	if order.CancelRequestedAt == nil {
		return nil, domain.ErrInvalidTransition
	}
	return s.CancelOrder(ctx, order.ID)
}

// RejectCancelRequest leaves the order in place and clears the customer request.
func (s *Service) RejectCancelRequest(ctx context.Context, id string) (*domain.Order, error) {
	order, err := s.repo.Get(ctx, strings.TrimSpace(id))
	if err != nil {
		return nil, err
	}
	if err := order.RejectCancelRequest(s.now()); err != nil {
		return nil, err
	}
	return s.saveStatusChange(ctx, order)
}

func (s *Service) refundCapturedPayment(ctx context.Context, order *domain.Order) error {
	paymentID := strings.TrimSpace(order.PaymentID)
	if paymentID == "" {
		return nil
	}
	if s.payment == nil {
		return fmt.Errorf("%w: cannot refund payment %s without a payment client", ports.ErrPaymentUnavailable, paymentID)
	}
	return s.payment.CancelPayment(ctx, paymentID, "order-cancel-"+order.ID)
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
	return s.present(order), nil
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
	return s.present(order), nil
}

func hashCreateOrderInput(input CreateOrderInput) string {
	fp := idempotencyFingerprint{
		CustomerID:      strings.TrimSpace(input.CustomerID),
		CouponCode:      strings.TrimSpace(input.CouponCode),
		DiscountKRW:     input.DiscountKRW,
		RecipientName:   strings.TrimSpace(input.RecipientName),
		RecipientPhone:  strings.TrimSpace(input.RecipientPhone),
		ShippingAddress: input.ShippingAddress,
		SourceAddressID: strings.TrimSpace(input.SourceAddressID),
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
	items := make([]events.OrderItem, len(order.Items))
	for i, item := range order.Items {
		items[i] = events.OrderItem{
			SkuID:        item.SkuID,
			SKU:          item.SKU,
			Quantity:     item.Quantity,
			UnitPriceKRW: item.UnitPriceKRW,
		}
	}
	payload, err := json.Marshal(events.Order{
		EventType:      subject,
		OrderID:        order.ID,
		CustomerID:     order.CustomerID,
		Status:         string(order.Status),
		SubtotalKRW:    order.SubtotalKRW,
		DiscountKRW:    order.DiscountKRW,
		ShippingFeeKRW: order.ShippingFeeKRW,
		TotalKRW:       order.TotalKRW,
		Items:          items,
		CreatedAt:      order.CreatedAt,
		Occurred:       s.now(),
	})
	if err != nil {
		return nil, fmt.Errorf("marshal %s event: %w", subject, err)
	}
	return payload, nil
}

// StartOutboxWorker periodically publishes pending outbox rows.
func (s *Service) StartOutboxWorker(ctx context.Context, interval time.Duration) {
	s.outboxDrainer.StartWorker(ctx, interval)
}

func (s *Service) tryDrainOutbox(ctx context.Context) {
	s.outboxDrainer.TryDrain(ctx)
}

// DrainOutbox publishes pending outbox messages. Failures are recorded and retried later.
func (s *Service) DrainOutbox(ctx context.Context) error {
	return s.outboxDrainer.Drain(ctx)
}

// priceItems resolves each line from the product catalog and ignores any client unit_price_krw.
// When any variants are missing, every failed line is collected into UnavailableVariantsError
// rather than failing on the first miss.
func (s *Service) priceItems(ctx context.Context, items []domain.OrderItem) ([]domain.OrderItem, error) {
	if s.product == nil {
		return nil, ports.ErrProductUnavailable
	}
	if len(items) == 0 {
		return nil, domain.ErrInvalidOrder
	}
	out := make([]domain.OrderItem, 0, len(items))
	var unavailable []domain.UnavailableItem
	for _, item := range items {
		info, err := s.resolveVariant(ctx, item)
		if err != nil {
			if errors.Is(err, ports.ErrVariantNotFound) {
				unavailable = append(unavailable, unavailableFromOrderItem(item))
				continue
			}
			return nil, err
		}
		if info.UnitPriceKRW <= 0 {
			return nil, domain.ErrInvalidOrder
		}
		out = append(out, domain.OrderItem{
			SkuID:        info.SkuID,
			SKU:          info.SKU,
			Quantity:     item.Quantity,
			UnitPriceKRW: info.UnitPriceKRW,
			ProductName:  info.ProductName,
			ImageURL:     info.ImageURL,
		})
	}
	if len(unavailable) > 0 {
		return nil, &UnavailableVariantsError{Items: unavailable}
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

func (s *Service) reinstateCanceledOrder(ctx context.Context, order *domain.Order) (string, error) {
	stockItems := make([]ports.StockItem, len(order.Items))
	for i, item := range order.Items {
		stockItems[i] = ports.StockItem{
			SkuID:    item.SkuID,
			SKU:      item.SKU,
			Quantity: item.Quantity,
		}
	}
	reservationID, err := s.stock.Reserve(ctx, order.ID, stockItems)
	if err != nil {
		return "", fmt.Errorf("reinstate canceled order %s: %w", order.ID, err)
	}
	if err := order.ReinstateForLatePayment(reservationID, s.now()); err != nil {
		_ = s.stock.ReleaseReservation(ctx, reservationID)
		return "", err
	}
	return reservationID, nil
}

// releaseReservationForCancel releases reserved stock after cancel is persisted.
// An already-released reservation is treated as success so retries do not block
// when release succeeded but a prior attempt failed before returning.
func (s *Service) releaseReservationForCancel(ctx context.Context, reservationID string) error {
	err := s.stock.ReleaseReservation(ctx, reservationID)
	if err == nil || errors.Is(err, ports.ErrReservationAlreadyReleased) {
		return nil
	}
	return err
}

// commitReservationForShip commits reserved stock when shipping. An already
// committed reservation is treated as success so ShipOrder can retry after a
// prior commit succeeded but the order status save failed.
func (s *Service) commitReservationForShip(ctx context.Context, reservationID string) error {
	err := s.stock.CommitReservation(ctx, reservationID)
	if err == nil || errors.Is(err, ports.ErrReservationAlreadyCommitted) {
		return nil
	}
	return err
}

func cloneOrder(order *domain.Order) *domain.Order {
	if order == nil {
		return nil
	}
	copied := *order
	copied.Items = make([]domain.OrderItem, len(order.Items))
	copy(copied.Items, order.Items)
	copied.ShippingAddress = order.ShippingAddress
	return &copied
}

func (s *Service) present(order *domain.Order) *domain.Order {
	out := cloneOrder(order)
	if out != nil {
		out.ApplyRefundPolicy(s.now())
	}
	return out
}

func (s *Service) presentOrders(orders []domain.Order) []domain.Order {
	copied := make([]domain.Order, len(orders))
	for i := range orders {
		copied[i] = *s.present(&orders[i])
	}
	return copied
}

func applyFulfillmentToOrder(order *domain.Order, input CreateOrderInput) error {
	if strings.TrimSpace(input.RecipientName) == "" &&
		strings.TrimSpace(input.RecipientPhone) == "" &&
		input.ShippingAddress == (domain.ShippingAddress{}) {
		return nil
	}
	snap, err := domain.ValidateFulfillmentSnapshot(
		input.RecipientName,
		input.RecipientPhone,
		input.ShippingAddress,
		input.SourceAddressID,
	)
	if err != nil {
		return err
	}
	return order.ApplyFulfillment(snap)
}

func applyCompleteCheckoutInput(input CompleteCheckoutInput) (*domain.FulfillmentSnapshot, error) {
	return domain.ValidateFulfillmentSnapshot(
		input.RecipientName,
		input.RecipientPhone,
		input.ShippingAddress,
		input.SourceAddressID,
	)
}
