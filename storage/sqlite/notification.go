package sqlite

import (
	"context"

	"github.com/deannos/notification-queue/models"
	"github.com/deannos/notification-queue/storage"
	"gorm.io/gorm"
)

type NotificationRepo struct{ db *gorm.DB }

func NewNotificationRepo(db *gorm.DB) *NotificationRepo { return &NotificationRepo{db: db} }

func (r *NotificationRepo) Create(ctx context.Context, n *models.Notification) error {
	return r.db.WithContext(ctx).Create(n).Error
}

func (r *NotificationRepo) FindByID(ctx context.Context, id string) (*models.Notification, error) {
	var n models.Notification
	if err := r.db.WithContext(ctx).Preload("App").First(&n, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &n, nil
}

// applyFilters returns a new scoped query with ownership and optional filters applied.
func (r *NotificationRepo) applyFilters(ctx context.Context, userID string, f storage.NotificationFilter) *gorm.DB {
	q := r.db.WithContext(ctx).
		Joins("JOIN apps ON apps.id = notifications.app_id").
		Where("apps.user_id = ?", userID)
	if f.AppID != "" {
		q = q.Where("notifications.app_id = ?", f.AppID)
	}
	if f.Read != nil {
		q = q.Where("notifications.read = ?", *f.Read)
	}
	if f.Priority != nil {
		q = q.Where("notifications.priority = ?", *f.Priority)
	}
	if f.Query != "" {
		like := "%" + f.Query + "%"
		q = q.Where("notifications.title LIKE ? OR notifications.message LIKE ?", like, like)
	}
	return q
}

func (r *NotificationRepo) List(ctx context.Context, userID string, f storage.NotificationFilter) ([]models.Notification, int64, error) {
	var total int64
	if err := r.applyFilters(ctx, userID, f).Model(&models.Notification{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if total == 0 {
		return nil, 0, nil
	}

	var notifs []models.Notification
	err := r.applyFilters(ctx, userID, f).
		Preload("App").
		Order("notifications.created_at DESC").
		Limit(f.Limit).
		Offset(f.Offset).
		Find(&notifs).Error
	return notifs, total, err
}

func (r *NotificationRepo) MarkRead(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Model(&models.Notification{}).Where("id = ?", id).Update("read", true).Error
}

func (r *NotificationRepo) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&models.Notification{}, "id = ?", id).Error
}

func (r *NotificationRepo) DeleteByAppIDs(ctx context.Context, appIDs []string) error {
	if len(appIDs) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Where("app_id IN ?", appIDs).Delete(&models.Notification{}).Error
}

func (r *NotificationRepo) Ping(ctx context.Context) error {
	return r.db.WithContext(ctx).Raw("SELECT 1").Error
}
