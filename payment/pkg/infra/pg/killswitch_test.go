package pg

import (
	"testing"
	"time"

	"github.com/elug3/dupli1/payment/pkg/domain"
)

func TestKillSwitchRoundTrip(t *testing.T) {
	repo := migratedRepo(t, "payment_test_kill_switch")
	ctx := t.Context()

	if got, err := repo.ListKillSwitches(ctx); err != nil || len(got) != 0 {
		t.Fatalf("fresh list = %v, %v; want empty", got, err)
	}
	now := time.Now().UTC().Truncate(time.Millisecond)
	sw, err := domain.NewKillSwitch(domain.KillSwitchChange{
		Key: "payment_method.credit_card", Engaged: true, Message: "점검 중", Reason: "PG outage", UpdatedBy: "ops@dupli1.com",
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveKillSwitch(ctx, sw); err != nil {
		t.Fatal(err)
	}
	sw.Engaged = false
	sw.Message = ""
	if err := repo.SaveKillSwitch(ctx, sw); err != nil {
		t.Fatal(err)
	}
	got, err := repo.ListKillSwitches(ctx)
	if err != nil || len(got) != 1 {
		t.Fatalf("list = %v, %v; want one row", got, err)
	}
	if got[0].Engaged || got[0].Message != "" || got[0].Reason != "PG outage" || got[0].UpdatedBy != "ops@dupli1.com" || !got[0].UpdatedAt.Equal(now) {
		t.Fatalf("row = %+v", got[0])
	}
}
