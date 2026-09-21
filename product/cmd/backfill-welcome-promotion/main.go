// backfill-welcome-promotion issues a single-user promotional code to every
// existing customer account, so the campaign reaches people who signed up
// before the registration issuer existed.
//
// Accounts are read from auth, which owns them; entitlements are written to
// product's own database. The command deliberately does not read auth's
// tables directly — that boundary is what keeps the two services independent.
//
// Issuing is idempotent on (code, customer_id, trigger_key) and this command
// uses a fixed key, so re-running it is safe: an account that already has the
// entitlement keeps the one it has, with its original expiry.
//
// Defaults to dry-run. Pass -confirm to write.
//
// Usage:
//
//	DUPLI1_PRODUCT_DB=postgres://dupli1:dupli1_dev@localhost:5433/products?sslmode=disable \
//	DUPLI1_AUTH_URL=http://localhost:8080 DUPLI1_AUTH_TOKEN=<access token with user.read> \
//	    go run ./cmd/backfill-welcome-promotion -code WELCOME50
//	… go run ./cmd/backfill-welcome-promotion -code WELCOME50 -confirm
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/elug3/dupli1/product/pkg/domain"
	"github.com/elug3/dupli1/product/pkg/infra/pg"
	"github.com/elug3/dupli1/product/pkg/service"
)

// backfillTriggerKey is fixed rather than per-run, so a second run of this
// command is a no-op instead of handing everyone a second entitlement.
const backfillTriggerKey = "backfill:welcome"

type authUser struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	AccountType string `json:"account_type"`
	IsActive    bool   `json:"is_active"`
}

func main() {
	fs := flag.NewFlagSet("backfill-welcome-promotion", flag.ExitOnError)
	productDB := fs.String("product-db", os.Getenv("DUPLI1_PRODUCT_DB"), "product database URL")
	authURL := fs.String("auth-url", os.Getenv("DUPLI1_AUTH_URL"), "auth base URL (gateway is fine)")
	authToken := fs.String("auth-token", os.Getenv("DUPLI1_AUTH_TOKEN"), "access token with user.read")
	code := fs.String("code", domain.WelcomeCode, "promotional code to issue")
	confirm := fs.Bool("confirm", false, "actually issue entitlements (dry-run without this)")
	includeInactive := fs.Bool("include-inactive", false, "also issue to deactivated accounts")
	limit := fs.Int("limit", 0, "cap accounts processed (0 = all)")
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `Usage: backfill-welcome-promotion [OPTIONS]

Issues a single-user promotional code to every existing customer account.
Idempotent: re-running issues nothing to accounts that already have it.

Options:
  -product-db string       Product DB URL (also DUPLI1_PRODUCT_DB)
  -auth-url string         Auth base URL (also DUPLI1_AUTH_URL)
  -auth-token string       Bearer token with user.read (also DUPLI1_AUTH_TOKEN)
  -code string             Code to issue (default: the sign-up campaign)
  -confirm                 Actually write (dry-run without this)
  -include-inactive        Also issue to deactivated accounts
  -limit int               Cap accounts processed (0 = all)
`)
	}
	if err := fs.Parse(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
	for name, value := range map[string]string{
		"-product-db": *productDB,
		"-auth-url":   *authURL,
		"-auth-token": *authToken,
		"-code":       *code,
	} {
		if strings.TrimSpace(value) == "" {
			fmt.Fprintf(os.Stderr, "%s is required\n", name)
			fs.Usage()
			os.Exit(1)
		}
	}

	ctx := context.Background()
	users, err := fetchCustomers(ctx, *authURL, *authToken)
	if err != nil {
		log.Fatalf("list accounts from auth: %v", err)
	}

	store, err := pg.NewProductStore(*productDB)
	if err != nil {
		log.Fatalf("open product db: %v", err)
	}
	defer store.Close()

	promotionStore, err := pg.NewPromotionStore(store.Pool())
	if err != nil {
		log.Fatalf("open promotions: %v", err)
	}
	promotions := service.NewPromotionService(promotionStore).
		WithLedger(pg.NewPromotionRedemptionStore(store.Pool())).
		WithEntitlements(pg.NewPromotionEntitlementStore(store.Pool()))

	// Check the campaign before touching a single account, so a typo in -code
	// fails once here instead of once per customer.
	definition, err := promotions.Get(ctx, *code)
	if err != nil {
		log.Fatalf("promotion %s: %v", *code, err)
	}
	if definition.EffectiveScope() != domain.ScopeSingleUser {
		log.Fatalf("promotion %s has scope %q; only single_user codes are issued to an account",
			definition.Code, definition.EffectiveScope())
	}
	if !definition.Active {
		log.Printf("warning: promotion %s is inactive — entitlements will be issued but cannot be used until it is enabled", definition.Code)
	}
	log.Printf("promotion %s: %d-day entitlement window, %d accounts to consider",
		definition.Code, definition.EntitlementTTLDays, len(users))

	var issued, skipped, failed int
	for i, user := range users {
		if *limit > 0 && i >= *limit {
			break
		}
		if !user.IsActive && !*includeInactive {
			skipped++
			continue
		}
		if !*confirm {
			issued++
			continue
		}
		entitlement, err := promotions.Issue(ctx, *code, user.ID, "backfill", backfillTriggerKey, "")
		if err != nil {
			log.Printf("issue to %s (%s): %v", user.ID, user.Email, err)
			failed++
			continue
		}
		issued++
		if entitlement.ExpiresAt != nil {
			log.Printf("issued %s to %s, expires %s", *code, user.ID, entitlement.ExpiresAt.Format(time.RFC3339))
		} else {
			log.Printf("issued %s to %s", *code, user.ID)
		}
	}

	mode := "DRY RUN — no entitlements written; re-run with -confirm"
	if *confirm {
		mode = "done"
	}
	log.Printf("%s: %d customer accounts, %d issued, %d skipped (inactive), %d failed",
		mode, len(users), issued, skipped, failed)
	if failed > 0 {
		os.Exit(1)
	}
}

// fetchCustomers reads accounts from auth and keeps the shoppers.
//
// Managers and service accounts are dropped for the same reason the
// registration issuer drops them: a discount in a wallet nobody buys from is
// noise in the campaign's numbers.
func fetchCustomers(ctx context.Context, baseURL, token string) ([]authUser, error) {
	url := strings.TrimRight(baseURL, "/") + "/api/v1/auth/users"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("auth returned %s", resp.Status)
	}

	var body struct {
		Users []authUser `json:"users"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}

	customers := make([]authUser, 0, len(body.Users))
	for _, u := range body.Users {
		if strings.EqualFold(u.AccountType, "customer") {
			customers = append(customers, u)
		}
	}
	return customers, nil
}
