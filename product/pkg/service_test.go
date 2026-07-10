package product

import (
	"context"
	"testing"

	"github.com/elug3/dupli1/product/pkg/domain"
)

type fakeProductStore struct {
	products []domain.Product
}

func (s fakeProductStore) SearchProducts(filter map[string]string) ([]domain.Product, error) {
	return s.products, nil
}

func (s fakeProductStore) ListProducts() ([]domain.Product, error) {
	return nil, nil
}

func (s fakeProductStore) GetProduct(id string) (*domain.Product, error) {
	return nil, nil
}

func (s fakeProductStore) GetActiveProduct(id string) (*domain.Product, error) {
	return nil, nil
}

func (s fakeProductStore) CreateProduct(p domain.Product) (*domain.Product, error) {
	return nil, nil
}

func (s fakeProductStore) UpdateProduct(p domain.Product) (*domain.Product, error) {
	return nil, nil
}

func (s fakeProductStore) DeleteProduct(id string) error {
	return nil
}

func (s fakeProductStore) ListVariants(productID string) ([]domain.Variant, error) {
	return nil, nil
}

func (s fakeProductStore) GetVariant(sku string) (*domain.Variant, error) {
	return nil, nil
}

func (s fakeProductStore) GetVariantBySkuID(skuID string) (*domain.Variant, error) {
	return nil, nil
}

func (s fakeProductStore) CreateVariant(v domain.Variant) (*domain.Variant, error) {
	return nil, nil
}

func (s fakeProductStore) UpdateVariant(v domain.Variant) (*domain.Variant, error) {
	return nil, nil
}

func (s fakeProductStore) DeleteVariant(sku string) error {
	return nil
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

func TestSearchProductsPublishesEvent(t *testing.T) {
	store := fakeProductStore{
		products: []domain.Product{
			{ID: "BOT-001", Brand: "Bottega Veneta", Category: "bags"},
			{ID: "BOT-002", Brand: "Bottega Veneta", Category: "bags"},
		},
	}
	publisher := &recordedProductEventPublisher{}
	svc := NewProductSearchService(store, publisher)

	results, err := svc.SearchProducts(map[string]string{"category": "bags", "brand": "Bottega Veneta"})
	if err != nil {
		t.Fatalf("SearchProducts returned error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}

	wantSubject := "product.search.bags"
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
	if event.Category != "bags" {
		t.Fatalf("event.Category = %q, want bags", event.Category)
	}
	if event.ResultCount != 2 {
		t.Fatalf("event.ResultCount = %d, want 2", event.ResultCount)
	}
	if event.Filters["brand"] != "Bottega Veneta" {
		t.Fatalf("event.Filters[brand] = %q, want Bottega Veneta", event.Filters["brand"])
	}
	if event.Occurred.IsZero() {
		t.Fatalf("event.Occurred is zero")
	}
}
