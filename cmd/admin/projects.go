package main

import (
	"context"
	"fmt"
	"os"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

func NewProjectsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "projects",
		Short:   "Manage projects",
		Aliases: []string{"project"},
	}

	cmd.AddCommand(NewProjectsAddCmd())
	return cmd
}

func NewProjectsAddCmd() *cobra.Command {
	var orgKey string

	cmd := &cobra.Command{
		Use:   "add <project-key>",
		Short: "Add a new project to an organization and create its S3 structure",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projectKey := args[0]
			ctx := context.Background()

			// 1. Find the organization ID from its key
			var orgID uuid.UUID
			err := dbpool.QueryRow(ctx, "SELECT id FROM organizations WHERE organization_key = $1", orgKey).Scan(&orgID)
			if err != nil {
				return fmt.Errorf("could not find organization with key '%s': %w", orgKey, err)
			}

			// 2. Insert the new project and get its generated API key
			var apiKey uuid.UUID
			query := `INSERT INTO projects (organization_id, project_key) VALUES ($1, $2) RETURNING api_key`
			err = dbpool.QueryRow(ctx, query, orgID, projectKey).Scan(&apiKey)
			if err != nil {
				return fmt.Errorf("failed to create project in database: %w", err)
			}
			fmt.Printf("Successfully created project '%s' for organization '%s'.\n", projectKey, orgKey)

			// 3. Create the S3 directory structure
			s3Client, err := initS3Client(ctx)
			if err != nil {
				return fmt.Errorf("failed to initialize S3 client: %w", err)
			}
			bucketName := os.Getenv("S3_BUCKET_NAME")
			if bucketName == "" {
				return fmt.Errorf("S3_BUCKET_NAME environment variable is not set")
			}

			err = createS3ProjectStructure(ctx, s3Client, bucketName, orgKey, projectKey)
			if err != nil {
				// Log the error but don't fail the whole command, as the DB part succeeded.
				// The user can create the directories manually if needed.
				fmt.Fprintf(os.Stderr, "Warning: failed to create S3 directory structure: %v\n", err)
			}

			fmt.Printf("🔑 API Key: %s\n", apiKey.String())
			return nil
		},
	}

	cmd.Flags().StringVar(&orgKey, "org", "", "The key of the organization this project belongs to (required)")
	cmd.MarkFlagRequired("org")

	return cmd
}
