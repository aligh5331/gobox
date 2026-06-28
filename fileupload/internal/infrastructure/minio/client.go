// Package minio implements the MinioClient interface expected by use cases.
package minio

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Client wraps the MinIO SDK and satisfies the usecase.MinioClient interface.
type Client struct {
	internalMC *minio.Client // for API calls (ObjectExists, RemoveObject)
	presignMC  *minio.Client // for presigned URL generation (uses public endpoint if configured)
	bucket     string
}

// NewClient creates a new MinIO client wrapper and ensures the bucket exists.
// endpoint is used for internal API calls. If publicEndpoint is non-empty, it
// is used for generating presigned URLs instead, so that presigned URLs are
// reachable from outside the Docker network.
func NewClient(endpoint, accessKey, secretKey, bucket string, secure bool, publicEndpoint string) (*Client, error) {
	internalMC, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: secure,
	})
	if err != nil {
		return nil, fmt.Errorf("minio: create internal client: %w", err)
	}

	// Use the public endpoint for presigned URL generation, falling back to
	// the internal endpoint if no public endpoint is configured.
	presignEndpoint := endpoint
	if publicEndpoint != "" {
		presignEndpoint = publicEndpoint
		// Strip http:// or https:// scheme — minio.New() expects host:port only.
		if strings.HasPrefix(presignEndpoint, "http://") {
			presignEndpoint = presignEndpoint[len("http://"):]
		} else if strings.HasPrefix(presignEndpoint, "https://") {
			presignEndpoint = presignEndpoint[len("https://"):]
		}
	}
	presignMC, err := minio.New(presignEndpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: secure,
		Region: "us-east-1", // explicit region avoids SDK auto-detection (HEAD request)
	})
	if err != nil {
		return nil, fmt.Errorf("minio: create presign client: %w", err)
	}

	// Ensure the bucket exists via the internal client; no-op if already present.
	ctx := context.Background()
	if err := internalMC.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
		exists, existsErr := internalMC.BucketExists(ctx, bucket)
		if existsErr != nil || !exists {
			return nil, fmt.Errorf("minio: make bucket: %w", err)
		}
	}

	return &Client{internalMC: internalMC, presignMC: presignMC, bucket: bucket}, nil
}

// PresignedPutURL generates a presigned PUT URL for uploading an object.
// Uses the presign client so the URL is reachable from outside Docker.
func (c *Client) PresignedPutURL(ctx context.Context, objectKey string, ttl time.Duration) (string, map[string]string, error) {
	url, err := c.presignMC.PresignedPutObject(ctx, c.bucket, objectKey, ttl)
	if err != nil {
		return "", nil, fmt.Errorf("minio: presign put: %w", err)
	}
	return url.String(), nil, nil
}

// PresignedGetURL generates a presigned GET URL for downloading an object.
// Uses the presign client so the URL is reachable from outside Docker.
func (c *Client) PresignedGetURL(ctx context.Context, objectKey string, ttl time.Duration) (string, time.Time, error) {
	expiry := time.Now().Add(ttl)
	url, err := c.presignMC.PresignedGetObject(ctx, c.bucket, objectKey, ttl, nil)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("minio: presign get: %w", err)
	}
	return url.String(), expiry, nil
}

// ObjectExists checks whether the object exists in S3 via a HEAD request.
// Uses the internal client to reach MinIO within the Docker network.
func (c *Client) ObjectExists(ctx context.Context, objectKey string) (bool, error) {
	_, err := c.internalMC.StatObject(ctx, c.bucket, objectKey, minio.StatObjectOptions{})
	if err != nil {
		errResp := minio.ToErrorResponse(err)
		if errResp.Code == "NoSuchKey" || errResp.Code == "NotFound" {
			return false, nil
		}
		return false, fmt.Errorf("minio: stat object: %w", err)
	}
	return true, nil
}

// RemoveObject deletes an object from S3.
// Uses the internal client to reach MinIO within the Docker network.
func (c *Client) RemoveObject(ctx context.Context, objectKey string) error {
	err := c.internalMC.RemoveObject(ctx, c.bucket, objectKey, minio.RemoveObjectOptions{})
	if err != nil {
		return fmt.Errorf("minio: remove object: %w", err)
	}
	return nil
}
