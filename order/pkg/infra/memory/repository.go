package memory

import (
	"context"
	"fmt"
	"sync"

	"github.com/elug3/schick/order/pkg/domain"
	"github.com/elug3/schick/order/pkg/ports"
)

type Repository struct {
	mu       sync.RWMutex
	orders   map[string]*domain.Order
	sessions map[string]*domain.CheckoutSession
	nextID   int
}

func NewRepository() *Repository {
	return &Repository{
		orders:   make(map[string]*domain.Order),
		sessions: make(map[string]*domain.CheckoutSession),
	}
}

func (r *Repository) NextOrderID(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.nextID++
	return fmt.Sprintf("ord_%06d", r.nextID), nil
}

func (r *Repository) NextCheckoutSessionID(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.nextID++
	return fmt.Sprintf("cs_%06d", r.nextID), nil
}

func (r *Repository) Save(ctx context.Context, order *domain.Order) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.orders[order.ID] = cloneOrder(order)
	return nil
}

func (r *Repository) Get(ctx context.Context, id string) (*domain.Order, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	order, ok := r.orders[id]
	if !ok {
		return nil, ports.ErrNotFound
	}
	return cloneOrder(order), nil
}

func (r *Repository) ListByCustomer(ctx context.Context, customerID string) ([]domain.Order, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	var orders []domain.Order
	for _, order := range r.orders {
		if order.CustomerID == customerID {
			orders = append(orders, *cloneOrder(order))
		}
	}
	return orders, nil
}

func (r *Repository) SaveCheckoutSession(ctx context.Context, session *domain.CheckoutSession) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.sessions[session.ID] = cloneCheckoutSession(session)
	return nil
}

func (r *Repository) GetCheckoutSession(ctx context.Context, id string) (*domain.CheckoutSession, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	session, ok := r.sessions[id]
	if !ok {
		return nil, ports.ErrNotFound
	}
	return cloneCheckoutSession(session), nil
}

func cloneOrder(order *domain.Order) *domain.Order {
	copied := *order
	copied.Items = make([]domain.OrderItem, len(order.Items))
	copy(copied.Items, order.Items)
	return &copied
}

func cloneCheckoutSession(session *domain.CheckoutSession) *domain.CheckoutSession {
	copied := *session
	copied.Items = make([]domain.OrderItem, len(session.Items))
	copy(copied.Items, session.Items)
	return &copied
}
