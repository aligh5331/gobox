package usecase

import (
	"context"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"

	"git.0lab.ir/aligh/gobox/auth/internal/domain/model"
)

// MockUserRepo is a mock implementation of repository.UserRepository.
type MockUserRepo struct {
	mock.Mock
}

func (m *MockUserRepo) Create(ctx context.Context, user *model.User) error {
	args := m.Called(ctx, user)
	return args.Error(0)
}

func (m *MockUserRepo) FindByID(ctx context.Context, id uuid.UUID) (*model.User, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.User), args.Error(1)
}

func (m *MockUserRepo) FindByEmail(ctx context.Context, email string) (*model.User, error) {
	args := m.Called(ctx, email)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.User), args.Error(1)
}

func (m *MockUserRepo) Update(ctx context.Context, user *model.User) error {
	args := m.Called(ctx, user)
	return args.Error(0)
}

// MockSessionRepo is a mock implementation of repository.SessionRepository.
type MockSessionRepo struct {
	mock.Mock
}

func (m *MockSessionRepo) Create(ctx context.Context, session *model.Session) error {
	args := m.Called(ctx, session)
	return args.Error(0)
}

func (m *MockSessionRepo) FindByID(ctx context.Context, id uuid.UUID) (*model.Session, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Session), args.Error(1)
}

func (m *MockSessionRepo) FindByUserID(ctx context.Context, userID uuid.UUID) ([]model.Session, error) {
	args := m.Called(ctx, userID)
	return args.Get(0).([]model.Session), args.Error(1)
}

func (m *MockSessionRepo) FindByRefreshToken(ctx context.Context, rawToken string) (*model.Session, error) {
	args := m.Called(ctx, rawToken)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Session), args.Error(1)
}

func (m *MockSessionRepo) Delete(ctx context.Context, id uuid.UUID) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockSessionRepo) Revoke(ctx context.Context, id uuid.UUID) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockSessionRepo) RevokeAllByUserID(ctx context.Context, userID uuid.UUID) error {
	args := m.Called(ctx, userID)
	return args.Error(0)
}

func (m *MockSessionRepo) Rotate(ctx context.Context, oldSessionID uuid.UUID, newSession *model.Session) (*model.Session, error) {
	args := m.Called(ctx, oldSessionID, newSession)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Session), args.Error(1)
}
