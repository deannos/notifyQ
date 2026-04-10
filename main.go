package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/deannos/notification-queue/config"
	"github.com/deannos/notification-queue/db"
	"github.com/deannos/notification-queue/handlers"
	"github.com/deannos/notification-queue/hub"
	"github.com/deannos/notification-queue/router"
)

func main() {
	cfg := config.Load()

	database, err := db.Open(cfg.DatabasePath)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}

	if err := db.Migrate(database); err != nil {
		log.Fatalf("migration failed: %v", err)
	}

	if err := handlers.EnsureAdminUser(database, cfg.DefaultAdminUser, cfg.DefaultAdminPass); err != nil {
		log.Fatalf("failed to ensure admin user: %v", err)
	}

	h := hub.New()
	go h.Run()

	srv := &http.Server{
		Addr:    cfg.ListenAddr,
		Handler: router.Setup(database, h, cfg),
	}

	go func() {
		log.Printf("NotifyQ listening on %s", cfg.ListenAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("forced shutdown: %v", err)
	}

	if sqlDB, err := database.DB(); err == nil {
		sqlDB.Close()
	}

	log.Println("shutdown complete")
}
