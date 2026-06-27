// Package minio implements the MinioClient interface expected by use cases.
package minio

import (
	"context"
	"fmt"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Client wraps the MinIO SDK and satisfies the usecase.MinioClient interface.
type Client struct {
	mc     *minio.Client
	bucket string
}

// NewClient creates a new MinIO client wrapper.
func NewClient(endpoint, accessKey, secretKey, bucket string, secure bool) (*Client, error) {
	mc, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: secure,
	})
	if err != nil {
		return nil, fmt.Errorf("minio: create client: %w", err)
	}
	return &Client{mc: mc, bucket: bucket}, nil
}

// PresignedPutURL generates a presigned PUT URL for uploading an object.
func (c *Client) PresignedPutURL(ctx context.Context, objectKey string, ttl time.Duration) (string, map[string]string, error) {
	url, err := c.mc.PresignedPutObject(ctx, c.bucket, objectKey, ttl)
	if err != nil {
		return "", nil, fmt.Errorf("minio: presign put: %w", err)
	}
	return url.String(), nil, nil
}

// PresignedGetURL generates a presigned GET URL for downloading an object.
func (c *Client) PresignedGetURL(ctx context.Context, objectKey string, ttl time.Duration) (string, time.Time, error) {
	expiry := time.Now().Add(ttl)
	url, err := c.mc.PresignedGetObject(ctx, c.bucket, objectKey, ttl, nil)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("minio: presign get: %w", err)
	}
	return url.String(), expiry, nil
}

// ObjectExists checks whether the object exists in S3 via a HEAD request.
func (c *Client) ObjectExists(ctx context.Context, objectKey string) (bool, error) {
	_, err := c.mc.StatObject(ctx, c.bucket, objectKey, minio.StatObjectOptions{})
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
func (c *Client) RemoveObject(ctx context.Context, objectKey string) error {
	err := c.mc.RemoveObject(ctx, c.bucket, objectKey, minio.RemoveObjectOptions{})
	if err != nil {
		return fmt.Errorf("minio: remove object: %w", err)
	}
	return nil
}
