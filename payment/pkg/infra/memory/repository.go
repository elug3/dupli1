package memory

import (
	"context"
	"fmt"
	"sync"

	"github.com/elug3/dupli1/payment/pkg/domain"
	"github.com/elug3/dupli1/payment/pkg/ports"
)

type Repository struct {
	mu       sync.RWMutex
	payments map[string]*domain.Payment
	byKey    map[string]string
	nextID   int
}

func NewRepository() *Repository {
	return &Repository{
		payments: make(map[string]*domain.Payment),
		byKey:    make(map[string]string),
	}
}

func (r *Repository) NextPaymentID(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextID++
	return fmt.Sprintf("pay_%06d", r.nextID), nil
}

func (r *Repository) Save(ctx context.Context, payment *domain.Payment) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	copied := *payment
	r.payments[payment.ID] = &copied
	if payment.IdempotencyKey != "" {
		r.byKey[payment.IdempotencyKey] = payment.ID
	}
	return nil
}

func (r *Repository) Get(ctx context.Context, id string) (*domain.Payment, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.payments[id]
	if !ok {
		return nil, ports.ErrNotFound
	}
	copied := *p
	return &copied, nil
}

func (r *Repository) GetByProviderRef(ctx context.Context, providerRef string) (*domain.Payment, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, p := range r.payments {
		if p.ProviderRef == providerRef {
			copied := *p
			return &copied, nil
		}
	}
	return nil, ports.ErrNotFound
}

func (r *Repository) FindByIdempotencyKey(ctx context.Context, key string) (*domain.Payment, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	id, ok := r.byKey[key]
	if !ok {
		return nil, ports.ErrNotFound
	}
	copied := *r.payments[id]
	return &copied, nil
}
