package rest

import (
	"net/http"

	authv1 "github.com/aligh5331/gobox-proto/gen/auth/v1"
	"github.com/labstack/echo/v4"

	"github.com/aligh5331/gobox/core/internal/infrastructure/grpcclient"
	"github.com/aligh5331/gobox/core/internal/interface/rest/middleware"
	"github.com/aligh5331/gobox/core/internal/interface/rest/response"
)

// AuthHandler handles authentication-related REST endpoints.
type AuthHandler struct {
	auth *grpcclient.AuthClient
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(auth *grpcclient.AuthClient) *AuthHandler {
	return &AuthHandler{auth: auth}
}

// RegisterRequest is the expected JSON body for POST /api/v1/auth/register.
type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

// Register creates a new user account.
func (h *AuthHandler) Register(c echo.Context) error {
	var req RegisterRequest
	if err := c.Bind(&req); err != nil {
		return middleware.NewHTTPError(http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
	}
	if req.Email == "" || req.Password == "" {
		return middleware.NewHTTPError(http.StatusBadRequest, "BAD_REQUEST", "email and password are required")
	}

	resp, err := h.auth.Register(c.Request().Context(), &authv1.RegisterRequest{
		Email:    req.Email,
		Password: req.Password,
		Name:     req.Name,
	})
	if err != nil {
		return middleware.MapGRPCError(err)
	}

	return response.ProtoJSON(c, http.StatusCreated, resp)
}

// LoginRequest is the expected JSON body for POST /api/v1/auth/login.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// Login authenticates a user and returns tokens.
func (h *AuthHandler) Login(c echo.Context) error {
	var req LoginRequest
	if err := c.Bind(&req); err != nil {
		return middleware.NewHTTPError(http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
	}
	if req.Email == "" || req.Password == "" {
		return middleware.NewHTTPError(http.StatusBadRequest, "BAD_REQUEST", "email and password are required")
	}

	userAgent := c.Request().UserAgent()
	ip := c.RealIP()

	resp, err := h.auth.Login(c.Request().Context(), &authv1.LoginRequest{
		Email:     req.Email,
		Password:  req.Password,
		UserAgent: userAgent,
		Ip:        ip,
	})
	if err != nil {
		return middleware.MapGRPCError(err)
	}

	return response.ProtoJSON(c, http.StatusOK, resp)
}

// RefreshRequest is the expected JSON body for POST /api/v1/auth/refresh.
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// Refresh rotates the token pair.
func (h *AuthHandler) Refresh(c echo.Context) error {
	var req RefreshRequest
	if err := c.Bind(&req); err != nil {
		return middleware.NewHTTPError(http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
	}
	if req.RefreshToken == "" {
		return middleware.NewHTTPError(http.StatusBadRequest, "BAD_REQUEST", "refresh_token is required")
	}

	resp, err := h.auth.RefreshToken(c.Request().Context(), &authv1.RefreshTokenRequest{
		RefreshToken: req.RefreshToken,
	})
	if err != nil {
		return middleware.MapGRPCError(err)
	}

	return response.ProtoJSON(c, http.StatusOK, resp)
}

// Logout revokes the session identified by the JWT's sid claim.
// The session_id is extracted from the validated JWT, not from the request body,
// to prevent cross-session revocation.
func (h *AuthHandler) Logout(c echo.Context) error {
	sessionID := middleware.GetSessionID(c)
	if sessionID == "" {
		return middleware.NewHTTPError(http.StatusUnauthorized, "UNAUTHORIZED", "invalid token claims")
	}

	if err := h.auth.Logout(c.Request().Context(), &authv1.LogoutRequest{
		SessionId: sessionID,
	}); err != nil {
		return middleware.MapGRPCError(err)
	}

	return c.NoContent(http.StatusNoContent)
}
