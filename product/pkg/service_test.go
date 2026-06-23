package product

import (
	"context"
	"testing"

	"github.com/schick/pkg/product/domain"
)

type fakeProductStore struct {
	shoes []domain.Shoes
}

func (s fakeProductStore) SearchConsultations(filter map[string]string) ([]domain.Consultation, error) {
	return nil, nil
}

func (s fakeProductStore) SearchShoes(filter map[string]string) ([]domain.Shoes, error) {
	return s.shoes, nil
}

func (s fakeProductStore) SearchOuterwear(filter map[string]string) ([]domain.Outerwear, error) {
	return nil, nil
}

func (s fakeProductStore) SearchBottoms(filter map[string]string) ([]domain.Bottoms, error) {
	return nil, nil
}

func (s fakeProductStore) SearchBags(filter map[string]string) ([]domain.Bag, error) {
	return nil, nil
}

func (s fakeProductStore) SearchClocks(filter map[string]string) ([]domain.Clock, error) {
	return nil, nil
}

type recordedProductEventPublisher struct {
	subject string
	event   any
}

func (p *recordedProductEventPublisher) Publish(ctx context.Context, subject string, event any) error {
	p.subject = subject
	p.event = event
	return nil
}

func TestSearchShoesPublishesEvent(t *testing.T) {
	store := fakeProductStore{
		shoes: []domain.Shoes{
			{Product: domain.Product{ID: "shoe-1", Brand: "Nike"}},
			{Product: domain.Product{ID: "shoe-2", Brand: "Nike"}},
		},
	}
	publisher := &recordedProductEventPublisher{}
	svc := NewProductSearchService(store, publisher)

	results, err := svc.SearchShoes(map[string]string{"brand": "Nike"})
	if err != nil {
		t.Fatalf("SearchShoes returned error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}

	wantSubject := "product.search.shoes"
	if publisher.subject != wantSubject {
		t.Fatalf("published subject = %q, want %q", publisher.subject, wantSubject)
	}

	event, ok := publisher.event.(productSearchEvent)
	if !ok {
		t.Fatalf("published event type = %T, want productSearchEvent", publisher.event)
	}
	if event.EventType != wantSubject {
		t.Fatalf("event.EventType = %q, want %q", event.EventType, wantSubject)
	}
	if event.Category != "shoes" {
		t.Fatalf("event.Category = %q, want shoes", event.Category)
	}
	if event.ResultCount != 2 {
		t.Fatalf("event.ResultCount = %d, want 2", event.ResultCount)
	}
	if event.Filters["brand"] != "Nike" {
		t.Fatalf("event.Filters[brand] = %q, want Nike", event.Filters["brand"])
	}
	if event.Occurred.IsZero() {
		t.Fatalf("event.Occurred is zero")
	}
}
