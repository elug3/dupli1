package service

import (
	"context"
	"strings"
	"time"

	"github.com/elug3/schick/pkg/order/domain"
	"github.com/elug3/schick/pkg/order/ports"
)

const (
	orderCreatedSubject = "order.created"
	orderUpdatedSubject = "order.status_updated"
)

type Service struct {
	repo           ports.Repository
	inventory      ports.InventoryClient
	eventPublisher ports.EventPublisher
	now            func() time.Time
}

type CreateOrderInput struct {
	CustomerID string
	Items      []domain.OrderItem
}

type orderEvent struct {
	EventType string             `json:"event_type"`
	OrderID   string             `json:"order_id"`
	Status    domain.OrderStatus `json:"status"`
	Occurred  time.Time          `json:"occurred_at"`
}

func New(repo ports.Repository, inventory ports.InventoryClient, eventPublisher ...ports.EventPublisher) *Service {
	s := &Service{
		repo:      repo,
		inventory: inventory,
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
	if len(eventPublisher) > 0 {
		s.eventPublisher = eventPublisher[0]
	}
	return s
}

func (s *Service) CreateOrder(ctx context.Context, input CreateOrderInput) (*domain.Order, error) {
	orderID, err := s.repo.NextOrderID(ctx)
	if err != nil {
		return nil, err
	}

	inventoryItems := make([]ports.InventoryItem, len(input.Items))
	for i, item := range input.Items {
		inventoryItems[i] = ports.InventoryItem{
			SKU:      item.SKU,
			Quantity: item.Quantity,
		}
	}

	reservationID, err := s.inventory.Reserve(ctx, orderID, inventoryItems)
	if err != nil {
		return nil, err
	}

	order, err := domain.NewOrder(orderID, input.CustomerID, reservationID, input.Items, s.now())
	if err != nil {
		_ = s.inventory.ReleaseReservation(ctx, reservationID)
		return nil, err
	}
	if err := s.repo.Save(ctx, order); err != nil {
		_ = s.inventory.ReleaseReservation(ctx, reservationID)
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

func (s *Service) ConfirmOrder(ctx context.Context, id string) (*domain.Order, error) {
	order, err := s.repo.Get(ctx, strings.TrimSpace(id))
	if err != nil {
		return nil, err
	}
	if order.Status != domain.StatusPending {
		return nil, domain.ErrInvalidTransition
	}
	if err := s.inventory.CommitReservation(ctx, order.ReservationID); err != nil {
		return nil, err
	}
	if err := order.Confirm(s.now()); err != nil {
		return nil, err
	}
	return s.saveStatusChange(ctx, order)
}

func (s *Service) CancelOrder(ctx context.Context, id string) (*domain.Order, error) {
	order, err := s.repo.Get(ctx, strings.TrimSpace(id))
	if err != nil {
		return nil, err
	}
	if order.Status != domain.StatusPending {
		return nil, domain.ErrInvalidTransition
	}
	if err := s.inventory.ReleaseReservation(ctx, order.ReservationID); err != nil {
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
	return s.eventPublisher.Publish(ctx, subject, orderEvent{
		EventType: subject,
		OrderID:   order.ID,
		Status:    order.Status,
		Occurred:  s.now(),
	})
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
