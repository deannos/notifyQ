package handlers

import (
	"net/http"
	"strings"

	"github.com/deannos/notification-queue/auth"
	"github.com/deannos/notification-queue/config"
	"github.com/deannos/notification-queue/hub"
	"github.com/deannos/notification-queue/middleware"
	"github.com/deannos/notification-queue/storage"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

func newUpgrader(cfg *config.Config) websocket.Upgrader {
	allowed := strings.Split(cfg.AllowedOrigins, ",")
	allowAll := len(allowed) == 1 && strings.TrimSpace(allowed[0]) == "*"
	return websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			if allowAll {
				return true
			}
			origin := r.Header.Get("Origin")
			for _, o := range allowed {
				if strings.TrimSpace(o) == origin {
					return true
				}
			}
			return false
		},
	}
}

func WebSocketHandler(h *hub.Hub, tickets *hub.TicketStore, cfg *config.Config) gin.HandlerFunc {
	upgrader := newUpgrader(cfg)
	return func(c *gin.Context) {
		userID, ok := resolveWSUser(c, tickets, cfg)
		if !ok {
			return
		}

		conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			return
		}

		client := h.NewClient(userID, conn)
		go client.WritePump()
		client.ReadPump()
	}
}

// resolveWSUser authenticates a WebSocket request via one-time ticket or JWT.
// Returns the userID and true on success; writes the error response and returns false on failure.
func resolveWSUser(c *gin.Context, tickets *hub.TicketStore, cfg *config.Config) (string, bool) {
	if ticket := c.Query("ticket"); ticket != "" {
		id, ok := tickets.Consume(ticket)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired ticket"})
			return "", false
		}
		return id, true
	}

	// Fall back to JWT — accept ?token= (WebSocket) or Authorization: Bearer.
	token := c.Query("token")
	if token == "" {
		token = middleware.ExtractBearerToken(c)
	}
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing token or ticket"})
		return "", false
	}

	claims, err := auth.ParseToken(token, cfg.JWTSecret)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired token"})
		return "", false
	}

	c.Set(middleware.CtxUserID, claims.UserID)
	c.Set(middleware.CtxIsAdmin, claims.IsAdmin)
	return claims.UserID, true
}

func IssueWSTicket(tickets *hub.TicketStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetString(middleware.CtxUserID)
		ticket, err := tickets.Issue(userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to issue ticket"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ticket": ticket})
	}
}

func HealthHandler(notifs storage.NotificationRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := notifs.Ping(c.Request.Context()); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "degraded", "reason": "database unavailable"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	}
}
