package usecase

import (
	"context"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"

	"github.com/aligh5331/gobox/shortener/internal/domain/model"
)

// MockShortLinkRepo is a mock implementation of repository.ShortLinkRepository.
type MockShortLinkRepo struct {
	mock.Mock
}

func (m *MockShortLinkRepo) Create(ctx context.Context, link *model.ShortLink) error {
	args := m.Called(ctx, link)
	return args.Error(0)
}

func (m *MockShortLinkRepo) FindBySlug(ctx context.Context, slug string) (*model.ShortLink, error) {
	args := m.Called(ctx, slug)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.ShortLink), args.Error(1)
}

func (m *MockShortLinkRepo) FindByID(ctx context.Context, id uuid.UUID) (*model.ShortLink, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.ShortLink), args.Error(1)
}

func (m *MockShortLinkRepo) FindByUserID(ctx context.Context, userID uuid.UUID, cursor string, limit int) ([]*model.ShortLink, string, error) {
	args := m.Called(ctx, userID, cursor, limit)
	if args.Get(0) == nil {
		return nil, args.String(1), args.Error(2)
	}
	return args.Get(0).([]*model.ShortLink), args.String(1), args.Error(2)
}

func (m *MockShortLinkRepo) Delete(ctx context.Context, id uuid.UUID) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockShortLinkRepo) IncrementHitCount(ctx context.Context, slug string) error {
	args := m.Called(ctx, slug)
	return args.Error(0)
}
