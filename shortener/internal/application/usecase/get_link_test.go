package usecase

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/aligh5331/gobox/shortener/internal/domain/model"
)

func TestGetLinkUseCase_Success(t *testing.T) {
	repo := new(MockShortLinkRepo)
	uc := NewGetLinkUseCase(repo)

	link := &model.ShortLink{
		ID:     uuid.New(),
		Slug:   "abc123",
		UserID: uuid.New(),
	}
	repo.On("FindBySlug", mock.Anything, "abc123").Return(link, nil)

	result, err := uc.Execute(context.Background(), "abc123")
	assert.NoError(t, err)
	assert.Equal(t, "abc123", result.Slug)
	repo.AssertExpectations(t)
}

func TestGetLinkUseCase_NotFound(t *testing.T) {
	repo := new(MockShortLinkRepo)
	uc := NewGetLinkUseCase(repo)

	repo.On("FindBySlug", mock.Anything, "nonexistent").Return(nil, nil)

	_, err := uc.Execute(context.Background(), "nonexistent")
	assert.ErrorIs(t, err, ErrLinkNotFound)
	repo.AssertExpectations(t)
}

func TestGetLinkUseCase_Expired(t *testing.T) {
	repo := new(MockShortLinkRepo)
	uc := NewGetLinkUseCase(repo)

	past := time.Now().Add(-1 * time.Hour)
	link := &model.ShortLink{
		ID:        uuid.New(),
		Slug:      "expired",
		ExpiresAt: &past,
	}
	repo.On("FindBySlug", mock.Anything, "expired").Return(link, nil)

	_, err := uc.Execute(context.Background(), "expired")
	assert.ErrorIs(t, err, ErrLinkExpired)
	repo.AssertExpectations(t)
}

func TestGetLinkByIDUseCase_Success(t *testing.T) {
	repo := new(MockShortLinkRepo)
	uc := NewGetLinkByIDUseCase(repo)

	userID := uuid.New()
	linkID := uuid.New()
	link := &model.ShortLink{
		ID:     linkID,
		UserID: userID,
		Slug:   "abc123",
	}
	repo.On("FindByID", mock.Anything, linkID).Return(link, nil)

	result, err := uc.Execute(context.Background(), linkID.String(), userID.String())
	assert.NoError(t, err)
	assert.Equal(t, linkID, result.ID)
	repo.AssertExpectations(t)
}

func TestGetLinkByIDUseCase_NotFound(t *testing.T) {
	repo := new(MockShortLinkRepo)
	uc := NewGetLinkByIDUseCase(repo)

	linkID := uuid.New()
	repo.On("FindByID", mock.Anything, linkID).Return(nil, nil)

	_, err := uc.Execute(context.Background(), linkID.String(), uuid.New().String())
	assert.ErrorIs(t, err, ErrLinkNotFound)
	repo.AssertExpectations(t)
}

func TestGetLinkByIDUseCase_PermissionDenied(t *testing.T) {
	repo := new(MockShortLinkRepo)
	uc := NewGetLinkByIDUseCase(repo)

	linkID := uuid.New()
	link := &model.ShortLink{
		ID:     linkID,
		UserID: uuid.New(), // different user
		Slug:   "abc123",
	}
	repo.On("FindByID", mock.Anything, linkID).Return(link, nil)

	_, err := uc.Execute(context.Background(), linkID.String(), uuid.New().String())
	assert.ErrorIs(t, err, ErrLinkNotFound)
	repo.AssertExpectations(t)
}

func TestGetLinkByIDUseCase_InvalidLinkID(t *testing.T) {
	repo := new(MockShortLinkRepo)
	uc := NewGetLinkByIDUseCase(repo)

	_, err := uc.Execute(context.Background(), "not-a-uuid", uuid.New().String())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid link_id")
}
