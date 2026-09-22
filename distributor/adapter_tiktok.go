package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

// TikTokConfig matches the token payload stored by orchestrator's /oauth/tiktok/callback.
// It also allows optional user-defined metadata like title/privacy.
type TikTokConfig struct {
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token,omitempty"`
	OpenID           string `json:"open_id,omitempty"`
	ExpiresIn        int    `json:"expires_in,omitempty"`
	RefreshExpiresIn int    `json:"refresh_expires_in,omitempty"`

	Title   string `json:"title,omitempty"`
	Privacy string `json:"privacy_level,omitempty"` // PUBLIC_TO_EVERYONE, MUTUAL_FOLLOW_FRIENDS, SELF_ONLY
}

type TikTokInitResponse struct {
	Data struct {
		UploadURL string `json:"upload_url"`
		PublishID string `json:"publish_id"`
	} `json:"data"`
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

type TikTokAdapter struct{}

func NewTikTokAdapter() *TikTokAdapter {
	return &TikTokAdapter{}
}

// Uploads video to TikTok *Inbox* (drafts) so the user finishes posting in TikTok app.
func (a *TikTokAdapter) Distribute(
	ctx context.Context,
	log zerolog.Logger,
	job DistributionJob,
	dest Destination,
) error {
	if os.Getenv("DRY_RUN_TIKTOK") == "true" {
		log.Warn().Msg("YOUTUBE_DRY_RUN is enabled. Skipping actual upload.")

		// Simulate upload latency
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}

		log.Info().Msg("Dry-run Tiktok upload successful")
		return nil
	}
	// 1. Read config
	var cfg TikTokConfig
	if err := json.Unmarshal(dest.Config, &cfg); err != nil {
		return fmt.Errorf("invalid tiktok config: %w", err)
	}
	if cfg.AccessToken == "" && cfg.RefreshToken == "" {
		return fmt.Errorf("tiktok config must contain at least access_token or refresh_token")
	}

	// Best-effort refresh of access token, if we have a refresh token.
	if cfg.RefreshToken != "" {
		if err := refreshTikTokAccessToken(ctx, &cfg, log); err != nil {
			log.Warn().Err(err).Msg("Failed to refresh TikTok access token; will try with existing token")
		}
	}

	if cfg.Privacy == "" {
		cfg.Privacy = "PUBLIC_TO_EVERYONE"
	}

	videoSize := getFileSize(job.OutputPath)
	if videoSize <= 0 {
		return fmt.Errorf("cannot stat video file at %s", job.OutputPath)
	}

	// 2. INIT: Ask TikTok for upload URL (Inbox / draft upload)
	initURL := "https://open.tiktokapis.com/v2/post/publish/inbox/video/init/"

	body := map[string]interface{}{
		"post_info": map[string]interface{}{
			"title":         cfg.Title,
			"privacy_level": cfg.Privacy,
		},
		"source_info": map[string]interface{}{
			"source":            "FILE_UPLOAD",
			"video_size":        videoSize,
			"chunk_size":        videoSize,
			"total_chunk_count": 1,
		},
	}

	jsonBody, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, initURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return fmt.Errorf("tiktok init request build failed: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+cfg.AccessToken)
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("tiktok init request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("tiktok init error %d: %s", resp.StatusCode, string(b))
	}

	var initResp TikTokInitResponse
	if err := json.NewDecoder(resp.Body).Decode(&initResp); err != nil {
		return fmt.Errorf("could not decode tiktok init response: %w", err)
	}

	if initResp.Error.Code != "" && initResp.Error.Code != "ok" {
		return fmt.Errorf("tiktok api error: %s - %s", initResp.Error.Code, initResp.Error.Message)
	}

	uploadURL := initResp.Data.UploadURL
	if uploadURL == "" {
		return fmt.Errorf("tiktok did not return upload_url")
	}

	log.Info().
		Str("publish_id", initResp.Data.PublishID).
		Msg("TikTok init successful")

	// 3. UPLOAD video binary
	uploadFunc := func() error {
		file, err := os.Open(job.OutputPath)
		if err != nil {
			return fmt.Errorf("open video: %w", err)
		}
		defer file.Close()

		// Read full file into memory (OK for short videos)
		fileBytes, err := io.ReadAll(file)
		if err != nil {
			return fmt.Errorf("read video: %w", err)
		}

		total := int64(len(fileBytes))
		end := total - 1

		req, err := http.NewRequestWithContext(ctx, "PUT", uploadURL, bytes.NewReader(fileBytes))
		if err != nil {
			return fmt.Errorf("upload request build failed: %w", err)
		}

		req.Header.Set("Content-Type", "application/octet-stream")
		req.Header.Set("Content-Length", fmt.Sprintf("%d", total))
		req.Header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", 0, end, total))

		client := &http.Client{Timeout: 60 * time.Second}
		upResp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("upload error: %w", err)
		}
		defer upResp.Body.Close()

		if upResp.StatusCode < 200 || upResp.StatusCode >= 300 {
			b, _ := io.ReadAll(io.LimitReader(upResp.Body, 4096))
			return fmt.Errorf("tiktok upload failed %d: %s", upResp.StatusCode, string(b))
		}

		return nil
	}

	if err := RetryWithBackoff(log, "TikTok Upload", uploadFunc, 3, 5*time.Second); err != nil {
		return fmt.Errorf("tiktok upload failed after retries: %w", err)
	}

	log.Info().Msg("TikTok upload completed; video now in TikTok inbox.")
	return nil
}

func refreshTikTokAccessToken(ctx context.Context, cfg *TikTokConfig, log zerolog.Logger) error {
	clientKey := os.Getenv("TIKTOK_CLIENT_KEY")
	clientSecret := os.Getenv("TIKTOK_CLIENT_SECRET")
	if clientKey == "" || clientSecret == "" {
		return fmt.Errorf("TIKTOK_CLIENT_KEY or TIKTOK_CLIENT_SECRET not set in environment")
	}

	// TikTok v2 Token Endpoint expects x-www-form-urlencoded
	values := url.Values{}
	values.Set("client_key", clientKey)
	values.Set("client_secret", clientSecret)
	values.Set("refresh_token", cfg.RefreshToken)
	values.Set("grant_type", "refresh_token")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://open.tiktokapis.com/v2/oauth/token/", strings.NewReader(values.Encode()))
	if err != nil {
		return fmt.Errorf("failed to build tiktok refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("tiktok refresh request failed: %w", err)
	}
	defer resp.Body.Close()

	// Check HTTP status
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("tiktok refresh error %d: %s", resp.StatusCode, string(b))
	}

	// Corrected: Response is a FLAT JSON object, not wrapped in "data"
	var out struct {
		AccessToken      string `json:"access_token"`
		RefreshToken     string `json:"refresh_token"`
		ExpiresIn        int    `json:"expires_in"`
		RefreshExpiresIn int    `json:"refresh_expires_in"`
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return fmt.Errorf("failed to decode tiktok refresh response: %w", err)
	}

	// Check for API-level errors (sometimes returned with 200 OK in OAuth)
	if out.Error != "" {
		return fmt.Errorf("tiktok api error: %s - %s", out.Error, out.ErrorDescription)
	}

	if out.AccessToken == "" {
		return fmt.Errorf("tiktok refresh returned empty access_token")
	}

	// Update config
	cfg.AccessToken = out.AccessToken
	if out.RefreshToken != "" {
		cfg.RefreshToken = out.RefreshToken
	}
	cfg.ExpiresIn = out.ExpiresIn
	cfg.RefreshExpiresIn = out.RefreshExpiresIn

	log.Info().Msg("TikTok access token refreshed successfully")
	return nil
}

// getFileSize returns file size or 0 on error.
func getFileSize(path string) int64 {
	fi, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return fi.Size()
}
