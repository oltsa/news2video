package main

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// Project holds identifying information for an authenticated project.
type Project struct {
	ID              uuid.UUID
	OrganizationID  uuid.UUID
	OrganizationKey string
	ProjectKey      string
}

// RenderRequest is the structure of an incoming API request to render a video.
type RenderRequest struct {
	TemplateID         string                 `json:"templateId" binding:"required"`
	Payload            map[string]interface{} `json:"payload" binding:"required"`
	BackgroundImageURL *string                `json:"backgroundImageUrl"`
}

// Destination represents a target for the final video, to be sent in the job payload.
type Destination struct {
	Platform     string          `json:"platform"`
	ConnectionID uuid.UUID       `json:"connection_id"`
	Config       json.RawMessage `json:"config"`
}

// S3Client holds clients and presigners for interacting with S3.
type S3Client struct {
	Client            *s3.Client
	Uploader          *manager.Uploader
	Bucket            string
	InternalPresigner *s3.PresignClient // For service-to-service URLs (Renderer)
	PublicPresigner   *s3.PresignClient // For external-facing URLs (end-user)
}

// --- AUTHENTICATION TYPES ALIGNED WITH DB SCHEMA ---

// User represents a user account retrieved from the database or JWT claims.
type User struct {
	ID              uuid.UUID `json:"id"`
	OrganizationID  uuid.UUID `json:"organization_id"`
	Email           string    `json:"email"`
	IsCustomerAdmin bool      `json:"is_customer_admin"`
}

// LoginRequest is the structure for a user login API call.
type LoginRequest struct {
	Email    string `json:"email" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// Claims represents the data stored in the JWT.
type Claims struct {
	UserID          string `json:"userId"`
	OrganizationID  string `json:"orgId"`
	IsCustomerAdmin bool   `json:"isCustomerAdmin"`
	jwt.RegisteredClaims
}

// ConnectionResponse is the simplified view of a connection for lists.
type ConnectionResponse struct {
	ID       uuid.UUID `json:"id"`
	Platform string    `json:"platform"`
}

// CreateConnectionRequest is the request body for creating a new connection.
type CreateConnectionRequest struct {
	Platform string          `json:"platform" binding:"required"`
	Config   json.RawMessage `json:"config" binding:"required"`
}

// --- API RESPONSE TYPES FOR DASHBOARD ---

// ProjectResponse is the simplified project view for lists.
type ProjectResponse struct {
	ID         uuid.UUID `json:"id"`
	ProjectKey string    `json:"project_key"`
}

// ProjectDetailResponse is the detailed project view, including the API key.
type ProjectDetailResponse struct {
	ID         uuid.UUID `json:"id"`
	ProjectKey string    `json:"project_key"`
	APIKey     uuid.UUID `json:"api_key"`
}

// AssetItem represents a single file or folder in S3.
type AssetItem struct {
	Key          string    `json:"key"`
	LastModified time.Time `json:"last_modified"`
	Size         int64     `json:"size"`
	IsDir        bool      `json:"is_dir"`
}

// UploadURLRequest is the request body for generating a presigned upload URL.
type UploadURLRequest struct {
	Filename    string `json:"filename" binding:"required"`
	ContentType string `json:"contentType" binding:"required"`
	AssetType   string `json:"assetType" binding:"required"`
}

type TemplateResponse struct {
	UUID     uuid.UUID `json:"uuid"`
	ID       string    `json:"id"`
	IsSystem bool      `json:"is_system"`
}

type CreateTemplateRequest struct {
	ID           string          `json:"id" binding:"required"`
	TemplateData json.RawMessage `json:"templateData" binding:"required"`
}

type CreateFolderRequest struct {
	Path       string `json:"path"` // The directory where the new folder will be created
	FolderName string `json:"folderName" binding:"required"`
}

// DashboardRenderRequest is the structure for submitting a new render job from the UI.
type DashboardRenderRequest struct {
	TemplateID         string                 `json:"templateId" binding:"required"`
	Payload            map[string]interface{} `json:"payload" binding:"required"`
	BackgroundImageURL string                 `json:"backgroundImageUrl"`
}

type JobResponse struct {
	ID               uuid.UUID      `json:"id"`
	ProjectKey       string         `json:"project_key"`
	TemplateID       string         `json:"template_id"`
	Status           string         `json:"status"`
	CreatedAt        time.Time      `json:"created_at"`
	CompletedAt      sql.NullTime   `json:"completed_at"`
	ErrorMessage     sql.NullString `json:"error_message"`
	OutputStorageKey sql.NullString `json:"output_storage_key"`
	OutputURL        *string        `json:"output_url,omitempty" pg:"-"`
}
