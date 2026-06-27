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

// RefreshTokenOutput contains the result of a successful token refresh.
type RefreshTokenOutput struct {
	AccessToken  string
	RefreshToken string
}

// RefreshTokenUseCase handles refresh token rotation.
type RefreshTokenUseCase struct {
	userRepo    repository.UserRepository
	sessionRepo repository.SessionRepository
	signer      TokenSigner
	logger      zerolog.Logger
}

// NewRefreshTokenUseCase creates a new RefreshTokenUseCase.
func NewRefreshTokenUseCase(
	userRepo repository.UserRepository,
	sessionRepo repository.SessionRepository,
	signer TokenSigner,
	logger zerolog.Logger,
) *RefreshTokenUseCase {
	return &RefreshTokenUseCase{
		userRepo:    userRepo,
		sessionRepo: sessionRepo,
		signer:      signer,
		logger:      logger,
	}
}

// Execute rotates the refresh token and issues new tokens.
func (uc *RefreshTokenUseCase) Execute(ctx context.Context, refreshToken string) (*RefreshTokenOutput, error) {
	// Find session by refresh token (scans + bcrypt compares)
	session, err := uc.sessionRepo.FindByRefreshToken(ctx, refreshToken)
	if err != nil {
		if isNotFound(err) {
			return nil, ErrInvalidCredentials
		}
		return nil, fmt.Errorf("refresh: find session: %w", err)
	}

	now := time.Now()

	// Check expiry
	if now.After(session.ExpiresAt) {
		return nil, ErrSessionExpired
	}

	// Check revocation
	if session.Revoked {
		return nil, ErrSessionRevoked
	}

	// Fetch user for claims
	user, err := uc.userRepo.FindByID(ctx, session.UserID)
	if err != nil {
		return nil, fmt.Errorf("refresh: find user: %w", err)
	}

	// Generate new refresh token
	newRefreshToken, err := generateRefreshToken()
	if err != nil {
		return nil, fmt.Errorf("refresh: generate token: %w", err)
	}
	newRefreshHash, err := hashToken(newRefreshToken)
	if err != nil {
		return nil, fmt.Errorf("refresh: hash token: %w", err)
	}

	// Create new session
	newSession := &model.Session{
		ID:               uuid.New(),
		UserID:           user.ID,
		RefreshTokenHash: newRefreshHash,
		UserAgent:        session.UserAgent,
		IP:               session.IP,
		ExpiresAt:        now.Add(30 * 24 * time.Hour),
		CreatedAt:        now,
		LastUsedAt:       now,
	}

	// Atomic rotation: delete old, insert new
	created, err := uc.sessionRepo.Rotate(ctx, session.ID, newSession)
	if err != nil {
		if isTokenReuse(err) {
			uc.logger.Warn().
				Str("user_id", user.ID.String()).
				Str("session_id", session.ID.String()).
				Msg("token theft detected")
			return nil, ErrTokenTheftDetected
		}
		return nil, fmt.Errorf("refresh: rotate session: %w", err)
	}

	// Generate new access token
	claims := jwtutil.NewAccessTokenClaims(
		user.ID.String(), user.Email, user.Name, created.ID.String(), now,
	)
	token, err := uc.signer.Sign(claims)
	if err != nil {
		return nil, fmt.Errorf("refresh: sign token: %w", err)
	}

	return &RefreshTokenOutput{
		AccessToken:  token,
		RefreshToken: newRefreshToken,
	}, nil
}

func isTokenReuse(err error) bool {
	return err != nil && err.Error() == repository.ErrTokenReuse.Error()
}
