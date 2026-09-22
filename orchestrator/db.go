package main

import (
	"context"

	"github.com/google/uuid"
)

// getDestinationsForProject queries the database for all configured destinations for a given project.
func getDestinationsForProject(ctx context.Context, projectID uuid.UUID) ([]Destination, error) {
	query := `SELECT id, platform, config FROM connections WHERE project_id = $1`
	rows, err := dbpool.Query(ctx, query, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var destinations []Destination
	for rows.Next() {
		var d Destination
		if err := rows.Scan(&d.ConnectionID, &d.Platform, &d.Config); err != nil {
			return nil, err
		}
		destinations = append(destinations, d)
	}

	return destinations, nil
}
