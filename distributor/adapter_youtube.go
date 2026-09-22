package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/rs/zerolog"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/api/youtube/v3"
)

// YouTubeConfig matches what we actually store in `connections.config`:
//
//  1. From init.sql seed (legacy):
//     { "client_id": "...", "client_secret": "...", "refresh_token": "...", "title": "...", "privacy": "unlisted", "tags": [...] }
//
//  2. From OAuth callback:
//     { "access_token": "...", "refresh_token": "...", "expires_in": 3599, "scope": "...", "token_type": "Bearer" }
//
// We only require `refresh_token` and optional metadata fields.
type YouTubeConfig struct {
	// From both seed and OAuth token response
	RefreshToken string `json:"refresh_token"`

	// From OAuth token response; tokenSource handles access token refresh.
	AccessToken string `json:"access_token,omitempty"`
	TokenType   string `json:"token_type,omitempty"`
	Scope       string `json:"scope,omitempty"`
	ExpiresIn   int    `json:"expires_in,omitempty"`

	// Optional metadata from your own config
	Title       string   `json:"title,omitempty"`
	Description string   `json:"description,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Privacy     string   `json:"privacy,omitempty"` // "private", "public", "unlisted"
}

type YouTubeAdapter struct{}

func NewYouTubeAdapter() *YouTubeAdapter {
	return &YouTubeAdapter{}
}

func (a *YouTubeAdapter) Distribute(ctx context.Context, log zerolog.Logger, job DistributionJob, dest Destination) error {
	if os.Getenv("DRY_RUN_YOUTUBE") == "true" {
		log.Warn().Msg("YOUTUBE_DRY_RUN is enabled. Skipping actual upload.")

		// Simulate upload latency
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}

		log.Info().Msg("Dry-run YouTube upload successful")
		return nil
	}
	var cfg YouTubeConfig
	if err := json.Unmarshal(dest.Config, &cfg); err != nil {
		return fmt.Errorf("invalid youtube config: %w", err)
	}

	if cfg.RefreshToken == "" {
		return fmt.Errorf("missing youtube refresh_token in connection config")
	}

	clientID := os.Getenv("YOUTUBE_CLIENT_ID")
	clientSecret := os.Getenv("YOUTUBE_CLIENT_SECRET")
	if clientID == "" || clientSecret == "" {
		return fmt.Errorf("YOUTUBE_CLIENT_ID or YOUTUBE_CLIENT_SECRET not set in environment")
	}

	// 1. Setup OAuth2 config
	oauthConfig := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint:     google.Endpoint,
		Scopes:       []string{youtube.YoutubeUploadScope},
	}

	// 2. TokenSource using only the refresh token (works with both seed + real OAuth)
	token := &oauth2.Token{RefreshToken: cfg.RefreshToken}
	tokenSource := oauthConfig.TokenSource(ctx, token)

	// 3. Create YouTube service
	service, err := youtube.NewService(ctx, option.WithTokenSource(tokenSource))
	if err != nil {
		return fmt.Errorf("failed to create youtube service: %w", err)
	}

	// 4. Open rendered video file
	file, err := os.Open(job.OutputPath)
	if err != nil {
		return fmt.Errorf("failed to open video file: %w", err)
	}
	defer file.Close()

	// 5. Prepare metadata
	title := cfg.Title
	if title == "" {
		title = fmt.Sprintf("Video %s", job.JobID)
	}
	privacy := cfg.Privacy
	if privacy == "" {
		privacy = "private" // safe default
	}

	upload := &youtube.Video{
		Snippet: &youtube.VideoSnippet{
			Title:       title,
			Description: cfg.Description,
			Tags:        cfg.Tags,
		},
		Status: &youtube.VideoStatus{PrivacyStatus: privacy},
	}

	log.Info().Msg("Starting YouTube upload")

	uploadCall := service.Videos.Insert([]string{"snippet", "status"}, upload)

	uploadFunc := func() error {
		// Reset file pointer on retry
		if _, err := file.Seek(0, 0); err != nil {
			return fmt.Errorf("failed to seek video file: %w", err)
		}
		_, err := uploadCall.Media(file).Do()
		return err
	}

	if err := RetryWithBackoff(log, "YouTube Upload", uploadFunc, 3, 5*time.Second); err != nil {
		return fmt.Errorf("youtube upload failed after retries: %w", err)
	}

	log.Info().Msg("YouTube upload successful")
	return nil
}
