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
	"imagecreate/api/internal/credits"
	"imagecreate/api/internal/database"
	"imagecreate/api/internal/worker"
)

type dailyFreeCreditRefresher interface {
	RefreshAllDailyFreeCredits(context.Context) (int, error)
}

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
	workerDone := worker.RunPool(workerCtx, application.Worker(), cfg.WorkerConcurrency)

	refreshCtx, cancelRefresh := context.WithCancel(context.Background())
	defer cancelRefresh()
	refreshDone := runDailyFreeCreditRefreshLoop(refreshCtx, credits.Service{DB: db})

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
		cancelRefresh()
		log.Fatal(err)
	}

	if ctx.Err() != nil {
		<-shutdownDone
	}
	cancelWorker()
	cancelRefresh()
	waitForWorker(workerDone, cfg.OpenAIRequestTimeout)
	waitForDailyFreeCreditRefresh(refreshDone, cfg.OpenAIRequestTimeout)
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

func runDailyFreeCreditRefreshLoop(ctx context.Context, refresher dailyFreeCreditRefresher) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			runAt := nextLocalMidnight().Add(5 * time.Second)
			timer := time.NewTimer(time.Until(runAt))
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}

			refreshCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
			count, err := refresher.RefreshAllDailyFreeCredits(refreshCtx)
			cancel()
			if err != nil {
				log.Printf("refresh daily free credits: %v", err)
				continue
			}
			log.Printf("refreshed daily free credits for %d users", count)
		}
	}()
	return done
}

func nextLocalMidnight() time.Time {
	now := time.Now()
	nextDay := now.AddDate(0, 0, 1)
	return time.Date(nextDay.Year(), nextDay.Month(), nextDay.Day(), 0, 0, 0, 0, now.Location())
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

func waitForDailyFreeCreditRefresh(done <-chan struct{}, timeout time.Duration) {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-done:
	case <-timer.C:
		log.Printf("daily free credit refresh loop did not stop within %s", timeout)
	}
}
