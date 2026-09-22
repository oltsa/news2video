package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

func NewOrgsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "orgs",
		Short:   "Manage organizations",
		Aliases: []string{"org"},
	}

	cmd.AddCommand(NewOrgsAddCmd())
	return cmd
}

func NewOrgsAddCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add <organization-key> <plan-id>",
		Short: "Add a new organization",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			orgKey := args[0]
			planID := args[1]

			var orgID string
			query := `INSERT INTO organizations (organization_key, plan_id) VALUES ($1, $2) RETURNING id`
			err := dbpool.QueryRow(context.Background(), query, orgKey, planID).Scan(&orgID)
			if err != nil {
				return fmt.Errorf("failed to create organization: %w", err)
			}

			fmt.Printf("Successfully created organization '%s' with ID: %s\n", orgKey, orgID)
			return nil
		},
	}
	return cmd
}
