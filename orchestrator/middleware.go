package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// authMiddleware authenticates a request by API key and attaches project info to the context.
func authMiddleware(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		apiKey := c.GetHeader("X-API-Key")
		if apiKey == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "X-API-Key header required"})
			return
		}

		var project Project
		query := `SELECT p.id, p.organization_id, o.organization_key, p.project_key
                  FROM projects p
                  JOIN organizations o ON p.organization_id = o.id
                  WHERE p.api_key = $1`
		err := db.QueryRow(c.Request.Context(), query, apiKey).Scan(
			&project.ID, &project.OrganizationID, &project.OrganizationKey, &project.ProjectKey,
		)
		if err != nil {
			zlog.Warn().Err(err).Msg("Invalid API key received")
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid API key"})
			return
		}

		c.Set("project", project)
		c.Next()
	}
}

// rateLimitMiddleware implements a sliding window rate limiter using Redis.
func rateLimitMiddleware(rdb *redis.Client, limit int, window time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		projectCtx, exists := c.Get("project")
		if !exists {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "project context not found"})
			return
		}
		project := projectCtx.(Project)
		key := fmt.Sprintf("rate_limit:%s", project.ID.String())

		now := time.Now().UnixNano()
		windowStart := now - window.Nanoseconds()

		pipe := rdb.TxPipeline()
		pipe.ZRemRangeByScore(c, key, "0", fmt.Sprintf("%d", windowStart))
		pipe.ZAdd(c, key, &redis.Z{Score: float64(now), Member: float64(now)})
		countCmd := pipe.ZCard(c, key)
		pipe.Expire(c, key, window)

		_, err := pipe.Exec(c)
		if err != nil {
			zlog.Error().Err(err).Msg("Failed to execute rate limit pipeline in Redis")
			c.Next() // Fail open on Redis error
			return
		}

		count := countCmd.Val()
		if count > int64(limit) {
			zlog.Warn().
				Str("projectId", project.ID.String()).
				Int64("rateLimitCount", count).
				Msg("Rate limit exceeded")
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "Too Many Requests"})
			return
		}

		c.Next()
	}
}

// quotaMiddleware is now flexible and works for both API-key and JWT-based routes.
func quotaMiddleware(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		var projectID uuid.UUID

		// --- MODIFICATION START ---
		// Check for project info from the API-key auth middleware first.
		projectCtx, exists := c.Get("project")
		if exists {
			project := projectCtx.(Project)
			projectID = project.ID
		} else {
			// If not found, get the project ID from the URL param (for JWT routes).
			idStr := c.Param("id")
			if idStr == "" {
				zlog.Error().Msg("FATAL: quotaMiddleware could not find project ID in context or URL param.")
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "project context not found"})
				return
			}
			var err error
			projectID, err = uuid.Parse(idStr)
			if err != nil {
				c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "Invalid project ID format in URL"})
				return
			}
		}
		// --- MODIFICATION END ---

		tx, err := db.Begin(context.Background())
		if err != nil {
			zlog.Error().Err(err).Msg("Failed to begin transaction for quota check")
			c.Next() // Fail open
			return
		}
		defer tx.Rollback(context.Background())

		resetQuery := `
			UPDATE projects
			SET jobs_today = 0, jobs_last_reset_at = NOW()
			WHERE id = $1 AND DATE_TRUNC('day', jobs_last_reset_at) < DATE_TRUNC('day', NOW())
		`
		_, err = tx.Exec(context.Background(), resetQuery, projectID)
		if err != nil {
			zlog.Error().Err(err).Str("projectId", projectID.String()).Msg("Failed to execute lazy reset for quota")
			c.Next()
			return
		}

		var currentUsage int
		var dailyLimit int
		atomicUpdateQuery := `
			UPDATE projects p
			SET jobs_today = p.jobs_today + 1
			FROM plans pl
			WHERE p.id = $1 AND p.plan_id = pl.id AND (pl.daily_video_limit = 0 OR p.jobs_today < pl.daily_video_limit)
			RETURNING p.jobs_today, pl.daily_video_limit
		`
		err = tx.QueryRow(context.Background(), atomicUpdateQuery, projectID).Scan(&currentUsage, &dailyLimit)

		if err != nil {
			if err == pgx.ErrNoRows {
				var limitOnly int
				_ = db.QueryRow(context.Background(), "SELECT pl.daily_video_limit FROM plans pl JOIN projects p ON p.plan_id = pl.id WHERE p.id = $1", projectID).Scan(&limitOnly)

				zlog.Warn().Str("projectId", projectID.String()).Int("dailyLimit", limitOnly).Msg("Daily project quota exceeded (atomic check)")
				c.Header("X-Quota-Limit", fmt.Sprintf("%d", limitOnly))
				c.Header("X-Quota-Remaining", "0")
				c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "Daily job quota exceeded"})
				return
			}

			zlog.Error().Err(err).Str("projectId", projectID.String()).Msg("Failed to execute atomic quota update")
			c.Next()
			return
		}

		if err := tx.Commit(context.Background()); err != nil {
			zlog.Error().Err(err).Msg("Failed to commit quota transaction")
			c.Next()
			return
		}

		remaining := 0
		if dailyLimit > 0 {
			remaining = dailyLimit - currentUsage
		}

		c.Header("X-Quota-Limit", fmt.Sprintf("%d", dailyLimit))
		c.Header("X-Quota-Remaining", fmt.Sprintf("%d", remaining))

		c.Next()
	}
}

// jwtAuthMiddleware validates the JWT from the cookie and attaches user info to the context.
func jwtAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenString, err := c.Cookie("token")
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Authorization token not provided"})
			return
		}

		claims := &Claims{}
		jwtSecret := []byte(os.Getenv("JWT_SECRET"))

		token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return jwtSecret, nil
		})

		if err != nil || !token.Valid {
			zlog.Warn().Err(err).Msg("JWT validation failed")
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired authorization token"})
			return
		}

		userID, err := uuid.Parse(claims.UserID)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid user ID in token"})
			return
		}

		orgID, err := uuid.Parse(claims.OrganizationID)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid organization ID in token"})
			return
		}

		user := User{
			ID:              userID,
			OrganizationID:  orgID,
			IsCustomerAdmin: claims.IsCustomerAdmin,
		}
		c.Set("user", user)

		c.Next()
	}
}
