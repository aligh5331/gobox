// Package middleware provides Echo HTTP middleware and utilities.
package middleware

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ErrorResponse is the standard error envelope for all API error responses.
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail contains the error code and human-readable message.
type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// MapGRPCError converts a gRPC error into an echo.HTTPError with the
// standard error envelope and appropriate HTTP status code.
func MapGRPCError(err error) *echo.HTTPError {
	if err == nil {
		return nil
	}

	st, ok := status.FromError(err)
	if !ok {
		// Not a gRPC error; treat as internal.
		return echo.NewHTTPError(http.StatusInternalServerError, ErrorResponse{
			Error: ErrorDetail{Code: "INTERNAL_ERROR", Message: "internal server error"},
		})
	}

	httpStatus, code := grpcToHTTP(st.Code())
	msg := st.Message()
	if st.Code() == codes.Internal {
		msg = "internal server error"
	}
	return echo.NewHTTPError(httpStatus, ErrorResponse{
		Error: ErrorDetail{Code: code, Message: msg},
	})
}

// NewHTTPError creates a plain HTTP error (not from gRPC) with the standard envelope.
func NewHTTPError(statusCode int, code, message string) *echo.HTTPError {
	return echo.NewHTTPError(statusCode, ErrorResponse{
		Error: ErrorDetail{Code: code, Message: message},
	})
}

// HTTPErrorHandler is Echo's global error handler that ensures all errors
// are returned in the standard envelope.
func HTTPErrorHandler(err error, c echo.Context) {
	if c.Response().Committed {
		return
	}

	var he *echo.HTTPError
	if errors.As(err, &he) {
		if he.Internal != nil {
			if httpErr, ok := he.Internal.(*echo.HTTPError); ok {
				he = httpErr
			}
		}
	} else {
		he = echo.NewHTTPError(http.StatusInternalServerError, ErrorResponse{
			Error: ErrorDetail{Code: "INTERNAL_ERROR", Message: "internal server error"},
		})
	}

	resp, ok := he.Message.(ErrorResponse)
	if !ok {
		resp = ErrorResponse{
			Error: ErrorDetail{Code: "INTERNAL_ERROR", Message: "internal server error"},
		}
	}

	if !c.Response().Committed {
		c.JSON(he.Code, resp)
	}
}

func grpcToHTTP(c codes.Code) (int, string) {
	switch c {
	case codes.NotFound:
		return http.StatusNotFound, "NOT_FOUND"
	case codes.Unauthenticated:
		return http.StatusUnauthorized, "UNAUTHORIZED"
	case codes.AlreadyExists:
		return http.StatusConflict, "CONFLICT"
	case codes.InvalidArgument:
		return http.StatusBadRequest, "BAD_REQUEST"
	case codes.PermissionDenied:
		return http.StatusForbidden, "FORBIDDEN"
	case codes.FailedPrecondition:
		return http.StatusBadRequest, "FAILED_PRECONDITION"
	case codes.Internal:
		return http.StatusInternalServerError, "INTERNAL_ERROR"
	default:
		return http.StatusInternalServerError, "INTERNAL_ERROR"
	}
}
