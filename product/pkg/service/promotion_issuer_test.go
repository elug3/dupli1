package service_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/product/pkg/infra/memory"
	"github.com/elug3/dupli1/product/pkg/service"
	"github.com/elug3/dupli1/shared/pkg/events"
)

func registrationPayload(t *testing.T, userID, accountType string) []byte {
	t.Helper()
	payload, err := json.Marshal(events.UserRegisteredEvent{
		EventType:   events.UserRegistered,
		UserID:      userID,
		Email:       userID + "@example.com",
		AccountType: accountType,
		Occurred:    time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	return payload
}

func newIssuer(t *testing.T) (*service.WelcomePromotionIssuer, *service.PromotionService) {
	t.Helper()
	svc, _ := newPromotionSvc(t)
	if _, err := svc.Create(context.Background(), welcomePromotion()); err != nil {
		t.Fatalf("create welcome promotion: %v", err)
	}
	return service.NewWelcomePromotionIssuer(svc, "WELCOME50"), svc
}

func TestRegistrationIssuesTheWelcomeCode(t *testing.T) {
	ctx := context.Background()
	issuer, svc := newIssuer(t)

	if err := issuer.Handle(ctx, events.UserRegistered, registrationPayload(t, "cust-1", "customer")); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	wallet, err := svc.Wallet(ctx, "cust-1", cartFor("cust-1", 150000))
	if err != nil {
		t.Fatalf("Wallet: %v", err)
	}
	if len(wallet) != 1 || wallet[0].Entitlement.Code != "WELCOME50" {
		t.Fatalf("wallet = %+v, want one WELCOME50 entitlement", wallet)
	}
	if !wallet[0].Eligible || wallet[0].DiscountWon != 50000 {
		t.Fatalf("entry eligible=%v discount=%d, want true/50000", wallet[0].Eligible, wallet[0].DiscountWon)
	}
}

// A republish after a failed publish reaches the subscriber twice. The second
// delivery must mint nothing.
func TestRedeliveredRegistrationIssuesNothingExtra(t *testing.T) {
	ctx := context.Background()
	issuer, svc := newIssuer(t)
	payload := registrationPayload(t, "cust-1", "customer")

	for i := 0; i < 3; i++ {
		if err := issuer.Handle(ctx, events.UserRegistered, payload); err != nil {
			t.Fatalf("Handle %d: %v", i, err)
		}
	}
	wallet, err := svc.Wallet(ctx, "cust-1", cartFor("cust-1", 150000))
	if err != nil {
		t.Fatalf("Wallet: %v", err)
	}
	if len(wallet) != 1 {
		t.Fatalf("wallet holds %d entitlements after 3 deliveries, want 1", len(wallet))
	}
}

// Managers and service accounts do not shop, so issuing to them would put a
// discount in a wallet nobody buys from and inflate the campaign's numbers.
func TestOnlyCustomerAccountsGetTheWelcomeCode(t *testing.T) {
	ctx := context.Background()
	issuer, svc := newIssuer(t)

	for _, accountType := range []string{"manager", "service", ""} {
		userID := "user-" + accountType
		if err := issuer.Handle(ctx, events.UserRegistered, registrationPayload(t, userID, accountType)); err != nil {
			t.Fatalf("Handle(%q): %v", accountType, err)
		}
		wallet, err := svc.Wallet(ctx, userID, cartFor(userID, 150000))
		if err != nil {
			t.Fatalf("Wallet: %v", err)
		}
		if len(wallet) != 0 {
			t.Fatalf("account_type %q received %d entitlements, want 0", accountType, len(wallet))
		}
	}
}

// account_type casing comes from whatever auth stored, so matching must not be
// case-sensitive.
func TestAccountTypeMatchIsCaseInsensitive(t *testing.T) {
	ctx := context.Background()
	issuer, svc := newIssuer(t)

	if err := issuer.Handle(ctx, events.UserRegistered, registrationPayload(t, "cust-2", "Customer")); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	wallet, err := svc.Wallet(ctx, "cust-2", cartFor("cust-2", 150000))
	if err != nil {
		t.Fatalf("Wallet: %v", err)
	}
	if len(wallet) != 1 {
		t.Fatalf("wallet holds %d entitlements, want 1", len(wallet))
	}
}

func TestMalformedRegistrationEventIsReportedNotSwallowed(t *testing.T) {
	issuer, _ := newIssuer(t)
	if err := issuer.Handle(context.Background(), events.UserRegistered, []byte("{not json")); err == nil {
		t.Fatal("a malformed payload should surface an error for the logs")
	}
	if err := issuer.Handle(context.Background(), events.UserRegistered, registrationPayload(t, "", "customer")); err == nil {
		t.Fatal("an event with no user_id should surface an error")
	}
}

// An environment that has not created the campaign should do nothing quietly,
// rather than log an error on every registration.
func TestIssuerWithNoCodeConfiguredIsInert(t *testing.T) {
	svc, _ := newPromotionSvc(t)
	issuer := service.NewWelcomePromotionIssuer(svc, "")
	if issuer.Enabled() {
		t.Fatal("an issuer with no code should not be enabled")
	}
	if err := issuer.Handle(context.Background(), events.UserRegistered, registrationPayload(t, "cust-1", "customer")); err != nil {
		t.Fatalf("a disabled issuer should be a no-op, got %v", err)
	}
}

// The whole point of the entitlement: it unlocks a code the customer could not
// otherwise use, and the ledger still limits them to one use.
func TestIssuedCodeIsUsableOnceThenSpent(t *testing.T) {
	ctx := context.Background()
	issuer, svc := newIssuer(t)
	if err := issuer.Handle(ctx, events.UserRegistered, registrationPayload(t, "cust-1", "customer")); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	_, first, err := svc.Reserve(ctx, "WELCOME50", "ord-1", cartFor("cust-1", 150000))
	if err != nil {
		t.Fatalf("first reserve: %v", err)
	}
	if !first.OK || first.DiscountWon != 50000 {
		t.Fatalf("first use = ok:%v discount:%d, want true/50000", first.OK, first.DiscountWon)
	}
	if got := svc.Evaluate(ctx, "WELCOME50", cartFor("cust-1", 150000)); got.Reason != domain.ReasonAlreadyUsed {
		t.Fatalf("second attempt = %s, want already_used", got.Reason)
	}
}

var _ = memory.NewPromotionStore
