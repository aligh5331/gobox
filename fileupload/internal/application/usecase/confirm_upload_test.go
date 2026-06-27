package usecase

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/aligh5331/gobox/fileupload/internal/domain/model"
	"github.com/aligh5331/gobox/fileupload/internal/domain/repository"
)

func TestConfirmUpload_Success(t *testing.T) {
	repo := new(MockFileRepo)
	minio := new(MockMinioClient)
	uc := NewConfirmUploadUseCase(repo, minio)

	fileID := uuid.MustParse("a1b2c3d4-e5f6-7890-abcd-ef1234567890")
	userID := uuid.MustParse("f47ac10b-58cc-4372-a567-0e02b2c3d479")

	existingFile := &model.File{
		ID:         fileID,
		UserID:     userID,
		Name:       "photo.jpg",
		Size:       0,
		MimeType:   "image/jpeg",
		StorageKey: "uploads/a1b2c3d4_photo.jpg",
		Status:     model.FileStatusPending,
	}

	repo.On("FindByID", mock.Anything, fileID).Return(existingFile, nil)
	minio.On("ObjectExists", mock.Anything, existingFile.StorageKey).Return(true, nil)
	repo.On("UpdateStatus", mock.Anything, fileID, model.FileStatusReady).Return(nil)

	file, err := uc.Execute(context.Background(), fileID, userID)
	assert.NoError(t, err)
	assert.NotNil(t, file)
	assert.Equal(t, model.FileStatusReady, file.Status)

	repo.AssertExpectations(t)
	minio.AssertExpectations(t)
}

func TestConfirmUpload_FileNotFound(t *testing.T) {
	repo := new(MockFileRepo)
	minio := new(MockMinioClient)
	uc := NewConfirmUploadUseCase(repo, minio)

	fileID := uuid.MustParse("00000000-0000-0000-0000-000000000000")
	userID := uuid.MustParse("f47ac10b-58cc-4372-a567-0e02b2c3d479")

	repo.On("FindByID", mock.Anything, fileID).Return(nil, repository.ErrNotFound)

	file, err := uc.Execute(context.Background(), fileID, userID)
	assert.ErrorIs(t, err, ErrFileNotFound)
	assert.Nil(t, file)

	repo.AssertExpectations(t)
}

func TestConfirmUpload_PermissionDenied(t *testing.T) {
	repo := new(MockFileRepo)
	minio := new(MockMinioClient)
	uc := NewConfirmUploadUseCase(repo, minio)

	fileID := uuid.MustParse("a1b2c3d4-e5f6-7890-abcd-ef1234567890")
	userID := uuid.MustParse("f47ac10b-58cc-4372-a567-0e02b2c3d479")
	otherUserID := uuid.MustParse("99999999-9999-9999-9999-999999999999")

	existingFile := &model.File{
		ID:     fileID,
		UserID: otherUserID,
		Name:   "private.doc",
		Status: model.FileStatusPending,
	}

	repo.On("FindByID", mock.Anything, fileID).Return(existingFile, nil)

	file, err := uc.Execute(context.Background(), fileID, userID)
	assert.ErrorIs(t, err, ErrPermissionDenied)
	assert.Nil(t, file)

	repo.AssertExpectations(t)
}

func TestConfirmUpload_FileNotPending(t *testing.T) {
	repo := new(MockFileRepo)
	minio := new(MockMinioClient)
	uc := NewConfirmUploadUseCase(repo, minio)

	fileID := uuid.MustParse("a1b2c3d4-e5f6-7890-abcd-ef1234567890")
	userID := uuid.MustParse("f47ac10b-58cc-4372-a567-0e02b2c3d479")

	existingFile := &model.File{
		ID:     fileID,
		UserID: userID,
		Name:   "already_ready.txt",
		Status: model.FileStatusReady,
	}

	repo.On("FindByID", mock.Anything, fileID).Return(existingFile, nil)

	file, err := uc.Execute(context.Background(), fileID, userID)
	assert.ErrorIs(t, err, ErrFileNotPending)
	assert.Nil(t, file)

	repo.AssertExpectations(t)
}
