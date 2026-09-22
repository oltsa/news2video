package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/rs/zerolog"
)

// Adapter is the interface that all distribution platforms must implement.
type Adapter interface {
	Distribute(ctx context.Context, log zerolog.Logger, job DistributionJob, dest Destination) error
}

// --- S3 Adapter ---

// S3Adapter handles distribution to an S3-compatible object store.
type S3Adapter struct {
	uploader *manager.Uploader
	bucket   string
}

// NewS3Adapter creates a new adapter for S3.
func NewS3Adapter(s3Client *s3.Client, bucketName string) *S3Adapter {
	return &S3Adapter{
		uploader: manager.NewUploader(s3Client),
		bucket:   bucketName,
	}
}

// Distribute uploads the rendered video to the primary S3 key specified in the job.
func (a *S3Adapter) Distribute(ctx context.Context, log zerolog.Logger, job DistributionJob, dest Destination) error {
	log.Info().Str("key", job.OutputStorageKey).Str("bucket", a.bucket).Msg("Distributing to S3")

	uploadFunc := func() error {
		file, err := os.Open(job.OutputPath)
		if err != nil {
			return fmt.Errorf("s3 adapter failed to open rendered video: %w", err)
		}
		defer file.Close()

		_, err = a.uploader.Upload(ctx, &s3.PutObjectInput{
			Bucket:      aws.String(a.bucket),
			Key:         aws.String(job.OutputStorageKey),
			Body:        file,
			ContentType: aws.String("video/mp4"),
		})
		return err
	}

	err := RetryWithBackoff(log, "S3 upload", uploadFunc, 3, 1*time.Second)
	if err != nil {
		return fmt.Errorf("s3 upload failed after retries: %w", err)
	}

	log.Info().Msg("S3 distribution successful")
	return nil
}

// --- Webhook Adapter ---

// WebhookConfig defines the expected JSON structure for a webhook destination's config.
type WebhookConfig struct {
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
}

// WebhookAdapter handles distribution to a generic HTTP endpoint.
type WebhookAdapter struct{}

// NewWebhookAdapter creates a new adapter for Webhooks.
func NewWebhookAdapter() *WebhookAdapter {
	return &WebhookAdapter{}
}

// Distribute POSTs the rendered video file to the configured URL.
func (a *WebhookAdapter) Distribute(ctx context.Context, log zerolog.Logger, job DistributionJob, dest Destination) error {
	var config WebhookConfig
	if err := json.Unmarshal(dest.Config, &config); err != nil {
		return fmt.Errorf("invalid webhook config: %w", err)
	}

	if config.URL == "" {
		return fmt.Errorf("webhook URL is missing in config")
	}

	log.Info().Str("url", config.URL).Msg("Distributing to webhook")

	postFunc := func() error {
		fileData, err := os.ReadFile(job.OutputPath)
		if err != nil {
			return fmt.Errorf("webhook adapter failed to read rendered video: %w", err)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, config.URL, bytes.NewReader(fileData))
		if err != nil {
			return fmt.Errorf("failed to create webhook request: %w", err)
		}

		req.Header.Set("Content-Type", "video/mp4")
		req.Header.Set("X-Job-ID", job.JobID)
		req.Header.Set("X-Correlation-ID", job.CorrelationID)
		for key, value := range config.Headers {
			req.Header.Set(key, value)
		}

		client := &http.Client{Timeout: 20 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("failed to execute webhook request: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("webhook returned non-success status: %s", resp.Status)
		}

		log.Info().Str("status", resp.Status).Msg("Webhook distribution successful")
		return nil
	}

	err := RetryWithBackoff(log, "Webhook POST", postFunc, 3, 2*time.Second)
	if err != nil {
		return fmt.Errorf("webhook post failed after retries: %w", err)
	}
	return nil
}

// --- GetAdapter Factory ---

// GetAdapter returns the correct adapter based on the platform string.
func GetAdapter(platform string, s3Adapter *S3Adapter) (Adapter, error) {
	switch platform {
	case "s3":
		return s3Adapter, nil
	case "webhook":
		return NewWebhookAdapter(), nil
	case "youtube":
		return NewYouTubeAdapter(), nil
	case "tiktok":
		return NewTikTokAdapter(), nil
	default:
		return nil, fmt.Errorf("unsupported distribution platform: %s", platform)
	}
}
