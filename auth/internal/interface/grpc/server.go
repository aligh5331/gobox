// Package grpc implements the AuthService gRPC server.
package grpc

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "git.0lab.ir/aligh/gobox-proto/gen/auth/v1"
	"git.0lab.ir/aligh/gobox/auth/internal/application/usecase"
	"git.0lab.ir/aligh/gobox/auth/internal/domain/model"
	"git.0lab.ir/aligh/gobox/auth/internal/domain/repository"
	"git.0lab.ir/aligh/gobox/auth/pkg/jwtutil"
)

// AuthServer implements the proto AuthServiceServer.
type AuthServer struct {
	pb.UnimplementedAuthServiceServer

	registerUC       *usecase.RegisterUseCase
	loginUC          *usecase.LoginUseCase
	refreshTokenUC   *usecase.RefreshTokenUseCase
	logoutUC         *usecase.LogoutUseCase
	logoutAllUC      *usecase.LogoutAllUseCase
	getUserUC        *usecase.GetUserUseCase
	updateProfileUC  *usecase.UpdateProfileUseCase
	changePasswordUC *usecase.ChangePasswordUseCase

	sessionRepo repository.SessionRepository
	keyManager  *jwtutil.KeyManager
}

// NewAuthServer creates a new AuthServer.
func NewAuthServer(
	registerUC *usecase.RegisterUseCase,
	loginUC *usecase.LoginUseCase,
	refreshTokenUC *usecase.RefreshTokenUseCase,
	logoutUC *usecase.LogoutUseCase,
	logoutAllUC *usecase.LogoutAllUseCase,
	getUserUC *usecase.GetUserUseCase,
	updateProfileUC *usecase.UpdateProfileUseCase,
	changePasswordUC *usecase.ChangePasswordUseCase,
	sessionRepo repository.SessionRepository,
	keyManager *jwtutil.KeyManager,
) *AuthServer {
	return &AuthServer{
		registerUC:       registerUC,
		loginUC:          loginUC,
		refreshTokenUC:   refreshTokenUC,
		logoutUC:         logoutUC,
		logoutAllUC:      logoutAllUC,
		getUserUC:        getUserUC,
		updateProfileUC:  updateProfileUC,
		changePasswordUC: changePasswordUC,
		sessionRepo:      sessionRepo,
		keyManager:       keyManager,
	}
}

// ---------------------------------------------------------------------------
// Register
// ---------------------------------------------------------------------------

func (s *AuthServer) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.RegisterResponse, error) {
	out, err := s.registerUC.Execute(ctx, req.GetEmail(), req.GetName(), req.GetPassword())
	if err != nil {
		return nil, mapError(err)
	}

	return &pb.RegisterResponse{
		User: userToProto(&out.User),
		Tokens: &pb.TokenPair{
			AccessToken:  out.AccessToken,
			RefreshToken: out.RefreshToken,
			ExpiresIn:    int64(15 * time.Minute.Seconds()),
		},
		Session: &pb.SessionProto{
			Id:        out.Session.ID.String(),
			UserId:    out.User.ID.String(),
			ExpiresAt: timestamppb.New(out.Session.ExpiresAt),
		},
	}, nil
}

// ---------------------------------------------------------------------------
// Login
// ---------------------------------------------------------------------------

func (s *AuthServer) Login(ctx context.Context, req *pb.LoginRequest) (*pb.LoginResponse, error) {
	out, err := s.loginUC.Execute(ctx, req.GetEmail(), req.GetPassword(), req.GetUserAgent(), req.GetIp())
	if err != nil {
		return nil, mapError(err)
	}

	return &pb.LoginResponse{
		User: userToProto(&out.User),
		Tokens: &pb.TokenPair{
			AccessToken:  out.AccessToken,
			RefreshToken: out.RefreshToken,
			ExpiresIn:    int64(15 * time.Minute.Seconds()),
		},
		Session: &pb.SessionProto{
			Id:        out.Session.ID.String(),
			UserId:    out.User.ID.String(),
			ExpiresAt: timestamppb.New(out.Session.ExpiresAt),
		},
	}, nil
}

// ---------------------------------------------------------------------------
// RefreshToken
// ---------------------------------------------------------------------------

func (s *AuthServer) RefreshToken(ctx context.Context, req *pb.RefreshTokenRequest) (*pb.RefreshTokenResponse, error) {
	out, err := s.refreshTokenUC.Execute(ctx, req.GetRefreshToken())
	if err != nil {
		return nil, mapError(err)
	}

	return &pb.RefreshTokenResponse{
		Tokens: &pb.TokenPair{
			AccessToken:  out.AccessToken,
			RefreshToken: out.RefreshToken,
			ExpiresIn:    int64(15 * time.Minute.Seconds()),
		},
	}, nil
}

// ---------------------------------------------------------------------------
// Logout
// ---------------------------------------------------------------------------

func (s *AuthServer) Logout(ctx context.Context, req *pb.LogoutRequest) (*emptypb.Empty, error) {
	sessionID, err := uuid.Parse(req.GetSessionId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid session_id")
	}

	if err := s.logoutUC.Execute(ctx, sessionID); err != nil {
		return nil, mapError(err)
	}

	return &emptypb.Empty{}, nil
}

// ---------------------------------------------------------------------------
// LogoutAll
// ---------------------------------------------------------------------------

func (s *AuthServer) LogoutAll(ctx context.Context, req *pb.LogoutAllRequest) (*emptypb.Empty, error) {
	userID, err := uuid.Parse(req.GetUserId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid user_id")
	}

	if err := s.logoutAllUC.Execute(ctx, userID); err != nil {
		return nil, mapError(err)
	}

	return &emptypb.Empty{}, nil
}

// ---------------------------------------------------------------------------
// GetUser
// ---------------------------------------------------------------------------

func (s *AuthServer) GetUser(ctx context.Context, req *pb.GetUserRequest) (*pb.GetUserResponse, error) {
	uid, err := uuid.Parse(req.GetUserId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid user_id")
	}

	user, err := s.getUserUC.Execute(ctx, uid)
	if err != nil {
		return nil, mapError(err)
	}

	return userToProto(user), nil
}

// ---------------------------------------------------------------------------
// UpdateProfile
// ---------------------------------------------------------------------------

func (s *AuthServer) UpdateProfile(ctx context.Context, req *pb.UpdateProfileRequest) (*pb.UpdateProfileResponse, error) {
	userID, err := uuid.Parse(req.GetUserId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid user_id")
	}

	user, err := s.updateProfileUC.Execute(ctx, userID, req.GetName())
	if err != nil {
		return nil, mapError(err)
	}

	return &pb.UpdateProfileResponse{
		User: userToProto(user),
	}, nil
}

// ---------------------------------------------------------------------------
// ChangePassword
// ---------------------------------------------------------------------------

func (s *AuthServer) ChangePassword(ctx context.Context, req *pb.ChangePasswordRequest) (*emptypb.Empty, error) {
	userID, err := uuid.Parse(req.GetUserId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid user_id")
	}

	if err := s.changePasswordUC.Execute(ctx, userID, req.GetOldPassword(), req.GetNewPassword()); err != nil {
		return nil, mapError(err)
	}

	return &emptypb.Empty{}, nil
}

// ---------------------------------------------------------------------------
// ValidateSession
// ---------------------------------------------------------------------------

func (s *AuthServer) ValidateSession(ctx context.Context, req *pb.ValidateSessionRequest) (*pb.ValidateSessionResponse, error) {
	sessionID, err := uuid.Parse(req.GetSessionId())
	if err != nil {
		return &pb.ValidateSessionResponse{Valid: false}, nil
	}

	session, err := s.sessionRepo.FindByID(ctx, sessionID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return &pb.ValidateSessionResponse{Valid: false}, nil
		}
		return nil, status.Errorf(codes.Internal, "validate session: %v", err)
	}

	if session.Revoked {
		return &pb.ValidateSessionResponse{Valid: false}, nil
	}

	if time.Now().After(session.ExpiresAt) {
		return &pb.ValidateSessionResponse{Valid: false}, nil
	}

	return &pb.ValidateSessionResponse{
		Valid:  true,
		UserId: session.UserID.String(),
	}, nil
}

// ---------------------------------------------------------------------------
// GetPublicKey
// ---------------------------------------------------------------------------

func (s *AuthServer) GetPublicKey(ctx context.Context, _ *emptypb.Empty) (*pb.GetPublicKeyResponse, error) {
	jwks := s.keyManager.JWKS()
	data, err := json.Marshal(jwks)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "marshal jwks: %v", err)
	}

	return &pb.GetPublicKeyResponse{
		JwksJson: string(data),
	}, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// userToProto converts a domain User to a proto GetUserResponse.
func userToProto(user *model.User) *pb.GetUserResponse {
	return &pb.GetUserResponse{
		Id:        user.ID.String(),
		Email:     user.Email,
		Name:      user.Name,
		CreatedAt: timestamppb.New(user.CreatedAt),
		UpdatedAt: timestamppb.New(user.UpdatedAt),
	}
}

// mapError maps a use case error to a gRPC status error.
func mapError(err error) error {
	switch {
	case errors.Is(err, usecase.ErrEmailAlreadyExists):
		return status.Error(codes.AlreadyExists, err.Error())
	case errors.Is(err, usecase.ErrWeakPassword):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, usecase.ErrInvalidCredentials):
		return status.Error(codes.Unauthenticated, err.Error())
	case errors.Is(err, usecase.ErrUserNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, usecase.ErrSessionNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, usecase.ErrSessionExpired):
		return status.Error(codes.Unauthenticated, err.Error())
	case errors.Is(err, usecase.ErrSessionRevoked):
		return status.Error(codes.Unauthenticated, err.Error())
	case errors.Is(err, usecase.ErrSessionAlreadyRevoked):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, usecase.ErrTokenTheftDetected):
		return status.Error(codes.Unauthenticated, err.Error())
	case errors.Is(err, usecase.ErrInvalidName):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, usecase.ErrInvalidPassword):
		return status.Error(codes.InvalidArgument, err.Error())
	default:
		return status.Errorf(codes.Internal, "%v", err)
	}
}

// Ensure compile-time interface compliance.
var _ pb.AuthServiceServer = (*AuthServer)(nil)
