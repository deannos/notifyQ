package handlers

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/deannos/notification-queue/hub"
	"github.com/deannos/notification-queue/middleware"
	"github.com/deannos/notification-queue/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type sendNotificationRequest struct {
	Title    string `json:"title"    binding:"required,max=255"`
	Message  string `json:"message"  binding:"required,max=4096"`
	Priority int    `json:"priority" binding:"min=0,max=10"`
}

type wsEvent struct {
	Event        string              `json:"event"`
	Notification *models.Notification `json:"notification"`
}

func SendNotification(database *gorm.DB, h *hub.Hub) gin.HandlerFunc {
	return func(c *gin.Context) {
		app := c.MustGet(middleware.CtxApp).(*models.App)

		var req sendNotificationRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if req.Priority == 0 {
			req.Priority = 5
		}

		notif := models.Notification{
			ID:        uuid.NewString(),
			AppID:     app.ID,
			Title:     req.Title,
			Message:   req.Message,
			Priority:  req.Priority,
			Read:      false,
			CreatedAt: time.Now(),
		}

		if err := database.Create(&notif).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save notification"})
			return
		}

		// Attach app info for the WebSocket payload.
		notif.App = app

		payload, _ := json.Marshal(wsEvent{Event: "notification", Notification: &notif})
		h.Send(app.UserID, payload)

		if app.WebhookURL != "" {
			go fireWebhook(app.WebhookURL, &notif)
		}

		c.JSON(http.StatusCreated, notif)
	}
}

func fireWebhook(url string, notif *models.Notification) {
	body, _ := json.Marshal(notif)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		log.Printf("webhook: build request error: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("webhook: delivery error for %s: %v", url, err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		log.Printf("webhook: non-2xx response %d from %s", resp.StatusCode, url)
	}
}

func ListNotifications(database *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetString(middleware.CtxUserID)

		limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
		offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
		if limit <= 0 || limit > 100 {
			limit = 20
		}
		if offset < 0 {
			offset = 0
		}

		// Optional filters
		appID   := c.Query("app_id")
		readStr := c.Query("read")
		prioStr := c.Query("priority")
		q       := c.Query("q")

		applyFilters := func(tx *gorm.DB) *gorm.DB {
			tx = tx.Joins("JOIN apps ON apps.id = notifications.app_id").
				Where("apps.user_id = ?", userID)
			if appID != "" {
				tx = tx.Where("notifications.app_id = ?", appID)
			}
			if readStr != "" {
				tx = tx.Where("notifications.read = ?", readStr == "true")
			}
			if prioStr != "" {
				if prio, err := strconv.Atoi(prioStr); err == nil {
					tx = tx.Where("notifications.priority = ?", prio)
				}
			}
			if q != "" {
				like := "%" + q + "%"
				tx = tx.Where("notifications.title LIKE ? OR notifications.message LIKE ?", like, like)
			}
			return tx
		}

		var total int64
		applyFilters(database.Model(&models.Notification{})).Count(&total)

		if total == 0 {
			c.JSON(http.StatusOK, gin.H{"notifications": []interface{}{}, "total": 0})
			return
		}

		var notifs []models.Notification
		if err := applyFilters(database.Preload("App")).
			Order("notifications.created_at DESC").
			Limit(limit).
			Offset(offset).
			Find(&notifs).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"notifications": notifs,
			"total":         total,
			"limit":         limit,
			"offset":        offset,
		})
	}
}

func GetNotification(database *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetString(middleware.CtxUserID)
		notifID := c.Param("id")

		var notif models.Notification
		err := database.
			Preload("App").
			First(&notif, "id = ?", notifID).Error
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "notification not found"})
			return
		}

		if notif.App == nil || notif.App.UserID != userID {
			c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
			return
		}

		c.JSON(http.StatusOK, notif)
	}
}

func MarkRead(database *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetString(middleware.CtxUserID)
		notifID := c.Param("id")

		var notif models.Notification
		if err := database.Preload("App").First(&notif, "id = ?", notifID).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "notification not found"})
			return
		}

		if notif.App == nil || notif.App.UserID != userID {
			c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
			return
		}

		database.Model(&notif).Update("read", true)
		c.JSON(http.StatusOK, gin.H{"message": "marked as read"})
	}
}

func DeleteNotification(database *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetString(middleware.CtxUserID)
		notifID := c.Param("id")

		var notif models.Notification
		if err := database.Preload("App").First(&notif, "id = ?", notifID).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "notification not found"})
			return
		}

		if notif.App == nil || notif.App.UserID != userID {
			c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
			return
		}

		database.Delete(&notif)
		c.JSON(http.StatusOK, gin.H{"message": "notification deleted"})
	}
}

func DeleteAllNotifications(database *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetString(middleware.CtxUserID)

		var appIDs []string
		database.Model(&models.App{}).Where("user_id = ?", userID).Pluck("id", &appIDs)

		if len(appIDs) > 0 {
			database.Where("app_id IN ?", appIDs).Delete(&models.Notification{})
		}

		c.JSON(http.StatusOK, gin.H{"message": "all notifications deleted"})
	}
}
