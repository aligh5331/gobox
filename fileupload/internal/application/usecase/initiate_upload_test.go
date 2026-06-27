package usecase

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/aligh5331/gobox/fileupload/internal/domain/model"
)

func TestInitiateUpload_Success(t *testing.T) {
	repo := new(MockFileRepo)
	minio := new(MockMinioClient)
	uc := NewInitiateUploadUseCase(repo, minio)

	userID := uuid.MustParse("f47ac10b-58cc-4372-a567-0e02b2c3d479")
	name := "report.pdf"
	size := int64(1048576)
	mimeType := "application/pdf"

	repo.On("Create", mock.Anything, mock.MatchedBy(func(f *model.File) bool {
		return f.UserID == userID &&
			f.Name == name &&
			f.Size == size &&
			f.MimeType == mimeType &&
			f.Status == model.FileStatusPending
	})).Return(nil)

	uploadURL := "https://minio.example.com/uploads/test_photo.pdf"
	uploadHeaders := map[string]string{"Content-Type": mimeType}
	minio.On("PresignedPutURL", mock.Anything, mock.AnythingOfType("string"), InitiateUploadTTL).
		Return(uploadURL, uploadHeaders, nil)

	output, err := uc.Execute(context.Background(), userID, name, size, mimeType)
	assert.NoError(t, err)
	assert.NotNil(t, output)
	assert.NotEqual(t, uuid.Nil, output.FileID)
	assert.Equal(t, uploadURL, output.UploadURL)
	assert.Equal(t, uploadHeaders, output.UploadHeaders)

	repo.AssertExpectations(t)
	minio.AssertExpectations(t)
}

func TestInitiateUpload_EmptyName(t *testing.T) {
	repo := new(MockFileRepo)
	minio := new(MockMinioClient)
	uc := NewInitiateUploadUseCase(repo, minio)

	_, err := uc.Execute(context.Background(), uuid.New(), "", 1024, "text/plain")
	assert.ErrorIs(t, err, ErrInvalidName)
}

func TestInitiateUpload_ZeroSize(t *testing.T) {
	repo := new(MockFileRepo)
	minio := new(MockMinioClient)
	uc := NewInitiateUploadUseCase(repo, minio)

	_, err := uc.Execute(context.Background(), uuid.New(), "empty.txt", 0, "text/plain")
	assert.ErrorIs(t, err, ErrInvalidSize)
}

func TestInitiateUpload_NegativeSize(t *testing.T) {
	repo := new(MockFileRepo)
	minio := new(MockMinioClient)
	uc := NewInitiateUploadUseCase(repo, minio)

	_, err := uc.Execute(context.Background(), uuid.New(), "bad.txt", -1, "text/plain")
	assert.ErrorIs(t, err, ErrInvalidSize)
}

func TestInitiateUpload_EmptyMimeType(t *testing.T) {
	repo := new(MockFileRepo)
	minio := new(MockMinioClient)
	uc := NewInitiateUploadUseCase(repo, minio)

	_, err := uc.Execute(context.Background(), uuid.New(), "photo.jpg", 524288, "")
	assert.ErrorIs(t, err, ErrInvalidMimeType)
}
