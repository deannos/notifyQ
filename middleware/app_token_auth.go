package middleware

import (
	"net/http"

	"github.com/deannos/notification-queue/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const CtxApp = "app"

// AppTokenAuth validates the app token from the X-App-Token header or ?token= query param.
func AppTokenAuth(database *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.GetHeader("X-App-Token")
		if token == "" {
			token = c.Query("token")
		}
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing app token"})
			return
		}

		var app models.App
		if err := database.Where("token = ?", token).First(&app).Error; err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid app token"})
			return
		}

		c.Set(CtxApp, &app)
		c.Next()
	}
}
