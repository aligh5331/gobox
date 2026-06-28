// Package grpc implements the ShortenerService gRPC server.
package grpc

import (
	"context"
	"errors"
	"fmt"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "github.com/aligh5331/gobox-proto/gen/shortener/v1"
	"github.com/aligh5331/gobox/shortener/internal/application/usecase"
	"github.com/aligh5331/gobox/shortener/internal/domain/model"
)

// Server implements the pb.ShortenerServiceServer interface.
type Server struct {
	pb.UnimplementedShortenerServiceServer

	createLinkUC *usecase.CreateLinkUseCase
	getLinkUC    *usecase.GetLinkUseCase
	deleteLinkUC *usecase.DeleteLinkUseCase
	listLinksUC  *usecase.ListLinksUseCase
}

// NewServer creates a new gRPC server wiring use cases.
func NewServer(
	createLinkUC *usecase.CreateLinkUseCase,
	getLinkUC *usecase.GetLinkUseCase,
	deleteLinkUC *usecase.DeleteLinkUseCase,
	listLinksUC *usecase.ListLinksUseCase,
) *Server {
	return &Server{
		createLinkUC: createLinkUC,
		getLinkUC:    getLinkUC,
		deleteLinkUC: deleteLinkUC,
		listLinksUC:  listLinksUC,
	}
}

// CreateLink creates a new short link.
func (s *Server) CreateLink(ctx context.Context, req *pb.CreateLinkRequest) (*pb.CreateLinkResponse, error) {
	if req.FileId == "" {
		return nil, status.Error(codes.InvalidArgument, "file_id is required")
	}
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}

	var expiresAt *time.Time
	if req.ExpiresAt != nil {
		t := req.ExpiresAt.AsTime()
		expiresAt = &t
	}

	output, err := s.createLinkUC.Execute(ctx, usecase.CreateLinkInput{
		UserID:    req.UserId,
		FileID:    req.FileId,
		TargetURL: req.TargetUrl,
		ExpiresAt: expiresAt,
	})
	if err != nil {
		return nil, mapUseCaseError(err)
	}

	return &pb.CreateLinkResponse{
		Link: shortLinkToProto(output.Link),
	}, nil
}

// GetLink retrieves a short link by slug.
func (s *Server) GetLink(ctx context.Context, req *pb.GetLinkRequest) (*pb.GetLinkResponse, error) {
	if req.Slug == "" {
		return nil, status.Error(codes.InvalidArgument, "slug is required")
	}

	link, err := s.getLinkUC.Execute(ctx, req.Slug)
	if err != nil {
		return nil, mapUseCaseError(err)
	}

	return &pb.GetLinkResponse{
		Link: shortLinkToProto(link),
	}, nil
}

// DeleteLink removes a short link.
func (s *Server) DeleteLink(ctx context.Context, req *pb.DeleteLinkRequest) (*emptypb.Empty, error) {
	if req.LinkId == "" {
		return nil, status.Error(codes.InvalidArgument, "link_id is required")
	}
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}

	if err := s.deleteLinkUC.Execute(ctx, req.LinkId, req.UserId); err != nil {
		return nil, mapUseCaseError(err)
	}

	return &emptypb.Empty{}, nil
}

// ListLinks returns paginated short links for a user.
func (s *Server) ListLinks(ctx context.Context, req *pb.ListLinksRequest) (*pb.ListLinksResponse, error) {
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}

	output, err := s.listLinksUC.Execute(ctx, usecase.ListLinksInput{
		UserID:    req.UserId,
		PageSize:  req.PageSize,
		PageToken: req.PageToken,
	})
	if err != nil {
		return nil, mapUseCaseError(err)
	}

	links := make([]*pb.ShortLinkResponse, 0, len(output.Links))
	for _, l := range output.Links {
		links = append(links, shortLinkToProto(l))
	}

	return &pb.ListLinksResponse{
		Links:         links,
		NextPageToken: output.NextPageToken,
	}, nil
}

// shortLinkToProto converts a domain ShortLink to a protobuf ShortLinkResponse.
func shortLinkToProto(l *model.ShortLink) *pb.ShortLinkResponse {
	pbMsg := &pb.ShortLinkResponse{
		Id:        l.ID.String(),
		UserId:    l.UserID.String(),
		Slug:      l.Slug,
		FileId:    l.FileID.String(),
		TargetUrl: l.TargetURL,
		HitCount:  l.HitCount,
		CreatedAt: timestamppb.New(l.CreatedAt),
	}
	if l.ExpiresAt != nil {
		pbMsg.ExpiresAt = timestamppb.New(*l.ExpiresAt)
	}
	return pbMsg
}

// mapUseCaseError converts a use case error to a gRPC status error.
func mapUseCaseError(err error) error {
	switch {
	case errors.Is(err, usecase.ErrLinkNotFound):
		return status.Error(codes.NotFound, "link not found")
	case errors.Is(err, usecase.ErrLinkExpired):
		return status.Error(codes.OutOfRange, "link has expired")
	case errors.Is(err, usecase.ErrPermissionDenied):
		return status.Error(codes.PermissionDenied, "permission denied")
	case errors.Is(err, usecase.ErrMissingFileID):
		return status.Error(codes.InvalidArgument, "file_id is required")
	case errors.Is(err, usecase.ErrSlugCollision):
		return status.Error(codes.Internal, "slug generation failed after retries")
	default:
		return status.Error(codes.Internal, fmt.Sprintf("internal error: %v", err))
	}
}
