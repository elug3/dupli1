package pg

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/elug3/dupli1/payment/pkg/domain"
	"github.com/elug3/dupli1/payment/pkg/ports"
)

func TestPointsLedgerRoundTrip(t *testing.T) {
	repo := migratedRepo(t, "payment_points_test")
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	if _, err := repo.MutatePointAccount(ctx, "u-1", func(a *domain.PointAccount) error {
		a.OrderFulfilled("o1", 500_000, now)
		_, err := a.Grant(domain.PointSourceReview, "r1", 2_000, "review", now)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.MutatePointAccount(ctx, "u-1", func(a *domain.PointAccount) error {
		_, err := a.Reserve("o2", 6_000, now)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	acct, err := repo.GetPointAccount(ctx, "u-1")
	if err != nil {
		t.Fatal(err)
	}
	if got := acct.Balance(now); got != 1_000 {
		t.Fatalf("balance = %d, want 1000", got)
	}
	h, err := repo.FindPointHold(ctx, "o2")
	if err != nil || h.CustomerID != "u-1" || len(h.Allocations) != 2 {
		t.Fatalf("hold = %+v, %v", h, err)
	}
	if _, err := repo.FindPointHold(ctx, "none"); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("missing hold err = %v", err)
	}

	// A failed mutation writes nothing.
	if _, err := repo.MutatePointAccount(ctx, "u-1", func(a *domain.PointAccount) error {
		_, err := a.Reserve("o3", 1_001, now)
		return err
	}); !errors.Is(err, domain.ErrInsufficientPoints) {
		t.Fatalf("err = %v", err)
	}

	if _, err := repo.MutatePointAccount(ctx, "u-1", func(a *domain.PointAccount) error {
		if _, err := a.Consume("o2", now); err != nil {
			return err
		}
		return a.Release("o2", now.Add(time.Second))
	}); err != nil {
		t.Fatal(err)
	}
	entries, err := repo.ListPointEntries(ctx, "u-1", ports.PointCursor{}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 4 || entries[0].Kind != domain.EntryReturn {
		t.Fatalf("entries = %+v", entries)
	}
	// earn, grant and spend share a timestamp; the id breaks the tie.
	older, err := repo.ListPointEntries(ctx, "u-1", ports.PointCursor{At: entries[1].CreatedAt, ID: entries[1].ID}, 10)
	if err != nil || len(older) != 2 {
		t.Fatalf("paged = %+v, %v", older, err)
	}

	lapsed, err := repo.CustomersWithLapsedLots(ctx, now.AddDate(1, 0, 1), 10)
	if err != nil || len(lapsed) != 1 || lapsed[0] != "u-1" {
		t.Fatalf("lapsed = %v, %v", lapsed, err)
	}
}

func TestPointsReserveIsSerializedPerCustomer(t *testing.T) {
	repo := migratedRepo(t, "payment_points_lock_test")
	ctx := context.Background()
	now := time.Now().UTC()
	if _, err := repo.MutatePointAccount(ctx, "u-1", func(a *domain.PointAccount) error {
		_, err := a.Grant(domain.PointSourceAdjust, "g", 10_000, "", now)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	// Ten checkouts race for 3,000 each; only three fit in 10,000.
	var wg sync.WaitGroup
	var mu sync.Mutex
	ok := 0
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := repo.MutatePointAccount(ctx, "u-1", func(a *domain.PointAccount) error {
				_, err := a.Reserve("race-"+string(rune('a'+i)), 3_000, now)
				return err
			})
			if err == nil {
				mu.Lock()
				ok++
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()
	if ok != 3 {
		t.Fatalf("%d reserves succeeded, want 3", ok)
	}
	acct, _ := repo.GetPointAccount(ctx, "u-1")
	if got := acct.Balance(now); got != 1_000 {
		t.Fatalf("balance = %d, want 1000", got)
	}
}
