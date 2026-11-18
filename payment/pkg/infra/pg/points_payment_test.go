package pg

import (
	"testing"
	"time"

	"github.com/elug3/dupli1/payment/pkg/domain"
)

// A database migrated before points still has amount_won > 0; migrating again
// must relax it so a points-only order can record its ₩0 payment.
func TestMigrateAllowsZeroAmountPointsPayment(t *testing.T) {
	repo := migratedRepo(t, "payment_points_zero_test")
	ctx := t.Context()
	for _, stmt := range []string{
		`ALTER TABLE payments DROP CONSTRAINT payments_amount_won_nonnegative`,
		`ALTER TABLE payments DROP CONSTRAINT IF EXISTS payments_amount_won_check`,
		`ALTER TABLE payments ADD CONSTRAINT payments_amount_cents_check CHECK (amount_won > 0)`,
	} {
		if _, err := repo.pool.Exec(ctx, stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	if err := repo.migrate(); err != nil {
		t.Fatalf("re-migrate: %v", err)
	}
	if err := repo.migrate(); err != nil {
		t.Fatalf("migrate is not idempotent: %v", err)
	}

	p, err := domain.NewPointsPayment("pay-0", "ord-0", "cust-1", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Save(ctx, p); err != nil {
		t.Fatalf("save ₩0 points payment: %v", err)
	}
	got, err := repo.Get(ctx, "pay-0")
	if err != nil || got.AmountWon != 0 || got.Method != domain.MethodPoints || got.Status != domain.StatusSucceeded {
		t.Fatalf("got %+v, %v", got, err)
	}
	if _, err := repo.pool.Exec(ctx, `UPDATE payments SET amount_won = -1 WHERE id = 'pay-0'`); err == nil {
		t.Fatal("negative amount accepted")
	}
}
