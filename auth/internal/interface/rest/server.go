// Package rest provides HTTP endpoints for JWKS and health check.
package rest

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog"

	"github.com/aligh5331/gobox/auth/pkg/jwtutil"
)

// Server serves HTTP endpoints using Echo.
type Server struct {
	e          *echo.Echo
	keyManager *jwtutil.KeyManager
	logger     zerolog.Logger
}

// NewServer creates a new REST server with Echo.
func NewServer(keyManager *jwtutil.KeyManager, logger zerolog.Logger) *Server {
	s := &Server{
		e:          echo.New(),
		keyManager: keyManager,
		logger:     logger,
	}

	s.setupRoutes()
	return s
}

// Start begins listening on the given address.
func (s *Server) Start(addr string) error {
	return s.e.Start(addr)
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.e.Shutdown(ctx)
}

func (s *Server) setupRoutes() {
	s.e.GET("/.well-known/jwks.json", s.handleJWKS)
	s.e.GET("/auth/v1/.well-known/jwks.json", s.handleJWKS)
	s.e.GET("/health", s.handleHealth)
}

func (s *Server) handleJWKS(c echo.Context) error {
	jwks := s.keyManager.JWKS()
	data, err := json.Marshal(jwks)
	if err != nil {
		s.logger.Error().Err(err).Msg("failed to marshal JWKS")
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal error"})
	}

	callback := c.QueryParam("callback")
	if callback != "" {
		return c.JSONP(http.StatusOK, callback, jwks)
	}

	return c.JSONBlob(http.StatusOK, data)
}

func (s *Server) handleHealth(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}
