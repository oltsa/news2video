package main

import (
	"context"
	"fmt"
	"os"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

func NewTemplatesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "templates",
		Short:   "Manage templates",
		Aliases: []string{"template"},
	}
	cmd.AddCommand(NewTemplatesAddCmd())
	return cmd
}

func NewTemplatesAddCmd() *cobra.Command {
	var projectKey string
	var isSystemTemplate bool

	cmd := &cobra.Command{
		Use:   "add <template-id> <path/to/template.json>",
		Short: "Add or update a template from a JSON file",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			templateID := args[0]
			filePath := args[1]

			// Read the template file content
			templateData, err := os.ReadFile(filePath)
			if err != nil {
				return fmt.Errorf("failed to read template file '%s': %w", filePath, err)
			}

			var projectID *uuid.UUID // Use a pointer to handle NULL for system templates
			if !isSystemTemplate {
				if projectKey == "" {
					return fmt.Errorf("you must specify either --project or --system")
				}
				var pID uuid.UUID
				err := dbpool.QueryRow(context.Background(), "SELECT id FROM projects WHERE project_key = $1", projectKey).Scan(&pID)
				if err != nil {
					return fmt.Errorf("could not find project with key '%s': %w", projectKey, err)
				}
				projectID = &pID
			}

			// Upsert the template
			// The ON CONFLICT clause needs to be different for system vs. project templates
			// due to the partial unique indexes in the schema.
			var query string
			if isSystemTemplate {
				query = `
                    INSERT INTO templates (id, project_id, template_data)
                    VALUES ($1, NULL, $2)
                    ON CONFLICT (id) WHERE project_id IS NULL
                    DO UPDATE SET template_data = EXCLUDED.template_data;
                `
				_, err = dbpool.Exec(context.Background(), query, templateID, templateData)
			} else {
				query = `
                    INSERT INTO templates (id, project_id, template_data)
                    VALUES ($1, $2, $3)
                    ON CONFLICT (id, project_id) WHERE project_id IS NOT NULL
                    DO UPDATE SET template_data = EXCLUDED.template_data;
                `
				_, err = dbpool.Exec(context.Background(), query, templateID, projectID, templateData)
			}

			if err != nil {
				return fmt.Errorf("failed to upload template: %w", err)
			}

			if isSystemTemplate {
				fmt.Printf("Successfully uploaded system template '%s'.\n", templateID)
			} else {
				fmt.Printf("Successfully uploaded template '%s' for project '%s'.\n", templateID, projectKey)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&projectKey, "project", "", "The key of the project to add this template to")
	cmd.Flags().BoolVar(&isSystemTemplate, "system", false, "Flag to mark this as a system-wide template")
	cmd.MarkFlagsMutuallyExclusive("project", "system")

	return cmd
}
