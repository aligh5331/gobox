// Package handler provides REST handlers for the Core API.
package handler

import (
	"net/http"
	"strconv"

	fileuploadv1 "github.com/aligh5331/gobox-proto/gen/fileupload/v1"
	thumbgenv1 "github.com/aligh5331/gobox-proto/gen/thumbgen/v1"
	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog"

	"github.com/aligh5331/gobox/core/internal/infrastructure/grpcclient/fileupload"
	"github.com/aligh5331/gobox/core/internal/infrastructure/grpcclient/thumbgen"
	"github.com/aligh5331/gobox/core/internal/interface/rest/middleware"
	"github.com/aligh5331/gobox/core/internal/interface/rest/response"
)

// FileHandler handles file-related REST endpoints.
type FileHandler struct {
	file    *fileupload.Client
	thumb   *thumbgen.Client
	log     zerolog.Logger
}

// NewFileHandler creates a new FileHandler.
func NewFileHandler(file *fileupload.Client, thumb *thumbgen.Client, log zerolog.Logger) *FileHandler {
	return &FileHandler{file: file, thumb: thumb, log: log}
}

// InitiateUploadRequest is the expected JSON body for POST /api/v1/files.
type InitiateUploadRequest struct {
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	MimeType string `json:"mime_type"`
}

// InitiateUpload begins an upload session and returns a presigned URL.
func (h *FileHandler) InitiateUpload(c echo.Context) error {
	userID := middleware.GetUserID(c)
	if userID == "" {
		return middleware.NewHTTPError(http.StatusUnauthorized, "UNAUTHORIZED", "invalid token claims")
	}

	var req InitiateUploadRequest
	if err := c.Bind(&req); err != nil {
		return middleware.NewHTTPError(http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
	}
	if req.Name == "" || req.Size <= 0 || req.MimeType == "" {
		return middleware.NewHTTPError(http.StatusBadRequest, "BAD_REQUEST", "name, size, and mime_type are required")
	}

	resp, err := h.file.InitiateUpload(c.Request().Context(), &fileuploadv1.InitiateUploadRequest{
		UserId:   userID,
		Name:     req.Name,
		Size:     req.Size,
		MimeType: req.MimeType,
	})
	if err != nil {
		return middleware.MapGRPCError(err)
	}

	return response.ProtoJSON(c, http.StatusCreated, resp)
}

// ConfirmUploadRequest is the expected JSON body for POST /api/v1/files/:id/confirm.
type ConfirmUploadRequest struct {
	StorageKey string `json:"storage_key"`
	Size       int64  `json:"size"`
	MimeType   string `json:"mime_type"`
}

// ConfirmUpload finalises a completed upload and triggers thumbnail generation.
func (h *FileHandler) ConfirmUpload(c echo.Context) error {
	userID := middleware.GetUserID(c)
	if userID == "" {
		return middleware.NewHTTPError(http.StatusUnauthorized, "UNAUTHORIZED", "invalid token claims")
	}

	fileID := c.Param("id")
	if fileID == "" {
		return middleware.NewHTTPError(http.StatusBadRequest, "BAD_REQUEST", "file id is required")
	}

	var req ConfirmUploadRequest
	if err := c.Bind(&req); err != nil {
		return middleware.NewHTTPError(http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
	}
	if req.StorageKey == "" || req.Size <= 0 {
		return middleware.NewHTTPError(http.StatusBadRequest, "BAD_REQUEST", "storage_key and size are required")
	}

	resp, err := h.file.ConfirmUpload(c.Request().Context(), &fileuploadv1.ConfirmUploadRequest{
		FileId:     fileID,
		UserId:     userID,
		StorageKey: req.StorageKey,
		Size:       req.Size,
	})
	if err != nil {
		return middleware.MapGRPCError(err)
	}

	// Fire-and-forget thumbnail generation.
	go func() {
		ctx := c.Request().Context()
		_, tgErr := h.thumb.EnqueueJob(ctx, &thumbgenv1.EnqueueJobRequest{
			FileId:   fileID,
			UserId:   userID,
			InputKey: req.StorageKey,
			MimeType: req.MimeType,
		})
		if tgErr != nil {
			h.log.Warn().Err(tgErr).Str("file_id", fileID).Msg("thumbgen: enqueue job failed (stub)")
		}
	}()

	return response.ProtoJSON(c, http.StatusOK, resp)
}

// ListFiles returns paginated file metadata for the authenticated user.
func (h *FileHandler) ListFiles(c echo.Context) error {
	userID := middleware.GetUserID(c)
	if userID == "" {
		return middleware.NewHTTPError(http.StatusUnauthorized, "UNAUTHORIZED", "invalid token claims")
	}

	pageSize := int32(50)
	if ps := c.QueryParam("page_size"); ps != "" {
		n, err := strconv.ParseInt(ps, 10, 32)
		if err == nil && n > 0 && n <= 200 {
			pageSize = int32(n)
		}
	}
	pageToken := c.QueryParam("page_token")

	resp, err := h.file.ListFiles(c.Request().Context(), &fileuploadv1.ListFilesRequest{
		UserId:    userID,
		PageSize:  pageSize,
		PageToken: pageToken,
	})
	if err != nil {
		return middleware.MapGRPCError(err)
	}

	return response.ProtoJSON(c, http.StatusOK, resp)
}

// GetFile retrieves a single file's metadata.
func (h *FileHandler) GetFile(c echo.Context) error {
	userID := middleware.GetUserID(c)
	if userID == "" {
		return middleware.NewHTTPError(http.StatusUnauthorized, "UNAUTHORIZED", "invalid token claims")
	}

	fileID := c.Param("id")
	if fileID == "" {
		return middleware.NewHTTPError(http.StatusBadRequest, "BAD_REQUEST", "file id is required")
	}

	resp, err := h.file.GetFile(c.Request().Context(), &fileuploadv1.GetFileRequest{
		FileId: fileID,
		UserId: userID,
	})
	if err != nil {
		return middleware.MapGRPCError(err)
	}

	return response.ProtoJSON(c, http.StatusOK, resp)
}

// DeleteFile soft-deletes a file.
func (h *FileHandler) DeleteFile(c echo.Context) error {
	userID := middleware.GetUserID(c)
	if userID == "" {
		return middleware.NewHTTPError(http.StatusUnauthorized, "UNAUTHORIZED", "invalid token claims")
	}

	fileID := c.Param("id")
	if fileID == "" {
		return middleware.NewHTTPError(http.StatusBadRequest, "BAD_REQUEST", "file id is required")
	}

	if err := h.file.DeleteFile(c.Request().Context(), &fileuploadv1.DeleteFileRequest{
		FileId: fileID,
		UserId: userID,
	}); err != nil {
		return middleware.MapGRPCError(err)
	}

	return c.NoContent(http.StatusNoContent)
}

// GetDownloadURL returns a presigned download URL for a file.
func (h *FileHandler) GetDownloadURL(c echo.Context) error {
	fileID := c.Param("id")
	if fileID == "" {
		return middleware.NewHTTPError(http.StatusBadRequest, "BAD_REQUEST", "file id is required")
	}

	ttlSeconds := int32(3600)
	if ttl := c.QueryParam("ttl_seconds"); ttl != "" {
		n, err := strconv.ParseInt(ttl, 10, 32)
		if err == nil && n > 0 {
			ttlSeconds = int32(n)
		}
	}

	resp, err := h.file.GetDownloadURL(c.Request().Context(), &fileuploadv1.GetDownloadURLRequest{
		FileId:     fileID,
		TtlSeconds: ttlSeconds,
	})
	if err != nil {
		return middleware.MapGRPCError(err)
	}

	return response.ProtoJSON(c, http.StatusOK, resp)
}
