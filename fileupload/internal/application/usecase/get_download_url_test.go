package usecase

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/aligh5331/gobox/fileupload/internal/domain/model"
	"github.com/aligh5331/gobox/fileupload/internal/domain/repository"
)

func TestGetDownloadURL_Success(t *testing.T) {
	repo := new(MockFileRepo)
	minio := new(MockMinioClient)
	uc := NewGetDownloadURLUseCase(repo, minio)

	fileID := uuid.MustParse("a1b2c3d4-e5f6-7890-abcd-ef1234567890")
	ttlSeconds := 600
	now := time.Now()
	expiresAt := now.Add(time.Duration(ttlSeconds) * time.Second)

	existingFile := &model.File{
		ID:         fileID,
		UserID:     uuid.MustParse("f47ac10b-58cc-4372-a567-0e02b2c3d479"),
		Name:       "download_me.pdf",
		Size:       32768,
		MimeType:   "application/pdf",
		StorageKey: "uploads/a1b2c3d4_download_me.pdf",
		Status:     model.FileStatusReady,
	}

	repo.On("FindByID", mock.Anything, fileID).Return(existingFile, nil)
	minio.On("PresignedGetURL", mock.Anything, existingFile.StorageKey, time.Duration(ttlSeconds)*time.Second).
		Return("https://minio.example.com/download", expiresAt, nil)

	output, err := uc.Execute(context.Background(), fileID, ttlSeconds)
	assert.NoError(t, err)
	assert.NotNil(t, output)
	assert.Equal(t, "https://minio.example.com/download", output.URL)
	assert.Equal(t, expiresAt, output.ExpiresAt)

	repo.AssertExpectations(t)
	minio.AssertExpectations(t)
}

func TestGetDownloadURL_TTLCapped(t *testing.T) {
	repo := new(MockFileRepo)
	minio := new(MockMinioClient)
	uc := NewGetDownloadURLUseCase(repo, minio)

	fileID := uuid.MustParse("a1b2c3d4-e5f6-7890-abcd-ef1234567890")
	ttlSeconds := 7200 // exceeds cap
	now := time.Now()
	expiresAt := now.Add(time.Duration(MaxDownloadURLTTL) * time.Second)

	existingFile := &model.File{
		ID:         fileID,
		UserID:     uuid.MustParse("f47ac10b-58cc-4372-a567-0e02b2c3d479"),
		Name:       "large_video.mov",
		Size:       104857600,
		MimeType:   "video/quicktime",
		StorageKey: "uploads/a1b2c3d4_large_video.mov",
		Status:     model.FileStatusReady,
	}

	repo.On("FindByID", mock.Anything, fileID).Return(existingFile, nil)
	minio.On("PresignedGetURL", mock.Anything, existingFile.StorageKey, time.Duration(MaxDownloadURLTTL)*time.Second).
		Return("https://minio.example.com/download", expiresAt, nil)

	output, err := uc.Execute(context.Background(), fileID, ttlSeconds)
	assert.NoError(t, err)
	assert.Equal(t, expiresAt, output.ExpiresAt)

	repo.AssertExpectations(t)
	minio.AssertExpectations(t)
}

func TestGetDownloadURL_FileNotFound(t *testing.T) {
	repo := new(MockFileRepo)
	minio := new(MockMinioClient)
	uc := NewGetDownloadURLUseCase(repo, minio)

	fileID := uuid.MustParse("00000000-0000-0000-0000-000000000000")

	repo.On("FindByID", mock.Anything, fileID).Return(nil, repository.ErrNotFound)

	output, err := uc.Execute(context.Background(), fileID, 3600)
	assert.ErrorIs(t, err, ErrFileNotFound)
	assert.Nil(t, output)

	repo.AssertExpectations(t)
}

func TestGetDownloadURL_FileNotReady(t *testing.T) {
	repo := new(MockFileRepo)
	minio := new(MockMinioClient)
	uc := NewGetDownloadURLUseCase(repo, minio)

	fileID := uuid.MustParse("a1b2c3d4-e5f6-7890-abcd-ef1234567890")

	existingFile := &model.File{
		ID:         fileID,
		UserID:     uuid.MustParse("f47ac10b-58cc-4372-a567-0e02b2c3d479"),
		Name:       "not_uploaded_yet.zip",
		StorageKey: "uploads/a1b2c3d4_not_uploaded_yet.zip",
		Status:     model.FileStatusPending,
	}

	repo.On("FindByID", mock.Anything, fileID).Return(existingFile, nil)

	output, err := uc.Execute(context.Background(), fileID, 300)
	assert.ErrorIs(t, err, ErrFileNotReady)
	assert.Nil(t, output)

	repo.AssertExpectations(t)
}
