package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/elug3/dupli1/inventory/pkg/domain"
	"github.com/elug3/dupli1/inventory/pkg/infra/memory"
	"github.com/elug3/dupli1/inventory/pkg/service"
)

func TestReserveAndCommitStock(t *testing.T) {
	ctx := context.Background()
	svc := service.New(memory.NewRepository())

	if _, err := svc.UpsertItem(ctx, "shoe-1", 5); err != nil {
		t.Fatalf("UpsertItem returned error: %v", err)
	}

	reservation, err := svc.Reserve(ctx, "order-1", []domain.ReservationItem{{SKU: "shoe-1", Quantity: 2}})
	if err != nil {
		t.Fatalf("Reserve returned error: %v", err)
	}

	item, err := svc.GetItem(ctx, "shoe-1")
	if err != nil {
		t.Fatalf("GetItem returned error: %v", err)
	}
	if item.Available() != 3 || item.Reserved != 2 || item.Quantity != 5 {
		t.Fatalf("item after reserve = %+v, want quantity 5 reserved 2 available 3", item)
	}

	reservation, err = svc.CommitReservation(ctx, reservation.ID)
	if err != nil {
		t.Fatalf("CommitReservation returned error: %v", err)
	}
	if reservation.Status != domain.ReservationCommitted {
		t.Fatalf("reservation status = %q, want committed", reservation.Status)
	}

	item, err = svc.GetItem(ctx, "shoe-1")
	if err != nil {
		t.Fatalf("GetItem returned error: %v", err)
	}
	if item.Quantity != 3 || item.Reserved != 0 || item.Available() != 3 {
		t.Fatalf("item after commit = %+v, want quantity 3 reserved 0 available 3", item)
	}
}

func TestReserveRejectsInsufficientStock(t *testing.T) {
	ctx := context.Background()
	svc := service.New(memory.NewRepository())

	if _, err := svc.UpsertItem(ctx, "bag-1", 1); err != nil {
		t.Fatalf("UpsertItem returned error: %v", err)
	}

	_, err := svc.Reserve(ctx, "order-1", []domain.ReservationItem{{SKU: "bag-1", Quantity: 2}})
	if !errors.Is(err, service.ErrInsufficientStock) {
		t.Fatalf("Reserve error = %v, want ErrInsufficientStock", err)
	}
}

func TestReleaseReservationRestoresAvailability(t *testing.T) {
	ctx := context.Background()
	svc := service.New(memory.NewRepository())

	if _, err := svc.UpsertItem(ctx, "clock-1", 4); err != nil {
		t.Fatalf("UpsertItem returned error: %v", err)
	}
	reservation, err := svc.Reserve(ctx, "order-1", []domain.ReservationItem{{SKU: "clock-1", Quantity: 3}})
	if err != nil {
		t.Fatalf("Reserve returned error: %v", err)
	}

	if _, err := svc.ReleaseReservation(ctx, reservation.ID); err != nil {
		t.Fatalf("ReleaseReservation returned error: %v", err)
	}

	item, err := svc.GetItem(ctx, "clock-1")
	if err != nil {
		t.Fatalf("GetItem returned error: %v", err)
	}
	if item.Quantity != 4 || item.Reserved != 0 || item.Available() != 4 {
		t.Fatalf("item after release = %+v, want quantity 4 reserved 0 available 4", item)
	}
}
