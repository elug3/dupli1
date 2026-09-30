package pg

import (
	"testing"

	"github.com/elug3/dupli1/product/pkg/domain"
)

// eligible_lines was retired on 2026-09-30. A stored definition still carrying
// it must come back as entire_subtotal after the migration, or the next save of
// that definition would be refused by Benefit.Validate.
func TestMigrationRetiresEligibleLines(t *testing.T) {
	store, _ := newPromotionStores(t)
	ctx := t.Context()

	promo := fixedPromotionForLedger("BRANDX", nil)
	if err := store.Create(ctx, promo); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := store.pool.Exec(ctx,
		`UPDATE promotions SET benefit = jsonb_set(benefit, '{apply_to}', '"eligible_lines"') WHERE code = 'BRANDX'`,
	); err != nil {
		t.Fatalf("seed legacy row: %v", err)
	}
	if err := store.backfillLegacyBenefit(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	got, err := store.Get(ctx, "BRANDX")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Benefit.ApplyTo != domain.ApplyToEntireSubtotal {
		t.Fatalf("apply_to = %q, want entire_subtotal", got.Benefit.ApplyTo)
	}
	if err := got.Benefit.Validate(); err != nil {
		t.Fatalf("migrated benefit no longer validates: %v", err)
	}
}
