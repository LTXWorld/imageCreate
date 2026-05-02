package app

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"imagecreate/api/internal/auth"
)

func (a *App) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(a.authMiddleware)

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	r.Route("/api/auth", func(r chi.Router) {
		r.Post("/register", a.authHandlers.Register)
		r.Post("/login", a.authHandlers.Login)
		r.Post("/logout", a.authHandlers.Logout)
		r.With(auth.RequireUser).Get("/me", a.authHandlers.Me)
	})

	r.Group(func(r chi.Router) {
		r.Use(auth.RequireUser)
		r.Post("/api/generations", a.generationHandlers.Create)
		r.Get("/api/generations", a.generationHandlers.List)
		r.Get("/api/generations/{id}", a.generationHandlers.Get)
		r.Delete("/api/generations/{id}", a.generationHandlers.Delete)
		r.Get("/api/generations/{id}/image", a.generationHandlers.Image)
	})

	r.Route("/api/admin", func(r chi.Router) {
		r.Use(auth.RequireAdmin)
		r.Get("/users", a.adminHandlers.ListUsers)
		r.Post("/password", a.adminHandlers.ChangeOwnPassword)
		r.Patch("/users/{id}/status", a.adminHandlers.UpdateUserStatus)
		r.Patch("/users/{id}/daily-free-limit", a.adminHandlers.UpdateDailyFreeLimit)
		r.Post("/users/{id}/credits", a.adminHandlers.AdjustCredits)
		r.Post("/users/{id}/password", a.adminHandlers.ResetUserPassword)
		r.Get("/invites", a.adminHandlers.ListInvites)
		r.Post("/invites", a.adminHandlers.CreateInvite)
		r.Get("/audit-logs", a.adminHandlers.ListAuditLogs)
		r.Get("/generation-tasks", a.adminHandlers.ListGenerationTasks)
	})

	return r
}
