package main

import (
	"context"
	"fmt"
	"os"

	"github.com/elug3/dupli1/auth/pkg/bootstrap"
	"github.com/spf13/cobra"
)

const adminID = "00000000-0000-0000-0000-000000000001"

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
			if email == "" {
				return fmt.Errorf("--email is required")
			}

			plainPassword, err := bootstrap.ResetAdminAccount(context.Background(), dbURL, adminID, email)
			if err != nil {
				return err
			}

			fmt.Printf("Admin account reset successfully.\n")
			fmt.Printf("  Email:    %s\n", email)
			fmt.Printf("  Password: %s\n", plainPassword)
			return nil
		},
	}

	cmd.Flags().StringVar(&dbURL, "db", defaultDB, "Database connection URL")
	cmd.Flags().StringVar(&email, "email", "", "Admin account email (required)")

	return cmd
}
