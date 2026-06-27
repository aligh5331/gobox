// Package fileupload provides a thin gRPC client wrapper for the FileUploadService.
package fileupload

import (
	"context"
	"fmt"
	"time"

	fileuploadv1 "github.com/aligh5331/gobox-proto/gen/fileupload/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
)

// Client is a thin wrapper around fileuploadv1.FileUploadServiceClient.
type Client struct {
	raw fileuploadv1.FileUploadServiceClient
	cc  *grpc.ClientConn
}

// NewClient dials the FileUploadService gRPC endpoint and returns a client wrapper.
func NewClient(ctx context.Context, addr string) (*Client, error) {
	cc, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithConnectParams(grpc.ConnectParams{
			MinConnectTimeout: 5 * time.Second,
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("grpcclient/fileupload: create client %q: %w", addr, err)
	}

	// Wait for the connection to become ready (with timeout).
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cc.Connect()
	for {
		s := cc.GetState()
		if s == connectivity.Ready {
			break
		}
		if !cc.WaitForStateChange(ctx, s) {
			cc.Close()
			return nil, fmt.Errorf("grpcclient/fileupload: timeout dialing %q", addr)
		}
	}

	return &Client{
		raw: fileuploadv1.NewFileUploadServiceClient(cc),
		cc:  cc,
	}, nil
}

// Close closes the underlying gRPC connection.
func (c *Client) Close() error {
	return c.cc.Close()
}

// InitiateUpload proxies the InitiateUpload RPC.
func (c *Client) InitiateUpload(ctx context.Context, req *fileuploadv1.InitiateUploadRequest) (*fileuploadv1.InitiateUploadResponse, error) {
	resp, err := c.raw.InitiateUpload(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("grpcclient/fileupload: initiate upload: %w", err)
	}
	return resp, nil
}

// ConfirmUpload proxies the ConfirmUpload RPC.
func (c *Client) ConfirmUpload(ctx context.Context, req *fileuploadv1.ConfirmUploadRequest) (*fileuploadv1.ConfirmUploadResponse, error) {
	resp, err := c.raw.ConfirmUpload(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("grpcclient/fileupload: confirm upload: %w", err)
	}
	return resp, nil
}

// GetFile proxies the GetFile RPC.
func (c *Client) GetFile(ctx context.Context, req *fileuploadv1.GetFileRequest) (*fileuploadv1.GetFileResponse, error) {
	resp, err := c.raw.GetFile(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("grpcclient/fileupload: get file: %w", err)
	}
	return resp, nil
}

// ListFiles proxies the ListFiles RPC.
func (c *Client) ListFiles(ctx context.Context, req *fileuploadv1.ListFilesRequest) (*fileuploadv1.ListFilesResponse, error) {
	resp, err := c.raw.ListFiles(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("grpcclient/fileupload: list files: %w", err)
	}
	return resp, nil
}

// DeleteFile proxies the DeleteFile RPC.
func (c *Client) DeleteFile(ctx context.Context, req *fileuploadv1.DeleteFileRequest) error {
	_, err := c.raw.DeleteFile(ctx, req)
	if err != nil {
		return fmt.Errorf("grpcclient/fileupload: delete file: %w", err)
	}
	return nil
}

// GetDownloadURL proxies the GetDownloadURL RPC.
func (c *Client) GetDownloadURL(ctx context.Context, req *fileuploadv1.GetDownloadURLRequest) (*fileuploadv1.GetDownloadURLResponse, error) {
	resp, err := c.raw.GetDownloadURL(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("grpcclient/fileupload: get download url: %w", err)
	}
	return resp, nil
}

// compile-time interface check
var _ interface {
	InitiateUpload(context.Context, *fileuploadv1.InitiateUploadRequest) (*fileuploadv1.InitiateUploadResponse, error)
	ConfirmUpload(context.Context, *fileuploadv1.ConfirmUploadRequest) (*fileuploadv1.ConfirmUploadResponse, error)
	GetFile(context.Context, *fileuploadv1.GetFileRequest) (*fileuploadv1.GetFileResponse, error)
	ListFiles(context.Context, *fileuploadv1.ListFilesRequest) (*fileuploadv1.ListFilesResponse, error)
	DeleteFile(context.Context, *fileuploadv1.DeleteFileRequest) error
	GetDownloadURL(context.Context, *fileuploadv1.GetDownloadURLRequest) (*fileuploadv1.GetDownloadURLResponse, error)
} = (*Client)(nil)
