// Package shortener provides a ShortenerService gRPC client wrapper.
package shortener

import (
	"context"
	"fmt"
	"time"

	shortenerv1 "github.com/aligh5331/gobox-proto/gen/shortener/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/emptypb"
)

// Client wraps the ShortenerService gRPC client.
type Client struct {
	raw shortenerv1.ShortenerServiceClient
	cc  *grpc.ClientConn
}

// NewClient creates a new Shortener gRPC client and waits for the connection
// to become ready.
func NewClient(ctx context.Context, addr string) (*Client, error) {
	cc, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithConnectParams(grpc.ConnectParams{
			MinConnectTimeout: 5 * time.Second,
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("grpcclient/shortener: create client %q: %w", addr, err)
	}

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
			return nil, fmt.Errorf("grpcclient/shortener: timeout dialing %q", addr)
		}
	}

	return &Client{
		raw: shortenerv1.NewShortenerServiceClient(cc),
		cc:  cc,
	}, nil
}

// Close closes the gRPC connection.
func (c *Client) Close() error {
	return c.cc.Close()
}

// CreateLink creates a new short link for the given file.
func (c *Client) CreateLink(ctx context.Context, req *shortenerv1.CreateLinkRequest) (*shortenerv1.CreateLinkResponse, error) {
	resp, err := c.raw.CreateLink(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("grpcclient/shortener: create link: %w", err)
	}
	return resp, nil
}

// ListLinks retrieves paginated short links for the given user.
func (c *Client) ListLinks(ctx context.Context, req *shortenerv1.ListLinksRequest) (*shortenerv1.ListLinksResponse, error) {
	resp, err := c.raw.ListLinks(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("grpcclient/shortener: list links: %w", err)
	}
	return resp, nil
}

// DeleteLink deletes a short link.
func (c *Client) DeleteLink(ctx context.Context, req *shortenerv1.DeleteLinkRequest) (*emptypb.Empty, error) {
	resp, err := c.raw.DeleteLink(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("grpcclient/shortener: delete link: %w", err)
	}
	return resp, nil
}

// compile-time interface check
var _ interface {
	CreateLink(context.Context, *shortenerv1.CreateLinkRequest) (*shortenerv1.CreateLinkResponse, error)
	ListLinks(context.Context, *shortenerv1.ListLinksRequest) (*shortenerv1.ListLinksResponse, error)
	DeleteLink(context.Context, *shortenerv1.DeleteLinkRequest) (*emptypb.Empty, error)
} = (*Client)(nil)
