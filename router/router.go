package router

import (
	"io/fs"
	"net/http"

	"github.com/deannos/notification-queue/config"
	"github.com/deannos/notification-queue/handlers"
	"github.com/deannos/notification-queue/hub"
	"github.com/deannos/notification-queue/middleware"
	"github.com/deannos/notification-queue/web"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func Setup(database *gorm.DB, h *hub.Hub, cfg *config.Config) *gin.Engine {
	r := gin.Default()

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
	r.GET("/health", handlers.HealthHandler())

	// --- Auth (public) ---
	r.POST("/auth/login", handlers.Login(database, cfg))
	r.POST("/auth/register", handlers.Register(database, cfg))

	// --- App-token authenticated (send notifications) ---
	appAuth := r.Group("/")
	appAuth.Use(middleware.AppTokenAuth(database))
	{
		appAuth.POST("/message", handlers.SendNotification(database, h))
	}

	// --- WebSocket (JWT via query param) ---
	r.GET("/ws", middleware.WSJWTAuth(cfg), handlers.WebSocketHandler(h))

	// --- User-authenticated API ---
	api := r.Group("/api/v1")
	api.Use(middleware.JWTAuth(cfg))
	{
		// Notifications
		api.GET("/notification", handlers.ListNotifications(database))
		api.GET("/notification/:id", handlers.GetNotification(database))
		api.PUT("/notification/:id/read", handlers.MarkRead(database))
		api.DELETE("/notification/:id", handlers.DeleteNotification(database))
		api.DELETE("/notification", handlers.DeleteAllNotifications(database))

		// Applications
		api.GET("/application", handlers.ListApps(database))
		api.POST("/application", handlers.CreateApp(database))
		api.PUT("/application/:id", handlers.UpdateApp(database))
		api.DELETE("/application/:id", handlers.DeleteApp(database))
		api.POST("/application/:id/token", handlers.RotateToken(database))

		// Admin only
		admin := api.Group("/")
		admin.Use(middleware.AdminOnly())
		{
			admin.GET("/user", handlers.ListUsers(database))
			admin.POST("/user", handlers.CreateUser(database, cfg))
			admin.DELETE("/user/:id", handlers.DeleteUser(database))
			admin.PUT("/user/:id/password", handlers.ChangePassword(database))
		}
	}

	return r
}
