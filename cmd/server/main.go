package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"picloud-server/internal/auth"
	"picloud-server/internal/config"
	"picloud-server/internal/database"
	"picloud-server/internal/httpapi"
	"picloud-server/internal/media"
)

func main() {
	cfg := config.Load()

	db, err := database.Open(cfg.SQLitePath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer db.Close()

	if err := database.Migrate(db); err != nil {
		log.Fatalf("migrate database: %v", err)
	}

	authService := auth.NewService(db, cfg.JWTSecret)
	if err := authService.SeedInitialUser(cfg.InitialUserEmail, cfg.InitialUserPassword); err != nil {
		log.Fatalf("seed initial user: %v", err)
	}

	store := media.NewStore(cfg.MediaRoot)
	if err := store.EnsureDirs(); err != nil {
		log.Fatalf("create media directories: %v", err)
	}

	repo := media.NewRepository(db)
	server := httpapi.NewServer(cfg, authService, repo, store)

	httpServer := &http.Server{
		Addr:              ":" + cfg.AppPort,
		Handler:           server.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      0,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		log.Printf("picloud-server listening on :%s", cfg.AppPort)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("http server: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("graceful shutdown failed: %v", err)
	}
}
