package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"os"

	"github.com/gin-gonic/gin"
)

func handleYouTubeOAuthStart(c *gin.Context) {
	projectID := c.Query("project_id")
	if projectID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing project_id"})
		return
	}

	redirectURI := url.QueryEscape(os.Getenv("YOUTUBE_REDIRECT_URI"))
	scope := url.QueryEscape("https://www.googleapis.com/auth/youtube.upload")

	authURL := "https://accounts.google.com/o/oauth2/v2/auth?" +
		"client_id=" + os.Getenv("YOUTUBE_CLIENT_ID") +
		"&redirect_uri=" + redirectURI +
		"&response_type=code" +
		"&access_type=offline&prompt=consent" +
		"&scope=" + scope +
		"&state=" + projectID

	c.Redirect(http.StatusFound, authURL)
}

func handleYouTubeOAuthCallback(c *gin.Context) {
	code := c.Query("code")
	projectID := c.Query("state")

	if code == "" || projectID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid youtube oauth callback"})
		return
	}

	form := url.Values{
		"client_id":     {os.Getenv("YOUTUBE_CLIENT_ID")},
		"client_secret": {os.Getenv("YOUTUBE_CLIENT_SECRET")},
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {os.Getenv("YOUTUBE_REDIRECT_URI")},
	}.Encode()

	resp, err := http.Post(
		"https://oauth2.googleapis.com/token",
		"application/x-www-form-urlencoded",
		bytes.NewBufferString(form),
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "token exchange failed"})
		return
	}
	defer resp.Body.Close()

	var tokens map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&tokens)

	_, err = dbpool.Exec(
		c.Request.Context(),
		`INSERT INTO connections (project_id, platform, config)
         VALUES ($1, 'youtube', $2)
         ON CONFLICT (project_id, platform) DO UPDATE SET config = EXCLUDED.config`,
		projectID,
		tokens,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save connection"})
		return
	}

	c.Redirect(http.StatusFound, "/dashboard/projects/"+projectID+"/connections?connected=youtube")
}
