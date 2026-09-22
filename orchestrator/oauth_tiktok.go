package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// --- TikTok OAuth: START ---

func handleTikTokOAuthStart(c *gin.Context) {
	projectID := c.Query("project_id")
	if projectID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing project_id"})
		return
	}

	clientKey := os.Getenv("TIKTOK_CLIENT_KEY")
	redirectURI := os.Getenv("TIKTOK_REDIRECT_URI")

	if clientKey == "" || redirectURI == "" {
		zlog.Error().
			Str("client_key_set", fmt.Sprintf("%t", clientKey != "")).
			Str("redirect_uri", redirectURI).
			Msg("TikTok OAuth env vars missing")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "TikTok OAuth not configured"})
		return
	}

	// TikTok Login Kit web: /v2/auth/authorize/
	authURL := "https://www.tiktok.com/v2/auth/authorize/?" +
		"client_key=" + url.QueryEscape(clientKey) +
		"&response_type=code" +
		"&scope=" + url.QueryEscape("video.upload,user.info.basic") +
		"&redirect_uri=" + url.QueryEscape(redirectURI) +
		"&state=" + url.QueryEscape(projectID)

	zlog.Info().Str("auth_url", authURL).Msg("Redirecting to TikTok OAuth")
	c.Redirect(http.StatusFound, authURL)
}

// --- TikTok OAuth: TOKEN RESPONSE ---

// Matches v2 docs: flat JSON with optional error fields.
type TikTokTokenResponse struct {
	AccessToken      string `json:"access_token"`
	ExpiresIn        int64  `json:"expires_in"`
	OpenID           string `json:"open_id"`
	RefreshExpiresIn int64  `json:"refresh_expires_in"`
	RefreshToken     string `json:"refresh_token"`
	Scope            string `json:"scope"`
	TokenType        string `json:"token_type"`
	Error            string `json:"error"`             // on failure
	ErrorDescription string `json:"error_description"` // on failure
	LogID            string `json:"log_id"`            // on failure
}

// Exchange authorization code for access/refresh tokens using v2 spec.
func exchangeTikTokCodeForTokens(code string) (*TikTokTokenResponse, error) {
	tokenURL := "https://open.tiktokapis.com/v2/oauth/token/"

	form := url.Values{}
	form.Set("client_key", os.Getenv("TIKTOK_CLIENT_KEY"))
	form.Set("client_secret", os.Getenv("TIKTOK_CLIENT_SECRET"))
	form.Set("code", code)
	form.Set("grant_type", "authorization_code")
	form.Set("redirect_uri", os.Getenv("TIKTOK_REDIRECT_URI"))

	req, err := http.NewRequest(http.MethodPost, tokenURL, io.NopCloser(strings.NewReader(form.Encode())))
	if err != nil {
		return nil, fmt.Errorf("create request failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	rawBody, _ := io.ReadAll(resp.Body)

	// Log raw response for debugging
	zlog.Info().
		Int("status", resp.StatusCode).
		RawJSON("body", rawBody).
		Msg("TikTok token endpoint response")

	var parsed TikTokTokenResponse
	if err := json.Unmarshal(rawBody, &parsed); err != nil {
		return nil, fmt.Errorf("invalid token response: %w; body=%s", err, string(rawBody))
	}

	// Handle explicit error shape from docs
	if parsed.Error != "" {
		return nil, fmt.Errorf("tiktok error %s: %s (log_id=%s)", parsed.Error, parsed.ErrorDescription, parsed.LogID)
	}

	if parsed.AccessToken == "" {
		return nil, fmt.Errorf("token response missing access_token; body=%s", string(rawBody))
	}

	return &parsed, nil
}

// --- TikTok OAuth: CALLBACK ---

func handleTikTokOAuthCallback(c *gin.Context) {
	code := c.Query("code")
	state := c.Query("state") // project_id

	if code == "" || state == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing code or state"})
		return
	}

	projectID, err := uuid.Parse(state)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project id"})
		return
	}

	zlog.Info().
		Str("project_id", projectID.String()).
		Str("code", code).
		Msg("Received TikTok OAuth callback")

	// Exchange code for tokens using v2 spec
	tokenResp, err := exchangeTikTokCodeForTokens(code)
	if err != nil {
		zlog.Error().Err(err).Msg("TikTok token exchange failed")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "token exchange failed",
			"details": err.Error(),
		})
		return
	}

	// Build JSON config to store in connections.config (JSONB)
	cfg := map[string]interface{}{
		"access_token":       tokenResp.AccessToken,
		"refresh_token":      tokenResp.RefreshToken,
		"open_id":            tokenResp.OpenID,
		"scope":              tokenResp.Scope,
		"expires_in":         tokenResp.ExpiresIn,
		"refresh_expires_in": tokenResp.RefreshExpiresIn,
		"token_type":         tokenResp.TokenType,
	}
	cfgJSON, _ := json.Marshal(cfg)

	// Upsert into connections: one TikTok connection per project
	_, err = dbpool.Exec(
		c.Request.Context(),
		`INSERT INTO connections (project_id, platform, config)
         VALUES ($1, 'tiktok', $2)
         ON CONFLICT (project_id, platform)
         DO UPDATE SET config = EXCLUDED.config`,
		projectID,
		cfgJSON,
	)
	if err != nil {
		zlog.Error().Err(err).Msg("Failed to upsert TikTok connection")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	zlog.Info().Str("project_id", projectID.String()).Msg("TikTok connection saved")

	// Redirect back to dashboard
	redirect := os.Getenv("DASHBOARD_URL")
	if redirect == "" {
		redirect = "http://localhost:3000/dashboard"
	}
	c.Redirect(http.StatusFound, redirect)
}
