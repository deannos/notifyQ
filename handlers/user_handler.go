package handlers

import (
	"net/http"
	"time"

	"github.com/deannos/notification-queue/config"
	"github.com/deannos/notification-queue/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type createUserRequest struct {
	Username string `json:"username" binding:"required,min=3,max=50"`
	Password string `json:"password" binding:"required,min=6"`
	IsAdmin  bool   `json:"is_admin"`
}

type changePasswordRequest struct {
	Password string `json:"password" binding:"required,min=6"`
}

func ListUsers(database *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var users []models.User
		if err := database.Find(&users).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
			return
		}
		c.JSON(http.StatusOK, users)
	}
}

func CreateUser(database *gorm.DB, cfg *config.Config) gin.HandlerFunc {
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

		if err := database.Create(&user).Error; err != nil {
			c.JSON(http.StatusConflict, gin.H{"error": "username already taken"})
			return
		}

		c.JSON(http.StatusCreated, user)
	}
}

func DeleteUser(database *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.Param("id")

		var user models.User
		if err := database.First(&user, "id = ?", userID).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}

		// Remove user's apps and their notifications first.
		var appIDs []string
		database.Model(&models.App{}).Where("user_id = ?", userID).Pluck("id", &appIDs)
		if len(appIDs) > 0 {
			database.Unscoped().Where("app_id IN ?", appIDs).Delete(&models.Notification{})
		}
		database.Where("user_id = ?", userID).Delete(&models.App{})
		database.Delete(&user)

		c.JSON(http.StatusOK, gin.H{"message": "user deleted"})
	}
}

func ChangePassword(database *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.Param("id")

		var user models.User
		if err := database.First(&user, "id = ?", userID).Error; err != nil {
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

		database.Model(&user).Update("password", string(hash))
		c.JSON(http.StatusOK, gin.H{"message": "password changed"})
	}
}
