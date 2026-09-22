package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// initS3Client initializes a new S3 client for the CLI tool.
func initS3Client(ctx context.Context) (*s3.Client, error) {
	endpoint := os.Getenv("S3_ENDPOINT")
	resolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
		if endpoint != "" {
			return aws.Endpoint{URL: endpoint, HostnameImmutable: true}, nil
		}
		return aws.Endpoint{}, &aws.EndpointNotFoundError{}
	})

	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion("us-east-1"),
		config.WithEndpointResolverWithOptions(resolver),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(os.Getenv("S3_ACCESS_KEY_ID"), os.Getenv("S3_SECRET_ACCESS_KEY"), "")),
	)
	if err != nil {
		return nil, err
	}
	return s3.NewFromConfig(cfg, func(o *s3.Options) { o.UsePathStyle = true }), nil
}

// createS3ProjectStructure creates the initial directories for a new project in MinIO.
func createS3ProjectStructure(ctx context.Context, s3Client *s3.Client, bucket, orgKey, projectKey string) error {
	// S3 doesn't have real directories, but creating empty objects with a trailing slash
	// makes them appear as folders in most UIs, like the MinIO console.

	// Sanitize keys to match the format used by the orchestrator
	sanitizedOrgKey := strings.ReplaceAll(strings.ToLower(orgKey), " ", "_")
	sanitizedProjectKey := strings.ReplaceAll(strings.ToLower(projectKey), " ", "_")

	basePath := fmt.Sprintf("organizations/%s/%s", sanitizedOrgKey, sanitizedProjectKey)
	pathsToCreate := []string{
		fmt.Sprintf("%s/assets/", basePath),
		fmt.Sprintf("%s/outputs/", basePath),
	}

	for _, path := range pathsToCreate {
		_, err := s3Client.PutObject(ctx, &s3.PutObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(path),
			Body:   strings.NewReader(""), // Empty body
		})
		if err != nil {
			return fmt.Errorf("failed to create S3 directory structure at '%s': %w", path, err)
		}
		fmt.Printf("📂 Created S3 directory: %s\n", path)
	}

	return nil
}
