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

func TestRecordProductViewSameVisitorSameDayDedupes(t *testing.T) {
	s := newStoreWithProduct("BOT-001")

	got1, err := s.RecordProductView("BOT-001", "1.2.3.4", "2024-01-01")
	if err != nil {
		t.Fatalf("first call: unexpected error: %v", err)
	}
	if !got1 {
		t.Fatalf("first call: want incremented=true")
	}

	got2, err := s.RecordProductView("BOT-001", "1.2.3.4", "2024-01-01")
	if err != nil {
		t.Fatalf("second call: unexpected error: %v", err)
	}
	if got2 {
		t.Fatalf("second call: want incremented=false (same visitor, same day)")
	}

	if s.Products[0].ViewCount != 1 {
		t.Fatalf("want ViewCount=1, got %d", s.Products[0].ViewCount)
	}
}

func TestRecordProductViewSameVisitorDifferentDayIncrementsAgain(t *testing.T) {
	s := newStoreWithProduct("BOT-001")

	if _, err := s.RecordProductView("BOT-001", "1.2.3.4", "2024-01-01"); err != nil {
		t.Fatalf("day 1: unexpected error: %v", err)
	}
	got, err := s.RecordProductView("BOT-001", "1.2.3.4", "2024-01-02")
	if err != nil {
		t.Fatalf("day 2: unexpected error: %v", err)
	}
	if !got {
		t.Fatalf("day 2: want incremented=true (new day)")
	}

	if s.Products[0].ViewCount != 2 {
		t.Fatalf("want ViewCount=2, got %d", s.Products[0].ViewCount)
	}
}

func TestRecordProductViewDifferentVisitorsSameDayBothCount(t *testing.T) {
	s := newStoreWithProduct("BOT-001")

	if _, err := s.RecordProductView("BOT-001", "1.2.3.4", "2024-01-01"); err != nil {
		t.Fatalf("visitor 1: unexpected error: %v", err)
	}
	got, err := s.RecordProductView("BOT-001", "5.6.7.8", "2024-01-01")
	if err != nil {
		t.Fatalf("visitor 2: unexpected error: %v", err)
	}
	if !got {
		t.Fatalf("visitor 2: want incremented=true (different visitor)")
	}

	if s.Products[0].ViewCount != 2 {
		t.Fatalf("want ViewCount=2, got %d", s.Products[0].ViewCount)
	}
}

func TestRecordProductViewUnknownProduct(t *testing.T) {
	s := memory.NewProductStore()

	_, err := s.RecordProductView("NOPE-001", "1.2.3.4", "2024-01-01")
	if err == nil {
		t.Fatalf("want error for unknown product")
	}
}
