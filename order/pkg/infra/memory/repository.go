package memory

import (
	"context"
	"fmt"
	"sync"

	"github.com/elug3/schick/pkg/order/domain"
	"github.com/elug3/schick/pkg/order/ports"
)

type Repository struct {
	mu     sync.RWMutex
	orders map[string]*domain.Order
	nextID int
}

func NewRepository() *Repository {
	return &Repository{
		orders: make(map[string]*domain.Order),
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

func cloneOrder(order *domain.Order) *domain.Order {
	copied := *order
	copied.Items = make([]domain.OrderItem, len(order.Items))
	copy(copied.Items, order.Items)
	return &copied
}
