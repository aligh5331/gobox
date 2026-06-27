// Package grpc implements the FileUploadService gRPC server.
package grpc

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "github.com/aligh5331/gobox-proto/gen/fileupload/v1"
	"github.com/aligh5331/gobox/fileupload/internal/application/usecase"
	"github.com/aligh5331/gobox/fileupload/internal/domain/model"
)

// Server implements the pb.FileUploadServiceServer interface.
type Server struct {
	pb.UnimplementedFileUploadServiceServer

	initiateUploadUC   *usecase.InitiateUploadUseCase
	confirmUploadUC    *usecase.ConfirmUploadUseCase
	getFileUC          *usecase.GetFileUseCase
	listFilesUC        *usecase.ListFilesUseCase
	deleteFileUC       *usecase.DeleteFileUseCase
	getDownloadURLUC   *usecase.GetDownloadURLUseCase
}

// NewServer creates a new gRPC server wiring use cases.
func NewServer(
	initiateUploadUC *usecase.InitiateUploadUseCase,
	confirmUploadUC *usecase.ConfirmUploadUseCase,
	getFileUC *usecase.GetFileUseCase,
	listFilesUC *usecase.ListFilesUseCase,
	deleteFileUC *usecase.DeleteFileUseCase,
	getDownloadURLUC *usecase.GetDownloadURLUseCase,
) *Server {
	return &Server{
		initiateUploadUC: initiateUploadUC,
		confirmUploadUC:  confirmUploadUC,
		getFileUC:        getFileUC,
		listFilesUC:      listFilesUC,
		deleteFileUC:     deleteFileUC,
		getDownloadURLUC: getDownloadURLUC,
	}
}

// InitiateUpload begins an upload and returns a presigned PUT URL.
func (s *Server) InitiateUpload(ctx context.Context, req *pb.InitiateUploadRequest) (*pb.InitiateUploadResponse, error) {
	userID, err := uuid.Parse(req.UserId)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid user_id")
	}

	output, err := s.initiateUploadUC.Execute(ctx, userID, req.Name, req.Size, req.MimeType)
	if err != nil {
		return nil, mapUseCaseError(err)
	}

	return &pb.InitiateUploadResponse{
		FileId:    output.FileID.String(),
		UploadUrl: output.UploadURL,
	}, nil
}

// ConfirmUpload finalises a completed upload.
func (s *Server) ConfirmUpload(ctx context.Context, req *pb.ConfirmUploadRequest) (*pb.ConfirmUploadResponse, error) {
	fileID, err := uuid.Parse(req.FileId)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid file_id")
	}
	userID, err := uuid.Parse(req.UserId)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid user_id")
	}

	file, err := s.confirmUploadUC.Execute(ctx, fileID, userID)
	if err != nil {
		return nil, mapUseCaseError(err)
	}

	return &pb.ConfirmUploadResponse{
		File: fileToProto(file),
	}, nil
}

// GetFile retrieves file metadata.
func (s *Server) GetFile(ctx context.Context, req *pb.GetFileRequest) (*pb.GetFileResponse, error) {
	fileID, err := uuid.Parse(req.FileId)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid file_id")
	}
	userID, err := uuid.Parse(req.UserId)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid user_id")
	}

	file, err := s.getFileUC.Execute(ctx, fileID, userID)
	if err != nil {
		return nil, mapUseCaseError(err)
	}

	return &pb.GetFileResponse{
		File: fileToProto(file),
	}, nil
}

// ListFiles returns paginated file metadata for a user.
func (s *Server) ListFiles(ctx context.Context, req *pb.ListFilesRequest) (*pb.ListFilesResponse, error) {
	userID, err := uuid.Parse(req.UserId)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid user_id")
	}

	output, err := s.listFilesUC.Execute(ctx, userID, int(req.PageSize), req.PageToken)
	if err != nil {
		return nil, mapUseCaseError(err)
	}

	files := make([]*pb.FileResponse, 0, len(output.Files))
	for _, f := range output.Files {
		files = append(files, fileToProto(f))
	}

	return &pb.ListFilesResponse{
		Files:         files,
		NextPageToken: output.NextPageToken,
	}, nil
}

// DeleteFile soft-deletes a file.
func (s *Server) DeleteFile(ctx context.Context, req *pb.DeleteFileRequest) (*emptypb.Empty, error) {
	fileID, err := uuid.Parse(req.FileId)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid file_id")
	}
	userID, err := uuid.Parse(req.UserId)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid user_id")
	}

	if err := s.deleteFileUC.Execute(ctx, fileID, userID); err != nil {
		return nil, mapUseCaseError(err)
	}

	return &emptypb.Empty{}, nil
}

// GetDownloadURL returns a presigned download URL.
func (s *Server) GetDownloadURL(ctx context.Context, req *pb.GetDownloadURLRequest) (*pb.GetDownloadURLResponse, error) {
	fileID, err := uuid.Parse(req.FileId)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid file_id")
	}

	ttlSeconds := int(req.TtlSeconds)
	if ttlSeconds <= 0 {
		ttlSeconds = 3600 // default 1 hour
	}

	output, err := s.getDownloadURLUC.Execute(ctx, fileID, ttlSeconds)
	if err != nil {
		return nil, mapUseCaseError(err)
	}

	return &pb.GetDownloadURLResponse{
		Url:       output.URL,
		ExpiresAt: timestamppb.New(output.ExpiresAt),
	}, nil
}

// fileToProto converts a domain File to a protobuf FileResponse.
func fileToProto(f *model.File) *pb.FileResponse {
	status := pb.FileStatus_FILE_STATUS_READY
	switch f.Status {
	case model.FileStatusPending:
		status = pb.FileStatus_FILE_STATUS_PENDING
	case model.FileStatusFailed:
		status = pb.FileStatus_FILE_STATUS_UNSPECIFIED
	}

	return &pb.FileResponse{
		Id:         f.ID.String(),
		UserId:     f.UserID.String(),
		Name:       f.Name,
		StorageKey: f.StorageKey,
		MimeType:   f.MimeType,
		Size:       f.Size,
		Status:     status,
		CreatedAt:  timestamppb.New(f.CreatedAt),
		UpdatedAt:  timestamppb.New(f.UpdatedAt),
	}
}

// mapUseCaseError converts a use case error to a gRPC status error.
func mapUseCaseError(err error) error {
	switch {
	case isError(err, usecase.ErrFileNotFound):
		return status.Error(codes.NotFound, "file not found")
	case isError(err, usecase.ErrInvalidName):
		return status.Error(codes.InvalidArgument, "filename is required")
	case isError(err, usecase.ErrInvalidSize):
		return status.Error(codes.InvalidArgument, "size must be positive")
	case isError(err, usecase.ErrInvalidMimeType):
		return status.Error(codes.InvalidArgument, "mime_type is required")
	case isError(err, usecase.ErrInvalidPageToken):
		return status.Error(codes.InvalidArgument, "invalid page token")
	case isError(err, usecase.ErrFileNotPending):
		return status.Error(codes.FailedPrecondition, "file is not in pending state")
	case isError(err, usecase.ErrFileNotReady):
		return status.Error(codes.FailedPrecondition, "file is not yet ready")
	case isError(err, usecase.ErrObjectNotExists):
		return status.Error(codes.NotFound, "upload not yet completed")
	default:
		return status.Error(codes.Internal, fmt.Sprintf("internal error: %v", err))
	}
}

// isError checks if err matches target using errors.Is.
func isError(err, target error) bool {
	return err == target
}
