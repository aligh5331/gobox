package usecase

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/aligh5331/gobox/shortener/internal/domain/model"
)

func TestDeleteLinkUseCase_Success(t *testing.T) {
	repo := new(MockShortLinkRepo)
	uc := NewDeleteLinkUseCase(repo)

	userID := uuid.New()
	linkID := uuid.New()
	link := &model.ShortLink{
		ID:     linkID,
		UserID: userID,
	}

	repo.On("FindByID", mock.Anything, linkID).Return(link, nil)
	repo.On("Delete", mock.Anything, linkID).Return(nil)

	err := uc.Execute(context.Background(), linkID.String(), userID.String())
	assert.NoError(t, err)
	repo.AssertExpectations(t)
}

func TestDeleteLinkUseCase_InvalidLinkID(t *testing.T) {
	repo := new(MockShortLinkRepo)
	uc := NewDeleteLinkUseCase(repo)

	err := uc.Execute(context.Background(), "not-a-uuid", uuid.New().String())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid link_id")
}

func TestDeleteLinkUseCase_NotFound(t *testing.T) {
	repo := new(MockShortLinkRepo)
	uc := NewDeleteLinkUseCase(repo)

	linkID := uuid.New()
	repo.On("FindByID", mock.Anything, linkID).Return(nil, nil)

	err := uc.Execute(context.Background(), linkID.String(), uuid.New().String())
	assert.ErrorIs(t, err, ErrLinkNotFound)
	repo.AssertExpectations(t)
}

func TestDeleteLinkUseCase_PermissionDenied(t *testing.T) {
	repo := new(MockShortLinkRepo)
	uc := NewDeleteLinkUseCase(repo)

	linkID := uuid.New()
	link := &model.ShortLink{
		ID:     linkID,
		UserID: uuid.New(), // different user
	}
	repo.On("FindByID", mock.Anything, linkID).Return(link, nil)

	err := uc.Execute(context.Background(), linkID.String(), uuid.New().String())
	assert.ErrorIs(t, err, ErrPermissionDenied)
	repo.AssertExpectations(t)
}
