package storage

import (
	"context"

	"github.com/deannos/notification-queue/models"
)

// NotificationFilter holds optional query parameters for listing notifications.
type NotificationFilter struct {
	AppID    string
	Read     *bool
	Priority *int
	Query    string
	Limit    int
	Offset   int
}

// UserRepository abstracts all user persistence operations.
type UserRepository interface {
	Create(ctx context.Context, u *models.User) error
	// CreateFirstAdmin atomically checks the user count; if zero the new user
	// becomes admin. Returns whether admin was granted.
	CreateFirstAdmin(ctx context.Context, u *models.User) (isAdmin bool, err error)
	FindByUsername(ctx context.Context, username string) (*models.User, error)
	FindByID(ctx context.Context, id string) (*models.User, error)
	List(ctx context.Context) ([]models.User, error)
	Count(ctx context.Context) (int64, error)
	UpdatePassword(ctx context.Context, id, hash string) error
	Delete(ctx context.Context, id string) error
}

// AppRepository abstracts all app persistence operations.
type AppRepository interface {
	Create(ctx context.Context, a *models.App) error
	FindByOwner(ctx context.Context, id, userID string) (*models.App, error)
	ListByUser(ctx context.Context, userID string) ([]models.App, error)
	Update(ctx context.Context, a *models.App, updates map[string]any) error
	Delete(ctx context.Context, id string) error
	FindByToken(ctx context.Context, prefix, hash string) (*models.App, error)
	IDsByUser(ctx context.Context, userID string) ([]string, error)
}

// NotificationRepository abstracts all notification persistence operations.
type NotificationRepository interface {
	Create(ctx context.Context, n *models.Notification) error
	FindByID(ctx context.Context, id string) (*models.Notification, error)
	List(ctx context.Context, userID string, f NotificationFilter) ([]models.Notification, int64, error)
	MarkRead(ctx context.Context, id string) error
	Delete(ctx context.Context, id string) error
	DeleteByAppIDs(ctx context.Context, appIDs []string) error
	Ping(ctx context.Context) error
}

// NotificationPublisher delivers a serialised notification payload to a user.
// Implementations: WebSocket hub today, Kafka tomorrow.
type NotificationPublisher interface {
	Publish(ctx context.Context, userID string, payload []byte)
}
