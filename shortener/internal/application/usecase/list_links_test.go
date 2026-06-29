package usecase

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/aligh5331/gobox/shortener/internal/domain/model"
)

func TestListLinksUseCase_Success(t *testing.T) {
	repo := new(MockShortLinkRepo)
	uc := NewListLinksUseCase(repo)

	userID := uuid.New()
	links := []*model.ShortLink{
		{ID: uuid.New(), UserID: userID, Slug: "abc123"},
		{ID: uuid.New(), UserID: userID, Slug: "def456"},
	}

	repo.On("FindByUserID", mock.Anything, userID, "", 20).Return(links, "", nil)

	output, err := uc.Execute(context.Background(), ListLinksInput{
		UserID:    userID.String(),
		PageSize:  0, // should default to 20
		PageToken: "",
	})

	assert.NoError(t, err)
	assert.NotNil(t, output)
	assert.Len(t, output.Links, 2)
	assert.Empty(t, output.NextPageToken)
	repo.AssertExpectations(t)
}

func TestListLinksUseCase_WithPagination(t *testing.T) {
	repo := new(MockShortLinkRepo)
	uc := NewListLinksUseCase(repo)

	userID := uuid.New()
	links := []*model.ShortLink{
		{ID: uuid.New(), UserID: userID, Slug: "abc123"},
	}
	nextToken := "next-page-token"

	repo.On("FindByUserID", mock.Anything, userID, "prev-token", 10).Return(links, nextToken, nil)

	output, err := uc.Execute(context.Background(), ListLinksInput{
		UserID:    userID.String(),
		PageSize:  10,
		PageToken: "prev-token",
	})

	assert.NoError(t, err)
	assert.Len(t, output.Links, 1)
	assert.Equal(t, nextToken, output.NextPageToken)
	repo.AssertExpectations(t)
}

func TestListLinksUseCase_ClampPageSizeMax(t *testing.T) {
	pageSize := clampPageSize(500) // exceeds MaxPageSize (200)
	assert.Equal(t, MaxPageSize, pageSize)
}

func TestListLinksUseCase_ClampPageSizeMin(t *testing.T) {
	pageSize := clampPageSize(-1)
	assert.Equal(t, DefaultPageSize, pageSize)

	pageSize = clampPageSize(0)
	assert.Equal(t, DefaultPageSize, pageSize)
}

func TestListLinksUseCase_InvalidUserID(t *testing.T) {
	repo := new(MockShortLinkRepo)
	uc := NewListLinksUseCase(repo)

	_, err := uc.Execute(context.Background(), ListLinksInput{
		UserID:    "not-a-uuid",
		PageSize:  10,
		PageToken: "",
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid user_id")
}
