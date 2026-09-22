package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oltsa/news2video/common/logger"
	"github.com/rs/zerolog"
)

// Global variables for shared services
var (
	redisClient *redis.Client
	dbpool      *pgxpool.Pool
	s3Client    *S3Client
	zlog        zerolog.Logger
)

func main() {
	// --- Initialization ---
	zlog = logger.New("orchestrator")
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Connect to services
	redisClient = redis.NewClient(&redis.Options{Addr: os.Getenv("JOB_QUEUE_ADDRESS")})
	var err error
	dbpool, err = pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		zlog.Fatal().Err(err).Msg("Unable to connect to database")
	}
	defer dbpool.Close()

	s3Client, err = initS3Client(ctx)
	if err != nil {
		zlog.Fatal().Err(err).Msg("Failed to initialize S3 client")
	}
	zlog.Info().Msg("All services connected successfully.")

	// --- Gin Router Setup ---
	rateLimitPerSecond, _ := strconv.Atoi(os.Getenv("RATE_LIMIT_PER_SECOND"))
	if rateLimitPerSecond == 0 {
		rateLimitPerSecond = 2 // Default value
	}
	rateLimitWindow := 1 * time.Second

	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(gin.LoggerWithConfig(gin.LoggerConfig{
		SkipPaths: []string{"/health"},
	}))

	// --- Configurable CORS Middleware ---
	allowedOrigins := os.Getenv("CORS_ALLOWED_ORIGINS")
	var origins []string
	if allowedOrigins != "" {
		origins = strings.Split(allowedOrigins, ",")
	} else {
		// Provide a sensible default for local development if the env var is not set.
		origins = []string{"http://localhost:3000"}
		zlog.Warn().Msg("CORS_ALLOWED_ORIGINS environment variable not set. Defaulting to http://localhost:3000")
	}

	zlog.Info().Strs("allowed_origins", origins).Msg("Configuring CORS")

	router.Use(cors.New(cors.Config{
		AllowOrigins:     origins,
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "X-API-Key"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	router.GET("/health", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })

	// --- API v1 Routes for the Web Dashboard ---
	v1 := router.Group("/api/v1")
	{
		// Public authentication route
		authRoutes := v1.Group("/auth")
		{
			authRoutes.POST("/login", handleLogin)
			authRoutes.POST("/logout", handleLogout)
		}

		v1.GET("/oauth/tiktok/callback", handleTikTokOAuthCallback)
		v1.GET("/oauth/youtube/callback", handleYouTubeOAuthCallback)

		// All routes below this require a valid JWT.
		api := v1.Group("/")
		api.Use(jwtAuthMiddleware())
		{
			// User and Project data endpoints
			api.GET("/me", handleGetMe)
			api.GET("/projects", handleGetProjects)
			api.GET("/projects/:id", handleGetProjectByID)

			api.GET("/jobs", handleListMyJobs)

			api.GET("/projects/:id/connections", handleListConnections)
			api.POST("/projects/:id/connections", handleCreateConnection)
			api.DELETE("/connections/:connectionId", handleDeleteConnection)

			// Asset management endpoints
			api.GET("/projects/:id/assets", handleListAssets)
			api.POST("/projects/:id/assets/upload-url", handleGetUploadURL)
			api.DELETE("/projects/:id/assets", handleDeleteAsset)
			api.POST("/projects/:id/assets/folder", handleCreateFolder)
			api.DELETE("/projects/:id/assets/folder", handleDeleteFolder)

			// Template management endpoints
			api.GET("/projects/:id/templates", handleListTemplates)
			api.GET("/templates/:template_uuid", handleGetTemplateByUUID)
			api.PUT("/templates/:template_uuid", handleUpdateTemplate)
			api.POST("/projects/:id/templates", handleCreateTemplate)
			api.DELETE("/templates/:template_uuid", handleDeleteTemplate)

			// Job submission endpoint (with quota)
			api.POST("/projects/:id/jobs", quotaMiddleware(dbpool), handleDashboardRenderRequest)

			// Oauth endpoints
			api.GET("/oauth/tiktok/start", handleTikTokOAuthStart)
			api.GET("/oauth/youtube/start", handleYouTubeOAuthStart)
		}
	}

	// --- Original API Key-based Routes for Service-to-Service communication ---
	apiKeyApi := router.Group("/")
	apiKeyApi.Use(authMiddleware(dbpool))
	apiKeyApi.Use(rateLimitMiddleware(redisClient, rateLimitPerSecond, rateLimitWindow))
	{
		quotaRoutes := apiKeyApi.Group("/")
		quotaRoutes.Use(quotaMiddleware(dbpool))
		{
			quotaRoutes.POST("/render", handleRenderRequest)
		}
		apiKeyApi.GET("/jobs/:jobId", handleJobStatusRequest)
	}

	// --- Start Server and Handle Graceful Shutdown ---
	srv := &http.Server{Addr: ":8080", Handler: router}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			zlog.Fatal().Err(err).Msg("Server failed to start")
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	zlog.Info().Msg("Shutting down server...")
	ctxShutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctxShutdown)
	zlog.Info().Msg("Server exiting.")
}
