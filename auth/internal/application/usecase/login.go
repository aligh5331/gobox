package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"golang.org/x/crypto/bcrypt"

	"git.0lab.ir/aligh/gobox/auth/internal/domain/model"
	"git.0lab.ir/aligh/gobox/auth/internal/domain/repository"
	"git.0lab.ir/aligh/gobox/auth/pkg/jwtutil"
)

// LoginOutput contains the result of a successful login.
type LoginOutput struct {
	User         model.User
	AccessToken  string
	RefreshToken string
	Session      model.Session
}

// LoginUseCase handles user authentication.
type LoginUseCase struct {
	userRepo    repository.UserRepository
	sessionRepo repository.SessionRepository
	signer      TokenSigner
	logger      zerolog.Logger
}

// NewLoginUseCase creates a new LoginUseCase.
func NewLoginUseCase(
	userRepo repository.UserRepository,
	sessionRepo repository.SessionRepository,
	signer TokenSigner,
	logger zerolog.Logger,
) *LoginUseCase {
	return &LoginUseCase{
		userRepo:    userRepo,
		sessionRepo: sessionRepo,
		signer:      signer,
		logger:      logger,
	}
}

// Execute authenticates a user and returns tokens.
func (uc *LoginUseCase) Execute(ctx context.Context, email, password, userAgent, ip string) (*LoginOutput, error) {
	if err := validateEmail(email); err != nil {
		return nil, err
	}

	// Find user
	user, err := uc.userRepo.FindByEmail(ctx, email)
	if err != nil {
		if isNotFound(err) {
			return nil, ErrInvalidCredentials
		}
		return nil, fmt.Errorf("login: find user: %w", err)
	}

	// Verify password
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, ErrInvalidCredentials
	}

	// Create session
	now := time.Now()
	refreshToken, err := generateRefreshToken()
	if err != nil {
		return nil, fmt.Errorf("login: generate refresh token: %w", err)
	}
	refreshHash, err := hashToken(refreshToken)
	if err != nil {
		return nil, fmt.Errorf("login: hash refresh token: %w", err)
	}

	session := &model.Session{
		ID:               uuid.New(),
		UserID:           user.ID,
		RefreshTokenHash: refreshHash,
		UserAgent:        userAgent,
		IP:               ip,
		ExpiresAt:        now.Add(30 * 24 * time.Hour),
		CreatedAt:        now,
		LastUsedAt:       now,
	}
	if err := uc.sessionRepo.Create(ctx, session); err != nil {
		return nil, fmt.Errorf("login: create session: %w", err)
	}

	// Generate access token
	claims := jwtutil.NewAccessTokenClaims(
		user.ID.String(), user.Email, user.Name, session.ID.String(), now,
	)
	token, err := uc.signer.Sign(claims)
	if err != nil {
		return nil, fmt.Errorf("login: sign token: %w", err)
	}

	uc.logger.Info().
		Str("user_id", user.ID.String()).
		Str("email", user.Email).
		Msg("user logged in")

	return &LoginOutput{
		User:         *user,
		AccessToken:  token,
		RefreshToken: refreshToken,
		Session:      *session,
	}, nil
}
