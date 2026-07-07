package bootstrap

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"

	"github.com/elug3/dupli1/auth/pkg/domain"
	"github.com/elug3/dupli1/auth/pkg/infra/postgres"
	"github.com/elug3/dupli1/shared/pkg/permissions"
	"github.com/rs/zerolog"
)

// ResetAdminAccount resets or creates the admin user and returns the new plaintext password.
func ResetAdminAccount(ctx context.Context, dbURL, adminID, email string) (string, error) {
	if email == "" {
		return "", fmt.Errorf("email is required")
	}

	plainPassword, err := generatePassword(16)
	if err != nil {
		return "", fmt.Errorf("generate password: %w", err)
	}

	db, err := openPostgres(ctx, dbURL, 0, zerolog.Nop())
	if err != nil {
		return "", err
	}
	defer db.Close()

	if err := migrateSchema(ctx, db); err != nil {
		return "", fmt.Errorf("migrate: %w", err)
	}

	repo := postgres.NewUserRepository(db)
	user, err := repo.FindByID(ctx, adminID)
	if err != nil {
		return "", fmt.Errorf("find admin: %w", err)
	}

	adminPerms := permissions.ExpandLegacyRoles([]string{permissions.RoleAdmin})

	if user == nil {
		user, err = domain.NewUser(adminID, email, plainPassword, domain.AccountTypeAdmin, adminPerms...)
		if err != nil {
			return "", fmt.Errorf("create admin: %w", err)
		}
	} else {
		user.Email = email
		user.SetPermissions(adminPerms)
		user.AccountType = domain.AccountTypeAdmin
		if err := user.UpdatePassword(plainPassword); err != nil {
			return "", fmt.Errorf("update password: %w", err)
		}
	}

	if err := repo.Save(ctx, user); err != nil {
		return "", fmt.Errorf("save admin: %w", err)
	}
	return plainPassword, nil
}

func generatePassword(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b)[:n], nil
}
