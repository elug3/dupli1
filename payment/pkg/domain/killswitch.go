package domain

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

// ErrUnknownKillSwitch is returned for a key no KillSwitchDef declares.
var ErrUnknownKillSwitch = errors.New("unknown kill switch")

// ErrInvalidKillSwitch is returned for a change the switch cannot take, such
// as a shopper message over KillSwitchMessageMax.
var ErrInvalidKillSwitch = errors.New("invalid kill switch")

// KillSwitchMessageMax is the longest shopper-facing message, in characters.
const KillSwitchMessageMax = 200

// Kill switch kinds. A kind says what the switch turns off and so which code
// path enforces it; the target names the one thing of that kind.
const (
	// KillSwitchKindPaymentMethod blocks new payments by one method. Payment
	// refuses to start one, order refuses to price an order for it, and the
	// storefront stops offering it. Payments already captured, refunds and
	// PG callbacks for a checkout already under way are never blocked.
	KillSwitchKindPaymentMethod = "payment_method"
)

// KillSwitchDef declares one switch. Switches are declared in code, not
// created at runtime: a switch is only worth having once something enforces
// it, so adding one means adding a def here and the check that honours it.
type KillSwitchDef struct {
	Key    string `json:"key"`
	Kind   string `json:"kind"`
	Target string `json:"target"`
}

// KillSwitchDefs lists every switch, in the order the console shows them.
var KillSwitchDefs = []KillSwitchDef{
	{Key: PaymentMethodKillSwitchKey(MethodCreditCard), Kind: KillSwitchKindPaymentMethod, Target: MethodCreditCard},
	{Key: PaymentMethodKillSwitchKey(MethodBypass), Kind: KillSwitchKindPaymentMethod, Target: MethodBypass},
}

// PaymentMethodKillSwitchKey is the key of the switch that blocks method.
func PaymentMethodKillSwitchKey(method string) string {
	return KillSwitchKindPaymentMethod + "." + method
}

// LookupKillSwitchDef returns the def for key.
func LookupKillSwitchDef(key string) (KillSwitchDef, bool) {
	for _, def := range KillSwitchDefs {
		if def.Key == key {
			return def, true
		}
	}
	return KillSwitchDef{}, false
}

// KillSwitch is a switch's state. A switch nobody has touched is released
// (Engaged false) and has no UpdatedAt.
type KillSwitch struct {
	KillSwitchDef
	Engaged bool `json:"engaged"`
	// Message is shown to shoppers while the switch is engaged, in place of
	// the storefront's generic "not available" copy. Optional.
	Message string `json:"message,omitempty"`
	// Reason is the operator's note on why it was flipped. Staff only.
	Reason    string     `json:"reason,omitempty"`
	UpdatedBy string     `json:"updated_by,omitempty"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

// KillSwitchChange is one operator's flip of a switch.
type KillSwitchChange struct {
	Key       string
	Engaged   bool
	Message   string
	Reason    string
	UpdatedBy string
}

// NewKillSwitch validates change and returns the switch state it sets.
func NewKillSwitch(change KillSwitchChange, now time.Time) (KillSwitch, error) {
	def, ok := LookupKillSwitchDef(strings.TrimSpace(change.Key))
	if !ok {
		return KillSwitch{}, ErrUnknownKillSwitch
	}
	message := strings.TrimSpace(change.Message)
	reason := strings.TrimSpace(change.Reason)
	if utf8.RuneCountInString(message) > KillSwitchMessageMax || utf8.RuneCountInString(reason) > KillSwitchMessageMax {
		return KillSwitch{}, ErrInvalidKillSwitch
	}
	at := now.UTC()
	return KillSwitch{
		KillSwitchDef: def,
		Engaged:       change.Engaged,
		Message:       message,
		Reason:        reason,
		UpdatedBy:     strings.TrimSpace(change.UpdatedBy),
		UpdatedAt:     &at,
	}, nil
}

// MergeKillSwitches returns one state per def, in def order: the stored one
// where there is one, otherwise released. Stored rows for a key no def
// declares any more are dropped.
func MergeKillSwitches(stored []KillSwitch) []KillSwitch {
	byKey := make(map[string]KillSwitch, len(stored))
	for _, sw := range stored {
		byKey[sw.Key] = sw
	}
	out := make([]KillSwitch, 0, len(KillSwitchDefs))
	for _, def := range KillSwitchDefs {
		sw, ok := byKey[def.Key]
		if !ok {
			sw = KillSwitch{}
		}
		sw.KillSwitchDef = def
		out = append(out, sw)
	}
	return out
}
