package product

import (
	"context"
	"fmt"
	"time"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/product/pkg/ports"
)

const productSearchSubjectPrefix = "product.search."

type ProductSearchService struct {
	store          ports.ProductStore
	eventPublisher ports.EventPublisher
}

type productSearchEvent struct {
	EventType   string            `json:"event_type"`
	Category    string            `json:"category"`
	Filters     map[string]string `json:"filters,omitempty"`
	ResultCount int               `json:"result_count"`
	Occurred    time.Time         `json:"occurred_at"`
}

func NewProductSearchService(store ports.ProductStore, eventPublisher ...ports.EventPublisher) *ProductSearchService {
	s := &ProductSearchService{
		store: store,
	}
	if len(eventPublisher) > 0 {
		s.eventPublisher = eventPublisher[0]
	}
	return s
}

func (s *ProductSearchService) SearchProducts(filter map[string]string) ([]domain.Product, error) {
	if s.store == nil {
		return nil, fmt.Errorf("store not initialized")
	}
	results, err := s.store.SearchProducts(filter)
	if err != nil {
		return nil, err
	}
	category := filter["category"]
	if category == "" {
		category = "all"
	}
	if err := s.publishSearchEvent(category, filter, len(results)); err != nil {
		return nil, err
	}
	return results, nil
}

func (s *ProductSearchService) CreateProduct(p domain.Product) (*domain.Product, error) {
	if s.store == nil {
		return nil, fmt.Errorf("store not initialized")
	}
	return s.store.CreateProduct(p)
}

func (s *ProductSearchService) publishSearchEvent(category string, filter map[string]string, resultCount int) error {
	if s.eventPublisher == nil {
		return nil
	}

	filters := make(map[string]string, len(filter))
	for key, value := range filter {
		filters[key] = value
	}

	eventType := productSearchSubjectPrefix + category
	return s.eventPublisher.Publish(context.Background(), eventType, productSearchEvent{
		EventType:   eventType,
		Category:    category,
		Filters:     filters,
		ResultCount: resultCount,
		Occurred:    time.Now().UTC(),
	})
}
