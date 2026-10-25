package service

import (
	"context"
	"log"

	"github.com/elug3/dupli1/payment/pkg/domain"
	"github.com/elug3/dupli1/payment/pkg/ports"
	"github.com/elug3/dupli1/shared/pkg/permissions"
)

// WithKillSwitches enables operator kill switches. Without a store nothing is
// ever blocked, which keeps the service safe to run unconfigured.
func (s *Service) WithKillSwitches(store ports.KillSwitchStore) *Service {
	s.killSwitches = store
	return s
}

// WithConfiguredMethods records which methods this deployment can take at
// all (card needs NANO credentials). Unset, every switchable method counts
// as configured.
func (s *Service) WithConfiguredMethods(configured map[string]bool) *Service {
	s.configuredMethods = configured
	return s
}

// ListKillSwitches returns every declared switch, released unless stored.
func (s *Service) ListKillSwitches(ctx context.Context) ([]domain.KillSwitch, error) {
	if s.killSwitches == nil {
		return domain.MergeKillSwitches(nil), nil
	}
	stored, err := s.killSwitches.ListKillSwitches(ctx)
	if err != nil {
		return nil, err
	}
	return domain.MergeKillSwitches(stored), nil
}

// SetKillSwitch engages or releases one switch.
func (s *Service) SetKillSwitch(ctx context.Context, change domain.KillSwitchChange) (domain.KillSwitch, error) {
	sw, err := domain.NewKillSwitch(change, s.now())
	if err != nil {
		return domain.KillSwitch{}, err
	}
	if s.killSwitches == nil {
		return domain.KillSwitch{}, ports.ErrKillSwitchesUnavailable
	}
	if err := s.killSwitches.SaveKillSwitch(ctx, sw); err != nil {
		return domain.KillSwitch{}, err
	}
	state := "released"
	if sw.Engaged {
		state = "engaged"
	}
	log.Printf("payment: kill switch %s %s by %s (reason: %q)", sw.Key, state, sw.UpdatedBy, sw.Reason)
	return sw, nil
}

// MethodStatus is one payment method as the storefront should present it.
type MethodStatus struct {
	Method string `json:"method"`
	// Available is Configured and not Disabled: whether checkout should offer it.
	Available bool `json:"available"`
	// Configured is false when this deployment cannot take the method at all.
	Configured bool `json:"configured"`
	// Disabled is true while an operator holds its kill switch engaged.
	Disabled bool `json:"disabled"`
	// Message is the operator's shopper-facing note while Disabled.
	Message string `json:"message,omitempty"`
	// RequiresPermission names the permission a caller needs to use it.
	RequiresPermission string `json:"requires_permission,omitempty"`
}

// MethodStatuses lists every switchable payment method with whether it can be
// used right now.
func (s *Service) MethodStatuses(ctx context.Context) ([]MethodStatus, error) {
	switches, err := s.ListKillSwitches(ctx)
	if err != nil {
		return nil, err
	}
	var out []MethodStatus
	for _, sw := range switches {
		if sw.Kind != domain.KillSwitchKindPaymentMethod {
			continue
		}
		st := MethodStatus{
			Method:     sw.Target,
			Configured: s.methodConfigured(sw.Target),
			Disabled:   sw.Engaged,
		}
		if sw.Engaged {
			st.Message = sw.Message
		}
		st.Available = st.Configured && !st.Disabled
		if sw.Target == domain.MethodBypass {
			st.RequiresPermission = permissions.PaymentBypass
		}
		out = append(out, st)
	}
	return out, nil
}

func (s *Service) methodConfigured(method string) bool {
	if s.configuredMethods == nil {
		return true
	}
	return s.configuredMethods[method]
}

// checkMethodAllowed refuses a new payment by a method whose kill switch is
// engaged. A store that cannot be read fails the payment rather than letting
// it through: the switch exists for when taking the payment is the problem.
func (s *Service) checkMethodAllowed(ctx context.Context, method string) error {
	if s.killSwitches == nil {
		return nil
	}
	switches, err := s.ListKillSwitches(ctx)
	if err != nil {
		return err
	}
	key := domain.PaymentMethodKillSwitchKey(method)
	for _, sw := range switches {
		if sw.Key == key && sw.Engaged {
			return &ports.MethodDisabledError{Method: method, Message: sw.Message}
		}
	}
	return nil
}

// CheckMethodAllowed is checkMethodAllowed for the NANO checkout bridge, which
// opens the card window for a payment created before the switch was engaged.
func (s *Service) CheckMethodAllowed(ctx context.Context, method string) error {
	return s.checkMethodAllowed(ctx, method)
}
