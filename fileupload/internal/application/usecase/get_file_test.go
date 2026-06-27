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

func TestGetFile_Success(t *testing.T) {
	repo := new(MockFileRepo)
	uc := NewGetFileUseCase(repo)

	fileID := uuid.MustParse("a1b2c3d4-e5f6-7890-abcd-ef1234567890")
	userID := uuid.MustParse("f47ac10b-58cc-4372-a567-0e02b2c3d479")

	expectedFile := &model.File{
		ID:         fileID,
		UserID:     userID,
		Name:       "document.docx",
		Size:       2048,
		MimeType:   "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		StorageKey: "uploads/a1b2c3d4_document.docx",
		Status:     model.FileStatusReady,
	}

	repo.On("FindByID", mock.Anything, fileID).Return(expectedFile, nil)

	file, err := uc.Execute(context.Background(), fileID, userID)
	assert.NoError(t, err)
	assert.Equal(t, expectedFile, file)

	repo.AssertExpectations(t)
}

func TestGetFile_NotFound(t *testing.T) {
	repo := new(MockFileRepo)
	uc := NewGetFileUseCase(repo)

	fileID := uuid.MustParse("00000000-0000-0000-0000-000000000000")
	userID := uuid.MustParse("f47ac10b-58cc-4372-a567-0e02b2c3d479")

	repo.On("FindByID", mock.Anything, fileID).Return(nil, repository.ErrNotFound)

	file, err := uc.Execute(context.Background(), fileID, userID)
	assert.ErrorIs(t, err, ErrFileNotFound)
	assert.Nil(t, file)

	repo.AssertExpectations(t)
}

func TestGetFile_PermissionDenied(t *testing.T) {
	repo := new(MockFileRepo)
	uc := NewGetFileUseCase(repo)

	fileID := uuid.MustParse("a1b2c3d4-e5f6-7890-abcd-ef1234567890")
	userID := uuid.MustParse("f47ac10b-58cc-4372-a567-0e02b2c3d479")
	otherUserID := uuid.MustParse("99999999-9999-9999-9999-999999999999")

	existingFile := &model.File{
		ID:     fileID,
		UserID: otherUserID,
		Name:   "private.pdf",
		Status: model.FileStatusReady,
	}

	repo.On("FindByID", mock.Anything, fileID).Return(existingFile, nil)

	file, err := uc.Execute(context.Background(), fileID, userID)
	assert.ErrorIs(t, err, ErrPermissionDenied)
	assert.Nil(t, file)

	repo.AssertExpectations(t)
}
