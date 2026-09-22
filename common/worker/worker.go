package worker

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

// MessageHandler is a function that processes a single message from a stream.
// It is responsible for unmarshaling the payload and executing the business logic.
// Returning an error will cause the message to be moved to the DLQ.
type MessageHandler func(ctx context.Context, payload string) error

// Config holds the configuration for a worker instance.
type Config struct {
	StreamName      string
	GroupName       string
	ConsumerName    string
	ReclaimEnabled  bool
	StuckThreshold  time.Duration
	MaxReclaimBatch int64
}

// Worker is a generic Redis Stream consumer.
type Worker struct {
	cfg     Config
	log     zerolog.Logger
	rdb     *redis.Client
	handler MessageHandler
}

// New creates a new Worker.
func New(cfg Config, log zerolog.Logger, rdb *redis.Client, handler MessageHandler) *Worker {
	if cfg.ConsumerName == "" {
		host, err := os.Hostname()
		if err != nil {
			panic(err)
		}
		cfg.ConsumerName = host
	}
	return &Worker{
		cfg:     cfg,
		log:     log,
		rdb:     rdb,
		handler: handler,
	}
}

// Run starts the worker's message processing loop. It blocks until the context is canceled.
func (w *Worker) Run(ctx context.Context) {
	err := w.rdb.XGroupCreateMkStream(ctx, w.cfg.StreamName, w.cfg.GroupName, "0").Err()
	if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		w.log.Fatal().Err(err).Msg("Could not create consumer group")
	}

	w.log.Info().
		Str("stream", w.cfg.StreamName).
		Str("group", w.cfg.GroupName).
		Msg("Worker starting.")

	// On startup, reclaim any messages that might have been stuck from a previous crash.
	w.reclaimStuckMessages(ctx)

	for {
		select {
		case <-ctx.Done():
			w.log.Info().Msg("Shutting down worker.")
			return
		default:
			w.processNextMessage(ctx)
		}
	}
}

func (w *Worker) processNextMessage(ctx context.Context) {
	readCtx, cancel := context.WithTimeout(ctx, 3*time.Second) // Long poll for 2s
	defer cancel()

	streams, err := w.rdb.XReadGroup(readCtx, &redis.XReadGroupArgs{
		Group:    w.cfg.GroupName,
		Consumer: w.cfg.ConsumerName,
		Streams:  []string{w.cfg.StreamName, ">"},
		Count:    1,
		Block:    2 * time.Second,
	}).Result()

	if err != nil {
		if err != redis.Nil && err != context.DeadlineExceeded {
			w.log.Error().Err(err).Msg("Error reading from stream")
		}
		return
	}
	if len(streams) == 0 || len(streams[0].Messages) == 0 {
		return
	}

	message := streams[0].Messages[0]
	payload, ok := message.Values["payload"].(string)
	if !ok {
		w.log.Error().Str("messageId", message.ID).Msg("Payload is not a string, moving to DLQ")
		w.moveToDLQ(context.Background(), message.Values, fmt.Errorf("payload not a string"))
		w.ackMessage(context.Background(), message.ID)
		return
	}

	// Execute the specific handler logic
	if err := w.handler(ctx, payload); err != nil {
		w.log.Error().Err(err).Str("messageId", message.ID).Msg("Message processing failed, moving to DLQ")
		w.moveToDLQ(context.Background(), message.Values, err)
	} else {
		w.log.Debug().Str("messageId", message.ID).Msg("Message processed successfully")
	}

	w.ackMessage(context.Background(), message.ID)
}

func (w *Worker) moveToDLQ(ctx context.Context, values map[string]interface{}, processingError error) {
	dlqStream := w.cfg.StreamName + "_dlq"
	dlqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	values["error_reason"] = processingError.Error()
	values["failed_at"] = time.Now().UTC().Format(time.RFC3339)

	err := w.rdb.XAdd(dlqCtx, &redis.XAddArgs{
		Stream: dlqStream,
		Values: values,
	}).Err()

	if err != nil {
		w.log.Error().Err(err).Str("dlq_stream", dlqStream).Msg("Failed to move message to DLQ")
	} else {
		w.log.Warn().Str("dlq_stream", dlqStream).Msg("Message moved to DLQ")
	}
}

func (w *Worker) ackMessage(ctx context.Context, messageID string) {
	ackCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	w.rdb.XAck(ackCtx, w.cfg.StreamName, w.cfg.GroupName, messageID)
}

func (w *Worker) reclaimStuckMessages(ctx context.Context) {
	if !w.cfg.ReclaimEnabled {
		return
	}

	w.log.Info().Msg("Checking for pending messages to reclaim")
	pending, err := w.rdb.XPendingExt(ctx, &redis.XPendingExtArgs{
		Stream: w.cfg.StreamName,
		Group:  w.cfg.GroupName,
		Start:  "-",
		End:    "+",
		Count:  w.cfg.MaxReclaimBatch,
	}).Result()
	if err != nil {
		w.log.Error().Err(err).Msg("Failed to fetch XPENDING entries")
		return
	}

	toReclaim := make([]string, 0)
	for _, entry := range pending {
		if entry.Idle > w.cfg.StuckThreshold {
			toReclaim = append(toReclaim, entry.ID)
		}
	}

	if len(toReclaim) == 0 {
		w.log.Info().Msg("No stale messages found")
		return
	}

	w.log.Info().
		Int("count", len(toReclaim)).
		Dur("threshold", w.cfg.StuckThreshold).
		Msg("Reclaiming stale messages")

	// Use a unique consumer name for reclaiming to avoid conflicts
	reclaimConsumer := fmt.Sprintf("reclaimer-%s-%s", w.cfg.ConsumerName, uuid.NewString())

	claimed, err := w.rdb.XClaim(ctx, &redis.XClaimArgs{
		Stream:   w.cfg.StreamName,
		Group:    w.cfg.GroupName,
		Consumer: reclaimConsumer,
		MinIdle:  w.cfg.StuckThreshold,
		Messages: toReclaim,
	}).Result()

	if err != nil {
		w.log.Error().Err(err).Msg("Failed to XCLAIM messages")
		return
	}

	if len(claimed) > 0 {
		w.log.Info().Int("count", len(claimed)).Msg("Successfully claimed messages. They will be re-processed by an active consumer.")
	}
}

// HealthCheckServer is a convenience function to run a standard health check server.
func HealthCheckServer(ctx context.Context, port string, log zerolog.Logger, db *pgxpool.Pool, rdb *redis.Client) {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		dbCtx, dbCancel := context.WithTimeout(ctx, 2*time.Second)
		defer dbCancel()
		if err := db.Ping(dbCtx); err != nil {
			http.Error(w, "Database connection failed", http.StatusServiceUnavailable)
			log.Warn().Err(err).Msg("Health check failed: DB ping")
			return
		}

		redisCtx, redisCancel := context.WithTimeout(ctx, 2*time.Second)
		defer redisCancel()
		if _, err := rdb.Ping(redisCtx).Result(); err != nil {
			http.Error(w, "Redis connection failed", http.StatusServiceUnavailable)
			log.Warn().Err(err).Msg("Health check failed: Redis ping")
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	server := &http.Server{Addr: ":" + port, Handler: mux}
	go func() {
		log.Info().Str("port", port).Msg("Health check server starting")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error().Err(err).Msg("Health check server failed")
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("Health check server shutdown failed")
	} else {
		log.Info().Msg("Health check server shut down gracefully.")
	}
}
