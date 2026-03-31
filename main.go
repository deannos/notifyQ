package main

import (
	"log"

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

	r := router.Setup(database, h, cfg)

	log.Printf("NotifyQ listening on %s", cfg.ListenAddr)
	if err := r.Run(cfg.ListenAddr); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
