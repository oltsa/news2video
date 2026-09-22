package main

import (
	"context"
	"fmt"
	"os"

	"github.com/go-redis/redis/v8"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"
)

var dbpool *pgxpool.Pool
var rdb *redis.Client

func main() {
	var databaseURL string
	var redisAddr string // Variable to hold the Redis address

	var rootCmd = &cobra.Command{
		Use:   "admin-cli",
		Short: "A CLI tool for managing the video generation system.",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			var err error

			// --- Postgres Connection ---
			// Use the flag if provided, otherwise fall back to the environment variable.
			if databaseURL == "" {
				databaseURL = os.Getenv("DATABASE_URL")
			}
			if databaseURL == "" {
				return fmt.Errorf("database-url flag or DATABASE_URL environment variable must be set")
			}
			dbpool, err = pgxpool.New(context.Background(), databaseURL)
			if err != nil {
				return fmt.Errorf("unable to connect to database: %w", err)
			}

			// --- Redis Connection ---
			// Use the flag if provided, otherwise fall back to the environment variable.
			if redisAddr == "" {
				redisAddr = os.Getenv("JOB_QUEUE_ADDRESS")
			}
			if redisAddr == "" {
				return fmt.Errorf("redis-addr flag or JOB_QUEUE_ADDRESS environment variable must be set")
			}

			// Use NewClient directly for a more robust connection.
			rdb = redis.NewClient(&redis.Options{
				Addr: redisAddr,
			})

			if err := rdb.Ping(context.Background()).Err(); err != nil {
				return fmt.Errorf("unable to connect to redis at '%s': %w", redisAddr, err)
			}

			return nil
		},
		PersistentPostRun: func(cmd *cobra.Command, args []string) {
			if dbpool != nil {
				dbpool.Close()
			}
			if rdb != nil {
				rdb.Close()
			}
		},
	}

	// Define flags that can optionally override environment variables.
	rootCmd.PersistentFlags().StringVar(&databaseURL, "database-url", "", "PostgreSQL database URL (overrides DATABASE_URL env var)")
	rootCmd.PersistentFlags().StringVar(&redisAddr, "redis-addr", "", "Redis address (e.g. redis:6379) (overrides JOB_QUEUE_ADDRESS env var)")

	// Register all top-level commands.
	rootCmd.AddCommand(NewOrgsCmd())
	rootCmd.AddCommand(NewProjectsCmd())
	rootCmd.AddCommand(NewTemplatesCmd())
	rootCmd.AddCommand(NewConnectionsCmd())
	rootCmd.AddCommand(NewDlqCmd())
	rootCmd.AddCommand(NewUsersCmd()) // Add the new users command suite

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
