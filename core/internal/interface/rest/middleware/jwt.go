package middleware

import (
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog"

	"github.com/aligh5331/gobox/core/pkg/jwtutil"
)

// JWTAuth returns an Echo middleware that validates the Bearer token in the
// Authorization header using the JWKSCache and injects user_id and session_id
// into the request context.
func JWTAuth(jwks *jwtutil.JWKSCache, log zerolog.Logger) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			authHeader := c.Request().Header.Get("Authorization")
			if authHeader == "" {
				return NewHTTPError(401, "UNAUTHORIZED", "missing authorization header")
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				return NewHTTPError(401, "UNAUTHORIZED", "invalid authorization header format")
			}

			claims, err := jwks.ParseAndValidate(parts[1])
			if err != nil {
				log.Debug().Err(err).Msg("jwt: token validation failed")
				return NewHTTPError(401, "UNAUTHORIZED", "invalid or expired token")
			}

			c.Set("user_id", claims.Subject)
			c.Set("session_id", claims.SID)

			return next(c)
		}
	}
}

// GetUserID extracts the user_id from the Echo context.
// Returns empty string if not found.
func GetUserID(c echo.Context) string {
	if v := c.Get("user_id"); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// GetSessionID extracts the session_id from the Echo context.
func GetSessionID(c echo.Context) string {
	if v := c.Get("session_id"); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
