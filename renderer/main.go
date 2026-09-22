package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oltsa/news2video/common/logger"
	"github.com/oltsa/news2video/common/worker"
	"github.com/rs/zerolog"
)

var (
	dbpool *pgxpool.Pool
	zlog   zerolog.Logger
)

const defaultTimeout = 5 * time.Second

func main() {
	zlog = logger.New("renderer")
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// --- Service Connections ---
	redisClient := redis.NewClient(&redis.Options{Addr: os.Getenv("JOB_QUEUE_ADDRESS")})
	var err error
	dbpool, err = pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		zlog.Fatal().Err(err).Msg("Unable to connect to database")
	}
	defer dbpool.Close()

	// --- Health Check Server (using the shared worker's function) ---
	go worker.HealthCheckServer(ctx, "8081", zlog, dbpool, redisClient)

	zlog.Info().Msg("Renderer connected successfully.")

	// --- Define the handler for render jobs ---
	rendererHandler := func(ctx context.Context, payload string) error {
		var job RenderJob
		if err := json.Unmarshal([]byte(payload), &job); err != nil {
			zlog.Error().Err(err).Msg("Failed to decode render job payload")
			return fmt.Errorf("failed to decode job payload: %w", err)
		}

		log := zlog.With().
			Str("correlationId", job.CorrelationID).
			Str("jobId", job.JobID).
			Str("templateId", job.TemplateID).
			Logger()

		log.Info().Msg("Processing job")
		setJobStatus(ctx, job.JobID, "rendering", "", 0)

		var renderDurationMs int64
		err = processJob(ctx, job, log, &renderDurationMs, redisClient)

		if err != nil {
			log.Error().Err(err).Msg("JOB FAILED")
			setJobStatus(ctx, job.JobID, "failed", err.Error(), renderDurationMs)
		} else {
			log.Info().Msg("Job rendering complete, handoff to distributor")
		}

		return err
	}

	// --- Configure and run the worker ---
	workerCfg := worker.Config{
		StreamName:      "render_jobs",
		GroupName:       "renderer_group",
		ReclaimEnabled:  true,
		StuckThreshold:  2 * time.Minute,
		MaxReclaimBatch: 50,
	}

	w := worker.New(workerCfg, zlog, redisClient, rendererHandler)
	w.Run(ctx) // This blocks until shutdown
}

func processJob(ctx context.Context, job RenderJob, log zerolog.Logger, renderDurationMs *int64, rdb *redis.Client) error {
	var tmpl Template
	if err := json.Unmarshal(job.Template, &tmpl); err != nil {
		return fmt.Errorf("failed to unmarshal concrete template from job: %w", err)
	}

	workDir := os.Getenv("WORK_DIR")
	if workDir == "" {
		return fmt.Errorf("WORK_DIR environment variable not set")
	}
	jobWorkDir := filepath.Join(workDir, job.JobID)
	if err := os.MkdirAll(jobWorkDir, 0755); err != nil {
		return fmt.Errorf("failed to create job work dir: %w", err)
	}
	// The distributor is now responsible for cleaning up its own input files.
	// We no longer need a deferred cleanup here.

	tmpDir, err := os.MkdirTemp(jobWorkDir, "assets_")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir) // Clean up temporary assets after render

	localOutputPath := filepath.Join(jobWorkDir, "final.mp4")

	renderStart := time.Now()
	err = generateVideo(tmpl, job.Payload, localOutputPath, tmpDir, log)
	*renderDurationMs = time.Since(renderStart).Milliseconds()

	if err != nil {
		// If render fails, clean up the whole job directory now
		os.RemoveAll(jobWorkDir)
		return fmt.Errorf("video generation failed: %w", err)
	}

	log.Info().Str("path", localOutputPath).Msg("Render complete. Emitting distribution job.")
	distJob := map[string]interface{}{
		"job_id":             job.JobID,
		"correlation_id":     job.CorrelationID,
		"output_path":        localOutputPath,
		"output_storage_key": job.OutputStorageKey,
		"destinations":       job.Destinations,
	}
	distJobPayload, err := json.Marshal(distJob)
	if err != nil {
		os.RemoveAll(jobWorkDir) // Clean up on failure
		return fmt.Errorf("failed to marshal distribution job payload: %w", err)
	}

	redisCtx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()
	return rdb.XAdd(redisCtx, &redis.XAddArgs{
		Stream: "distribution_jobs", Values: map[string]interface{}{"payload": distJobPayload},
	}).Err()
}

func setJobStatus(ctx context.Context, jobID, status, message string, renderDurationMs int64) {
	dbCtx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	var query string
	var err error
	switch status {
	case "rendering":
		query = "UPDATE jobs SET status = 'rendering', started_at = NOW(), updated_at = NOW() WHERE id = $1"
		_, err = dbpool.Exec(dbCtx, query, jobID)
	case "failed":
		query = `UPDATE jobs SET status = 'failed', error_message = $1, updated_at = NOW(), completed_at = NOW(), render_duration_ms = $2 WHERE id = $3`
		_, err = dbpool.Exec(dbCtx, query, message, renderDurationMs, jobID)
	}
	if err != nil {
		zlog.Error().Err(err).Str("jobId", jobID).Msg("Failed to update job status in DB")
	}
}

func runCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("command '%s %s' failed. Error: %w. Stderr: %s", name, strings.Join(args, " "), err, stderr.String())
	}
	return nil
}
