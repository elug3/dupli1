package bootstrap

import (
	"context"
	"fmt"

	"github.com/elug3/dupli1/auth/pkg/domain"
	"github.com/elug3/dupli1/auth/pkg/ports"
	"github.com/elug3/dupli1/shared/pkg/permissions"
	"github.com/google/uuid"
)

var webServicePermissions = []string{permissions.UserCreate}

// seedWebServiceAccount creates or updates the dupli1-web service account when configured.
// It is idempotent: repeated calls keep the same user id and sync password, permissions,
// account type, and active status so ECS secret rotations take effect on next auth boot.
func seedWebServiceAccount(ctx context.Context, cfg Config, repo ports.UserRepository) error {
	if cfg.WebServiceEmail == "" {
		return nil
	}
	if cfg.WebServicePassword == "" {
		return fmt.Errorf("seed web service account: DUPLI1_WEB_SERVICE_PASSWORD is required when DUPLI1_WEB_SERVICE_EMAIL is set")
	}

	existing, err := repo.FindByEmail(ctx, cfg.WebServiceEmail)
	if err != nil {
		return fmt.Errorf("seed web service account: lookup: %w", err)
	}
	if existing != nil {
		return syncWebServiceAccount(ctx, cfg, repo, existing)
	}

	u, err := domain.NewUser(
		uuid.New().String(),
		cfg.WebServiceEmail,
		cfg.WebServicePassword,
		domain.AccountTypeService,
		webServicePermissions...,
	)
	if err != nil {
		return fmt.Errorf("seed web service account: create: %w", err)
	}
	if err := repo.Save(ctx, u); err != nil {
		return fmt.Errorf("seed web service account: save: %w", err)
	}

	cfg.Logger.Info().
		Str("event", "web_service_account_seeded").
		Str("email", cfg.WebServiceEmail).
		Msg("dupli1-web service account seeded")
	return nil
}

func syncWebServiceAccount(ctx context.Context, cfg Config, repo ports.UserRepository, u *domain.User) error {
	changed := false
	if !u.ValidatePassword(cfg.WebServicePassword) {
		if err := u.UpdatePassword(cfg.WebServicePassword); err != nil {
			return fmt.Errorf("seed web service account: update password: %w", err)
		}
		changed = true
	}
	if u.AccountType != domain.AccountTypeService {
		u.AccountType = domain.AccountTypeService
		changed = true
	}
	if !u.HasPermission(permissions.UserCreate) || len(u.Permissions) != 1 {
		u.SetPermissions(webServicePermissions)
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
		return fmt.Errorf("seed web service account: sync save: %w", err)
	}
	cfg.Logger.Info().
		Str("event", "web_service_account_synced").
		Str("email", cfg.WebServiceEmail).
		Msg("dupli1-web service account credentials/permissions synced")
	return nil
}
