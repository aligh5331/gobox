// Package thumbgen provides a no-op stub for the ThumbGenService gRPC client.
// This is a placeholder until Phase 5 delivers the real thumbgen service.
// It compiles and runs without the thumbgen binary, logging instead of calling gRPC.
package thumbgen

import (
	"context"
	"fmt"

	thumbgenv1 "github.com/aligh5331/gobox-proto/gen/thumbgen/v1"
)

// Client is a no-op stub for ThumbGenServiceClient.
// All methods return a successful empty response without making any RPC.
type Client struct {
	// enabled, when true, would hold a real gRPC connection.
	// Currently unused — kept for future wiring.
	enabled bool
}

// NewClient creates a no-op ThumbGen stub. The addr parameter is accepted
// for forward compatibility but ignored until the real service is wired.
func NewClient(_ context.Context, addr string) (*Client, error) {
	if addr == "" {
		return nil, fmt.Errorf("grpcclient/thumbgen: addr must not be empty")
	}
	return &Client{enabled: false}, nil
}

// Close is a no-op.
func (c *Client) Close() error { return nil }

// EnqueueJob is a no-op that returns a stub response with QUEUED status.
func (c *Client) EnqueueJob(_ context.Context, req *thumbgenv1.EnqueueJobRequest) (*thumbgenv1.EnqueueJobResponse, error) {
	return &thumbgenv1.EnqueueJobResponse{
		Job: &thumbgenv1.JobResponse{
			Id:       "stub-" + req.FileId,
			FileId:   req.FileId,
			UserId:   req.UserId,
			Status:   thumbgenv1.JobStatus_JOB_STATUS_QUEUED,
			InputKey: req.InputKey,
		},
	}, nil
}

// compile-time interface check
var _ interface {
	EnqueueJob(context.Context, *thumbgenv1.EnqueueJobRequest) (*thumbgenv1.EnqueueJobResponse, error)
} = (*Client)(nil)
