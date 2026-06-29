package bootstrap

import (
	"context"
	"fmt"

	"github.com/elug3/schick/auth/pkg/domain"
	"github.com/elug3/schick/auth/pkg/ports"
	"github.com/google/uuid"
)

// seedWebServiceAccount creates the schick-web service account when configured.
// It is idempotent: repeated calls with the same email are no-ops.
func seedWebServiceAccount(ctx context.Context, cfg Config, repo ports.UserRepository) error {
	if cfg.WebServiceEmail == "" {
		return nil
	}
	if cfg.WebServicePassword == "" {
		return fmt.Errorf("seed web service account: SCHICK_WEB_SERVICE_PASSWORD is required when SCHICK_WEB_SERVICE_EMAIL is set")
	}

	existing, err := repo.FindByEmail(ctx, cfg.WebServiceEmail)
	if err != nil {
		return fmt.Errorf("seed web service account: lookup: %w", err)
	}
	if existing != nil {
		return nil
	}

	u, err := domain.NewUser(
		uuid.New().String(),
		cfg.WebServiceEmail,
		cfg.WebServicePassword,
		domain.RoleCustomerRegistrar,
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
		Msg("schick-web service account seeded")
	return nil
}
