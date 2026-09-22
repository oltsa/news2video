package main

import (
	"context"
	"fmt"
	"os"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

func NewConnectionsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "connections",
		Short:   "Manage destination connections",
		Aliases: []string{"conn"},
	}
	cmd.AddCommand(NewConnectionsAddCmd())
	return cmd
}

func NewConnectionsAddCmd() *cobra.Command {
	var projectKey string
	var orgKey string // Add a variable to hold the organization key

	cmd := &cobra.Command{
		Use:   "add <platform> <path/to/config.json>",
		Short: "Add a new destination connection for a project",
		Long:  `Adds a new destination. Platform can be 's3', 'webhook', etc. The config file should be a JSON file. For S3, it can be an empty JSON object: '{}'.`,
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			platform := args[0]
			configPath := args[1]

			configData, err := os.ReadFile(configPath)
			if err != nil {
				return fmt.Errorf("failed to read config file '%s': %w", configPath, err)
			}

			// --- MODIFICATION START ---
			// Find the project ID using BOTH the organization and project key
			var projectID uuid.UUID
			query := `
                SELECT p.id 
                FROM projects p
                JOIN organizations o ON p.organization_id = o.id
                WHERE p.project_key = $1 AND o.organization_key = $2
            `
			err = dbpool.QueryRow(context.Background(), query, projectKey, orgKey).Scan(&projectID)
			if err != nil {
				return fmt.Errorf("could not find project with key '%s' in organization '%s': %w", projectKey, orgKey, err)
			}
			// --- MODIFICATION END ---

			// Insert the connection
			var connID uuid.UUID
			insertQuery := `INSERT INTO connections (project_id, platform, config) VALUES ($1, $2, $3) RETURNING id`
			err = dbpool.QueryRow(context.Background(), insertQuery, projectID, platform, configData).Scan(&connID)
			if err != nil {
				return fmt.Errorf("failed to create connection: %w", err)
			}

			fmt.Printf("Successfully created '%s' connection for project '%s' (Org: %s) with ID: %s\n", platform, projectKey, orgKey, connID)
			return nil
		},
	}

	// --- MODIFICATION START ---
	// Update flags to require both org and project
	cmd.Flags().StringVar(&projectKey, "project", "", "The key of the project (required)")
	cmd.Flags().StringVar(&orgKey, "org", "", "The key of the organization (required)")
	cmd.MarkFlagRequired("project")
	cmd.MarkFlagRequired("org")
	// --- MODIFICATION END ---

	return cmd
}
