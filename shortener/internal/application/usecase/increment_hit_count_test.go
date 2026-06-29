package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestIncrementHitCountUseCase_Success(t *testing.T) {
	repo := new(MockShortLinkRepo)
	uc := NewIncrementHitCountUseCase(repo)

	repo.On("IncrementHitCount", mock.Anything, "abc123").Return(nil)

	err := uc.Execute(context.Background(), "abc123")
	assert.NoError(t, err)
	repo.AssertExpectations(t)
}

func TestIncrementHitCountUseCase_RepoError(t *testing.T) {
	repo := new(MockShortLinkRepo)
	uc := NewIncrementHitCountUseCase(repo)

	repo.On("IncrementHitCount", mock.Anything, "abc123").Return(errors.New("db error"))

	err := uc.Execute(context.Background(), "abc123")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "increment hit count")
	repo.AssertExpectations(t)
}
