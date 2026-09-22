package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// getPresignedURL generates a short-lived URL for a given S3 object for GET requests.
func getPresignedURL(ctx context.Context, presigner *s3.PresignClient, path string) (string, error) {
	lifetime := 15 * time.Minute
	req, err := presigner.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s3Client.Bucket), Key: aws.String(path),
	}, func(opts *s3.PresignOptions) { opts.Expires = lifetime })
	if err != nil {
		return "", err
	}
	return req.URL, nil
}

// initS3Client initializes S3 clients for internal (service-to-service) and public (user-facing) access.
func initS3Client(ctx context.Context) (*S3Client, error) {
	baseCfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(os.Getenv("S3_ACCESS_KEY_ID"), os.Getenv("S3_SECRET_ACCESS_KEY"), "")),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load base aws config: %w", err)
	}

	// Client for internal traffic (e.g., orchestrator -> renderer)
	internalEndpoint := os.Getenv("S3_ENDPOINT")
	internalResolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
		if internalEndpoint != "" {
			return aws.Endpoint{URL: internalEndpoint, HostnameImmutable: true}, nil
		}
		return aws.Endpoint{}, &aws.EndpointNotFoundError{}
	})
	internalCfg := baseCfg.Copy()
	internalCfg.EndpointResolverWithOptions = internalResolver
	internalClient := s3.NewFromConfig(internalCfg, func(o *s3.Options) { o.UsePathStyle = true })

	// Client for public-facing URLs (e.g., for the final download link and asset uploads)
	publicEndpoint := os.Getenv("S3_PUBLIC_ENDPOINT")
	if publicEndpoint == "" {
		publicEndpoint = internalEndpoint
	}
	publicResolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
		if publicEndpoint != "" {
			return aws.Endpoint{URL: publicEndpoint, HostnameImmutable: true}, nil
		}
		return aws.Endpoint{}, &aws.EndpointNotFoundError{}
	})
	publicCfg := baseCfg.Copy()
	publicCfg.EndpointResolverWithOptions = publicResolver
	publicClient := s3.NewFromConfig(publicCfg, func(o *s3.Options) { o.UsePathStyle = true })

	return &S3Client{
		Client:            internalClient, // For internal operations
		Uploader:          manager.NewUploader(internalClient),
		Bucket:            os.Getenv("S3_BUCKET_NAME"),
		InternalPresigner: s3.NewPresignClient(internalClient), // For internal GETs (renderer)
		PublicPresigner:   s3.NewPresignClient(publicClient),   // For public GETs (downloads) AND public PUTs (uploads)
	}, nil
}
