// Package grpcclient provides thin wrappers around gRPC service clients.
package grpcclient

import (
	"context"
	"fmt"
	"time"

	authv1 "github.com/aligh5331/gobox-proto/gen/auth/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/emptypb"
)

// AuthClient is a thin wrapper around authv1.AuthServiceClient.
type AuthClient struct {
	raw authv1.AuthServiceClient
	cc  *grpc.ClientConn
}

// NewAuthClient dials the Auth gRPC service and returns a client wrapper.
func NewAuthClient(ctx context.Context, addr string) (*AuthClient, error) {
	cc, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithConnectParams(grpc.ConnectParams{
			MinConnectTimeout: 5 * time.Second,
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("grpcclient: create auth client %q: %w", addr, err)
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
			return nil, fmt.Errorf("grpcclient: timeout dialing auth %q", addr)
		}
	}

	return &AuthClient{
		raw: authv1.NewAuthServiceClient(cc),
		cc:  cc,
	}, nil
}

// Close closes the underlying gRPC connection.
func (c *AuthClient) Close() error {
	return c.cc.Close()
}

// Register proxies the Register RPC.
func (c *AuthClient) Register(ctx context.Context, req *authv1.RegisterRequest) (*authv1.RegisterResponse, error) {
	resp, err := c.raw.Register(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("grpcclient: register: %w", err)
	}
	return resp, nil
}

// Login proxies the Login RPC.
func (c *AuthClient) Login(ctx context.Context, req *authv1.LoginRequest) (*authv1.LoginResponse, error) {
	resp, err := c.raw.Login(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("grpcclient: login: %w", err)
	}
	return resp, nil
}

// RefreshToken proxies the RefreshToken RPC.
func (c *AuthClient) RefreshToken(ctx context.Context, req *authv1.RefreshTokenRequest) (*authv1.RefreshTokenResponse, error) {
	resp, err := c.raw.RefreshToken(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("grpcclient: refresh: %w", err)
	}
	return resp, nil
}

// Logout proxies the Logout RPC.
func (c *AuthClient) Logout(ctx context.Context, req *authv1.LogoutRequest) error {
	_, err := c.raw.Logout(ctx, req)
	if err != nil {
		return fmt.Errorf("grpcclient: logout: %w", err)
	}
	return nil
}

// GetUser proxies the GetUser RPC.
func (c *AuthClient) GetUser(ctx context.Context, req *authv1.GetUserRequest) (*authv1.GetUserResponse, error) {
	resp, err := c.raw.GetUser(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("grpcclient: get user: %w", err)
	}
	return resp, nil
}

// UpdateProfile proxies the UpdateProfile RPC.
func (c *AuthClient) UpdateProfile(ctx context.Context, req *authv1.UpdateProfileRequest) (*authv1.UpdateProfileResponse, error) {
	resp, err := c.raw.UpdateProfile(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("grpcclient: update profile: %w", err)
	}
	return resp, nil
}

// ChangePassword proxies the ChangePassword RPC.
func (c *AuthClient) ChangePassword(ctx context.Context, req *authv1.ChangePasswordRequest) error {
	_, err := c.raw.ChangePassword(ctx, req)
	if err != nil {
		return fmt.Errorf("grpcclient: change password: %w", err)
	}
	return nil
}

// GetPublicKey proxies the GetPublicKey RPC.
func (c *AuthClient) GetPublicKey(ctx context.Context) (*authv1.GetPublicKeyResponse, error) {
	resp, err := c.raw.GetPublicKey(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, fmt.Errorf("grpcclient: get public key: %w", err)
	}
	return resp, nil
}
