// Package rest wires Echo routes for the Core API.
package rest

import (
	"github.com/labstack/echo/v4"
)

// RegisterRoutes registers all Phase 2 routes (auth and /me endpoints).
// The jwtMiddleware is applied to authenticated routes only.
// Later phases extend this function with new handler parameters and route groups.
func RegisterRoutes(
	e *echo.Echo,
	authHandler *AuthHandler,
	meHandler *MeHandler,
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
}

// healthCheck returns a simple health status.
func healthCheck(c echo.Context) error {
	return c.JSON(200, map[string]string{"status": "ok"})
}
