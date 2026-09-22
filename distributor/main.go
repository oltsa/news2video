package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/go-redis/redis/v8"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oltsa/news2video/common/logger"
	"github.com/oltsa/news2video/common/worker"
	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"
)

type Destination struct {
	Platform     string          `json:"platform"`
	ConnectionID string          `json:"connection_id"`
	Config       json.RawMessage `json:"config"`
}

type DistributionJob struct {
	JobID            string        `json:"job_id"`
	CorrelationID    string        `json:"correlation_id"`
	OutputPath       string        `json:"output_path"`
	OutputStorageKey string        `json:"output_storage_key"`
	Destinations     []Destination `json:"destinations"`
}

var (
	dbpool    *pgxpool.Pool
	zlog      zerolog.Logger
	s3Adapter *S3Adapter
)

const defaultTimeout = 10 * time.Second

func main() {
	zlog = logger.New("distributor")
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

	s3Client, err := initS3Client(ctx)
	if err != nil {
		zlog.Fatal().Err(err).Msg("Failed to initialize S3 client")
	}
	s3Adapter = NewS3Adapter(s3Client, os.Getenv("S3_BUCKET_NAME"))

	// --- Health Check Server (using the shared worker's function) ---
	go worker.HealthCheckServer(ctx, "8082", zlog, dbpool, redisClient)

	zlog.Info().Msg("Distributor connected to all services successfully.")

	// --- Define the handler for distribution jobs ---
	distributorHandler := func(ctx context.Context, payload string) error {
		var job DistributionJob
		if err := json.Unmarshal([]byte(payload), &job); err != nil {
			zlog.Error().Err(err).Msg("Failed to decode distribution job payload")
			return fmt.Errorf("failed to decode job payload: %w", err)
		}

		log := zlog.With().
			Str("correlationId", job.CorrelationID).
			Str("jobId", job.JobID).
			Logger()

		log.Info().Msg("Processing distribution job")
		err := processJob(ctx, job, log)

		if err != nil {
			log.Error().Err(err).Msg("DISTRIBUTION FAILED")
			setJobStatus(ctx, job.JobID, "failed", err.Error())
		} else {
			log.Info().Msg("Distribution job completed successfully")
			setJobStatus(ctx, job.JobID, "complete", job.OutputStorageKey)
		}

		cleanupWorkDir(log, job.OutputPath)
		return err
	}

	// --- Configure and run the worker ---
	workerCfg := worker.Config{
		StreamName:      "distribution_jobs",
		GroupName:       "distributor_group",
		ReclaimEnabled:  true,
		StuckThreshold:  2 * time.Minute,
		MaxReclaimBatch: 50,
	}

	w := worker.New(workerCfg, zlog, redisClient, distributorHandler)
	w.Run(ctx) // This blocks until shutdown
}

func processJob(ctx context.Context, job DistributionJob, log zerolog.Logger) error {
	if len(job.Destinations) == 0 {
		log.Warn().Msg("Job has no destinations, marking as complete.")
		return nil
	}

	g, gCtx := errgroup.WithContext(ctx)

	for _, dest := range job.Destinations {
		currentDest := dest
		adapter, err := GetAdapter(currentDest.Platform, s3Adapter)
		if err != nil {
			log.Warn().Err(err).Str("platform", currentDest.Platform).Msg("Skipping destination, no adapter found")
			continue
		}

		g.Go(func() error {
			distributeCtx, cancel := context.WithTimeout(gCtx, 30*time.Second)
			defer cancel()
			return adapter.Distribute(distributeCtx, log, job, currentDest)
		})
	}

	return g.Wait()
}

func cleanupWorkDir(log zerolog.Logger, localOutputPath string) {
	jobWorkDir := filepath.Dir(localOutputPath)
	if err := os.RemoveAll(jobWorkDir); err != nil {
		log.Error().Err(err).Str("dir", jobWorkDir).Msg("Failed to clean up job work directory")
	} else {
		log.Info().Str("dir", jobWorkDir).Msg("Cleaned up work directory")
	}
}

func setJobStatus(ctx context.Context, jobID, status, message string) {
	dbCtx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	var query string
	var err error
	switch status {
	case "complete":
		query = `UPDATE jobs SET status = 'complete', output_storage_key = $1, updated_at = NOW(), completed_at = NOW() WHERE id = $2`
		_, err = dbpool.Exec(dbCtx, query, message, jobID)
	case "failed":
		query = `UPDATE jobs SET status = 'failed', error_message = $1, updated_at = NOW(), completed_at = NOW() WHERE id = $2`
		_, err = dbpool.Exec(dbCtx, query, message, jobID)
	}
	if err != nil {
		zlog.Error().Err(err).Str("jobId", jobID).Msg("Failed to update final job status in DB")
	}
}

func initS3Client(ctx context.Context) (*s3.Client, error) {
	s3Ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	endpoint := os.Getenv("S3_ENDPOINT")
	resolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
		if endpoint != "" {
			return aws.Endpoint{URL: endpoint, HostnameImmutable: true}, nil
		}
		return aws.Endpoint{}, &aws.EndpointNotFoundError{}
	})
	cfg, err := config.LoadDefaultConfig(s3Ctx,
		config.WithRegion("us-east-1"),
		config.WithEndpointResolverWithOptions(resolver),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(os.Getenv("S3_ACCESS_KEY_ID"), os.Getenv("S3_SECRET_ACCESS_KEY"), "")),
	)
	if err != nil {
		return nil, err
	}
	return s3.NewFromConfig(cfg, func(o *s3.Options) { o.UsePathStyle = true }), nil
}
