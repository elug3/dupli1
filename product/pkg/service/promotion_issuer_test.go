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
	enableWelcomePromotion(t, svc)
	return service.NewWelcomePromotionIssuer(svc), svc
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

// The issuer is no longer configurable, so what has to hold is that the code
// it issues is the one the stores seed. If the constant and the seed drifted
// apart, every registration would log a not-found and no customer would get a
// code — which is exactly the silent failure removing the env var was meant to
// end.
func TestIssuerIssuesTheSeededCampaign(t *testing.T) {
	ctx := context.Background()
	svc, store := newPromotionSvc(t)
	if _, err := store.Get(ctx, domain.WelcomeCode); err != nil {
		t.Fatalf("the stores must seed %s: %v", domain.WelcomeCode, err)
	}

	issuer := service.NewWelcomePromotionIssuer(svc)
	if err := issuer.Handle(ctx, events.UserRegistered, registrationPayload(t, "cust-1", "customer")); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	// Issued against the seeded definition even while the campaign is off:
	// enabling it later makes every entitlement already minted work.
	wallet, err := svc.Wallet(ctx, "cust-1", cartFor("cust-1", 150000))
	if err != nil {
		t.Fatalf("Wallet: %v", err)
	}
	if len(wallet) != 1 || wallet[0].Entitlement.Code != domain.WelcomeCode {
		t.Fatalf("wallet = %+v, want one %s entitlement", wallet, domain.WelcomeCode)
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
