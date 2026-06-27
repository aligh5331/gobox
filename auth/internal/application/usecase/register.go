package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"git.0lab.ir/aligh/gobox/auth/internal/domain/model"
	"git.0lab.ir/aligh/gobox/auth/internal/domain/repository"
	"git.0lab.ir/aligh/gobox/auth/pkg/jwtutil"
)

// RegisterOutput contains the result of a successful registration.
type RegisterOutput struct {
	User         model.User
	AccessToken  string
	RefreshToken string
	Session      *model.Session
}

// RegisterUseCase handles new user registration with implicit login.
type RegisterUseCase struct {
	userRepo    repository.UserRepository
	sessionRepo repository.SessionRepository
	signer      TokenSigner
	logger      zerolog.Logger
}

// NewRegisterUseCase creates a new RegisterUseCase.
func NewRegisterUseCase(
	userRepo repository.UserRepository,
	sessionRepo repository.SessionRepository,
	signer TokenSigner,
	logger zerolog.Logger,
) *RegisterUseCase {
	return &RegisterUseCase{
		userRepo:    userRepo,
		sessionRepo: sessionRepo,
		signer:      signer,
		logger:      logger,
	}
}

// Execute registers a new user and returns tokens.
func (uc *RegisterUseCase) Execute(ctx context.Context, email, name, password string) (*RegisterOutput, error) {
	if err := validateEmail(email); err != nil {
		return nil, err
	}
	if name == "" {
		return nil, ErrInvalidName
	}
	if err := validatePassword(password); err != nil {
		return nil, err
	}

	// Check email uniqueness
	existing, err := uc.userRepo.FindByEmail(ctx, email)
	if err != nil && !isNotFound(err) {
		return nil, fmt.Errorf("register: check email: %w", err)
	}
	if existing != nil {
		return nil, ErrEmailAlreadyExists
	}

	// Hash password
	hashed, err := hashToken(password)
	if err != nil {
		return nil, fmt.Errorf("register: hash password: %w", err)
	}

	// Create user
	now := time.Now()
	user := &model.User{
		ID:           uuid.New(),
		Email:        email,
		Name:         name,
		PasswordHash: string(hashed),
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := uc.userRepo.Create(ctx, user); err != nil {
		return nil, fmt.Errorf("register: create user: %w", err)
	}

	// Create session
	refreshToken, err := generateRefreshToken()
	if err != nil {
		return nil, fmt.Errorf("register: generate refresh token: %w", err)
	}
	refreshHash, err := hashToken(refreshToken)
	if err != nil {
		return nil, fmt.Errorf("register: hash refresh token: %w", err)
	}

	session := &model.Session{
		ID:               uuid.New(),
		UserID:           user.ID,
		RefreshTokenHash: refreshHash,
		ExpiresAt:        now.Add(30 * 24 * time.Hour),
		CreatedAt:        now,
		LastUsedAt:       now,
	}
	if err := uc.sessionRepo.Create(ctx, session); err != nil {
		return nil, fmt.Errorf("register: create session: %w", err)
	}

	// Generate access token
	claims := jwtutil.NewAccessTokenClaims(
		user.ID.String(), user.Email, user.Name, session.ID.String(), now,
	)
	token, err := uc.signer.Sign(claims)
	if err != nil {
		return nil, fmt.Errorf("register: sign token: %w", err)
	}

	uc.logger.Info().
		Str("user_id", user.ID.String()).
		Str("email", user.Email).
		Msg("user registered")

	return &RegisterOutput{
		User:         *user,
		AccessToken:  token,
		RefreshToken: refreshToken,
		Session:      session,
	}, nil
}

func isNotFound(err error) bool {
	return err != nil && err.Error() == repository.ErrNotFound.Error()
}
