package service

import (
	"context"
	"errors"
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
	CustomerID    string
	Items         []domain.OrderItem
	CouponCode    string
	DiscountCents int64
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
	if err := s.repo.Save(ctx, order); err != nil {
		_ = s.stock.ReleaseReservation(ctx, reservationID)
		return nil, err
	}
	if err := s.publish(ctx, orderCreatedSubject, order); err != nil {
		return nil, err
	}
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
	if err := s.repo.Save(ctx, order); err != nil {
		return nil, err
	}
	if err := s.publish(ctx, orderPaidSubject, order); err != nil {
		return nil, err
	}
	if err := s.publish(ctx, orderUpdatedSubject, order); err != nil {
		return nil, err
	}
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
	if err := s.repo.Save(ctx, order); err != nil {
		return nil, err
	}
	if err := s.publish(ctx, orderUpdatedSubject, order); err != nil {
		return nil, err
	}
	return cloneOrder(order), nil
}

func (s *Service) publish(ctx context.Context, subject string, order *domain.Order) error {
	if s.eventPublisher == nil {
		return nil
	}
	items := make([]orderItemEvent, len(order.Items))
	for i, item := range order.Items {
		items[i] = orderItemEvent{
			SkuID:          item.SkuID,
			SKU:            item.SKU,
			Quantity:       item.Quantity,
			UnitPriceCents: item.UnitPriceCents,
		}
	}
	return s.eventPublisher.Publish(ctx, subject, orderEvent{
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
