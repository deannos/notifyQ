package router

import (
	"context"
	"io/fs"
	"net/http"
	"strings"

	"github.com/deannos/notification-queue/config"
	"github.com/deannos/notification-queue/handlers"
	"github.com/deannos/notification-queue/hub"
	"github.com/deannos/notification-queue/middleware"
	"github.com/deannos/notification-queue/storage"
	"github.com/deannos/notification-queue/web"
	"github.com/gin-gonic/gin"
)

func Setup(
	ctx context.Context,
	users storage.UserRepository,
	apps storage.AppRepository,
	notifs storage.NotificationRepository,
	pub storage.NotificationPublisher,
	h *hub.Hub,
	tickets *hub.TicketStore,
	cfg *config.Config,
) *gin.Engine {
	r := gin.New()

	// Restrict which upstream IPs may set X-Forwarded-For so that ClientIP()
	// and the rate limiter cannot be spoofed by arbitrary clients.
	if cfg.TrustedProxies == "" {
		_ = r.SetTrustedProxies(nil)
	} else {
		proxies := strings.Split(cfg.TrustedProxies, ",")
		for i, p := range proxies {
			proxies[i] = strings.TrimSpace(p)
		}
		_ = r.SetTrustedProxies(proxies)
	}

	r.Use(gin.Recovery(), middleware.RequestID(), middleware.ZapLogger())

	r.Use(func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 5<<20)
		c.Next()
	})

	// --- Static web UI ---
	staticFS, err := fs.Sub(web.StaticFiles, "static")
	if err != nil {
		panic("web embed not found: " + err.Error())
	}
	r.StaticFS("/static", http.FS(staticFS))
	r.GET("/", func(c *gin.Context) {
		data, err := web.StaticFiles.ReadFile("static/index.html")
		if err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", data)
	})

	// --- Health ---
	r.GET("/health", handlers.HealthHandler(notifs))

	// --- Auth (public, 5 req/min per IP) ---
	authLimiter := middleware.RateLimit(ctx, 5.0/60, 5)
	r.POST("/auth/login", authLimiter, handlers.Login(users, cfg))
	r.POST("/auth/register", authLimiter, handlers.Register(users, cfg))

	// --- App-token authenticated ---
	appAuth := r.Group("/")
	appAuth.Use(middleware.AppTokenAuth(apps), middleware.RateLimit(ctx, 1, 60))
	{
		appAuth.POST("/message", handlers.SendNotification(notifs, pub))
	}

	// --- WebSocket --- auth is handled inside the handler (ticket or JWT)
	r.GET("/ws", handlers.WebSocketHandler(h, tickets, cfg))

	// --- User-authenticated API ---
	api := r.Group("/api/v1")
	api.Use(middleware.JWTAuth(cfg))
	{
		api.GET("/ws/ticket", handlers.IssueWSTicket(tickets))

		api.GET("/notification", handlers.ListNotifications(notifs))
		api.GET("/notification/:id", handlers.GetNotification(notifs))
		api.PUT("/notification/:id/read", handlers.MarkRead(notifs))
		api.DELETE("/notification/:id", handlers.DeleteNotification(notifs))
		api.DELETE("/notification", handlers.DeleteAllNotifications(apps, notifs))

		api.GET("/application", handlers.ListApps(apps))
		api.POST("/application", handlers.CreateApp(apps))
		api.PUT("/application/:id", handlers.UpdateApp(apps))
		api.DELETE("/application/:id", handlers.DeleteApp(apps))
		api.POST("/application/:id/token", handlers.RotateToken(apps))

		admin := api.Group("/")
		admin.Use(middleware.AdminOnly())
		{
			admin.GET("/user", handlers.ListUsers(users))
			admin.POST("/user", handlers.CreateUser(users, cfg))
			admin.DELETE("/user/:id", handlers.DeleteUser(users))
			admin.PUT("/user/:id/password", handlers.ChangePassword(users))
		}
	}

	return r
}
