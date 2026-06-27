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

func TestDeleteFile_Success(t *testing.T) {
	repo := new(MockFileRepo)
	minio := new(MockMinioClient)
	uc := NewDeleteFileUseCase(repo, minio)

	fileID := uuid.MustParse("a1b2c3d4-e5f6-7890-abcd-ef1234567890")
	userID := uuid.MustParse("f47ac10b-58cc-4372-a567-0e02b2c3d479")

	existingFile := &model.File{
		ID:         fileID,
		UserID:     userID,
		Name:       "to_delete.png",
		StorageKey: "uploads/a1b2c3d4_to_delete.png",
		Status:     model.FileStatusReady,
	}

	repo.On("FindByID", mock.Anything, fileID).Return(existingFile, nil)
	repo.On("SoftDelete", mock.Anything, fileID).Return(nil)
	// Async S3 cleanup — expect RemoveObject to be called eventually.
	minio.On("RemoveObject", mock.Anything, existingFile.StorageKey).Return(nil)

	err := uc.Execute(context.Background(), fileID, userID)
	assert.NoError(t, err)

	repo.AssertExpectations(t)
	// Wait for the async goroutine to execute.
	time.Sleep(50 * time.Millisecond)
	minio.AssertCalled(t, "RemoveObject", mock.Anything, existingFile.StorageKey)
}

func TestDeleteFile_NotFound(t *testing.T) {
	repo := new(MockFileRepo)
	minio := new(MockMinioClient)
	uc := NewDeleteFileUseCase(repo, minio)

	fileID := uuid.MustParse("00000000-0000-0000-0000-000000000000")
	userID := uuid.MustParse("f47ac10b-58cc-4372-a567-0e02b2c3d479")

	repo.On("FindByID", mock.Anything, fileID).Return(nil, repository.ErrNotFound)

	err := uc.Execute(context.Background(), fileID, userID)
	assert.ErrorIs(t, err, ErrFileNotFound)

	repo.AssertExpectations(t)
}

func TestDeleteFile_PermissionDenied(t *testing.T) {
	repo := new(MockFileRepo)
	minio := new(MockMinioClient)
	uc := NewDeleteFileUseCase(repo, minio)

	fileID := uuid.MustParse("a1b2c3d4-e5f6-7890-abcd-ef1234567890")
	userID := uuid.MustParse("f47ac10b-58cc-4372-a567-0e02b2c3d479")
	otherUserID := uuid.MustParse("99999999-9999-9999-9999-999999999999")

	existingFile := &model.File{
		ID:     fileID,
		UserID: otherUserID,
		Name:   "not_mine.mp4",
		Status: model.FileStatusReady,
	}

	repo.On("FindByID", mock.Anything, fileID).Return(existingFile, nil)

	err := uc.Execute(context.Background(), fileID, userID)
	assert.ErrorIs(t, err, ErrPermissionDenied)

	repo.AssertExpectations(t)
}
