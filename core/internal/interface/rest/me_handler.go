package rest

import (
	"net/http"

	authv1 "github.com/aligh5331/gobox-proto/gen/auth/v1"
	"github.com/labstack/echo/v4"

	"github.com/aligh5331/gobox/core/internal/infrastructure/grpcclient"
	"github.com/aligh5331/gobox/core/internal/interface/rest/middleware"
	"github.com/aligh5331/gobox/core/internal/interface/rest/response"
)

// MeHandler handles user profile REST endpoints.
type MeHandler struct {
	auth *grpcclient.AuthClient
}

// NewMeHandler creates a new MeHandler.
func NewMeHandler(auth *grpcclient.AuthClient) *MeHandler {
	return &MeHandler{auth: auth}
}

// GetMe returns the authenticated user's profile.
func (h *MeHandler) GetMe(c echo.Context) error {
	userID := middleware.GetUserID(c)
	if userID == "" {
		return middleware.NewHTTPError(http.StatusUnauthorized, "UNAUTHORIZED", "invalid token claims")
	}

	resp, err := h.auth.GetUser(c.Request().Context(), &authv1.GetUserRequest{
		UserId: userID,
	})
	if err != nil {
		return middleware.MapGRPCError(err)
	}

	return response.ProtoJSON(c, http.StatusOK, resp)
}

// UpdateMeRequest is the expected JSON body for PUT /api/v1/me.
type UpdateMeRequest struct {
	Name string `json:"name"`
}

// UpdateMe updates the authenticated user's profile.
func (h *MeHandler) UpdateMe(c echo.Context) error {
	userID := middleware.GetUserID(c)
	if userID == "" {
		return middleware.NewHTTPError(http.StatusUnauthorized, "UNAUTHORIZED", "invalid token claims")
	}

	var req UpdateMeRequest
	if err := c.Bind(&req); err != nil {
		return middleware.NewHTTPError(http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
	}
	if req.Name == "" {
		return middleware.NewHTTPError(http.StatusBadRequest, "BAD_REQUEST", "name is required and must not be empty")
	}

	resp, err := h.auth.UpdateProfile(c.Request().Context(), &authv1.UpdateProfileRequest{
		UserId: userID,
		Name:   req.Name,
	})
	if err != nil {
		return middleware.MapGRPCError(err)
	}

	return response.ProtoJSON(c, http.StatusOK, resp)
}

// ChangePasswordRequest is the expected JSON body for PUT /api/v1/me/password.
type ChangePasswordRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

// ChangePassword changes the authenticated user's password.
func (h *MeHandler) ChangePassword(c echo.Context) error {
	userID := middleware.GetUserID(c)
	if userID == "" {
		return middleware.NewHTTPError(http.StatusUnauthorized, "UNAUTHORIZED", "invalid token claims")
	}

	var req ChangePasswordRequest
	if err := c.Bind(&req); err != nil {
		return middleware.NewHTTPError(http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
	}
	if req.OldPassword == "" || req.NewPassword == "" {
		return middleware.NewHTTPError(http.StatusBadRequest, "BAD_REQUEST", "old_password and new_password are required")
	}

	if err := h.auth.ChangePassword(c.Request().Context(), &authv1.ChangePasswordRequest{
		UserId:      userID,
		OldPassword: req.OldPassword,
		NewPassword: req.NewPassword,
	}); err != nil {
		return middleware.MapGRPCError(err)
	}

	return c.NoContent(http.StatusNoContent)
}
