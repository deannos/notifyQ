package sqlite

import (
	"context"

	"github.com/deannos/notification-queue/models"
	"gorm.io/gorm"
)

type AppRepo struct{ db *gorm.DB }

func NewAppRepo(db *gorm.DB) *AppRepo { return &AppRepo{db: db} }

func (r *AppRepo) Create(ctx context.Context, a *models.App) error {
	return r.db.WithContext(ctx).Create(a).Error
}

func (r *AppRepo) FindByOwner(ctx context.Context, id, userID string) (*models.App, error) {
	var a models.App
	if err := r.db.WithContext(ctx).Where("id = ? AND user_id = ?", id, userID).First(&a).Error; err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *AppRepo) ListByUser(ctx context.Context, userID string) ([]models.App, error) {
	var apps []models.App
	return apps, r.db.WithContext(ctx).Where("user_id = ?", userID).Find(&apps).Error
}

func (r *AppRepo) Update(ctx context.Context, a *models.App, updates map[string]any) error {
	return r.db.WithContext(ctx).Model(a).Updates(updates).Error
}

// Delete hard-deletes the app's notifications then the app itself in one transaction.
func (r *AppRepo) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Unscoped().Where("app_id = ?", id).Delete(&models.Notification{}).Error; err != nil {
			return err
		}
		return tx.Delete(&models.App{}, "id = ?", id).Error
	})
}

func (r *AppRepo) FindByToken(ctx context.Context, prefix, hash string) (*models.App, error) {
	var a models.App
	if err := r.db.WithContext(ctx).Where("token_prefix = ? AND token = ?", prefix, hash).First(&a).Error; err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *AppRepo) IDsByUser(ctx context.Context, userID string) ([]string, error) {
	var ids []string
	return ids, r.db.WithContext(ctx).Model(&models.App{}).Where("user_id = ?", userID).Pluck("id", &ids).Error
}
