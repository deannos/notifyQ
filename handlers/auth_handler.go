package handlers

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/deannos/notification-queue/auth"
	"github.com/deannos/notification-queue/config"
	"github.com/deannos/notification-queue/models"
	"github.com/deannos/notification-queue/storage"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type loginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type registerRequest struct {
	Username string `json:"username" binding:"required,min=3,max=50"`
	Password string `json:"password" binding:"required,min=6"`
}

func Login(users storage.UserRepository, cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req loginRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		user, err := users.FindByUsername(c.Request.Context(), req.Username)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid username or password"})
			return
		}

		if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid username or password"})
			return
		}

		token, err := auth.GenerateToken(user.ID, user.IsAdmin, cfg.JWTSecret, cfg.JWTExpiryHours)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"token":      token,
			"user":       user,
			"expires_in": cfg.JWTExpiryHours * 3600,
		})
	}
}

func Register(users storage.UserRepository, cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !cfg.AllowRegistration {
			count, _ := users.Count(c.Request.Context())
			if count > 0 {
				c.JSON(http.StatusForbidden, gin.H{"error": "registration is disabled"})
				return
			}
		}

		var req registerRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to hash password"})
			return
		}

		u := &models.User{
			ID:        uuid.NewString(),
			Username:  req.Username,
			Password:  string(hash),
			CreatedAt: time.Now(),
		}

		isAdmin, err := users.CreateFirstAdmin(c.Request.Context(), u)
		if err != nil {
			if strings.Contains(err.Error(), "UNIQUE") {
				c.JSON(http.StatusConflict, gin.H{"error": "username already taken"})
			} else {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create user"})
			}
			return
		}

		c.JSON(http.StatusCreated, gin.H{
			"message":  "user created",
			"username": req.Username,
			"is_admin": isAdmin,
		})
	}
}

// EnsureAdminUser creates the default admin account if no users exist.
func EnsureAdminUser(users storage.UserRepository, username, password string) error {
	ctx := context.Background()
	count, err := users.Count(ctx)
	if err != nil || count > 0 {
		return err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	return users.Create(ctx, &models.User{
		ID:        uuid.NewString(),
		Username:  username,
		Password:  string(hash),
		IsAdmin:   true,
		CreatedAt: time.Now(),
	})
}
