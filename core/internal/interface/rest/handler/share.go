// Package handler implements HTTP handlers that proxy to downstream gRPC services.
package handler

import (
	"net/http"

	shortenerv1 "github.com/aligh5331/gobox-proto/gen/shortener/v1"
	"github.com/labstack/echo/v4"

	"github.com/aligh5331/gobox/core/internal/infrastructure/grpcclient/shortener"
	"github.com/aligh5331/gobox/core/internal/interface/rest/middleware"
	"github.com/aligh5331/gobox/core/internal/interface/rest/response"
)

// ShareHandler handles short link (share) operations.
type ShareHandler struct {
	shortener *shortener.Client
}

// NewShareHandler creates a new ShareHandler.
func NewShareHandler(shortener *shortener.Client) *ShareHandler {
	return &ShareHandler{shortener: shortener}
}

// CreateShareRequest is the JSON body for creating a share link.
type CreateShareRequest struct {
	ExpiresAt string `json:"expires_at,omitempty"`
}

// CreateShare handles POST /api/v1/files/:id/share
// Calls Shortener CreateLink gRPC and returns the short URL.
func (h *ShareHandler) CreateShare(c echo.Context) error {
	userID := middleware.GetUserID(c)
	if userID == "" {
		return middleware.NewHTTPError(http.StatusUnauthorized, "UNAUTHORIZED", "invalid token claims")
	}

	fileID := c.Param("id")
	if fileID == "" {
		return middleware.NewHTTPError(http.StatusBadRequest, "BAD_REQUEST", "file id is required")
	}

	var req CreateShareRequest
	if err := c.Bind(&req); err != nil {
		// Ignore body errors — the request is optional.
	}

	resp, err := h.shortener.CreateLink(c.Request().Context(), &shortenerv1.CreateLinkRequest{
		UserId: userID,
		FileId: fileID,
	})
	if err != nil {
		return middleware.MapGRPCError(err)
	}

	return response.ProtoJSON(c, http.StatusCreated, resp)
}

// ListLinks handles GET /api/v1/files/:id/links
// Calls Shortener ListLinks gRPC for the authenticated user.
// Note: The :id path param (file_id) is accepted but the current proto
// does not support file_id filtering. All links for the user are returned.
func (h *ShareHandler) ListLinks(c echo.Context) error {
	userID := middleware.GetUserID(c)
	if userID == "" {
		return middleware.NewHTTPError(http.StatusUnauthorized, "UNAUTHORIZED", "invalid token claims")
	}

	resp, err := h.shortener.ListLinks(c.Request().Context(), &shortenerv1.ListLinksRequest{
		UserId: userID,
	})
	if err != nil {
		return middleware.MapGRPCError(err)
	}

	return response.ProtoJSON(c, http.StatusOK, resp)
}

// DeleteLink handles DELETE /api/v1/links/:link_id
// Calls Shortener DeleteLink gRPC.
func (h *ShareHandler) DeleteLink(c echo.Context) error {
	userID := middleware.GetUserID(c)
	if userID == "" {
		return middleware.NewHTTPError(http.StatusUnauthorized, "UNAUTHORIZED", "invalid token claims")
	}

	linkID := c.Param("link_id")
	if linkID == "" {
		return middleware.NewHTTPError(http.StatusBadRequest, "BAD_REQUEST", "link id is required")
	}

	if _, err := h.shortener.DeleteLink(c.Request().Context(), &shortenerv1.DeleteLinkRequest{
		LinkId: linkID,
		UserId: userID,
	}); err != nil {
		return middleware.MapGRPCError(err)
	}

	return c.NoContent(http.StatusNoContent)
}
