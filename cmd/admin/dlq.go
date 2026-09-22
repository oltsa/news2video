package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/go-redis/redis/v8"
	"github.com/spf13/cobra"
)

func NewDlqCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "dlq",
		Short:   "Manage Dead-Letter Queues (DLQs)",
		Aliases: []string{"dlqs"},
	}
	cmd.AddCommand(NewDlqListCmd())
	cmd.AddCommand(NewDlqReplayCmd())
	return cmd
}

func NewDlqListCmd() *cobra.Command {
	var count int64

	cmd := &cobra.Command{
		Use:   "list <stream-name>",
		Short: "List messages in a DLQ",
		Long:  "Lists messages from the top of a Dead-Letter Queue. Stream name should be the original stream (e.g., 'render_jobs').",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			streamName := args[0]
			dlqStreamName := streamName + "_dlq"
			ctx := context.Background()

			messages, err := rdb.XRangeN(ctx, dlqStreamName, "-", "+", count).Result()
			if err != nil {
				return fmt.Errorf("failed to read from DLQ '%s': %w", dlqStreamName, err)
			}

			if len(messages) == 0 {
				fmt.Printf("DLQ '%s' is empty.\n", dlqStreamName)
				return nil
			}

			fmt.Printf("Displaying last %d messages from DLQ '%s':\n\n", len(messages), dlqStreamName)
			for _, msg := range messages {
				fmt.Printf("--------------------------------------------------\n")
				fmt.Printf("ID: %s\n", msg.ID)
				if reason, ok := msg.Values["error_reason"].(string); ok {
					fmt.Printf("Error Reason: %s\n", reason)
				}
				if failedAt, ok := msg.Values["failed_at"].(string); ok {
					fmt.Printf("Failed At: %s\n", failedAt)
				}
				if payload, ok := msg.Values["payload"].(string); ok {
					// Pretty print the JSON payload
					var prettyPayload map[string]interface{}
					if json.Unmarshal([]byte(payload), &prettyPayload) == nil {
						prettyJSON, _ := json.MarshalIndent(prettyPayload, "", "  ")
						fmt.Printf("Payload:\n%s\n", string(prettyJSON))
					} else {
						fmt.Printf("Payload: %s\n", payload)
					}
				}
				fmt.Printf("--------------------------------------------------\n\n")
			}
			return nil
		},
	}

	cmd.Flags().Int64Var(&count, "count", 10, "Number of messages to list")
	return cmd
}

func NewDlqReplayCmd() *cobra.Command {
	var limit int64

	cmd := &cobra.Command{
		Use:   "replay <stream-name>",
		Short: "Replay messages from a DLQ back to the main stream",
		Long:  "Reads messages from the DLQ, re-queues them in the original stream for another processing attempt, and removes them from the DLQ.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			streamName := args[0]
			dlqStreamName := streamName + "_dlq"
			ctx := context.Background()

			// 1. Read messages from the DLQ
			messages, err := rdb.XRangeN(ctx, dlqStreamName, "-", "+", limit).Result()
			if err != nil {
				return fmt.Errorf("failed to read from DLQ '%s': %w", dlqStreamName, err)
			}

			if len(messages) == 0 {
				fmt.Printf("DLQ '%s' is empty. Nothing to replay.\n", dlqStreamName)
				return nil
			}

			fmt.Printf("Found %d messages to replay from '%s' to '%s'...\n", len(messages), dlqStreamName, streamName)
			var replayedCount int
			var messageIDsToAck []string

			// 2. Re-queue them and collect their IDs
			for _, msg := range messages {
				// Remove the DLQ-specific fields before re-queueing
				delete(msg.Values, "error_reason")
				delete(msg.Values, "failed_at")

				err := rdb.XAdd(ctx, &redis.XAddArgs{
					Stream: streamName,
					Values: msg.Values,
				}).Err()

				if err != nil {
					fmt.Printf("Failed to replay message %s: %v. Aborting.\n", msg.ID, err)
					// Acknowledge the messages we successfully replayed before aborting
					if len(messageIDsToAck) > 0 {
						rdb.XDel(ctx, dlqStreamName, messageIDsToAck...)
						fmt.Printf("Cleaned up %d successfully replayed messages from DLQ.\n", len(messageIDsToAck))
					}
					return err
				}
				messageIDsToAck = append(messageIDsToAck, msg.ID)
				replayedCount++
				fmt.Printf("  -> Replayed message %s\n", msg.ID)
			}

			// 3. Delete the re-queued messages from the DLQ
			if len(messageIDsToAck) > 0 {
				_, err := rdb.XDel(ctx, dlqStreamName, messageIDsToAck...).Result()
				if err != nil {
					return fmt.Errorf("failed to delete replayed messages from DLQ: %w", err)
				}
			}

			fmt.Printf("\nSuccessfully replayed %d messages.\n", replayedCount)
			return nil
		},
	}

	cmd.Flags().Int64Var(&limit, "limit", 10, "Maximum number of messages to replay")
	return cmd
}
