package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/deannos/notification-queue/config"
	"github.com/deannos/notification-queue/db"
	"github.com/deannos/notification-queue/handlers"
	"github.com/deannos/notification-queue/hub"
	"github.com/deannos/notification-queue/logger"
	"github.com/deannos/notification-queue/router"
	"github.com/deannos/notification-queue/storage/sqlite"
	"go.uber.org/zap"
)

func main() {
	cfg := config.Load()

	logger.Init(cfg.Env == "development")
	defer logger.Sync()
	log := logger.L

	if err := cfg.Validate(); err != nil {
		log.Fatal("invalid configuration", zap.Error(err))
	}

	database, err := db.Open(cfg.DatabasePath)
	if err != nil {
		log.Fatal("failed to open database", zap.Error(err))
	}

	if err := db.Migrate(database); err != nil {
		log.Fatal("migration failed", zap.Error(err))
	}

	// Build repository adapters.
	userRepo := sqlite.NewUserRepo(database)
	appRepo := sqlite.NewAppRepo(database)
	notifRepo := sqlite.NewNotificationRepo(database)

	if err := handlers.EnsureAdminUser(userRepo, cfg.DefaultAdminUser, cfg.DefaultAdminPass); err != nil {
		log.Fatal("failed to ensure admin user", zap.Error(err))
	}

	h := hub.New()
	go h.Run()

	// Root context — cancelled on shutdown to stop all background goroutines.
	rootCtx, rootCancel := context.WithCancel(context.Background())
	defer rootCancel()

	tickets := hub.NewTicketStore(rootCtx)

	db.StartRetentionWorker(rootCtx, database, cfg.RetentionDays)

	srv := &http.Server{
		Addr:    cfg.ListenAddr,
		Handler: router.Setup(rootCtx, userRepo, appRepo, notifRepo, h, h, tickets, cfg),
	}

	go func() {
		log.Info("NotifyQ listening", zap.String("addr", cfg.ListenAddr), zap.String("env", cfg.Env))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal("server error", zap.Error(err))
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info("shutting down...")

	rootCancel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Error("forced shutdown", zap.Error(err))
	}

	if sqlDB, err := database.DB(); err == nil {
		_ = sqlDB.Close()
	}

	log.Info("shutdown complete")
}
