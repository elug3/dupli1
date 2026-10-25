package ports

import (
	"context"
	"errors"

	"github.com/elug3/dupli1/payment/pkg/domain"
)

// ErrMethodDisabled is returned when an operator has engaged the kill switch
// for the requested payment method. Unlike ErrMethodUnavailable (the method is
// not configured on this deployment), this is a deliberate, temporary block.
var ErrMethodDisabled = errors.New("payment method is temporarily disabled")

// KillSwitchStore persists kill switch state. It holds only switches someone
// has flipped; domain.MergeKillSwitches fills in the rest.
type KillSwitchStore interface {
	ListKillSwitches(ctx context.Context) ([]domain.KillSwitch, error)
	SaveKillSwitch(ctx context.Context, sw domain.KillSwitch) error
}

// MethodDisabledError carries the operator's shopper-facing message with
// ErrMethodDisabled, so the storefront can show why card payments are off.
type MethodDisabledError struct {
	Method  string
	Message string
}

func (e *MethodDisabledError) Error() string {
	return ErrMethodDisabled.Error() + ": " + e.Method
}

func (e *MethodDisabledError) Is(target error) bool { return target == ErrMethodDisabled }

// ErrKillSwitchesUnavailable is returned when no kill switch store is wired.
var ErrKillSwitchesUnavailable = errors.New("kill switches are not configured")
