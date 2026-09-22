package main

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/bcrypt"
)

// NewUsersCmd creates the `users` command and its subcommands.
func NewUsersCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "users",
		Short:   "Manage users",
		Aliases: []string{"user"},
	}
	cmd.AddCommand(NewUsersListCmd())
	cmd.AddCommand(NewUsersAddCmd())
	cmd.AddCommand(NewUsersRemoveCmd())
	cmd.AddCommand(NewUsersSetPasswordCmd())
	return cmd
}

// NewUsersListCmd creates the `users list` command.
func NewUsersListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all users in the database",
		RunE: func(cmd *cobra.Command, args []string) error {
			query := `
                SELECT 
                    u.email, 
                    COALESCE(o.organization_key, 'N/A (System Admin)'), 
                    u.is_customer_admin,
                    u.is_system_admin
                FROM users u
                LEFT JOIN organizations o ON u.organization_id = o.id
                ORDER BY o.organization_key, u.email
            `
			rows, err := dbpool.Query(context.Background(), query)
			if err != nil {
				return fmt.Errorf("failed to query users: %w", err)
			}
			defer rows.Close()

			fmt.Println("---------------------------------------------------------------------")
			fmt.Printf("%-35s | %-20s | %-10s\n", "EMAIL", "ORGANIZATION", "ADMIN TYPE")
			fmt.Println("---------------------------------------------------------------------")

			count := 0
			for rows.Next() {
				var email, orgKey string
				var isCustomerAdmin, isSystemAdmin bool
				if err := rows.Scan(&email, &orgKey, &isCustomerAdmin, &isSystemAdmin); err != nil {
					return fmt.Errorf("failed to scan user row: %w", err)
				}

				adminStatus := "Regular"
				if isCustomerAdmin {
					adminStatus = "Customer"
				}
				if isSystemAdmin {
					adminStatus = "System"
				}

				fmt.Printf("%-35s | %-20s | %-10s\n", email, orgKey, adminStatus)
				count++
			}

			fmt.Println("---------------------------------------------------------------------")
			if count == 0 {
				fmt.Println("No users found in the database.")
			} else {
				fmt.Printf("Found %d user(s).\n", count)
			}

			return nil
		},
	}
	return cmd
}

// NewUsersAddCmd creates the `users add` command.
func NewUsersAddCmd() *cobra.Command {
	var orgKey string
	var isAdmin bool

	cmd := &cobra.Command{
		Use:   "add <email> <password>",
		Short: "Add a new user to an organization",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			email := args[0]
			password := args[1]

			// Find the organization ID from its key
			var orgID uuid.UUID
			err := dbpool.QueryRow(context.Background(), "SELECT id FROM organizations WHERE organization_key = $1", orgKey).Scan(&orgID)
			if err != nil {
				return fmt.Errorf("could not find organization with key '%s': %w", orgKey, err)
			}

			// Generate the password hash
			hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
			if err != nil {
				return fmt.Errorf("failed to hash password: %w", err)
			}

			// Insert the new user
			var userID string
			query := `
                INSERT INTO users (organization_id, email, hashed_password, is_customer_admin) 
                VALUES ($1, $2, $3, $4) 
                RETURNING id
            `
			err = dbpool.QueryRow(context.Background(), query, orgID, email, string(hashedPassword), isAdmin).Scan(&userID)
			if err != nil {
				return fmt.Errorf("failed to create user: %w", err)
			}

			fmt.Printf("✅ Successfully created user '%s' in organization '%s' with ID: %s\n", email, orgKey, userID)
			return nil
		},
	}

	cmd.Flags().StringVar(&orgKey, "org", "", "The key of the organization this user belongs to (required)")
	cmd.Flags().BoolVar(&isAdmin, "admin", false, "Set to true to make the user a customer admin for their organization")
	cmd.MarkFlagRequired("org")

	return cmd
}

// NewUsersRemoveCmd creates the `users remove` command.
func NewUsersRemoveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove <email>",
		Short: "Remove a user from the database",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			email := args[0]

			query := `DELETE FROM users WHERE email = $1`
			result, err := dbpool.Exec(context.Background(), query, email)
			if err != nil {
				return fmt.Errorf("failed to remove user: %w", err)
			}

			if result.RowsAffected() == 0 {
				return fmt.Errorf("no user found with email '%s'", email)
			}

			fmt.Printf("✅ Successfully removed user '%s'.\n", email)
			return nil
		},
	}
	return cmd
}

// NewUsersSetPasswordCmd creates the `users set-password` command.
func NewUsersSetPasswordCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set-password <email> <password>",
		Short: "Set or update a user's password",
		Long:  "Generates a secure bcrypt hash for the given password and updates the user in the database.",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			email := args[0]
			password := args[1]

			hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
			if err != nil {
				return fmt.Errorf("failed to hash password: %w", err)
			}

			query := `UPDATE users SET hashed_password = $1 WHERE email = $2`
			result, err := dbpool.Exec(context.Background(), query, string(hashedPassword), email)
			if err != nil {
				return fmt.Errorf("failed to update password in database: %w", err)
			}

			if result.RowsAffected() == 0 {
				return fmt.Errorf("no user found with email '%s'", email)
			}

			fmt.Printf("✅ Successfully updated password for '%s'.\n", email)
			return nil
		},
	}
	return cmd
}
