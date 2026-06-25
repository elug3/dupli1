package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"fmt"
	"os"

	_ "github.com/lib/pq"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/bcrypt"
)

const (
	adminId    = "00000000-0000-0000-0000-000000000001"
	adminEmail = "admin@dupli1.com"
)

func newResetCmd() *cobra.Command {
	defaultDB := os.Getenv("DB_URL")

	var dbURL string
	var email string

	cmd := &cobra.Command{
		Use:           "reset",
		Short:         "Reset the admin account and print new credentials",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			plainPassword, err := generatePassword(16)
			if err != nil {
				return fmt.Errorf("generate password: %w", err)
			}

			hashed, err := bcrypt.GenerateFromPassword([]byte(plainPassword), bcrypt.DefaultCost)
			if err != nil {
				return fmt.Errorf("hash password: %w", err)
			}
			password := string(hashed)

			db, err := sql.Open("postgres", dbURL)
			if err != nil {
				return fmt.Errorf("open db: %w", err)
			}
			defer db.Close()

			if err := db.PingContext(context.Background()); err != nil {
				return fmt.Errorf("connect db: %w", err)
			}

			// TODO: Use authService.updatePassword() instead direct query
			const query = `
				INSERT INTO users (id, email, password, roles)
				VALUES ($1, $2, $3, $4)
				ON CONFLICT (id) DO UPDATE SET email = EXCLUDED.email, password = EXCLUDED.password, roles = EXCLUDED.roles`

			if _, err := db.ExecContext(context.Background(), query, adminId, email, password, "{admin}"); err != nil {
				return fmt.Errorf("upsert admin: %w", err)
			}

			fmt.Printf("Admin account reset successfully.\n")
			fmt.Printf("  Email:    %s\n", email)
			fmt.Printf("  Password: %s\n", plainPassword)
			return nil
		},
	}

	cmd.Flags().StringVar(&dbURL, "db", defaultDB, "Database connection URL")
	cmd.Flags().StringVar(&email, "email", adminEmail, "Admin account email")

	return cmd
}

func generatePassword(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b)[:n], nil
}
