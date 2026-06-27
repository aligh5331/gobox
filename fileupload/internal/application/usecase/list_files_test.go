package usecase

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/aligh5331/gobox/fileupload/internal/domain/model"
)

func TestListFiles_FirstPage(t *testing.T) {
	repo := new(MockFileRepo)
	uc := NewListFilesUseCase(repo)

	userID := uuid.MustParse("f47ac10b-58cc-4372-a567-0e02b2c3d479")
	pageSize := 3

	files := []*model.File{
		{ID: uuid.New(), UserID: userID, Name: "file1.txt"},
		{ID: uuid.New(), UserID: userID, Name: "file2.txt"},
		{ID: uuid.New(), UserID: userID, Name: "file3.txt"},
	}

	nextToken := encodeCursor(files[2].ID)
	repo.On("FindByUserID", mock.Anything, userID, "", pageSize).Return(files, nextToken, nil)

	output, err := uc.Execute(context.Background(), userID, pageSize, "")
	assert.NoError(t, err)
	assert.Len(t, output.Files, 3)
	assert.NotEmpty(t, output.NextPageToken)

	repo.AssertExpectations(t)
}

func TestListFiles_SecondPage(t *testing.T) {
	repo := new(MockFileRepo)
	uc := NewListFilesUseCase(repo)

	userID := uuid.MustParse("f47ac10b-58cc-4372-a567-0e02b2c3d479")
	pageSize := 3
	firstFileID := uuid.MustParse("a1b2c3d4-e5f6-7890-abcd-ef1234567890")
	cursor := encodeCursor(firstFileID)

	files := []*model.File{
		{ID: uuid.New(), UserID: userID, Name: "file4.txt"},
		{ID: uuid.New(), UserID: userID, Name: "file5.txt"},
	}

	repo.On("FindByUserID", mock.Anything, userID, cursor, pageSize).Return(files, "", nil)

	output, err := uc.Execute(context.Background(), userID, pageSize, cursor)
	assert.NoError(t, err)
	assert.Len(t, output.Files, 2)
	assert.Empty(t, output.NextPageToken)

	repo.AssertExpectations(t)
}

func TestListFiles_InvalidPageToken(t *testing.T) {
	repo := new(MockFileRepo)
	uc := NewListFilesUseCase(repo)

	userID := uuid.MustParse("f47ac10b-58cc-4372-a567-0e02b2c3d479")

	output, err := uc.Execute(context.Background(), userID, 10, "not-valid-base64!!")
	assert.ErrorIs(t, err, ErrInvalidPageToken)
	assert.Nil(t, output)
}

func TestListFiles_EmptyResult(t *testing.T) {
	repo := new(MockFileRepo)
	uc := NewListFilesUseCase(repo)

	userID := uuid.MustParse("f47ac10b-58cc-4372-a567-0e02b2c3d479")
	pageSize := 10

	repo.On("FindByUserID", mock.Anything, userID, "", pageSize).Return([]*model.File{}, "", nil)

	output, err := uc.Execute(context.Background(), userID, pageSize, "")
	assert.NoError(t, err)
	assert.Empty(t, output.Files)
	assert.Empty(t, output.NextPageToken)

	repo.AssertExpectations(t)
}

func TestListFiles_PageSizeCapped(t *testing.T) {
	repo := new(MockFileRepo)
	uc := NewListFilesUseCase(repo)

	userID := uuid.MustParse("f47ac10b-58cc-4372-a567-0e02b2c3d479")

	files := make([]*model.File, MaxPageSize)
	for i := range files {
		files[i] = &model.File{ID: uuid.New(), UserID: userID, Name: "file.txt"}
	}

	repo.On("FindByUserID", mock.Anything, userID, "", MaxPageSize).Return(files, "next-token", nil)

	output, err := uc.Execute(context.Background(), userID, 500, "")
	assert.NoError(t, err)
	assert.Len(t, output.Files, MaxPageSize)
	assert.NotEmpty(t, output.NextPageToken)

	repo.AssertExpectations(t)
}
