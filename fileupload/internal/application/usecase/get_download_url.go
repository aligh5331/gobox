package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/aligh5331/gobox/fileupload/internal/domain/model"
	"github.com/aligh5331/gobox/fileupload/internal/domain/repository"
)

const (
	// MaxDownloadURLTTL is the maximum allowed TTL for a presigned download URL.
	MaxDownloadURLTTL = 3600
)

// DownloadURLOutput contains the presigned download URL and its expiry.
type DownloadURLOutput struct {
	URL       string
	ExpiresAt time.Time
}

// GetDownloadURLUseCase handles generating presigned GET URLs for ready files.
type GetDownloadURLUseCase struct {
	repo        repository.FileRepository
	minioClient MinioClient
}

// NewGetDownloadURLUseCase creates a new GetDownloadURLUseCase.
func NewGetDownloadURLUseCase(repo repository.FileRepository, minioClient MinioClient) *GetDownloadURLUseCase {
	return &GetDownloadURLUseCase{
		repo:        repo,
		minioClient: minioClient,
	}
}

// Execute generates a presigned GET URL for a file, capping ttlSeconds at 3600.
func (uc *GetDownloadURLUseCase) Execute(ctx context.Context, fileID uuid.UUID, ttlSeconds int) (*DownloadURLOutput, error) {
	file, err := uc.repo.FindByID(ctx, fileID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrFileNotFound
		}
		return nil, fmt.Errorf("get download url: find file: %w", err)
	}
	if file == nil {
		return nil, ErrFileNotFound
	}

	if file.Status != model.FileStatusReady {
		return nil, ErrFileNotReady
	}

	if ttlSeconds <= 0 || ttlSeconds > MaxDownloadURLTTL {
		ttlSeconds = MaxDownloadURLTTL
	}

	ttl := time.Duration(ttlSeconds) * time.Second

	url, expiresAt, err := uc.minioClient.PresignedGetURL(ctx, file.StorageKey, ttl)
	if err != nil {
		return nil, fmt.Errorf("get download url: presigned get url: %w", err)
	}

	return &DownloadURLOutput{
		URL:       url,
		ExpiresAt: expiresAt,
	}, nil
}
