package pg

import (
	"testing"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/product/pkg/ports"
)

// apply_mode is what makes a definition a tier; losing it on a read or an
// update would turn a VIP tier into a code anyone holding it could type.
func TestPromotionApplyModeRoundTrip(t *testing.T) {
	store, _ := newPromotionStores(t)
	ctx := t.Context()

	tier := fixedPromotionForLedger("VIP", nil)
	tier.Scope = domain.ScopeSingleUser
	tier.ApplyMode = domain.ApplyModeAuto
	if err := store.Create(ctx, tier); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := store.Get(ctx, "VIP")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !got.IsTier() {
		t.Fatalf("apply_mode = %q after create, want auto", got.ApplyMode)
	}

	code := domain.ApplyModeCode
	if _, err := store.Update(ctx, "VIP", ports.PromotionPatch{ApplyMode: &code}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	listed, err := store.List(ctx)
	if err != nil || len(listed) != 1 {
		t.Fatalf("List: %v %d", err, len(listed))
	}
	if listed[0].ApplyMode != domain.ApplyModeCode {
		t.Fatalf("apply_mode = %q after update, want code", listed[0].ApplyMode)
	}
}
