package handlers

import (
	"net/http"
	"time"

	"github.com/deannos/notification-queue/auth"
	"github.com/deannos/notification-queue/middleware"
	"github.com/deannos/notification-queue/models"
	"github.com/deannos/notification-queue/storage"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type createAppRequest struct {
	Name        string `json:"name"        binding:"required,min=1,max=100"`
	Description string `json:"description" binding:"max=255"`
	WebhookURL  string `json:"webhook_url" binding:"omitempty,url,max=500"`
}

type updateAppRequest struct {
	Name        string `json:"name"        binding:"max=100"`
	Description string `json:"description" binding:"max=255"`
	WebhookURL  string `json:"webhook_url" binding:"omitempty,url,max=500"`
}

func ListApps(apps storage.AppRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetString(middleware.CtxUserID)

		list, err := apps.ListByUser(c.Request.Context(), userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
			return
		}

		for i := range list {
			list[i].Token = ""
		}
		c.JSON(http.StatusOK, list)
	}
}

func CreateApp(apps storage.AppRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetString(middleware.CtxUserID)

		var req createAppRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		plainToken, err := auth.GenerateAppToken()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
			return
		}

		app := models.App{
			ID:          uuid.NewString(),
			UserID:      userID,
			Name:        req.Name,
			Description: req.Description,
			WebhookURL:  req.WebhookURL,
			TokenPrefix: auth.TokenPrefix(plainToken),
			TokenHash:   auth.HashToken(plainToken),
			Token:       plainToken,
			CreatedAt:   time.Now(),
		}

		if err := apps.Create(c.Request.Context(), &app); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create app"})
			return
		}

		c.JSON(http.StatusCreated, app)
	}
}

func UpdateApp(apps storage.AppRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetString(middleware.CtxUserID)
		appID := c.Param("id")

		app, err := apps.FindByOwner(c.Request.Context(), appID, userID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "app not found"})
			return
		}

		var req updateAppRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		updates := map[string]any{}
		if req.Name != "" {
			updates["name"] = req.Name
		}
		if req.Description != "" {
			updates["description"] = req.Description
		}
		if req.WebhookURL != "" {
			updates["webhook_url"] = req.WebhookURL
		}

		if err := apps.Update(c.Request.Context(), app, updates); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update app"})
			return
		}

		app.Token = ""
		c.JSON(http.StatusOK, app)
	}
}

func DeleteApp(apps storage.AppRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetString(middleware.CtxUserID)
		appID := c.Param("id")

		if _, err := apps.FindByOwner(c.Request.Context(), appID, userID); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "app not found"})
			return
		}

		if err := apps.Delete(c.Request.Context(), appID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete app"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "app deleted"})
	}
}

func RotateToken(apps storage.AppRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetString(middleware.CtxUserID)
		appID := c.Param("id")

		app, err := apps.FindByOwner(c.Request.Context(), appID, userID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "app not found"})
			return
		}

		plainToken, err := auth.GenerateAppToken()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
			return
		}

		updates := map[string]any{
			"token":        auth.HashToken(plainToken),
			"token_prefix": auth.TokenPrefix(plainToken),
		}
		if err := apps.Update(c.Request.Context(), app, updates); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to rotate token"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"token": plainToken, "app_id": app.ID})
	}
}
