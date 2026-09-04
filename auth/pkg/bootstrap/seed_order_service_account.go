package bootstrap

import (
	"context"
	"fmt"

	"github.com/elug3/dupli1/auth/pkg/domain"
	"github.com/elug3/dupli1/auth/pkg/ports"
	"github.com/elug3/dupli1/shared/pkg/permissions"
	"github.com/google/uuid"
)

var orderServicePermissions = []string{
	permissions.OrderShip,
	permissions.OrderStatusUpdate,
	permissions.InventoryReservationManage,
}

// seedOrderServiceAccount creates or updates the dupli1-order service account when configured.
// It is idempotent: repeated calls keep the same user id and sync password, permissions,
// account type, and active status so ECS secret rotations take effect on next auth boot.
func seedOrderServiceAccount(ctx context.Context, cfg Config, repo ports.UserRepository) error {
	if cfg.OrderServiceEmail == "" {
		return nil
	}
	if cfg.OrderServicePassword == "" {
		return fmt.Errorf("seed order service account: DUPLI1_ORDER_SERVICE_PASSWORD is required when DUPLI1_ORDER_SERVICE_EMAIL is set")
	}

	existing, err := repo.FindByEmail(ctx, cfg.OrderServiceEmail)
	if err != nil {
		return fmt.Errorf("seed order service account: lookup: %w", err)
	}
	if existing != nil {
		return syncOrderServiceAccount(ctx, cfg, repo, existing)
	}

	u, err := domain.NewUser(
		uuid.New().String(),
		cfg.OrderServiceEmail,
		cfg.OrderServicePassword,
		domain.AccountTypeService,
		orderServicePermissions...,
	)
	if err != nil {
		return fmt.Errorf("seed order service account: create: %w", err)
	}
	if err := repo.Save(ctx, u); err != nil {
		return fmt.Errorf("seed order service account: save: %w", err)
	}

	cfg.Logger.Info().
		Str("event", "order_service_account_seeded").
		Str("email", cfg.OrderServiceEmail).
		Msg("dupli1-order service account seeded")
	return nil
}

func syncOrderServiceAccount(ctx context.Context, cfg Config, repo ports.UserRepository, u *domain.User) error {
	changed := false
	if !u.ValidatePassword(cfg.OrderServicePassword) {
		if err := u.UpdatePassword(cfg.OrderServicePassword); err != nil {
			return fmt.Errorf("seed order service account: update password: %w", err)
		}
		changed = true
	}
	if u.AccountType != domain.AccountTypeService {
		u.AccountType = domain.AccountTypeService
		changed = true
	}
	if !hasExactPermissions(u, orderServicePermissions) {
		u.SetPermissions(orderServicePermissions)
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
	if !changed {
		return nil
	}
	if err := repo.Save(ctx, u); err != nil {
		return fmt.Errorf("seed order service account: sync save: %w", err)
	}
	cfg.Logger.Info().
		Str("event", "order_service_account_synced").
		Str("email", cfg.OrderServiceEmail).
		Msg("dupli1-order service account credentials/permissions synced")
	return nil
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
