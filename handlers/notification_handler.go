package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/deannos/notification-queue/logger"
	"github.com/deannos/notification-queue/middleware"
	"github.com/deannos/notification-queue/models"
	"github.com/deannos/notification-queue/storage"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type sendNotificationRequest struct {
	Title    string `json:"title"    binding:"required,max=255"`
	Message  string `json:"message"  binding:"required,max=4096"`
	Priority int    `json:"priority" binding:"min=0,max=10"`
}

type wsEvent struct {
	Event        string               `json:"event"`
	Notification *models.Notification `json:"notification"`
}

func SendNotification(notifs storage.NotificationRepository, pub storage.NotificationPublisher) gin.HandlerFunc {
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

		if err := notifs.Create(c.Request.Context(), &notif); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save notification"})
			return
		}

		notif.App = app
		payload, _ := json.Marshal(wsEvent{Event: "notification", Notification: &notif})
		pub.Publish(c.Request.Context(), app.UserID, payload)

		if app.WebhookURL != "" {
			go fireWebhook(app.WebhookURL, &notif)
		}

		c.JSON(http.StatusCreated, notif)
	}
}

func fireWebhook(rawURL string, notif *models.Notification) {
	if err := validateWebhookURL(rawURL); err != nil {
		logger.L.Warn("webhook blocked", zap.String("url", rawURL), zap.Error(err))
		return
	}

	body, _ := json.Marshal(notif)
	req, err := http.NewRequest(http.MethodPost, rawURL, bytes.NewReader(body))
	if err != nil {
		logger.L.Error("webhook build request failed", zap.String("url", rawURL), zap.Error(err))
		return
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		logger.L.Warn("webhook delivery failed", zap.String("url", rawURL), zap.Error(err))
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		logger.L.Warn("webhook non-2xx response",
			zap.String("url", rawURL),
			zap.Int("status", resp.StatusCode),
		)
	}
}

// validateWebhookURL resolves the hostname and rejects private/loopback/link-local
// addresses to prevent SSRF attacks.
func validateWebhookURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("scheme %q not allowed", u.Scheme)
	}
	host := u.Hostname()
	addrs, err := net.LookupHost(host)
	if err != nil {
		return fmt.Errorf("dns lookup failed: %w", err)
	}
	for _, addr := range addrs {
		ip := net.ParseIP(addr)
		if ip == nil {
			continue
		}
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return fmt.Errorf("destination %s resolves to a private/internal address", host)
		}
	}
	return nil
}

func ListNotifications(notifs storage.NotificationRepository) gin.HandlerFunc {
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

		f := storage.NotificationFilter{
			AppID:  c.Query("app_id"),
			Query:  c.Query("q"),
			Limit:  limit,
			Offset: offset,
		}
		if readStr := c.Query("read"); readStr != "" {
			b := readStr == "true"
			f.Read = &b
		}
		if prioStr := c.Query("priority"); prioStr != "" {
			if p, err := strconv.Atoi(prioStr); err == nil {
				f.Priority = &p
			}
		}

		list, total, err := notifs.List(c.Request.Context(), userID, f)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
			return
		}
		if list == nil {
			list = []models.Notification{}
		}

		c.JSON(http.StatusOK, gin.H{
			"notifications": list,
			"total":         total,
			"limit":         limit,
			"offset":        offset,
		})
	}
}

// fetchOwnedNotification fetches a notification by ID and verifies it belongs to userID.
// On failure it writes the error response and returns nil.
func fetchOwnedNotification(c *gin.Context, notifs storage.NotificationRepository, notifID, userID string) *models.Notification {
	notif, err := notifs.FindByID(c.Request.Context(), notifID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "notification not found"})
		return nil
	}
	if notif.App == nil || notif.App.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return nil
	}
	return notif
}

func GetNotification(notifs storage.NotificationRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetString(middleware.CtxUserID)
		notif := fetchOwnedNotification(c, notifs, c.Param("id"), userID)
		if notif == nil {
			return
		}
		c.JSON(http.StatusOK, notif)
	}
}

func MarkRead(notifs storage.NotificationRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetString(middleware.CtxUserID)
		notifID := c.Param("id")

		if fetchOwnedNotification(c, notifs, notifID, userID) == nil {
			return
		}

		if err := notifs.MarkRead(c.Request.Context(), notifID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "marked as read"})
	}
}

func DeleteNotification(notifs storage.NotificationRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetString(middleware.CtxUserID)
		notifID := c.Param("id")

		if fetchOwnedNotification(c, notifs, notifID, userID) == nil {
			return
		}

		if err := notifs.Delete(c.Request.Context(), notifID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "notification deleted"})
	}
}

func DeleteAllNotifications(apps storage.AppRepository, notifs storage.NotificationRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetString(middleware.CtxUserID)

		appIDs, err := apps.IDsByUser(c.Request.Context(), userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
			return
		}

		if err := notifs.DeleteByAppIDs(c.Request.Context(), appIDs); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "all notifications deleted"})
	}
}
