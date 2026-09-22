package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// handleRenderRequest is the main handler for creating new render jobs.
func handleRenderRequest(c *gin.Context) {
	projectCtx, _ := c.Get("project")
	project := projectCtx.(Project)
	jobID := uuid.New()
	correlationID := uuid.New().String()

	var req RenderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	log := zlog.With().
		Str("correlationId", correlationID).
		Str("jobId", jobID.String()).
		Str("projectId", project.ID.String()).
		Logger()

	log.Info().Msg("Received render request")

	destinations, err := getDestinationsForProject(c.Request.Context(), project.ID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get destinations for project")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not retrieve project destinations"})
		return
	}
	if len(destinations) == 0 {
		log.Warn().Msg("Project has no destinations configured")
		c.JSON(http.StatusBadRequest, gin.H{"error": "No destinations configured for this project"})
		return
	}

	var rawTemplate json.RawMessage
	query := `SELECT template_data FROM templates WHERE id = $1 AND (project_id IS NULL OR project_id = $2) ORDER BY project_id DESC NULLS LAST LIMIT 1`
	err = dbpool.QueryRow(c.Request.Context(), query, req.TemplateID, project.ID).Scan(&rawTemplate)
	if err != nil {
		zlog.Warn().
			Err(err).
			Str("templateId", req.TemplateID).
			Str("projectId", project.ID.String()).
			Msg("Template not found")
		c.JSON(http.StatusNotFound, gin.H{"error": "Template not found"})
		return
	}

	bgImageURL := ""
	if req.BackgroundImageURL != nil {
		bgImageURL = *req.BackgroundImageURL
	}

	finalTemplate, err := createFinalTemplate(c.Request.Context(), rawTemplate, req.Payload, bgImageURL, project)
	if err != nil {
		log.Error().Err(err).Msg("Failed to process and create the final template")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to prepare render job", "reason": err.Error()})
		return
	}

	_, err = dbpool.Exec(c.Request.Context(), `INSERT INTO jobs (id, project_id, template_id, status) VALUES ($1, $2, $3, 'queued')`, jobID, project.ID, req.TemplateID)
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
	jobPayload, err := json.Marshal(jobPayloadMap)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to marshal final job payload"})
		return
	}

	if err := redisClient.XAdd(c.Request.Context(), &redis.XAddArgs{Stream: "render_jobs", Values: map[string]interface{}{"payload": jobPayload}}).Err(); err != nil {
		zlog.Error().
			Err(err).
			Str("jobId", jobID.String()).
			Str("stream", "render_jobs").
			Msg("Failed to dispatch job to queue")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to dispatch job"})
		return
	}

	log.Info().Int("destination_count", len(destinations)).Msg("Successfully dispatched job")
	c.JSON(http.StatusAccepted, gin.H{"message": "Render job accepted", "jobId": jobID})
}

// handleJobStatusRequest handles polling for job status.
func handleJobStatusRequest(c *gin.Context) {
	projectCtx, _ := c.Get("project")
	project := projectCtx.(Project)
	jobIDStr := c.Param("jobId")

	var status string
	var errorMessage, outputKey sql.NullString

	query := `SELECT status, error_message, output_storage_key FROM jobs WHERE id = $1 AND project_id = $2`
	err := dbpool.QueryRow(c.Request.Context(), query, jobIDStr, project.ID).Scan(&status, &errorMessage, &outputKey)

	if err != nil {
		if err == pgx.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "Job not found"})
			return
		}
		zlog.Error().Err(err).Str("jobId", jobIDStr).Msg("Error querying for job status")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve job status"})
		return
	}

	response := gin.H{"status": status}
	if status == "complete" && outputKey.Valid {
		url, err := getPresignedURL(c.Request.Context(), s3Client.PublicPresigner, outputKey.String)
		if err != nil {
			zlog.Error().Err(err).Str("jobId", jobIDStr).Msg("Failed to presign public output URL")
		} else {
			response["output_url"] = url
		}
	} else if status == "failed" && errorMessage.Valid {
		response["error_message"] = errorMessage.String
	}

	c.JSON(http.StatusOK, response)
}
