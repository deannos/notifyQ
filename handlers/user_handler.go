package handlers

import (
	"net/http"
	"strings"
	"time"

	"github.com/deannos/notification-queue/config"
	"github.com/deannos/notification-queue/models"
	"github.com/deannos/notification-queue/storage"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type createUserRequest struct {
	Username string `json:"username" binding:"required,min=3,max=50"`
	Password string `json:"password" binding:"required,min=6"`
	IsAdmin  bool   `json:"is_admin"`
}

type changePasswordRequest struct {
	Password string `json:"password" binding:"required,min=6"`
}

func ListUsers(users storage.UserRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		list, err := users.List(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
			return
		}
		c.JSON(http.StatusOK, list)
	}
}

func CreateUser(users storage.UserRepository, cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req createUserRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to hash password"})
			return
		}

		user := models.User{
			ID:        uuid.NewString(),
			Username:  req.Username,
			Password:  string(hash),
			IsAdmin:   req.IsAdmin,
			CreatedAt: time.Now(),
		}

		if err := users.Create(c.Request.Context(), &user); err != nil {
			if strings.Contains(err.Error(), "UNIQUE") {
				c.JSON(http.StatusConflict, gin.H{"error": "username already taken"})
			} else {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create user"})
			}
			return
		}

		c.JSON(http.StatusCreated, user)
	}
}

func DeleteUser(users storage.UserRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.Param("id")

		if _, err := users.FindByID(c.Request.Context(), userID); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}

		if err := users.Delete(c.Request.Context(), userID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete user"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "user deleted"})
	}
}

func ChangePassword(users storage.UserRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.Param("id")

		if _, err := users.FindByID(c.Request.Context(), userID); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}

		var req changePasswordRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to hash password"})
			return
		}

		if err := users.UpdatePassword(c.Request.Context(), userID, string(hash)); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update password"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "password changed"})
	}
}
