package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/product/pkg/ports"
	"github.com/elug3/dupli1/shared/pkg/events"
)

// WelcomePromotionIssuer grants a new customer every promotional code a manager
// marked auto_issue = user_registered, when auth publishes user.registered.
//
// Everything about it is built to survive redelivery. Core NATS does not
// redeliver on its own, but a republish after a failed publish does reach the
// subscriber twice, and issuing is keyed on the event's user id so the second
// delivery mints nothing.
// Which codes it issues is data, set at runtime through the promotions admin
// API: nothing is compiled in or seeded. Whether customers can spend what it
// grants is each definition's `active` flag, so entitlements minted while a
// campaign is off all start working the moment a manager enables it. To stop
// issuing a campaign, clear its auto_issue.
type WelcomePromotionIssuer struct {
	promotions *PromotionService
}

func NewWelcomePromotionIssuer(promotions *PromotionService) *WelcomePromotionIssuer {
	return &WelcomePromotionIssuer{promotions: promotions}
}

func (i *WelcomePromotionIssuer) ready() bool {
	return i != nil && i.promotions != nil
}

// Register subscribes the issuer to user.registered.
func (i *WelcomePromotionIssuer) Register(ctx context.Context, subscriber ports.EventSubscriber) error {
	if !i.ready() || subscriber == nil {
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
	if !i.ready() {
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

	codes, err := i.promotions.AutoIssued(ctx, domain.AutoIssueUserRegistered)
	if err != nil {
		return fmt.Errorf("list sign-up promotions for %s: %w", event.UserID, err)
	}
	if len(codes) == 0 {
		// Not an error — a shop may run no sign-up campaign — but say so,
		// because a campaign someone expected to be running looks the same.
		log.Printf("no promotional code is set to auto-issue on sign-up; %s was issued nothing", event.UserID)
		return nil
	}

	// The trigger key is the event's own identity, so a redelivery of the same
	// registration is a no-op rather than a second entitlement. It is shared
	// across codes; issuing is idempotent per (code, customer, key).
	triggerKey := events.UserRegistered + ":" + event.UserID
	var failed []error
	for _, promotion := range codes {
		entitlement, err := i.promotions.Issue(ctx, promotion.Code, event.UserID, "system", triggerKey, "")
		if err != nil {
			// One broken campaign must not cost the customer the others.
			failed = append(failed, fmt.Errorf("issue %s to %s: %w", promotion.Code, event.UserID, err))
			continue
		}
		log.Printf("promotion %s issued to %s (entitlement %s)", promotion.Code, event.UserID, entitlement.ID)
	}
	return errors.Join(failed...)
}
