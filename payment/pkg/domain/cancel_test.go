package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/elug3/dupli1/payment/pkg/domain"
)

func succeededPayment(t *testing.T, amountCents int64) *domain.Payment {
	t.Helper()
	now := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
	p, err := domain.NewPayment("pay_000001", "ord_1", "cust_1", amountCents,
		domain.DefaultCurrency, domain.ProviderNano, "2409030071109", "", now)
	if err != nil {
		t.Fatalf("NewPayment: %v", err)
	}
	p.MarkSucceeded(now)
	return p
}

func TestApplyCancel_FullMovesToCanceled(t *testing.T) {
	p := succeededPayment(t, 70000)
	now := time.Date(2026, 9, 5, 13, 0, 0, 0, time.UTC)

	if err := p.ApplyCancel(70000, "ops reject", "mgr_1", now); err != nil {
		t.Fatalf("ApplyCancel: %v", err)
	}
	if p.Status != domain.StatusCanceled {
		t.Fatalf("status = %q, want canceled", p.Status)
	}
	if p.RemainingCancelableCents() != 0 {
		t.Fatalf("remaining = %d, want 0", p.RemainingCancelableCents())
	}
	if p.CanceledAmountCents != 70000 {
		t.Fatalf("canceled = %d, want 70000", p.CanceledAmountCents)
	}
	if p.CancelReason != "ops reject" || p.CanceledBy != "mgr_1" {
		t.Fatalf("reason/by = %q/%q", p.CancelReason, p.CanceledBy)
	}
	if p.CanceledAt == nil || !p.CanceledAt.Equal(now) {
		t.Fatalf("canceled_at = %v, want %v", p.CanceledAt, now)
	}
}

// A partial cancel must leave the payment succeeded: NANO keeps the original
// transaction alive with a reduced remainAmt, and marking it canceled here
// would strand the amount still captured.
func TestApplyCancel_PartialStaysSucceeded(t *testing.T) {
	p := succeededPayment(t, 70000)
	now := time.Date(2026, 9, 5, 13, 0, 0, 0, time.UTC)

	if err := p.ApplyCancel(20000, "partial", "mgr_1", now); err != nil {
		t.Fatalf("ApplyCancel: %v", err)
	}
	if p.Status != domain.StatusSucceeded {
		t.Fatalf("status = %q, want succeeded after partial cancel", p.Status)
	}
	if got := p.RemainingCancelableCents(); got != 50000 {
		t.Fatalf("remaining = %d, want 50000", got)
	}
	if !p.Cancelable() {
		t.Fatal("partially canceled payment must stay cancelable")
	}
}

func TestApplyCancel_PartialsAccumulateThenClose(t *testing.T) {
	p := succeededPayment(t, 70000)
	now := time.Date(2026, 9, 5, 13, 0, 0, 0, time.UTC)

	if err := p.ApplyCancel(20000, "", "", now); err != nil {
		t.Fatalf("first partial: %v", err)
	}
	if err := p.ApplyCancel(50000, "", "", now.Add(time.Minute)); err != nil {
		t.Fatalf("second partial: %v", err)
	}
	if p.Status != domain.StatusCanceled {
		t.Fatalf("status = %q, want canceled once fully refunded", p.Status)
	}
	if p.CanceledAmountCents != 70000 {
		t.Fatalf("canceled = %d, want 70000", p.CanceledAmountCents)
	}
}

func TestApplyCancel_RejectsOverRemaining(t *testing.T) {
	p := succeededPayment(t, 70000)
	now := time.Date(2026, 9, 5, 13, 0, 0, 0, time.UTC)
	if err := p.ApplyCancel(50000, "", "", now); err != nil {
		t.Fatalf("first partial: %v", err)
	}

	err := p.ApplyCancel(20001, "", "", now)
	if !errors.Is(err, domain.ErrCancelAmountInvalid) {
		t.Fatalf("err = %v, want ErrCancelAmountInvalid", err)
	}
	if p.CanceledAmountCents != 50000 {
		t.Fatalf("rejected cancel mutated state: canceled = %d", p.CanceledAmountCents)
	}
}

func TestApplyCancel_RejectsNonPositive(t *testing.T) {
	now := time.Date(2026, 9, 5, 13, 0, 0, 0, time.UTC)
	for _, amount := range []int64{0, -1} {
		p := succeededPayment(t, 70000)
		if err := p.ApplyCancel(amount, "", "", now); !errors.Is(err, domain.ErrCancelAmountInvalid) {
			t.Fatalf("amount %d: err = %v, want ErrCancelAmountInvalid", amount, err)
		}
	}
}

// Only a captured payment can be refunded. requires_payment was never charged,
// failed/expired never captured, and canceled is already fully refunded.
func TestApplyCancel_RejectsNonSucceededStatuses(t *testing.T) {
	now := time.Date(2026, 9, 5, 13, 0, 0, 0, time.UTC)
	for _, status := range []domain.PaymentStatus{
		domain.StatusRequiresPayment,
		domain.StatusFailed,
		domain.StatusCanceled,
		domain.StatusExpired,
	} {
		p := succeededPayment(t, 70000)
		p.Status = status
		if err := p.ApplyCancel(70000, "", "", now); !errors.Is(err, domain.ErrNotCancelable) {
			t.Fatalf("status %q: err = %v, want ErrNotCancelable", status, err)
		}
	}
}

func TestApplyCancel_DoubleFullCancelRejected(t *testing.T) {
	p := succeededPayment(t, 70000)
	now := time.Date(2026, 9, 5, 13, 0, 0, 0, time.UTC)
	if err := p.ApplyCancel(70000, "", "", now); err != nil {
		t.Fatalf("first cancel: %v", err)
	}

	err := p.ApplyCancel(70000, "", "", now.Add(time.Minute))
	if !errors.Is(err, domain.ErrNotCancelable) {
		t.Fatalf("err = %v, want ErrNotCancelable", err)
	}
	if p.CanceledAmountCents != 70000 {
		t.Fatalf("double cancel changed total: %d", p.CanceledAmountCents)
	}
}

func TestValidateCancel_DoesNotMutate(t *testing.T) {
	p := succeededPayment(t, 70000)
	if err := p.ValidateCancel(70000); err != nil {
		t.Fatalf("ValidateCancel: %v", err)
	}
	if p.CanceledAmountCents != 0 || p.Status != domain.StatusSucceeded || p.CanceledAt != nil {
		t.Fatal("ValidateCancel must not mutate the payment")
	}
}

// A later cancel that supplies no reason must not erase the reason an earlier
// partial cancel recorded — that would silently drop audit data.
func TestApplyCancel_LaterBlankReasonDoesNotEraseEarlier(t *testing.T) {
	p := succeededPayment(t, 70000)
	now := time.Date(2026, 9, 5, 13, 0, 0, 0, time.UTC)

	if err := p.ApplyCancel(20000, "partial refund", "mgr_1", now); err != nil {
		t.Fatalf("first cancel: %v", err)
	}
	if err := p.ApplyCancel(50000, "", "", now.Add(time.Minute)); err != nil {
		t.Fatalf("second cancel: %v", err)
	}
	if p.CancelReason != "partial refund" {
		t.Fatalf("reason = %q, want the earlier reason preserved", p.CancelReason)
	}
	if p.CanceledBy != "mgr_1" {
		t.Fatalf("canceled_by = %q, want the earlier actor preserved", p.CanceledBy)
	}
}

// An explicit new reason still wins over the earlier one.
func TestApplyCancel_LaterReasonOverridesEarlier(t *testing.T) {
	p := succeededPayment(t, 70000)
	now := time.Date(2026, 9, 5, 13, 0, 0, 0, time.UTC)

	if err := p.ApplyCancel(20000, "partial refund", "mgr_1", now); err != nil {
		t.Fatalf("first cancel: %v", err)
	}
	if err := p.ApplyCancel(50000, "ops reject", "mgr_2", now.Add(time.Minute)); err != nil {
		t.Fatalf("second cancel: %v", err)
	}
	if p.CancelReason != "ops reject" || p.CanceledBy != "mgr_2" {
		t.Fatalf("reason/by = %q / %q, want the newer values", p.CancelReason, p.CanceledBy)
	}
}
