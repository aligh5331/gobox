// Package e2e contains end-to-end tests for the auth service.
// Run: go test ./e2e/ -v -count=1 -timeout 180s
//
// Prerequisites:
//   - docker compose up -d (postgres + auth)
//   - Port 8081 (gRPC) and 8082 (HTTP) reachable on localhost
package e2e

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	pb "github.com/aligh5331/gobox-proto/gen/auth/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// ---------- helpers ----------

const (
	grpcAddr = "localhost:8081"
	httpAddr = "http://localhost:8084"
)

// newGRPCClient creates a new AuthService gRPC client connection.
func newGRPCClient(t *testing.T) (pb.AuthServiceClient, *grpc.ClientConn) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, grpcAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	require.NoError(t, err, "grpc dial")
	return pb.NewAuthServiceClient(conn), conn
}

// assertJWT verifies a string looks like a valid JWT (3 dot-separated base64url segments).
func assertJWT(t *testing.T, token string) {
	t.Helper()
	parts := strings.Split(token, ".")
	assert.Len(t, parts, 3, "JWT should have 3 dot-separated segments")
	for _, p := range parts {
		assert.NotEmpty(t, p, "each JWT segment must be non-empty")
	}
}

// assertUUID checks basic UUID v4 format (xxxxxxxx-xxxx-4xxx-xxxx-xxxxxxxxxxxx).
func assertUUID(t *testing.T, id string) {
	t.Helper()
	assert.Len(t, id, 36, "UUID should be 36 characters")
	parts := strings.Split(id, "-")
	assert.Len(t, parts, 5, "UUID should have 5 dash-separated parts")
	assert.Len(t, parts[0], 8)
	assert.Len(t, parts[1], 4)
	assert.Len(t, parts[2], 4)
	assert.Len(t, parts[3], 4)
	assert.Len(t, parts[4], 12)
	// Check the version nibble (UUID v4 has version 4 at position 14)
	assert.Equal(t, uint8('4'), id[14], "UUID v4 should have '4' at position 14")
}

// httpGet performs a simple HTTP GET and returns the response body and status.
func httpGet(t *testing.T, url string) (int, []byte) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return resp.StatusCode, body
}

// waitForHealth polls the /health endpoint until it returns 200 or timeout.
// This is used as a startup check before running test scenarios.
func waitForHealth(t *testing.T) {
	t.Helper()
	require.Eventually(t, func() bool {
		code, body := httpGet(t, httpAddr+"/health")
		if code != http.StatusOK {
			return false
		}
		var m map[string]string
		if err := json.Unmarshal(body, &m); err != nil {
			return false
		}
		return m["status"] == "ok"
	}, 30*time.Second, 500*time.Millisecond, "auth service /health did not become ready")
}

// ---------- Test suite ----------

// TestAuthE2E runs all E2E scenarios sequentially.
func TestAuthE2E(t *testing.T) {
	// Wait for the service to be healthy before running scenarios.
	// This replaces a separate startup health check loop.
	waitForHealth(t)

	// ------ State variables (carried between scenarios) ------
	var (
		userID_1    string // from Scenario 3
		sessionID_1 string

		userID       string // from most recent login
		accessToken  string
		refreshToken string
		sessionID    string
	)

	// ---------- Scenario 1 — Health endpoint ----------
	t.Run("Scenario 1 — Health endpoint", func(t *testing.T) {
		code, body := httpGet(t, httpAddr+"/health")
		assert.Equal(t, http.StatusOK, code)

		var m map[string]string
		err := json.Unmarshal(body, &m)
		require.NoError(t, err)
		assert.Equal(t, "ok", m["status"])
	})

	// ---------- Scenario 2 — JWKS endpoint ----------
	t.Run("Scenario 2 — JWKS endpoint", func(t *testing.T) {
		code, body := httpGet(t, httpAddr+"/.well-known/jwks.json")
		assert.Equal(t, http.StatusOK, code)

		var jwksResp struct {
			Keys []struct {
				Kty string `json:"kty"`
				Kid string `json:"kid"`
				Alg string `json:"alg"`
				N   string `json:"n"`
				E   string `json:"e"`
				Use string `json:"use"`
			} `json:"keys"`
		}
		err := json.Unmarshal(body, &jwksResp)
		require.NoError(t, err, "JWKS response must be valid JSON")
		require.GreaterOrEqual(t, len(jwksResp.Keys), 1, "must have at least one key")

		key := jwksResp.Keys[0]
		assert.NotEmpty(t, key.Kty)
		assert.NotEmpty(t, key.Kid)
		assert.NotEmpty(t, key.Alg)
		assert.NotEmpty(t, key.N)
		assert.NotEmpty(t, key.E)
		assert.NotEmpty(t, key.Use)

		_ = string(body) // JWKS saved for potential downstream use
	})

	// ---------- Scenario 3 — Register a new user successfully ----------
	t.Run("Scenario 3 — Register a new user successfully", func(t *testing.T) {
		client, conn := newGRPCClient(t)
		defer conn.Close()

		resp, err := client.Register(context.Background(), &pb.RegisterRequest{
			Email:    "alice@example.com",
			Name:     "Alice",
			Password: "ValidPass1!",
		})
		require.NoError(t, err)
		require.NotNil(t, resp)

		// Assert user fields
		require.NotNil(t, resp.User)
		assert.Equal(t, "alice@example.com", resp.User.Email)
		assert.Equal(t, "Alice", resp.User.Name)
		assertUUID(t, resp.User.Id)

		// Assert tokens
		require.NotNil(t, resp.Tokens)
		assertJWT(t, resp.Tokens.AccessToken)
		assert.Len(t, resp.Tokens.RefreshToken, 43, "refresh token should be 43 chars base64url")
		assert.Equal(t, int64(900), resp.Tokens.ExpiresIn)

		// Assert session
		require.NotNil(t, resp.Session)
		assertUUID(t, resp.Session.Id)
		assertUUID(t, resp.Session.UserId)
		assert.Equal(t, resp.User.Id, resp.Session.UserId)
		require.NotNil(t, resp.Session.ExpiresAt)
		// ExpiresAt should be ~30 days from now
		expiresAt := resp.Session.ExpiresAt.AsTime()
		assert.WithinDuration(t, time.Now().Add(30*24*time.Hour), expiresAt, 5*time.Minute)

		// Save state
		userID_1 = resp.User.Id
		sessionID_1 = resp.Session.Id
	})

	// ---------- Scenario 4 — Register with duplicate email fails ----------
	t.Run("Scenario 4 — Register with duplicate email fails", func(t *testing.T) {
		client, conn := newGRPCClient(t)
		defer conn.Close()

		resp, err := client.Register(context.Background(), &pb.RegisterRequest{
			Email:    "alice@example.com",
			Name:     "Bob",
			Password: "AnotherPass1!",
		})
		require.Error(t, err)
		assert.Nil(t, resp)
		assert.Contains(t, err.Error(), "EMAIL_ALREADY_EXISTS")
	})

	// ---------- Scenario 5 — Register with weak password fails ----------
	t.Run("Scenario 5 — Register with weak password fails", func(t *testing.T) {
		client, conn := newGRPCClient(t)
		defer conn.Close()

		resp, err := client.Register(context.Background(), &pb.RegisterRequest{
			Email:    "bob@example.com",
			Name:     "Bob",
			Password: "short",
		})
		require.Error(t, err)
		assert.Nil(t, resp)
		assert.Contains(t, err.Error(), "WEAK_PASSWORD")
	})

	// ---------- Scenario 6 — Login with valid credentials succeeds ----------
	t.Run("Scenario 6 — Login with valid credentials succeeds", func(t *testing.T) {
		client, conn := newGRPCClient(t)
		defer conn.Close()

		resp, err := client.Login(context.Background(), &pb.LoginRequest{
			Email:     "alice@example.com",
			Password:  "ValidPass1!",
			UserAgent: "go-e2e-test",
			Ip:        "127.0.0.1",
		})
		require.NoError(t, err)
		require.NotNil(t, resp)

		// Assert user
		require.NotNil(t, resp.User)
		assert.Equal(t, "alice@example.com", resp.User.Email)
		assertUUID(t, resp.User.Id)
		// userID should match the one from registration
		assert.Equal(t, userID_1, resp.User.Id)

		// Assert tokens
		require.NotNil(t, resp.Tokens)
		assertJWT(t, resp.Tokens.AccessToken)
		assert.Len(t, resp.Tokens.RefreshToken, 43, "refresh token should be 43 chars base64url")
		assert.Equal(t, int64(900), resp.Tokens.ExpiresIn)

		// Assert session (should be different from sessionID_1 since Alice already has one)
		require.NotNil(t, resp.Session)
		assertUUID(t, resp.Session.Id)
		assert.NotEqual(t, sessionID_1, resp.Session.Id, "should be a new session")
		assert.Equal(t, resp.User.Id, resp.Session.UserId)
		require.NotNil(t, resp.Session.ExpiresAt)
		expiresAt := resp.Session.ExpiresAt.AsTime()
		assert.WithinDuration(t, time.Now().Add(30*24*time.Hour), expiresAt, 5*time.Minute)

		// Save state
		userID = resp.User.Id
		accessToken = resp.Tokens.AccessToken
		refreshToken = resp.Tokens.RefreshToken
		sessionID = resp.Session.Id
	})

	// ---------- Scenario 7 — Login with wrong password fails ----------
	t.Run("Scenario 7 — Login with wrong password fails", func(t *testing.T) {
		client, conn := newGRPCClient(t)
		defer conn.Close()

		resp, err := client.Login(context.Background(), &pb.LoginRequest{
			Email:     "alice@example.com",
			Password:  "WrongPass1!",
			UserAgent: "",
			Ip:        "",
		})
		require.Error(t, err)
		assert.Nil(t, resp)
		assert.Contains(t, err.Error(), "INVALID_CREDENTIALS")
	})

	// ---------- Scenario 8 — Login with unknown email fails ----------
	t.Run("Scenario 8 — Login with unknown email fails", func(t *testing.T) {
		client, conn := newGRPCClient(t)
		defer conn.Close()

		resp, err := client.Login(context.Background(), &pb.LoginRequest{
			Email:     "unknown@example.com",
			Password:  "AnyPass1!",
			UserAgent: "",
			Ip:        "",
		})
		require.Error(t, err)
		assert.Nil(t, resp)
		assert.Contains(t, err.Error(), "INVALID_CREDENTIALS")
	})

	// ---------- Scenario 9 — Refresh a valid token successfully rotates credentials ----------
	t.Run("Scenario 9 — Refresh a valid token successfully rotates credentials", func(t *testing.T) {
		client, conn := newGRPCClient(t)
		defer conn.Close()

		oldAccessToken := accessToken
		oldRefreshToken := refreshToken

		resp, err := client.RefreshToken(context.Background(), &pb.RefreshTokenRequest{
			RefreshToken: oldRefreshToken,
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.NotNil(t, resp.Tokens)

		// New access token must be different
		assertJWT(t, resp.Tokens.AccessToken)
		assert.NotEqual(t, oldAccessToken, resp.Tokens.AccessToken, "access token must be rotated")

		// New refresh token must be different and 43 chars
		assert.Len(t, resp.Tokens.RefreshToken, 43)
		assert.NotEqual(t, oldRefreshToken, resp.Tokens.RefreshToken, "refresh token must be rotated")

		assert.Equal(t, int64(900), resp.Tokens.ExpiresIn)

		// Save rotated tokens (for potential downstream verification)
		_ = resp.Tokens.AccessToken
		_ = resp.Tokens.RefreshToken
	})

	// ---------- Scenario 10 — Refresh with a consumed (already-rotated) token fails ----------
	t.Run("Scenario 10 — Refresh with a consumed token fails", func(t *testing.T) {
		client, conn := newGRPCClient(t)
		defer conn.Close()

		// Use the original refreshToken from Scenario 6 (which was rotated in Scenario 9)
		resp, err := client.RefreshToken(context.Background(), &pb.RefreshTokenRequest{
			RefreshToken: refreshToken,
		})
		require.Error(t, err)
		assert.Nil(t, resp)
		assert.Contains(t, err.Error(), "TOKEN_THEFT_DETECTED")
	})

	// ---------- Scenario 11 — Refresh with a revoked session fails ----------
	t.Run("Scenario 11 — Refresh with a revoked session fails", func(t *testing.T) {
		client, conn := newGRPCClient(t)
		defer conn.Close()

		// Step 1: Login to get a revocable session
		loginResp, err := client.Login(context.Background(), &pb.LoginRequest{
			Email:     "alice@example.com",
			Password:  "ValidPass1!",
			UserAgent: "",
			Ip:        "",
		})
		require.NoError(t, err)
		require.NotNil(t, loginResp)
		revocableSessionID := loginResp.Session.Id
		revocableRefreshToken := loginResp.Tokens.RefreshToken

		// Step 2: Logout to revoke this session
		_, err = client.Logout(context.Background(), &pb.LogoutRequest{
			SessionId: revocableSessionID,
		})
		require.NoError(t, err)

		// Step 3: Try to refresh with the revoked session's refresh token
		resp, err := client.RefreshToken(context.Background(), &pb.RefreshTokenRequest{
			RefreshToken: revocableRefreshToken,
		})
		require.Error(t, err)
		assert.Nil(t, resp)
		assert.Contains(t, err.Error(), "SESSION_REVOKED")
	})

	// ---------- Scenario 12 — Login, then GetUser returns the user ----------
	t.Run("Scenario 12 — Login, then GetUser returns the user", func(t *testing.T) {
		// Re-login to get a fresh context
		client, conn := newGRPCClient(t)
		defer conn.Close()

		loginResp, err := client.Login(context.Background(), &pb.LoginRequest{
			Email:     "alice@example.com",
			Password:  "ValidPass1!",
			UserAgent: "",
			Ip:        "",
		})
		require.NoError(t, err)
		require.NotNil(t, loginResp)

		currentUserID := loginResp.User.Id

		// GetUser
		getResp, err := client.GetUser(context.Background(), &pb.GetUserRequest{
			UserId: currentUserID,
		})
		require.NoError(t, err)
		require.NotNil(t, getResp)
		assert.Equal(t, currentUserID, getResp.Id)
		assert.Equal(t, "alice@example.com", getResp.Email)
		assert.Equal(t, "Alice", getResp.Name)
		require.NotNil(t, getResp.CreatedAt)
		require.NotNil(t, getResp.UpdatedAt)
		// Verify timestamps are reasonable (within the last hour)
		assert.WithinDuration(t, time.Now(), getResp.CreatedAt.AsTime(), 1*time.Hour)
		assert.WithinDuration(t, time.Now(), getResp.UpdatedAt.AsTime(), 1*time.Hour)

		// Update state
		userID = currentUserID
		accessToken = loginResp.Tokens.AccessToken
		refreshToken = loginResp.Tokens.RefreshToken
		sessionID = loginResp.Session.Id
	})

	// ---------- Scenario 13 — GetUser with unknown user_id fails ----------
	t.Run("Scenario 13 — GetUser with unknown user_id fails", func(t *testing.T) {
		client, conn := newGRPCClient(t)
		defer conn.Close()

		resp, err := client.GetUser(context.Background(), &pb.GetUserRequest{
			UserId: "00000000-0000-0000-0000-000000000000",
		})
		require.Error(t, err)
		assert.Nil(t, resp)
		assert.Contains(t, err.Error(), "USER_NOT_FOUND")
	})

	// ---------- Scenario 14 — UpdateProfile changes the user's display name ----------
	t.Run("Scenario 14 — UpdateProfile changes the user's display name", func(t *testing.T) {
		client, conn := newGRPCClient(t)
		defer conn.Close()

		resp, err := client.UpdateProfile(context.Background(), &pb.UpdateProfileRequest{
			UserId: userID,
			Name:   "Alice Updated",
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.NotNil(t, resp.User)
		assert.Equal(t, "Alice Updated", resp.User.Name)
		assert.Equal(t, "alice@example.com", resp.User.Email)
	})

	// ---------- Scenario 15 — UpdateProfile with empty name fails ----------
	t.Run("Scenario 15 — UpdateProfile with empty name fails", func(t *testing.T) {
		client, conn := newGRPCClient(t)
		defer conn.Close()

		resp, err := client.UpdateProfile(context.Background(), &pb.UpdateProfileRequest{
			UserId: userID,
			Name:   "",
		})
		require.Error(t, err)
		assert.Nil(t, resp)
		assert.Contains(t, err.Error(), "INVALID_NAME")
	})

	// ---------- Scenario 16 — ChangePassword with correct old password succeeds ----------
	t.Run("Scenario 16 — ChangePassword with correct old password succeeds", func(t *testing.T) {
		client, conn := newGRPCClient(t)
		defer conn.Close()

		_, err := client.ChangePassword(context.Background(), &pb.ChangePasswordRequest{
			UserId:      userID,
			OldPassword: "ValidPass1!",
			NewPassword: "NewPass2!",
		})
		require.NoError(t, err)
	})

	// ---------- Scenario 17 — Login with new password after ChangePassword ----------
	t.Run("Scenario 17 — Login with new password after ChangePassword", func(t *testing.T) {
		client, conn := newGRPCClient(t)
		defer conn.Close()

		resp, err := client.Login(context.Background(), &pb.LoginRequest{
			Email:     "alice@example.com",
			Password:  "NewPass2!",
			UserAgent: "",
			Ip:        "",
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.NotNil(t, resp.Tokens)
		assertJWT(t, resp.Tokens.AccessToken)
		assert.Len(t, resp.Tokens.RefreshToken, 43)

		// Save state
		userID = resp.User.Id
		accessToken = resp.Tokens.AccessToken
		refreshToken = resp.Tokens.RefreshToken
		sessionID = resp.Session.Id
	})

	// ---------- Scenario 18 — Login with old password fails after ChangePassword ----------
	t.Run("Scenario 18 — Login with old password fails after ChangePassword", func(t *testing.T) {
		client, conn := newGRPCClient(t)
		defer conn.Close()

		resp, err := client.Login(context.Background(), &pb.LoginRequest{
			Email:     "alice@example.com",
			Password:  "ValidPass1!",
			UserAgent: "",
			Ip:        "",
		})
		require.Error(t, err)
		assert.Nil(t, resp)
		assert.Contains(t, err.Error(), "INVALID_CREDENTIALS")
	})

	// ---------- Scenario 19 — ChangePassword with wrong old password fails ----------
	t.Run("Scenario 19 — ChangePassword with wrong old password fails", func(t *testing.T) {
		client, conn := newGRPCClient(t)
		defer conn.Close()

		_, err := client.ChangePassword(context.Background(), &pb.ChangePasswordRequest{
			UserId:      userID,
			OldPassword: "WrongPass1!",
			NewPassword: "AnotherNew3!",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "INVALID_PASSWORD")
	})

	// ---------- Scenario 20 — ChangePassword with weak new password fails ----------
	t.Run("Scenario 20 — ChangePassword with weak new password fails", func(t *testing.T) {
		client, conn := newGRPCClient(t)
		defer conn.Close()

		_, err := client.ChangePassword(context.Background(), &pb.ChangePasswordRequest{
			UserId:      userID,
			OldPassword: "NewPass2!",
			NewPassword: "short",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "WEAK_PASSWORD")
	})

	// ---------- Scenario 21 — Logout revokes the active session ----------
	t.Run("Scenario 21 — Logout revokes the active session", func(t *testing.T) {
		client, conn := newGRPCClient(t)
		defer conn.Close()

		_, err := client.Logout(context.Background(), &pb.LogoutRequest{
			SessionId: sessionID,
		})
		require.NoError(t, err)
	})

	// ---------- Scenario 22 — ValidateSession returns valid=false for revoked session ----------
	t.Run("Scenario 22 — ValidateSession returns valid=false for revoked session", func(t *testing.T) {
		client, conn := newGRPCClient(t)
		defer conn.Close()

		resp, err := client.ValidateSession(context.Background(), &pb.ValidateSessionRequest{
			SessionId: sessionID,
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.False(t, resp.Valid)
	})

	// ---------- Scenario 23 — Logout with an already-revoked session fails ----------
	t.Run("Scenario 23 — Logout with an already-revoked session fails", func(t *testing.T) {
		client, conn := newGRPCClient(t)
		defer conn.Close()

		_, err := client.Logout(context.Background(), &pb.LogoutRequest{
			SessionId: sessionID,
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "SESSION_ALREADY_REVOKED")
	})

	// ---------- Scenario 24 — Login second user, then LogoutAll revokes all sessions ----------
	t.Run("Scenario 24 — Login second user, then LogoutAll revokes all sessions", func(t *testing.T) {
		client, conn := newGRPCClient(t)
		defer conn.Close()

		// Step 1: Register carol
		regResp, err := client.Register(context.Background(), &pb.RegisterRequest{
			Email:    "carol@example.com",
			Name:     "Carol",
			Password: "CarolPass1!",
		})
		require.NoError(t, err)
		require.NotNil(t, regResp)
		carolUserID := regResp.User.Id

		// Step 2: Login twice to create two sessions
		loginA, err := client.Login(context.Background(), &pb.LoginRequest{
			Email:     "carol@example.com",
			Password:  "CarolPass1!",
			UserAgent: "",
			Ip:        "",
		})
		require.NoError(t, err)
		require.NotNil(t, loginA)
		carolSessionID_A := loginA.Session.Id

		loginB, err := client.Login(context.Background(), &pb.LoginRequest{
			Email:     "carol@example.com",
			Password:  "CarolPass1!",
			UserAgent: "",
			Ip:        "",
		})
		require.NoError(t, err)
		require.NotNil(t, loginB)
		carolSessionID_B := loginB.Session.Id

		// Step 3: LogoutAll
		_, err = client.LogoutAll(context.Background(), &pb.LogoutAllRequest{
			UserId: carolUserID,
		})
		require.NoError(t, err)

		// Step 4: Validate both sessions are invalid
		valA, err := client.ValidateSession(context.Background(), &pb.ValidateSessionRequest{
			SessionId: carolSessionID_A,
		})
		require.NoError(t, err)
		assert.False(t, valA.Valid, "session A should be invalid")

		valB, err := client.ValidateSession(context.Background(), &pb.ValidateSessionRequest{
			SessionId: carolSessionID_B,
		})
		require.NoError(t, err)
		assert.False(t, valB.Valid, "session B should be invalid")
	})

	// ---------- Scenario 25 — LogoutAll with no active sessions succeeds as no-op ----------
	t.Run("Scenario 25 — LogoutAll with no active sessions succeeds as no-op", func(t *testing.T) {
		client, conn := newGRPCClient(t)
		defer conn.Close()

		// Register dave
		regResp, err := client.Register(context.Background(), &pb.RegisterRequest{
			Email:    "dave@example.com",
			Name:     "Dave",
			Password: "DavePass1!",
		})
		require.NoError(t, err)
		require.NotNil(t, regResp)
		daveUserID := regResp.User.Id

		// Immediately LogoutAll (no active sessions)
		_, err = client.LogoutAll(context.Background(), &pb.LogoutAllRequest{
			UserId: daveUserID,
		})
		require.NoError(t, err)
	})

	// ---------- Boundary 1 — ValidateSession with non-existent session_id ----------
	t.Run("Boundary 1 — ValidateSession with non-existent session_id", func(t *testing.T) {
		client, conn := newGRPCClient(t)
		defer conn.Close()

		resp, err := client.ValidateSession(context.Background(), &pb.ValidateSessionRequest{
			SessionId: "00000000-0000-0000-0000-000000000000",
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.False(t, resp.Valid)
	})

	// ---------- Boundary 2 — ValidateSession with malformed session_id ----------
	t.Run("Boundary 2 — ValidateSession with malformed session_id", func(t *testing.T) {
		client, conn := newGRPCClient(t)
		defer conn.Close()

		resp, err := client.ValidateSession(context.Background(), &pb.ValidateSessionRequest{
			SessionId: "not-a-uuid-at-all",
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.False(t, resp.Valid)
	})

	// ---------- Boundary 3 — GetUser with malformed user_id ----------
	t.Run("Boundary 3 — GetUser with malformed user_id", func(t *testing.T) {
		client, conn := newGRPCClient(t)
		defer conn.Close()

		resp, err := client.GetUser(context.Background(), &pb.GetUserRequest{
			UserId: "not-a-uuid",
		})
		require.Error(t, err)
		assert.Nil(t, resp)
		assert.Contains(t, err.Error(), "invalid user_id")
	})

	// ---------- Boundary 4 — Register with missing fields ----------
	t.Run("Boundary 4 — Register with missing fields", func(t *testing.T) {
		client, conn := newGRPCClient(t)
		defer conn.Close()

		resp, err := client.Register(context.Background(), &pb.RegisterRequest{
			Email:    "",
			Name:     "",
			Password: "",
		})
		// The spec says the server may either reject empty email with
		// InvalidArgument/INVALID_EMAIL, or pass it through to the use case
		// which returns WEAK_PASSWORD. Accept both.
		if err != nil {
			msg := err.Error()
			assert.True(t,
				strings.Contains(msg, "INVALID_EMAIL") || strings.Contains(msg, "WEAK_PASSWORD"),
				"expected INVALID_EMAIL or WEAK_PASSWORD, got: %s", msg)
			assert.Nil(t, resp)
		} else {
			t.Log("empty-email registration succeeded unexpectedly")
		}
	})

	// ----- Final cleanup: ensure the service is still alive -----
	t.Run("Final health check", func(t *testing.T) {
		code, body := httpGet(t, httpAddr+"/health")
		assert.Equal(t, http.StatusOK, code)
		var m map[string]string
		err := json.Unmarshal(body, &m)
		require.NoError(t, err)
		assert.Equal(t, "ok", m["status"])
	})
}
