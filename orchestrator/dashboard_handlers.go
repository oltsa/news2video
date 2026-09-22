package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func handleGetMe(c *gin.Context) {
	userCtx, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User context not found in request"})
		return
	}
	user := userCtx.(User)

	c.JSON(http.StatusOK, gin.H{
		"id":                user.ID,
		"organization_id":   user.OrganizationID,
		"is_customer_admin": user.IsCustomerAdmin,
	})
}

func handleGetProjects(c *gin.Context) {
	userCtx, _ := c.Get("user")
	user := userCtx.(User)

	var query string
	var rows pgx.Rows
	var err error

	if user.IsCustomerAdmin {
		query = `SELECT id, project_key FROM projects WHERE organization_id = $1 ORDER BY project_key`
		rows, err = dbpool.Query(c.Request.Context(), query, user.OrganizationID)
	} else {
		query = `
            SELECT p.id, p.project_key
            FROM projects p
            JOIN user_project_roles upr ON p.id = upr.project_id
            WHERE upr.user_id = $1
            ORDER BY p.project_key
        `
		rows, err = dbpool.Query(c.Request.Context(), query, user.ID)
	}

	if err != nil {
		zlog.Error().Err(err).Msg("Failed to query for projects")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not retrieve projects"})
		return
	}
	defer rows.Close()

	projects, err := pgx.CollectRows(rows, pgx.RowToStructByPos[ProjectResponse])
	if err != nil {
		zlog.Error().Err(err).Msg("Failed to scan project rows")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not process project data"})
		return
	}

	c.JSON(http.StatusOK, projects)
}

func handleGetProjectByID(c *gin.Context) {
	userCtx, _ := c.Get("user")
	user := userCtx.(User)

	projectIDStr := c.Param("id")
	projectID, err := uuid.Parse(projectIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid project ID format"})
		return
	}

	var projectDetails ProjectDetailResponse
	query := `
        SELECT id, project_key, api_key
        FROM projects
        WHERE id = $1 AND organization_id = $2
    `
	err = dbpool.QueryRow(c.Request.Context(), query, projectID, user.OrganizationID).Scan(
		&projectDetails.ID,
		&projectDetails.ProjectKey,
		&projectDetails.APIKey,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "Project not found or you do not have permission to view it"})
			return
		}
		zlog.Error().Err(err).Str("projectId", projectIDStr).Msg("Error querying for single project")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve project details"})
		return
	}

	c.JSON(http.StatusOK, projectDetails)
}

func handleListAssets(c *gin.Context) {
	userCtx, _ := c.Get("user")
	user := userCtx.(User)

	project, err := getProjectForUser(c, c.Param("id"), user)
	if err != nil {
		return
	}

	requestedPath := c.Query("path")
	cleanPath := path.Clean(requestedPath)
	if cleanPath == "." {
		cleanPath = ""
	}
	if strings.HasPrefix(cleanPath, "..") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid path"})
		return
	}

	basePrefix := fmt.Sprintf("organizations/%s/%s/assets/",
		sanitizeKey(project.OrganizationKey),
		sanitizeKey(project.ProjectKey),
	)

	finalPrefix := basePrefix
	if cleanPath != "" {
		finalPrefix = path.Join(basePrefix, cleanPath) + "/"
	}

	output, err := s3Client.Client.ListObjectsV2(c.Request.Context(), &s3.ListObjectsV2Input{
		Bucket:    aws.String(s3Client.Bucket),
		Prefix:    aws.String(finalPrefix),
		Delimiter: aws.String("/"),
	})
	if err != nil {
		zlog.Error().Err(err).Str("prefix", finalPrefix).Msg("Failed to list S3 objects")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not list assets"})
		return
	}

	var assets []AssetItem
	for _, p := range output.CommonPrefixes {
		relativeKey := strings.TrimPrefix(*p.Prefix, finalPrefix)
		assets = append(assets, AssetItem{
			Key:   strings.TrimSuffix(relativeKey, "/"),
			IsDir: true,
		})
	}
	for _, obj := range output.Contents {
		if *obj.Key == finalPrefix {
			continue
		}
		relativeKey := strings.TrimPrefix(*obj.Key, finalPrefix)
		assets = append(assets, AssetItem{
			Key:          relativeKey,
			LastModified: *obj.LastModified,
			Size:         *obj.Size,
			IsDir:        false,
		})
	}

	c.JSON(http.StatusOK, assets)
}

// handleGetUploadURL generates a presigned URL for a client-side PUT upload.
func handleGetUploadURL(c *gin.Context) {
	userCtx, _ := c.Get("user")
	user := userCtx.(User)

	project, err := getProjectForUser(c, c.Param("id"), user)
	if err != nil {
		zlog.Warn().
			Err(err).
			Str("projectId", project.ID.String()).
			Str("userId", user.ID.String()).
			Msg("Failed to bind upload URL request")
		return
	}

	var req UploadURLRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		zlog.Warn().
			Err(err).
			Str("projectId", project.ID.String()).
			Str("userId", user.ID.String()).
			Msg("Failed to bind upload URL request")
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// --- MODIFICATION START ---
	// Sanitize the user-provided path and filename
	uploadPath := path.Clean(req.AssetType)
	if strings.HasPrefix(uploadPath, "..") { // Security check for directory traversal
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid upload path"})
		return
	}
	if uploadPath == "." {
		uploadPath = ""
	}

	filename := path.Base(req.Filename) // Ensure we only get the filename

	// Safely construct the base path for all assets in this project
	baseAssetPath := fmt.Sprintf("organizations/%s/%s/assets",
		sanitizeKey(project.OrganizationKey),
		sanitizeKey(project.ProjectKey),
	)

	// Use path.Join to safely combine the parts. It handles empty strings correctly.
	s3Key := path.Join(baseAssetPath, uploadPath, filename)
	// --- MODIFICATION END ---

	presignedReq, err := s3Client.PublicPresigner.PresignPutObject(c.Request.Context(), &s3.PutObjectInput{
		Bucket:      aws.String(s3Client.Bucket),
		Key:         aws.String(s3Key),
		ContentType: aws.String(req.ContentType),
	}, func(opts *s3.PresignOptions) {
		opts.Expires = 15 * time.Minute
	})

	if err != nil {
		zlog.Error().Err(err).Str("key", s3Key).Msg("Failed to generate presigned PUT URL")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not generate upload URL"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"url": presignedReq.URL})
}

func handleDeleteAsset(c *gin.Context) {
	userCtx, _ := c.Get("user")
	user := userCtx.(User)

	project, err := getProjectForUser(c, c.Param("id"), user)
	if err != nil {
		return
	}

	assetKey := c.Query("key")
	if assetKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing 'key' query parameter"})
		return
	}

	basePrefix := fmt.Sprintf("organizations/%s/%s/assets/",
		sanitizeKey(project.OrganizationKey),
		sanitizeKey(project.ProjectKey),
	)
	fullS3Key := path.Join(basePrefix, assetKey)

	if !strings.HasPrefix(fullS3Key, basePrefix) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Forbidden: access to the requested asset is denied"})
		return
	}

	_, err = s3Client.Client.DeleteObject(c.Request.Context(), &s3.DeleteObjectInput{
		Bucket: aws.String(s3Client.Bucket),
		Key:    aws.String(fullS3Key),
	})
	if err != nil {
		zlog.Error().Err(err).Str("key", fullS3Key).Msg("Failed to delete S3 object")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not delete asset"})
		return
	}

	c.Status(http.StatusNoContent)
}

func handleListTemplates(c *gin.Context) {
	userCtx, _ := c.Get("user")
	user := userCtx.(User)

	project, err := getProjectForUser(c, c.Param("id"), user)
	if err != nil {
		return
	}

	query := `
        SELECT template_uuid, id, (project_id IS NULL) as is_system
        FROM templates
        WHERE project_id = $1 OR project_id IS NULL
        ORDER BY is_system, id
    `
	rows, err := dbpool.Query(c.Request.Context(), query, project.ID)
	if err != nil {
		zlog.Error().Err(err).Msg("Failed to query for templates")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not retrieve templates"})
		return
	}
	defer rows.Close()

	templates, err := pgx.CollectRows(rows, pgx.RowToStructByPos[TemplateResponse])
	if err != nil {
		zlog.Error().Err(err).Msg("Failed to scan template rows")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not process template data"})
		return
	}

	c.JSON(http.StatusOK, templates)
}

func handleGetTemplateByUUID(c *gin.Context) {
	userCtx, _ := c.Get("user")
	user := userCtx.(User)

	templateUUID, err := uuid.Parse(c.Param("template_uuid"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid template UUID format"})
		return
	}

	var rawTemplate json.RawMessage
	query := `
        SELECT t.template_data
        FROM templates t
        LEFT JOIN projects p ON t.project_id = p.id
        WHERE t.template_uuid = $1 AND (t.project_id IS NULL OR p.organization_id = $2)
    `
	err = dbpool.QueryRow(c.Request.Context(), query, templateUUID, user.OrganizationID).Scan(&rawTemplate)
	if err != nil {
		if err == pgx.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "Template not found or you do not have permission to view it"})
			return
		}
		zlog.Error().Err(err).Msg("Failed to query for single template")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not retrieve template"})
		return
	}

	c.Data(http.StatusOK, "application/json", rawTemplate)
}

func handleUpdateTemplate(c *gin.Context) {
	userCtx, _ := c.Get("user")
	user := userCtx.(User)

	templateUUID, err := uuid.Parse(c.Param("template_uuid"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid template UUID format"})
		return
	}

	templateData, err := c.GetRawData()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}
	if !json.Valid(templateData) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Request body is not valid JSON"})
		return
	}

	query := `
        UPDATE templates t
        SET template_data = $1
        FROM projects p
        WHERE t.template_uuid = $2
          AND t.project_id = p.id
          AND p.organization_id = $3
    `
	result, err := dbpool.Exec(c.Request.Context(), query, templateData, templateUUID, user.OrganizationID)
	if err != nil {
		zlog.Error().Err(err).Msg("Failed to update template")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not update template"})
		return
	}

	if result.RowsAffected() == 0 {
		c.JSON(http.StatusForbidden, gin.H{"error": "Template not found or you do not have permission to update it"})
		return
	}

	c.Status(http.StatusNoContent)
}

func handleCreateFolder(c *gin.Context) {
	userCtx, _ := c.Get("user")
	user := userCtx.(User)

	project, err := getProjectForUser(c, c.Param("id"), user)
	if err != nil {
		return
	}

	var req CreateFolderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Sanitize inputs
	folderName := path.Base(req.FolderName) // Use path.Base to strip any slashes
	if folderName == "" || folderName == "." || folderName == ".." {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid folder name"})
		return
	}

	currentPath := path.Clean(req.Path)
	if strings.HasPrefix(currentPath, "..") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid path"})
		return
	}
	if currentPath == "." {
		currentPath = ""
	}

	// Construct the final S3 key for the new folder
	baseAssetPath := fmt.Sprintf("organizations/%s/%s/assets",
		sanitizeKey(project.OrganizationKey),
		sanitizeKey(project.ProjectKey),
	)
	folderKey := path.Join(baseAssetPath, currentPath, folderName) + "/"

	// Create the empty object in S3
	_, err = s3Client.Client.PutObject(c.Request.Context(), &s3.PutObjectInput{
		Bucket: aws.String(s3Client.Bucket),
		Key:    aws.String(folderKey),
		Body:   strings.NewReader(""),
	})

	if err != nil {
		zlog.Error().Err(err).Str("key", folderKey).Msg("Failed to create S3 folder object")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not create folder"})
		return
	}

	c.Status(http.StatusCreated)
}

// handleDashboardRenderRequest submits a new render job from the authenticated dashboard.
func handleDashboardRenderRequest(c *gin.Context) {
	userCtx, _ := c.Get("user")
	user := userCtx.(User)
	jobID := uuid.New()
	correlationID := uuid.New().String()

	project, err := getProjectForUser(c, c.Param("id"), user)
	if err != nil {
		return
	}

	var req DashboardRenderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	log := zlog.With().
		Str("correlationId", correlationID).
		Str("jobId", jobID.String()).
		Str("projectId", project.ID.String()).
		Str("userId", user.ID.String()).
		Logger()

	log.Info().Interface("received_payload", req.Payload).Msg("Received dashboard render request")

	destinations, err := getDestinationsForProject(c.Request.Context(), project.ID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get destinations for project")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not retrieve project destinations"})
		return
	}
	if len(destinations) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No destinations configured for this project"})
		return
	}

	var rawTemplate json.RawMessage
	query := `SELECT template_data FROM templates WHERE id = $1 AND (project_id IS NULL OR project_id = $2) ORDER BY project_id DESC NULLS LAST LIMIT 1`
	err = dbpool.QueryRow(c.Request.Context(), query, req.TemplateID, project.ID).Scan(&rawTemplate)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Template not found"})
		return
	}

	finalTemplate, err := createFinalTemplate(c.Request.Context(), rawTemplate, req.Payload, req.BackgroundImageURL, *project)
	if err != nil {
		log.Error().Err(err).Msg("Failed to process final template")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to prepare render job", "reason": err.Error()})
		return
	}

	_, err = dbpool.Exec(c.Request.Context(), `INSERT INTO jobs (id, project_id, user_id, template_id, status) VALUES ($1, $2, $3, $4, 'queued')`, jobID, project.ID, user.ID, req.TemplateID)
	if err != nil {
		zlog.Error().
			Err(err).
			Str("jobId", jobID.String()).
			Str("projectId", project.ID.String()).
			Msg("Failed to create job record in database")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create job record"})
		return
	}

	sanitizedOrganizationKey := sanitizeKey(project.OrganizationKey)
	sanitizedProjectKey := sanitizeKey(project.ProjectKey)
	outputKey := fmt.Sprintf("organizations/%s/%s/outputs/final_%s.mp4", sanitizedOrganizationKey, sanitizedProjectKey, jobID)

	jobPayloadMap := map[string]interface{}{
		"job_id":             jobID.String(),
		"correlation_id":     correlationID,
		"payload":            req.Payload,
		"template":           json.RawMessage(finalTemplate),
		"output_storage_key": outputKey,
		"destinations":       destinations,
		"template_id":        req.TemplateID,
	}
	jobPayload, _ := json.Marshal(jobPayloadMap)

	if err := redisClient.XAdd(c.Request.Context(), &redis.XAddArgs{Stream: "render_jobs", Values: map[string]interface{}{"payload": jobPayload}}).Err(); err != nil {
		zlog.Error().
			Err(err).
			Str("jobId", jobID.String()).
			Str("stream", "render_jobs").
			Msg("Failed to dispatch job to queue")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to dispatch job"})
		return
	}

	log.Info().Int("destination_count", len(destinations)).Msg("Successfully dispatched job from dashboard")
	c.JSON(http.StatusAccepted, gin.H{"message": "Render job accepted", "jobId": jobID})
}

func getProjectForUser(c *gin.Context, projectIDStr string, user User) (*Project, error) {
	projectID, err := uuid.Parse(projectIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid project ID format"})
		return nil, err
	}

	var project Project
	query := `
        SELECT p.id, p.organization_id, o.organization_key, p.project_key
        FROM projects p
        JOIN organizations o ON p.organization_id = o.id
        WHERE p.id = $1 AND p.organization_id = $2
    `
	err = dbpool.QueryRow(context.Background(), query, projectID, user.OrganizationID).Scan(
		&project.ID, &project.OrganizationID, &project.OrganizationKey, &project.ProjectKey,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "Project not found or you do not have permission to access it"})
			return nil, err
		}
		zlog.Error().Err(err).Str("projectId", projectIDStr).Msg("Error fetching project for user")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve project"})
		return nil, err
	}
	return &project, nil
}

func handleDeleteFolder(c *gin.Context) {
	userCtx, _ := c.Get("user")
	user := userCtx.(User)

	project, err := getProjectForUser(c, c.Param("id"), user)
	if err != nil {
		return
	}

	// Get the folder path from the query parameter
	folderPath := c.Query("path")
	if folderPath == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing 'path' query parameter"})
		return
	}

	// Sanitize and construct the full S3 prefix
	cleanPath := path.Clean(folderPath)
	if strings.HasPrefix(cleanPath, "..") || cleanPath == "." {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid path"})
		return
	}

	basePrefix := fmt.Sprintf("organizations/%s/%s/assets/",
		sanitizeKey(project.OrganizationKey),
		sanitizeKey(project.ProjectKey),
	)
	folderPrefix := path.Join(basePrefix, cleanPath) + "/"

	// Security check: ensure we are not deleting outside the project's assets
	if !strings.HasPrefix(folderPrefix, basePrefix) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Forbidden"})
		return
	}

	// 1. List all objects under the prefix
	var objectsToDelete []types.ObjectIdentifier
	paginator := s3.NewListObjectsV2Paginator(s3Client.Client, &s3.ListObjectsV2Input{
		Bucket: aws.String(s3Client.Bucket),
		Prefix: aws.String(folderPrefix),
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(c.Request.Context())
		if err != nil {
			zlog.Error().Err(err).Str("prefix", folderPrefix).Msg("Failed to list objects for deletion")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not list folder contents"})
			return
		}
		for _, obj := range page.Contents {
			objectsToDelete = append(objectsToDelete, types.ObjectIdentifier{Key: obj.Key})
		}
	}

	// If there are no objects, there's nothing to do.
	if len(objectsToDelete) == 0 {
		c.Status(http.StatusNoContent)
		return
	}

	// 2. Perform a bulk delete
	_, err = s3Client.Client.DeleteObjects(c.Request.Context(), &s3.DeleteObjectsInput{
		Bucket: aws.String(s3Client.Bucket),
		Delete: &types.Delete{Objects: objectsToDelete},
	})

	if err != nil {
		zlog.Error().Err(err).Str("prefix", folderPrefix).Msg("Failed to bulk delete S3 objects")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not delete folder contents"})
		return
	}

	c.Status(http.StatusNoContent)
}

// handleListConnections retrieves all connections for a given project.
func handleListConnections(c *gin.Context) {
	userCtx, _ := c.Get("user")
	user := userCtx.(User)

	// We use getProjectForUser to verify the user has access to this project
	project, err := getProjectForUser(c, c.Param("id"), user)
	if err != nil {
		return // getProjectForUser handles the error response
	}

	query := `SELECT id, platform FROM connections WHERE project_id = $1 ORDER BY created_at`
	rows, err := dbpool.Query(c.Request.Context(), query, project.ID)
	if err != nil {
		zlog.Error().Err(err).Msg("Failed to query for connections")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not retrieve connections"})
		return
	}
	defer rows.Close()

	connections, err := pgx.CollectRows(rows, pgx.RowToStructByPos[ConnectionResponse])
	if err != nil {
		zlog.Error().Err(err).Msg("Failed to scan connection rows")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not process connection data"})
		return
	}

	c.JSON(http.StatusOK, connections)
}

// handleCreateConnection adds a new destination connection to a project.
func handleCreateConnection(c *gin.Context) {
	userCtx, _ := c.Get("user")
	user := userCtx.(User)

	project, err := getProjectForUser(c, c.Param("id"), user)
	if err != nil {
		return
	}

	var req CreateConnectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Basic validation for supported platforms
	if req.Platform != "s3" && req.Platform != "webhook" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unsupported platform"})
		return
	}

	var newID uuid.UUID
	query := `INSERT INTO connections (project_id, platform, config) VALUES ($1, $2, $3) RETURNING id`
	err = dbpool.QueryRow(c.Request.Context(), query, project.ID, req.Platform, req.Config).Scan(&newID)
	if err != nil {
		zlog.Error().Err(err).Msg("Failed to create connection in database")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not create connection"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"id": newID, "platform": req.Platform})
}

// handleDeleteConnection removes a destination connection.
func handleDeleteConnection(c *gin.Context) {
	userCtx, _ := c.Get("user")
	user := userCtx.(User)

	connectionID, err := uuid.Parse(c.Param("connectionId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid connection ID format"})
		return
	}

	// Security: This query ensures a user can only delete connections
	// that belong to a project within their own organization.
	query := `
		DELETE FROM connections c
		USING projects p
		WHERE c.id = $1
		  AND c.project_id = p.id
		  AND p.organization_id = $2
	`
	result, err := dbpool.Exec(c.Request.Context(), query, connectionID, user.OrganizationID)
	if err != nil {
		zlog.Error().Err(err).Str("connectionId", connectionID.String()).Msg("Failed to delete connection")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not delete connection"})
		return
	}

	if result.RowsAffected() == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Connection not found or you do not have permission to delete it"})
		return
	}

	c.Status(http.StatusNoContent)
}

func handleCreateTemplate(c *gin.Context) {
	userCtx, _ := c.Get("user")
	user := userCtx.(User)

	project, err := getProjectForUser(c, c.Param("id"), user)
	if err != nil {
		return // getProjectForUser handles the error response
	}

	var req CreateTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate that the provided data is valid JSON
	if !json.Valid(req.TemplateData) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "templateData is not valid JSON"})
		return
	}

	var newTemplate TemplateResponse
	query := `
		INSERT INTO templates (project_id, id, template_data)
		VALUES ($1, $2, $3)
		RETURNING template_uuid, id, (project_id IS NULL) as is_system
	`
	err = dbpool.QueryRow(c.Request.Context(), query, project.ID, req.ID, req.TemplateData).Scan(
		&newTemplate.UUID, &newTemplate.ID, &newTemplate.IsSystem,
	)

	if err != nil {
		// Check for unique constraint violation
		if strings.Contains(err.Error(), "unique constraint") {
			c.JSON(http.StatusConflict, gin.H{"error": "A template with this ID already exists for this project."})
			return
		}
		zlog.Error().Err(err).Msg("Failed to create template in database")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not create template"})
		return
	}

	c.JSON(http.StatusCreated, newTemplate)
}

func handleDeleteTemplate(c *gin.Context) {
	userCtx, _ := c.Get("user")
	user := userCtx.(User)

	templateUUID, err := uuid.Parse(c.Param("template_uuid"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid template UUID format"})
		return
	}

	// Security: This query ensures a user can only delete a non-system template
	// that belongs to a project within their own organization.
	query := `
		DELETE FROM templates t
		USING projects p
		WHERE t.template_uuid = $1
		  AND t.project_id IS NOT NULL -- Prevent deleting system templates
		  AND t.project_id = p.id
		  AND p.organization_id = $2
	`
	result, err := dbpool.Exec(c.Request.Context(), query, templateUUID, user.OrganizationID)
	if err != nil {
		zlog.Error().Err(err).Str("templateUuid", templateUUID.String()).Msg("Failed to delete template")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not delete template"})
		return
	}

	if result.RowsAffected() == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Template not found, is a system template, or you do not have permission to delete it"})
		return
	}

	c.Status(http.StatusNoContent)
}

func handleListMyJobs(c *gin.Context) {
	userCtx, _ := c.Get("user")
	user := userCtx.(User)

	limit := c.Query("limit")
	limitClause := ""
	if limit != "" {
		if _, err := strconv.Atoi(limit); err == nil {
			limitClause = "LIMIT " + limit
		}
	}

	query := fmt.Sprintf(`
		SELECT
			j.id,
			p.project_key,
			j.template_id,
			j.status,
			j.created_at,
			j.completed_at,
			j.error_message,
			j.output_storage_key
		FROM jobs j
		JOIN projects p ON j.project_id = p.id
		WHERE j.user_id = $1
		ORDER BY j.created_at DESC
		%s
	`, limitClause)

	rows, err := dbpool.Query(c.Request.Context(), query, user.ID)
	if err != nil {
		zlog.Error().Err(err).Msg("Failed to query for user's jobs")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not retrieve jobs"})
		return
	}
	defer rows.Close()

	var jobs []JobResponse
	for rows.Next() {
		var job JobResponse
		err := rows.Scan(
			&job.ID,
			&job.ProjectKey,
			&job.TemplateID,
			&job.Status,
			&job.CreatedAt,
			&job.CompletedAt,
			&job.ErrorMessage,
			&job.OutputStorageKey,
		)
		if err != nil {
			zlog.Error().Err(err).Msg("Failed to scan job row")
			// Return immediately because this is a critical error
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not process job data"})
			return
		}
		jobs = append(jobs, job)
	}

	if rows.Err() != nil {
		zlog.Error().Err(rows.Err()).Msg("Error iterating over job rows")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not process job data"})
		return
	}

	for i := range jobs {
		if jobs[i].Status == "complete" && jobs[i].OutputStorageKey.Valid {
			url, err := getPresignedURL(c.Request.Context(), s3Client.PublicPresigner, jobs[i].OutputStorageKey.String)
			if err != nil {
				zlog.Error().Err(err).Str("jobId", jobs[i].ID.String()).Msg("Failed to presign output URL for job list")
			} else {
				jobs[i].OutputURL = &url
			}
		}
	}

	c.JSON(http.StatusOK, jobs)
}
