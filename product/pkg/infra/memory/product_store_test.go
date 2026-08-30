package memory_test

import (
	"testing"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/product/pkg/infra/memory"
)

func newStoreWithProduct(id string) *memory.ProductStore {
	s := memory.NewProductStore()
	s.Products = []domain.Product{{ID: id, Status: "active"}}
	return s
}

func TestRecordUniqueViewSameGuestDedupes(t *testing.T) {
	s := newStoreWithProduct("BOT-001")

	got1, count1, err := s.RecordUniqueView(t.Context(), "1.2.3.4", "BOT-001")
	if err != nil {
		t.Fatalf("first call: unexpected error: %v", err)
	}
	if !got1 {
		t.Fatalf("first call: want inserted=true")
	}
	if count1 != 1 {
		t.Fatalf("first call: want viewCount=1, got %d", count1)
	}

	got2, count2, err := s.RecordUniqueView(t.Context(), "1.2.3.4", "BOT-001")
	if err != nil {
		t.Fatalf("second call: unexpected error: %v", err)
	}
	if got2 {
		t.Fatalf("second call: want inserted=false (same guest)")
	}
	if count2 != 1 {
		t.Fatalf("second call: want viewCount=1, got %d", count2)
	}

	if s.Products[0].ViewCount != 1 {
		t.Fatalf("want ViewCount=1, got %d", s.Products[0].ViewCount)
	}
}

func TestRecordUniqueViewDifferentGuestsBothCount(t *testing.T) {
	s := newStoreWithProduct("BOT-001")

	if _, _, err := s.RecordUniqueView(t.Context(), "1.2.3.4", "BOT-001"); err != nil {
		t.Fatalf("guest 1: unexpected error: %v", err)
	}
	got, count, err := s.RecordUniqueView(t.Context(), "5.6.7.8", "BOT-001")
	if err != nil {
		t.Fatalf("guest 2: unexpected error: %v", err)
	}
	if !got {
		t.Fatalf("guest 2: want inserted=true (different guest)")
	}
	if count != 2 {
		t.Fatalf("guest 2: want viewCount=2, got %d", count)
	}

	if s.Products[0].ViewCount != 2 {
		t.Fatalf("want ViewCount=2, got %d", s.Products[0].ViewCount)
	}
}

func TestRecordUniqueViewUnknownProduct(t *testing.T) {
	s := memory.NewProductStore()

	_, _, err := s.RecordUniqueView(t.Context(), "1.2.3.4", "NOPE-001")
	if err == nil {
		t.Fatalf("want error for unknown product")
	}
}
