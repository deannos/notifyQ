package handlers

import (
	"net/http"
	"time"

	"github.com/deannos/notification-queue/auth"
	"github.com/deannos/notification-queue/middleware"
	"github.com/deannos/notification-queue/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type createAppRequest struct {
	Name        string `json:"name"        binding:"required,min=1,max=100"`
	Description string `json:"description" binding:"max=255"`
}

type updateAppRequest struct {
	Name        string `json:"name"        binding:"max=100"`
	Description string `json:"description" binding:"max=255"`
}

func ListApps(database *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetString(middleware.CtxUserID)

		var apps []models.App
		if err := database.Where("user_id = ?", userID).Find(&apps).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
			return
		}

		// Hide tokens in list response for security.
		for i := range apps {
			apps[i].Token = ""
		}
		c.JSON(http.StatusOK, apps)
	}
}

func CreateApp(database *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetString(middleware.CtxUserID)

		var req createAppRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		token, err := auth.GenerateAppToken()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
			return
		}

		app := models.App{
			ID:          uuid.NewString(),
			UserID:      userID,
			Name:        req.Name,
			Description: req.Description,
			Token:       token,
			CreatedAt:   time.Now(),
		}

		if err := database.Create(&app).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create app"})
			return
		}

		// Token is returned once — subsequent list calls will omit it.
		c.JSON(http.StatusCreated, app)
	}
}

func UpdateApp(database *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetString(middleware.CtxUserID)
		appID := c.Param("id")

		var app models.App
		if err := database.Where("id = ? AND user_id = ?", appID, userID).First(&app).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "app not found"})
			return
		}

		var req updateAppRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		updates := map[string]interface{}{}
		if req.Name != "" {
			updates["name"] = req.Name
		}
		if req.Description != "" {
			updates["description"] = req.Description
		}

		if err := database.Model(&app).Updates(updates).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update app"})
			return
		}

		app.Token = ""
		c.JSON(http.StatusOK, app)
	}
}

func DeleteApp(database *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetString(middleware.CtxUserID)
		appID := c.Param("id")

		var app models.App
		if err := database.Where("id = ? AND user_id = ?", appID, userID).First(&app).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "app not found"})
			return
		}

		// Hard-delete associated notifications first.
		database.Unscoped().Where("app_id = ?", appID).Delete(&models.Notification{})

		if err := database.Delete(&app).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete app"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "app deleted"})
	}
}

func RotateToken(database *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetString(middleware.CtxUserID)
		appID := c.Param("id")

		var app models.App
		if err := database.Where("id = ? AND user_id = ?", appID, userID).First(&app).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "app not found"})
			return
		}

		token, err := auth.GenerateAppToken()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
			return
		}

		if err := database.Model(&app).Update("token", token).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to rotate token"})
			return
		}

		app.Token = token
		c.JSON(http.StatusOK, gin.H{"token": app.Token, "app_id": app.ID})
	}
}
