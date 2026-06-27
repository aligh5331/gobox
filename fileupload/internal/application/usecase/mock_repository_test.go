package usecase

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"

	"github.com/aligh5331/gobox/fileupload/internal/domain/model"
)

// MockFileRepo is a mock implementation of repository.FileRepository.
type MockFileRepo struct {
	mock.Mock
}

func (m *MockFileRepo) Create(ctx context.Context, file *model.File) error {
	args := m.Called(ctx, file)
	return args.Error(0)
}

func (m *MockFileRepo) FindByID(ctx context.Context, id uuid.UUID) (*model.File, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.File), args.Error(1)
}

func (m *MockFileRepo) FindByIDAndUser(ctx context.Context, id uuid.UUID, userID string) (*model.File, error) {
	args := m.Called(ctx, id, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.File), args.Error(1)
}

func (m *MockFileRepo) FindByUserID(ctx context.Context, userID uuid.UUID, cursor string, limit int) ([]*model.File, string, error) {
	args := m.Called(ctx, userID, cursor, limit)
	return args.Get(0).([]*model.File), args.String(1), args.Error(2)
}

func (m *MockFileRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status model.FileStatus) error {
	args := m.Called(ctx, id, status)
	return args.Error(0)
}

func (m *MockFileRepo) Update(ctx context.Context, file *model.File) error {
	args := m.Called(ctx, file)
	return args.Error(0)
}

func (m *MockFileRepo) SoftDelete(ctx context.Context, id uuid.UUID) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

// MockMinioClient is a mock implementation of MinioClient.
type MockMinioClient struct {
	mock.Mock
}

func (m *MockMinioClient) PresignedPutURL(ctx context.Context, objectKey string, ttl time.Duration) (string, map[string]string, error) {
	args := m.Called(ctx, objectKey, ttl)
	return args.String(0), args.Get(1).(map[string]string), args.Error(2)
}

func (m *MockMinioClient) PresignedGetURL(ctx context.Context, objectKey string, ttl time.Duration) (string, time.Time, error) {
	args := m.Called(ctx, objectKey, ttl)
	return args.String(0), args.Get(1).(time.Time), args.Error(2)
}

func (m *MockMinioClient) ObjectExists(ctx context.Context, objectKey string) (bool, error) {
	args := m.Called(ctx, objectKey)
	return args.Bool(0), args.Error(1)
}

func (m *MockMinioClient) RemoveObject(ctx context.Context, objectKey string) error {
	args := m.Called(ctx, objectKey)
	return args.Error(0)
}
