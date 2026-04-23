package middleware

import (
	"time"

	"github.com/deannos/notification-queue/logger"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

const CtxRequestID = "requestID"

// RequestID generates a unique ID per request and sets it on the context and response header.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader("X-Request-Id")
		if id == "" {
			id = uuid.NewString()
		}
		c.Set(CtxRequestID, id)
		c.Header("X-Request-Id", id)
		c.Next()
	}
}

// ZapLogger logs each request with method, path, status, latency, and request ID.
func ZapLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		reqID, _ := c.Get(CtxRequestID)
		logger.L.Info("request",
			zap.String("request_id", reqID.(string)),
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.Int("status", c.Writer.Status()),
			zap.Duration("latency", time.Since(start)),
			zap.String("client_ip", c.ClientIP()),
			zap.Int("bytes", c.Writer.Size()),
		)
	}
}
