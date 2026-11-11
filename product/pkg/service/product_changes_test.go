package service_test

import (
	"context"
	"testing"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/product/pkg/infra/memory"
	"github.com/elug3/dupli1/product/pkg/service"
	"github.com/elug3/dupli1/shared/pkg/events"
)

type capturedPublisher struct {
	subjects []string
	events   []any
}

func (p *capturedPublisher) Publish(_ context.Context, subject string, event any) error {
	p.subjects = append(p.subjects, subject)
	p.events = append(p.events, event)
	return nil
}

func (p *capturedPublisher) lastUpdate(t *testing.T) events.Product {
	t.Helper()
	for i := len(p.subjects) - 1; i >= 0; i-- {
		if p.subjects[i] == events.ProductUpdated {
			return p.events[i].(events.Product)
		}
	}
	t.Fatal("no product.updated published")
	return events.Product{}
}

// product.updated says which fields an edit changed and what they were, so
// the ops alert can show "가격: ₩2,500 → ₩2,000" instead of only the new price.
func TestUpdateProduct_PublishesChangedFields(t *testing.T) {
	store := memory.NewProductStore()
	store.Products = []domain.Product{{
		ID: "BOT-001", Name: "Cassette", Category: "bags", Status: "active", Price: 2500,
		Attributes: map[string]string{"condition": "new"},
	}}
	pub := &capturedPublisher{}
	svc := service.NewProductSearchService(store, nil, pub)

	if _, err := svc.UpdateProduct(t.Context(), domain.Product{
		ID: "BOT-001", Price: 2000, Status: "draft",
		Attributes: map[string]string{"condition": "used", "care": "dry clean"},
	}); err != nil {
		t.Fatal(err)
	}
	got := pub.lastUpdate(t)
	want := []events.FieldChange{
		{Field: "status", Before: "active", After: "draft"},
		{Field: "price", Before: "2500", After: "2000"},
		{Field: "attributes.care", Before: "", After: "dry clean"},
		{Field: "attributes.condition", Before: "new", After: "used"},
	}
	if len(got.Changes) != len(want) {
		t.Fatalf("changes = %+v, want %+v", got.Changes, want)
	}
	for i := range want {
		if got.Changes[i] != want[i] {
			t.Fatalf("change %d = %+v, want %+v", i, got.Changes[i], want[i])
		}
	}
	if got.Reason != "" {
		t.Fatalf("a manager edit has no reason, got %q", got.Reason)
	}

	// Saving the same values again changes nothing.
	if _, err := svc.UpdateProduct(t.Context(), domain.Product{ID: "BOT-001", Price: 2000}); err != nil {
		t.Fatal(err)
	}
	if got := pub.lastUpdate(t); len(got.Changes) != 0 {
		t.Fatalf("no-op save changes = %+v, want none", got.Changes)
	}
}

// The automatic move to draft says it was automatic and what it changed.
func TestAutoDraft_PublishesReasonAndStatusChange(t *testing.T) {
	store := memory.NewProductStore()
	store.Products = []domain.Product{{ID: "BOT-001", Name: "Cassette", Category: "bags", Status: "active", Price: 2500}}
	store.Variants = []domain.Variant{{SkuID: "SKUID-GRN", SKU: "BOT-001-GRN", ProductID: "BOT-001", Color: "Green", Status: "active"}}
	pub := &capturedPublisher{}
	svc := service.NewProductSearchService(store, nil, pub)

	if _, err := svc.UpdateVariant(t.Context(), "BOT-001", "BOT-001-GRN", domain.Variant{Status: "archived"}); err != nil {
		t.Fatal(err)
	}
	got := pub.lastUpdate(t)
	if got.Reason != events.ProductUpdateAutoDraft {
		t.Fatalf("reason = %q, want %q", got.Reason, events.ProductUpdateAutoDraft)
	}
	if len(got.Changes) != 1 || got.Changes[0] != (events.FieldChange{Field: "status", Before: "active", After: "draft"}) {
		t.Fatalf("changes = %+v, want status active → draft", got.Changes)
	}
}
