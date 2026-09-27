package bootstrap

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"

	"github.com/elug3/dupli1/auth/pkg/domain"
	"github.com/elug3/dupli1/auth/pkg/ports"
	"github.com/elug3/dupli1/shared/pkg/permissions"
	"github.com/elug3/dupli1/shared/pkg/serviceaccount"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

var webServicePermissions = []string{permissions.UserCreate}

var orderServicePermissions = []string{
	permissions.OrderShip,
	permissions.OrderStatusUpdate,
	permissions.InventoryReservationManage,
	permissions.PaymentCancel,   // paid-order cancel refunds via the payment service
	permissions.PromotionRedeem, // checkout reserve/consume/release on product ledger
}

// envKeySyncer is the part of the auth service the seeders use to keep a
// service account's env-seeded API key in step with its env var.
type envKeySyncer interface {
	SyncEnvAPIKey(ctx context.Context, userID, plaintext string) (string, error)
}

// serviceAccountSpec is one machine account auth seeds at boot.
type serviceAccountSpec struct {
	label       string // for logs and errors: "web", "order"
	email       string
	password    string
	apiKey      string
	name        string // service_name claim
	permissions []string
	envPrefix   string // DUPLI1_<X>_SERVICE, for error messages
}

// seedWebServiceAccount seeds the dupli1-web service account when configured.
func seedWebServiceAccount(ctx context.Context, cfg Config, repo ports.UserRepository, keys envKeySyncer) error {
	return seedServiceAccount(ctx, cfg.Logger, repo, keys, serviceAccountSpec{
		label: "web", email: cfg.WebServiceEmail, password: cfg.WebServicePassword, apiKey: cfg.WebServiceAPIKey,
		name: serviceaccount.Web, permissions: webServicePermissions, envPrefix: "DUPLI1_WEB_SERVICE",
	})
}

// seedOrderServiceAccount seeds the dupli1-order service account when configured.
func seedOrderServiceAccount(ctx context.Context, cfg Config, repo ports.UserRepository, keys envKeySyncer) error {
	return seedServiceAccount(ctx, cfg.Logger, repo, keys, serviceAccountSpec{
		label: "order", email: cfg.OrderServiceEmail, password: cfg.OrderServicePassword, apiKey: cfg.OrderServiceAPIKey,
		name: serviceaccount.Order, permissions: orderServicePermissions, envPrefix: "DUPLI1_ORDER_SERVICE",
	})
}

// seedServiceAccount creates or updates a service account and its env API
// key. It is idempotent: repeated boots keep the same user id and re-sync the
// password, permissions, account type, service name, active status and API
// key, so a rotated secret takes effect on the next auth boot.
//
// The account authenticates with its API key, its password, or both while
// callers move to keys. With no password configured the account has none at
// all (User.RetirePassword) — so unsetting *_SERVICE_PASSWORD retires
// password login for it.
func seedServiceAccount(ctx context.Context, log zerolog.Logger, repo ports.UserRepository, keys envKeySyncer, spec serviceAccountSpec) error {
	if spec.email == "" {
		return nil
	}
	if spec.password == "" && spec.apiKey == "" {
		return fmt.Errorf("seed %s service account: %s_API_KEY (or, during the move to keys, %s_PASSWORD) is required when %s_EMAIL is set",
			spec.label, spec.envPrefix, spec.envPrefix, spec.envPrefix)
	}

	u, err := repo.FindByEmail(ctx, spec.email)
	if err != nil {
		return fmt.Errorf("seed %s service account: lookup: %w", spec.label, err)
	}
	event := ""
	if u == nil {
		password := spec.password
		if password == "" {
			// NewUser needs something to hash; it is replaced right below.
			if password, err = unusablePassword(); err != nil {
				return fmt.Errorf("seed %s service account: %w", spec.label, err)
			}
		}
		u, err = domain.NewUser(uuid.New().String(), spec.email, password, domain.AccountTypeService, spec.permissions...)
		if err != nil {
			return fmt.Errorf("seed %s service account: create: %w", spec.label, err)
		}
		if spec.password == "" {
			u.RetirePassword()
		}
		u.ServiceName = spec.name
		event = "seeded"
	} else if changed, err := syncServiceAccount(u, spec); err != nil {
		return fmt.Errorf("seed %s service account: %w", spec.label, err)
	} else if changed {
		event = "synced"
	}
	if event != "" {
		if err := repo.Save(ctx, u); err != nil {
			return fmt.Errorf("seed %s service account: save: %w", spec.label, err)
		}
		log.Info().Str("event", spec.label+"_service_account_"+event).Str("email", spec.email).
			Bool("password_login", spec.password != "").Msgf("dupli1-%s service account %s", spec.label, event)
	}

	if keys == nil {
		return nil
	}
	outcome, err := keys.SyncEnvAPIKey(ctx, u.ID, spec.apiKey)
	if err != nil {
		return fmt.Errorf("seed %s service account: %s_API_KEY: %w", spec.label, spec.envPrefix, err)
	}
	if outcome != "" {
		log.Info().Str("event", "api_key_env_"+outcome).Str("user_id", u.ID).Str("service", spec.name).
			Msgf("dupli1-%s env api key %s", spec.label, outcome)
	}
	return nil
}

// syncServiceAccount brings an existing account in line with spec and
// reports whether anything changed.
func syncServiceAccount(u *domain.User, spec serviceAccountSpec) (bool, error) {
	changed := false
	if spec.password != "" && !u.ValidatePassword(spec.password) {
		if err := u.UpdatePassword(spec.password); err != nil {
			return false, fmt.Errorf("update password: %w", err)
		}
		changed = true
	}
	// No password configured: drop whatever was set before, so a password
	// removed from the environment stops working.
	if spec.password == "" && !u.PasswordRetired() {
		u.RetirePassword()
		changed = true
	}
	if u.AccountType != domain.AccountTypeService {
		u.AccountType = domain.AccountTypeService
		changed = true
	}
	if u.ServiceName != spec.name {
		u.ServiceName = spec.name
		changed = true
	}
	if !hasExactPermissions(u, spec.permissions) {
		u.SetPermissions(spec.permissions)
		changed = true
	}
	if !u.IsActive {
		u.SetActive(true)
		changed = true
	}
	if u.IsLocked() {
		u.Unlock()
		changed = true
	}
	return changed, nil
}

// unusablePassword is 32 random bytes, only for NewUser to hash before the
// password is retired.
func unusablePassword() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate password: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func hasExactPermissions(u *domain.User, want []string) bool {
	if len(u.Permissions) != len(want) {
		return false
	}
	for _, p := range want {
		if !u.HasPermission(p) {
			return false
		}
	}
	return true
}
