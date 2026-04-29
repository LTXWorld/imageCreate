package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"imagecreate/api/internal/app"
	"imagecreate/api/internal/auth"
	"imagecreate/api/internal/config"
	"imagecreate/api/internal/database"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	db, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("connect database: %v", err)
	}
	defer db.Close()

	if err := database.RunMigrations(cfg.DatabaseURL, migrationsPath()); err != nil {
		log.Fatalf("run migrations: %v", err)
	}

	if err := (auth.Service{DB: db}).EnsureAdmin(ctx, cfg.AdminUsername, cfg.AdminPassword); err != nil {
		log.Fatalf("ensure admin: %v", err)
	}

	application, err := app.New(cfg, db)
	if err != nil {
		log.Fatalf("create app: %v", err)
	}

	workerCtx, cancelWorker := context.WithCancel(context.Background())
	defer cancelWorker()
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		application.Worker().Run(workerCtx)
	}()

	addr := getenv("ADDR", ":8080")
	server := &http.Server{
		Addr:    addr,
		Handler: application.Routes(),
	}

	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.OpenAIRequestTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("shutdown server: %v", err)
		}
	}()

	log.Printf("listening on %s", addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		cancelWorker()
		log.Fatal(err)
	}

	if ctx.Err() != nil {
		<-shutdownDone
	}
	cancelWorker()
	waitForWorker(workerDone, cfg.OpenAIRequestTimeout)
}

func getenv(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func migrationsPath() string {
	candidates := []string{
		filepath.Join("migrations"),
		filepath.Join("api", "migrations"),
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			if absolute, err := filepath.Abs(candidate); err == nil {
				return absolute
			}
			return candidate
		}
	}
	return filepath.Join("migrations")
}

func waitForWorker(done <-chan struct{}, timeout time.Duration) {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-done:
	case <-timer.C:
		log.Printf("worker did not stop within %s", timeout)
	}
}
