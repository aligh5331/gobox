package usecase

import (
	"context"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/aligh5331/gobox/auth/internal/domain/model"
	"github.com/aligh5331/gobox/auth/internal/domain/repository"
)

func TestRegisterUseCase_Success(t *testing.T) {
	userRepo := new(MockUserRepo)
	sessionRepo := new(MockSessionRepo)
	signer := newTestSigner(t)
	logger := zerolog.Nop()

	uc := NewRegisterUseCase(userRepo, sessionRepo, signer, logger)

	email := "alice@example.com"
	name := "Alice"
	password := "ValidPass1!"

	// No existing user
	userRepo.On("FindByEmail", mock.Anything, email).
		Return(nil, repository.ErrNotFound)

	// Create user (capture the user to get back its generated ID)
	userRepo.On("Create", mock.Anything, mock.MatchedBy(func(u *model.User) bool {
		return u.Email == email && u.Name == name
	})).Return(nil)

	// Create session
	sessionRepo.On("Create", mock.Anything, mock.MatchedBy(func(s *model.Session) bool {
		return s.UserID != uuid.Nil && s.ExpiresAt.After(s.CreatedAt)
	})).Return(nil)

	output, err := uc.Execute(context.Background(), email, name, password)
	assert.NoError(t, err)
	assert.NotNil(t, output)
	assert.Equal(t, email, output.User.Email)
	assert.Equal(t, name, output.User.Name)
	assert.NotEmpty(t, output.AccessToken)
	assert.NotEmpty(t, output.RefreshToken)
	assert.Len(t, output.RefreshToken, 43)
	assert.NotEmpty(t, output.User.PasswordHash)
	assert.NotNil(t, output.Session)
	assert.NotEqual(t, output.User.ID, output.Session.ID, "session ID must differ from user ID")

	userRepo.AssertExpectations(t)
	sessionRepo.AssertExpectations(t)
}

func TestRegisterUseCase_DuplicateEmail(t *testing.T) {
	userRepo := new(MockUserRepo)
	sessionRepo := new(MockSessionRepo)
	signer := newTestSigner(t)
	logger := zerolog.Nop()

	uc := NewRegisterUseCase(userRepo, sessionRepo, signer, logger)

	email := "alice@example.com"
	existingUser := &model.User{
		ID:    uuid.New(),
		Email: email,
	}

	userRepo.On("FindByEmail", mock.Anything, email).Return(existingUser, nil)

	output, err := uc.Execute(context.Background(), email, "Alice", "ValidPass1!")
	assert.ErrorIs(t, err, ErrEmailAlreadyExists)
	assert.Nil(t, output)
	userRepo.AssertExpectations(t)
}

func TestRegisterUseCase_WeakPassword(t *testing.T) {
	userRepo := new(MockUserRepo)
	sessionRepo := new(MockSessionRepo)
	signer := newTestSigner(t)
	logger := zerolog.Nop()

	uc := NewRegisterUseCase(userRepo, sessionRepo, signer, logger)

	output, err := uc.Execute(context.Background(), "bob@example.com", "Bob", "short")
	assert.ErrorIs(t, err, ErrWeakPassword)
	assert.Nil(t, output)
}

func TestRegisterUseCase_EmptyName(t *testing.T) {
	userRepo := new(MockUserRepo)
	sessionRepo := new(MockSessionRepo)
	signer := newTestSigner(t)
	logger := zerolog.Nop()

	uc := NewRegisterUseCase(userRepo, sessionRepo, signer, logger)

	output, err := uc.Execute(context.Background(), "bob@example.com", "", "ValidPass1!")
	assert.ErrorIs(t, err, ErrInvalidName)
	assert.Nil(t, output)
}

func TestRegisterUseCase_InvalidEmail(t *testing.T) {
	userRepo := new(MockUserRepo)
	sessionRepo := new(MockSessionRepo)
	signer := newTestSigner(t)
	logger := zerolog.Nop()

	uc := NewRegisterUseCase(userRepo, sessionRepo, signer, logger)

	output, err := uc.Execute(context.Background(), "not-an-email", "Bob", "ValidPass1!")
	assert.ErrorIs(t, err, ErrInvalidCredentials)
	assert.Nil(t, output)
}

func newTestSigner(t *testing.T) *testSigner {
	return &testSigner{}
}

// testSigner is a minimal TokenSigner for testing use cases.
type testSigner struct{}

func (s *testSigner) Sign(claims jwt.Claims) (string, error) {
	return "test-access-token", nil
}
