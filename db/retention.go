package db

import (
	"context"
	"log"
	"time"

	"github.com/deannos/notification-queue/models"
	"gorm.io/gorm"
)

// StartRetentionWorker deletes notifications older than retentionDays every 24 hours.
// It stops when ctx is cancelled (on graceful shutdown).
// If retentionDays <= 0, it does nothing.
func StartRetentionWorker(ctx context.Context, database *gorm.DB, retentionDays int) {
	if retentionDays <= 0 {
		return
	}
	go func() {
		purge := func() {
			cutoff := time.Now().AddDate(0, 0, -retentionDays)
			result := database.Unscoped().Where("created_at < ?", cutoff).Delete(&models.Notification{})
			if result.Error != nil {
				log.Printf("retention: cleanup error: %v", result.Error)
			} else if result.RowsAffected > 0 {
				log.Printf("retention: deleted %d notifications older than %d days", result.RowsAffected, retentionDays)
			}
		}

		purge() // run immediately on startup
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				purge()
			case <-ctx.Done():
				return
			}
		}
	}()
}
