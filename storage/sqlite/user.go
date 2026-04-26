package sqlite

import (
	"context"

	"github.com/deannos/notification-queue/models"
	"gorm.io/gorm"
)

type UserRepo struct{ db *gorm.DB }

func NewUserRepo(db *gorm.DB) *UserRepo { return &UserRepo{db: db} }

func (r *UserRepo) Create(ctx context.Context, u *models.User) error {
	return r.db.WithContext(ctx).Create(u).Error
}

// CreateFirstAdmin atomically checks whether any users exist. If none, the
// new user is created as admin. Returns whether the admin role was granted.
func (r *UserRepo) CreateFirstAdmin(ctx context.Context, u *models.User) (bool, error) {
	var isAdmin bool
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&models.User{}).Count(&count).Error; err != nil {
			return err
		}
		isAdmin = count == 0
		u.IsAdmin = isAdmin
		return tx.Create(u).Error
	})
	return isAdmin, err
}

func (r *UserRepo) FindByUsername(ctx context.Context, username string) (*models.User, error) {
	var u models.User
	if err := r.db.WithContext(ctx).Where("username = ?", username).First(&u).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *UserRepo) FindByID(ctx context.Context, id string) (*models.User, error) {
	var u models.User
	if err := r.db.WithContext(ctx).First(&u, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *UserRepo) List(ctx context.Context) ([]models.User, error) {
	var users []models.User
	return users, r.db.WithContext(ctx).Find(&users).Error
}

func (r *UserRepo) Count(ctx context.Context) (int64, error) {
	var n int64
	return n, r.db.WithContext(ctx).Model(&models.User{}).Count(&n).Error
}

func (r *UserRepo) UpdatePassword(ctx context.Context, id, hash string) error {
	return r.db.WithContext(ctx).Model(&models.User{}).Where("id = ?", id).Update("password", hash).Error
}

// Delete removes the user and all their apps and notifications in one transaction.
func (r *UserRepo) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var appIDs []string
		if err := tx.Model(&models.App{}).Where("user_id = ?", id).Pluck("id", &appIDs).Error; err != nil {
			return err
		}
		if len(appIDs) > 0 {
			if err := tx.Unscoped().Where("app_id IN ?", appIDs).Delete(&models.Notification{}).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("user_id = ?", id).Delete(&models.App{}).Error; err != nil {
			return err
		}
		return tx.Delete(&models.User{}, "id = ?", id).Error
	})
}
