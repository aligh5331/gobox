// Package rest wires Echo routes for the Core API.
package rest

import (
	"github.com/labstack/echo/v4"

	"github.com/aligh5331/gobox/core/internal/interface/rest/handler"
)

// RegisterRoutes registers all routes for the Core API.
// The jwtMiddleware is applied to authenticated routes only.
func RegisterRoutes(
	e *echo.Echo,
	authHandler *AuthHandler,
	meHandler *MeHandler,
	fileHandler *handler.FileHandler,
	shareHandler *handler.ShareHandler,
	jwtMiddleware echo.MiddlewareFunc,
) {
	// Public group — no JWT required.
	public := e.Group("")
	public.POST("/api/v1/auth/register", authHandler.Register)
	public.POST("/api/v1/auth/login", authHandler.Login)
	public.POST("/api/v1/auth/refresh", authHandler.Refresh)
	public.GET("/health", healthCheck)

	// Authenticated group — JWT middleware applied.
	authed := e.Group("")
	authed.Use(jwtMiddleware)
	authed.DELETE("/api/v1/auth/logout", authHandler.Logout)
	authed.GET("/api/v1/me", meHandler.GetMe)
	authed.PUT("/api/v1/me", meHandler.UpdateMe)
	authed.PUT("/api/v1/me/password", meHandler.ChangePassword)

	// File routes.
	authed.POST("/api/v1/files", fileHandler.InitiateUpload)
	authed.POST("/api/v1/files/:id/confirm", fileHandler.ConfirmUpload)
	authed.GET("/api/v1/files", fileHandler.ListFiles)
	authed.GET("/api/v1/files/:id", fileHandler.GetFile)
	authed.DELETE("/api/v1/files/:id", fileHandler.DeleteFile)
	authed.GET("/api/v1/files/:id/download", fileHandler.GetDownloadURL)

	// Share (short link) routes.
	authed.POST("/api/v1/files/:id/share", shareHandler.CreateShare)
	authed.GET("/api/v1/files/:id/links", shareHandler.ListLinks)
	authed.DELETE("/api/v1/links/:link_id", shareHandler.DeleteLink)
}

// healthCheck returns a simple health status.
func healthCheck(c echo.Context) error {
	return c.JSON(200, map[string]string{"status": "ok"})
}
