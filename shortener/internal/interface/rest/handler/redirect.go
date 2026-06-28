// Package handler implements the public HTTP redirect handler for short links.
package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	fileuploadv1 "github.com/aligh5331/gobox-proto/gen/fileupload/v1"
	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/aligh5331/gobox/shortener/internal/application/usecase"
	"github.com/aligh5331/gobox/shortener/internal/infrastructure/redis"
)

// RedirectHandler handles public short link redirects.
type RedirectHandler struct {
	getLinkUC      *usecase.GetLinkUseCase
	incrHitCountUC *usecase.IncrementHitCountUseCase
	cache          *redis.Cache
	fileClient     fileuploadv1.FileUploadServiceClient
	log            zerolog.Logger
	cc             *grpc.ClientConn
}

// NewRedirectHandler creates a new RedirectHandler.
func NewRedirectHandler(
	getLinkUC *usecase.GetLinkUseCase,
	incrHitCountUC *usecase.IncrementHitCountUseCase,
	cache *redis.Cache,
	fileUploadGRPCAddr string,
	log zerolog.Logger,
) (*RedirectHandler, error) {
	cc, err := grpc.NewClient(fileUploadGRPCAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("redirect: create fileupload client: %w", err)
	}

	return &RedirectHandler{
		getLinkUC:      getLinkUC,
		incrHitCountUC: incrHitCountUC,
		cache:          cache,
		fileClient:     fileuploadv1.NewFileUploadServiceClient(cc),
		log:            log,
		cc:             cc,
	}, nil
}

// Close closes the gRPC connection.
func (h *RedirectHandler) Close() {
	if h.cc != nil {
		h.cc.Close()
	}
}

// Redirect handles GET /s/:slug
// Flow:
//  1. Check Redis cache for slug→file_id mapping
//  2. On miss, query Postgres via GetLinkUseCase, populate Redis
//  3. Call FileUpload gRPC GetDownloadURL for a fresh presigned URL
//  4. Return 302 with Location set to presigned URL
//  5. Increment hit_count asynchronously (fire-and-forget)
func (h *RedirectHandler) Redirect(c echo.Context) error {
	slug := c.Param("slug")
	if slug == "" {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error": map[string]string{"code": "BAD_REQUEST", "message": "slug is required"},
		})
	}

	// Step 1: Check Redis cache.
	fileID, err := h.cache.Get(c.Request().Context(), slug)
	if err != nil {
		h.log.Warn().Err(err).Str("slug", slug).Msg("redirect: redis get failed, falling through to postgres")
	}

	// Step 2: Cache miss — query Postgres.
	if fileID == "" {
		link, linkErr := h.getLinkUC.Execute(c.Request().Context(), slug)
		if linkErr != nil {
			if errors.Is(linkErr, usecase.ErrLinkNotFound) {
				return c.JSON(http.StatusNotFound, map[string]any{
					"error": map[string]string{"code": "NOT_FOUND", "message": "link not found"},
				})
			}
			if errors.Is(linkErr, usecase.ErrLinkExpired) {
				return c.JSON(http.StatusGone, map[string]any{
					"error": map[string]string{"code": "GONE", "message": "link has expired"},
				})
			}
			h.log.Error().Err(linkErr).Str("slug", slug).Msg("redirect: get link failed")
			return c.JSON(http.StatusInternalServerError, map[string]any{
				"error": map[string]string{"code": "INTERNAL_ERROR", "message": "internal server error"},
			})
		}

		fileID = link.FileID.String()

		// Populate Redis cache (best-effort, ignore errors).
		if cacheErr := h.cache.Set(c.Request().Context(), slug, fileID, 5*time.Minute); cacheErr != nil {
			h.log.Warn().Err(cacheErr).Str("slug", slug).Msg("redirect: failed to set redis cache")
		}
	}

	// Step 3: Call FileUpload gRPC for a fresh presigned URL.
	downloadResp, err := h.fileClient.GetDownloadURL(c.Request().Context(), &fileuploadv1.GetDownloadURLRequest{
		FileId:     fileID,
		TtlSeconds: 3600, // 1 hour
	})
	if err != nil {
		h.log.Error().Err(err).Str("file_id", fileID).Msg("redirect: fileupload get download url failed")
		return c.JSON(http.StatusBadGateway, map[string]any{
			"error": map[string]string{"code": "BAD_GATEWAY", "message": "failed to generate download URL"},
		})
	}

	// Step 5: Increment hit_count asynchronously (fire-and-forget).
	go func() {
		bgCtx := context.Background()
		if incErr := h.incrHitCountUC.Execute(bgCtx, slug); incErr != nil {
			h.log.Error().Err(incErr).Str("slug", slug).Msg("redirect: failed to increment hit count")
		}
	}()

	// Step 4: Return 302 redirect.
	return c.Redirect(http.StatusFound, downloadResp.Url)
}
