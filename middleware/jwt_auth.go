package middleware

import (
	"net/http"
	"strings"

	"github.com/deannos/notification-queue/auth"
	"github.com/deannos/notification-queue/config"
	"github.com/gin-gonic/gin"
)

const (
	CtxUserID  = "userID"
	CtxIsAdmin = "isAdmin"
)

// JWTAuth validates the Bearer JWT and injects user claims into the context.
func JWTAuth(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := ExtractBearerToken(c)
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing or invalid authorization header"})
			return
		}

		claims, err := auth.ParseToken(token, cfg.JWTSecret)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired token"})
			return
		}

		c.Set(CtxUserID, claims.UserID)
		c.Set(CtxIsAdmin, claims.IsAdmin)
		c.Next()
	}
}

// AdminOnly requires JWTAuth to have run first and rejects non-admin users.
func AdminOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		isAdmin, _ := c.Get(CtxIsAdmin)
		if admin, ok := isAdmin.(bool); !ok || !admin {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "admin access required"})
			return
		}
		c.Next()
	}
}

// ExtractBearerToken returns the token from an "Authorization: Bearer <token>" header,
// or an empty string if the header is absent or malformed.
func ExtractBearerToken(c *gin.Context) string {
	header := c.GetHeader("Authorization")
	if header == "" {
		return ""
	}
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		return ""
	}
	return parts[1]
}
