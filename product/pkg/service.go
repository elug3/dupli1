package product

import (
	"context"
	"fmt"
	"time"

	"github.com/schick/pkg/product/domain"
	"github.com/schick/pkg/product/ports"
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

func (s *ProductSearchService) SearchConsultations(filter map[string]string) ([]domain.Consultation, error) {
	if s.store == nil {
		return nil, fmt.Errorf("store not initialized")
	}
	results, err := s.store.SearchConsultations(filter)
	if err != nil {
		return nil, err
	}
	if err := s.publishSearchEvent("consultations", filter, len(results)); err != nil {
		return nil, err
	}
	return results, nil
}

func (s *ProductSearchService) SearchShoes(filter map[string]string) ([]domain.Shoes, error) {
	if s.store == nil {
		return nil, fmt.Errorf("store not initialized")
	}
	results, err := s.store.SearchShoes(filter)
	if err != nil {
		return nil, err
	}
	if err := s.publishSearchEvent("shoes", filter, len(results)); err != nil {
		return nil, err
	}
	return results, nil
}

func (s *ProductSearchService) SearchOuterwear(filter map[string]string) ([]domain.Outerwear, error) {
	if s.store == nil {
		return nil, fmt.Errorf("store not initialized")
	}
	results, err := s.store.SearchOuterwear(filter)
	if err != nil {
		return nil, err
	}
	if err := s.publishSearchEvent("outerwear", filter, len(results)); err != nil {
		return nil, err
	}
	return results, nil
}

func (s *ProductSearchService) SearchBottoms(filter map[string]string) ([]domain.Bottoms, error) {
	if s.store == nil {
		return nil, fmt.Errorf("store not initialized")
	}
	results, err := s.store.SearchBottoms(filter)
	if err != nil {
		return nil, err
	}
	if err := s.publishSearchEvent("bottoms", filter, len(results)); err != nil {
		return nil, err
	}
	return results, nil
}

func (s *ProductSearchService) SearchBags(filter map[string]string) ([]domain.Bag, error) {
	if s.store == nil {
		return nil, fmt.Errorf("store not initialized")
	}
	results, err := s.store.SearchBags(filter)
	if err != nil {
		return nil, err
	}
	if err := s.publishSearchEvent("bags", filter, len(results)); err != nil {
		return nil, err
	}
	return results, nil
}

func (s *ProductSearchService) SearchClocks(filter map[string]string) ([]domain.Clock, error) {
	if s.store == nil {
		return nil, fmt.Errorf("store not initialized")
	}
	results, err := s.store.SearchClocks(filter)
	if err != nil {
		return nil, err
	}
	if err := s.publishSearchEvent("clocks", filter, len(results)); err != nil {
		return nil, err
	}
	return results, nil
}

func (s *ProductSearchService) Search(category string, filter map[string]string) (interface{}, error) {
	switch category {
	case "consultations":
		return s.SearchConsultations(filter)
	case "shoes":
		return s.SearchShoes(filter)
	case "outerwear":
		return s.SearchOuterwear(filter)
	case "bottoms":
		return s.SearchBottoms(filter)
	case "bags":
		return s.SearchBags(filter)
	case "clocks":
		return s.SearchClocks(filter)
	default:
		return nil, fmt.Errorf("unknown category: %s", category)
	}
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
