package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/elug3/dupli1/product/pkg/ports"
	"github.com/elug3/dupli1/shared/pkg/events"
)

// WelcomePromotionIssuer grants a new customer their welcome promotional code
// when auth publishes user.registered.
//
// Everything about it is built to survive redelivery. Core NATS does not
// redeliver on its own, but a republish after a failed publish does reach the
// subscriber twice, and issuing is keyed on the event's user id so the second
// delivery mints nothing.
type WelcomePromotionIssuer struct {
	promotions *PromotionService
	// code is the definition to issue. Empty disables the issuer entirely, so
	// an environment that has not created the campaign simply does nothing
	// rather than logging an error on every registration.
	code string
}

func NewWelcomePromotionIssuer(promotions *PromotionService, code string) *WelcomePromotionIssuer {
	return &WelcomePromotionIssuer{promotions: promotions, code: strings.TrimSpace(code)}
}

// Enabled reports whether a welcome code is configured.
func (i *WelcomePromotionIssuer) Enabled() bool {
	return i != nil && i.code != "" && i.promotions != nil
}

// Register subscribes the issuer to user.registered.
func (i *WelcomePromotionIssuer) Register(ctx context.Context, subscriber ports.EventSubscriber) error {
	if !i.Enabled() || subscriber == nil {
		return nil
	}
	return subscriber.Subscribe(ctx, events.UserRegistered, i.Handle)
}

// Handle issues the welcome entitlement for one registration event.
//
// It returns nil for anything it deliberately skips. A returned error is only
// logged — core NATS will not retry — so the useful distinction is between
// "nothing to do" and "something went wrong that a human should see".
func (i *WelcomePromotionIssuer) Handle(ctx context.Context, subject string, payload []byte) error {
	if !i.Enabled() {
		return nil
	}
	var event events.UserRegisteredEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return fmt.Errorf("%s: decode: %w", subject, err)
	}
	if event.UserID == "" {
		return fmt.Errorf("%s: event carries no user_id", subject)
	}
	// Only shoppers. A manager or service account would get a discount in a
	// wallet nobody buys from, and would inflate the campaign's numbers.
	if !strings.EqualFold(event.AccountType, "customer") {
		return nil
	}

	// The trigger key is the event's own identity, so a redelivery of the same
	// registration is a no-op rather than a second entitlement.
	triggerKey := events.UserRegistered + ":" + event.UserID
	entitlement, err := i.promotions.Issue(ctx, i.code, event.UserID, "system", triggerKey, "")
	if err != nil {
		return fmt.Errorf("issue %s to %s: %w", i.code, event.UserID, err)
	}
	log.Printf("promotion %s issued to %s (entitlement %s)", i.code, event.UserID, entitlement.ID)
	return nil
}
