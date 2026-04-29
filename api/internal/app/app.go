package app

import (
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"imagecreate/api/internal/admin"
	"imagecreate/api/internal/auth"
	"imagecreate/api/internal/config"
	"imagecreate/api/internal/generations"
	"imagecreate/api/internal/upstream"
	"imagecreate/api/internal/worker"
)

type App struct {
	cfg                config.Config
	adminHandlers      admin.Handlers
	authHandlers       auth.Handlers
	authMiddleware     func(http.Handler) http.Handler
	generationHandlers generations.Handlers
	worker             worker.Worker
}

func New(cfg config.Config, db *pgxpool.Pool) (*App, error) {
	authService := auth.Service{DB: db}
	authHandlers := auth.NewHandlers(authService, auth.HandlerOptions{
		SessionSecret: cfg.SessionSecret,
	})
	generationService := generations.Service{
		DB:          db,
		Model:       cfg.OpenAIImageModel,
		SizePresets: cfg.ImageSizePresets,
	}
	storage := generations.ImageStorage{Root: cfg.ImageStorageDir}
	upstreamClient := upstream.Client{
		BaseURL: cfg.OpenAIBaseURL,
		APIKey:  cfg.OpenAIAPIKey,
		Model:   cfg.OpenAIImageModel,
		HTTPClient: &http.Client{
			Timeout: cfg.OpenAIRequestTimeout,
		},
	}

	return &App{
		cfg:                cfg,
		adminHandlers:      admin.NewHandlers(db),
		authHandlers:       authHandlers,
		authMiddleware:     auth.WithUser(authService, auth.NewSessionCodec(cfg.SessionSecret)),
		generationHandlers: generations.NewHandlers(generationService, storage),
		worker: worker.Worker{
			DB:          db,
			Generations: generationService,
			Upstream:    upstreamClient,
			Storage:     storage,
		},
	}, nil
}

func (a *App) Worker() worker.Worker {
	return a.worker
}
