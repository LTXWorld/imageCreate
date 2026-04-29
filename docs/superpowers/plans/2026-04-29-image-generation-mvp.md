# Image Generation MVP Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a small-invite Chinese AI text-to-image web app that calls an OpenAI-compatible `gpt-image-2` reverse proxy, manages invite registration, credits, async generation tasks, local image storage, and a minimal admin console.

**Architecture:** Monorepo with a Go REST API, an in-process database-backed worker, a React/Vite SPA, PostgreSQL, local image storage, and Caddy for HTTPS. The task table and upstream adapter are designed so the worker can later move to a Redis-backed standalone service without replacing the user-facing API.

**Tech Stack:** Go 1.22+, Chi, pgx, golang-migrate, bcrypt, cookie sessions, React, Vite, TypeScript, TanStack Query, Vitest, React Testing Library, PostgreSQL, Docker Compose, Caddy.

---

## File Structure

Create this structure:

```text
.
├── .env.example
├── Caddyfile
├── Dockerfile.api
├── Dockerfile.web
├── docker-compose.yml
├── api
│   ├── cmd
│   │   ├── server
│   │   │   └── main.go
│   │   └── migrate
│   │       └── main.go
│   ├── go.mod
│   ├── go.sum
│   ├── internal
│   │   ├── app
│   │   │   ├── app.go
│   │   │   └── routes.go
│   │   ├── auth
│   │   │   ├── handlers.go
│   │   │   ├── middleware.go
│   │   │   ├── service.go
│   │   │   └── service_test.go
│   │   ├── admin
│   │   │   ├── handlers.go
│   │   │   └── handlers_test.go
│   │   ├── config
│   │   │   ├── config.go
│   │   │   └── config_test.go
│   │   ├── credits
│   │   │   ├── service.go
│   │   │   └── service_test.go
│   │   ├── database
│   │   │   ├── db.go
│   │   │   ├── migrations.go
│   │   │   └── testdb.go
│   │   ├── generations
│   │   │   ├── handlers.go
│   │   │   ├── service.go
│   │   │   ├── service_test.go
│   │   │   └── storage.go
│   │   ├── models
│   │   │   └── models.go
│   │   ├── upstream
│   │   │   ├── client.go
│   │   │   └── client_test.go
│   │   └── worker
│   │       ├── worker.go
│   │       └── worker_test.go
│   └── migrations
│       └── 000001_initial.up.sql
├── docs
│   └── superpowers
│       ├── plans
│       │   └── 2026-04-29-image-generation-mvp.md
│       └── specs
│           └── 2026-04-29-image-generation-mvp-design.md
└── web
    ├── index.html
    ├── package.json
    ├── tsconfig.json
    ├── vite.config.ts
    └── src
        ├── App.tsx
        ├── main.tsx
        ├── api
        │   └── client.ts
        ├── components
        │   ├── Layout.tsx
        │   └── RequireAuth.tsx
        ├── pages
        │   ├── AdminPage.tsx
        │   ├── HistoryPage.tsx
        │   ├── LoginPage.tsx
        │   ├── RegisterPage.tsx
        │   └── WorkspacePage.tsx
        ├── styles
        │   └── app.css
        └── test
            └── setup.ts
```

---

### Task 1: Repository Scaffold And Environment Contract

**Files:**
- Create: `.env.example`
- Create: `api/go.mod`
- Create: `api/cmd/server/main.go`
- Create: `api/internal/config/config.go`
- Create: `api/internal/config/config_test.go`
- Create: `web/package.json`
- Create: `web/index.html`
- Create: `web/src/main.tsx`
- Create: `web/src/App.tsx`
- Modify: `.gitignore`

- [ ] **Step 1: Write failing config tests**

Create `api/internal/config/config_test.go`:

```go
package config

import "testing"

func TestLoadDefaultsImagePresets(t *testing.T) {
	t.Setenv("APP_BASE_URL", "https://img.example.com")
	t.Setenv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/imagecreate?sslmode=disable")
	t.Setenv("SESSION_SECRET", "test-secret-with-32-characters")
	t.Setenv("ADMIN_USERNAME", "admin")
	t.Setenv("ADMIN_PASSWORD", "admin-password")
	t.Setenv("OPENAI_BASE_URL", "https://proxy.example.com")
	t.Setenv("OPENAI_API_KEY", "sk-test")
	t.Setenv("OPENAI_IMAGE_MODEL", "gpt-image-2")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.ImageSizePresets["1:1"] != "1024x1024" {
		t.Fatalf("expected 1:1 preset 1024x1024, got %q", cfg.ImageSizePresets["1:1"])
	}
	if cfg.ImageRetentionDays != 30 {
		t.Fatalf("expected default retention 30, got %d", cfg.ImageRetentionDays)
	}
}

func TestLoadRequiresSecrets(t *testing.T) {
	_, err := Load()
	if err == nil {
		t.Fatal("expected missing required environment variables to fail")
	}
}
```

- [ ] **Step 2: Run config test to verify it fails**

Run: `cd api && go test ./internal/config -run TestLoad -v`

Expected: FAIL because `go.mod` and the `config` package do not exist.

- [ ] **Step 3: Add Go module and config implementation**

Create `api/go.mod`:

```go
module imagecreate/api

go 1.22

require (
	github.com/go-chi/chi/v5 v5.0.12
	github.com/jackc/pgx/v5 v5.5.5
	github.com/golang-migrate/migrate/v4 v4.17.1
	golang.org/x/crypto v0.24.0
)
```

Create `api/internal/config/config.go`:

```go
package config

import (
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	AppBaseURL            string
	DatabaseURL           string
	SessionSecret         string
	AdminUsername         string
	AdminPassword         string
	OpenAIBaseURL         string
	OpenAIAPIKey          string
	OpenAIImageModel      string
	OpenAIRequestTimeout  time.Duration
	ImageStorageDir       string
	ImageRetentionDays    int
	ImageSizePresets      map[string]string
}

func Load() (Config, error) {
	cfg := Config{
		AppBaseURL:           strings.TrimRight(os.Getenv("APP_BASE_URL"), "/"),
		DatabaseURL:          os.Getenv("DATABASE_URL"),
		SessionSecret:        os.Getenv("SESSION_SECRET"),
		AdminUsername:        os.Getenv("ADMIN_USERNAME"),
		AdminPassword:        os.Getenv("ADMIN_PASSWORD"),
		OpenAIBaseURL:        strings.TrimRight(os.Getenv("OPENAI_BASE_URL"), "/"),
		OpenAIAPIKey:         os.Getenv("OPENAI_API_KEY"),
		OpenAIImageModel:     getenv("OPENAI_IMAGE_MODEL", "gpt-image-2"),
		ImageStorageDir:      getenv("IMAGE_STORAGE_DIR", "./storage/images"),
		ImageRetentionDays:   getenvInt("IMAGE_RETENTION_DAYS", 30),
		ImageSizePresets: map[string]string{
			"1:1": "1024x1024",
			"3:4": "768x1024",
			"4:3": "1024x768",
			"9:16": "720x1280",
			"16:9": "1280x720",
		},
	}
	timeoutSeconds := getenvInt("OPENAI_REQUEST_TIMEOUT_SECONDS", 120)
	cfg.OpenAIRequestTimeout = time.Duration(timeoutSeconds) * time.Second
	if raw := os.Getenv("IMAGE_SIZE_PRESETS"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &cfg.ImageSizePresets); err != nil {
			return Config{}, err
		}
	}
	if cfg.AppBaseURL == "" || cfg.DatabaseURL == "" || cfg.SessionSecret == "" ||
		cfg.AdminUsername == "" || cfg.AdminPassword == "" ||
		cfg.OpenAIBaseURL == "" || cfg.OpenAIAPIKey == "" {
		return Config{}, errors.New("missing required environment variables")
	}
	return cfg, nil
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getenvInt(key string, fallback int) int {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			return parsed
		}
	}
	return fallback
}
```

- [ ] **Step 4: Add minimal server and frontend stubs**

Create `api/cmd/server/main.go`:

```go
package main

import (
	"log"
	"net/http"
	"os"

	"imagecreate/api/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	addr := ":8080"
	if value := os.Getenv("PORT"); value != "" {
		addr = ":" + value
	}
	log.Printf("starting API for %s on %s", cfg.AppBaseURL, addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
```

Create `web/package.json`:

```json
{
  "scripts": {
    "dev": "vite --host 0.0.0.0",
    "build": "tsc && vite build",
    "test": "vitest run"
  },
  "dependencies": {
    "@vitejs/plugin-react": "^4.3.1",
    "vite": "^5.2.0",
    "typescript": "^5.4.5",
    "react": "^18.2.0",
    "react-dom": "^18.2.0",
    "@tanstack/react-query": "^5.40.0",
    "lucide-react": "^0.468.0"
  },
  "devDependencies": {
    "@testing-library/jest-dom": "^6.4.6",
    "@testing-library/react": "^15.0.7",
    "@types/react": "^18.2.66",
    "@types/react-dom": "^18.2.22",
    "vitest": "^1.6.0",
    "jsdom": "^24.1.0"
  }
}
```

Create minimal `web/index.html`, `web/src/main.tsx`, and `web/src/App.tsx` with a rendered title `AI 生图`.

Update `.gitignore`:

```text
.superpowers/
.env
api/storage/
web/dist/
web/node_modules/
```

- [ ] **Step 5: Run tests and commit**

Run: `cd api && go mod tidy && go test ./internal/config -v`

Expected: PASS.

Run: `git add . && git commit -m "chore: scaffold app configuration"`

---

### Task 2: Database Schema And Test Database Helpers

**Files:**
- Create: `api/migrations/000001_initial.up.sql`
- Create: `api/internal/database/db.go`
- Create: `api/internal/database/migrations.go`
- Create: `api/internal/database/testdb.go`
- Create: `api/internal/models/models.go`

- [ ] **Step 1: Write migration with schema**

Create `api/migrations/000001_initial.up.sql`:

```sql
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE users (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  username TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  role TEXT NOT NULL CHECK (role IN ('user', 'admin')),
  status TEXT NOT NULL CHECK (status IN ('active', 'disabled')),
  credit_balance INTEGER NOT NULL DEFAULT 0 CHECK (credit_balance >= 0),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE invites (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  code TEXT NOT NULL UNIQUE,
  initial_credits INTEGER NOT NULL CHECK (initial_credits >= 0),
  status TEXT NOT NULL CHECK (status IN ('unused', 'used', 'disabled')),
  created_by UUID REFERENCES users(id),
  used_by UUID UNIQUE REFERENCES users(id),
  used_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE generation_tasks (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id),
  prompt TEXT NOT NULL,
  size TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('queued', 'running', 'succeeded', 'failed', 'canceled')),
  upstream_model TEXT NOT NULL,
  upstream_request_id TEXT,
  image_path TEXT,
  error_code TEXT,
  error_message TEXT,
  latency_ms INTEGER,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  started_at TIMESTAMPTZ,
  completed_at TIMESTAMPTZ,
  deleted_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX generation_tasks_one_active_per_user
ON generation_tasks(user_id)
WHERE status IN ('queued', 'running') AND deleted_at IS NULL;

CREATE TABLE credit_ledger (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id),
  task_id UUID REFERENCES generation_tasks(id),
  type TEXT NOT NULL CHECK (type IN ('invite_grant', 'admin_adjustment', 'generation_debit', 'generation_refund')),
  amount INTEGER NOT NULL,
  balance_after INTEGER NOT NULL CHECK (balance_after >= 0),
  reason TEXT NOT NULL,
  actor_user_id UUID REFERENCES users(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE audit_logs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  actor_user_id UUID REFERENCES users(id),
  target_user_id UUID REFERENCES users(id),
  action TEXT NOT NULL,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

- [ ] **Step 2: Add model constants**

Create `api/internal/models/models.go`:

```go
package models

const (
	RoleUser  = "user"
	RoleAdmin = "admin"

	UserStatusActive   = "active"
	UserStatusDisabled = "disabled"

	TaskQueued    = "queued"
	TaskRunning   = "running"
	TaskSucceeded = "succeeded"
	TaskFailed    = "failed"
	TaskCanceled  = "canceled"

	LedgerInviteGrant     = "invite_grant"
	LedgerAdminAdjustment = "admin_adjustment"
	LedgerGenerationDebit = "generation_debit"
	LedgerGenerationRefund = "generation_refund"
)
```

- [ ] **Step 3: Add database connection and migration helpers**

Create `api/internal/database/db.go` and `api/internal/database/migrations.go` with `Connect(ctx, databaseURL string) (*pgxpool.Pool, error)` and `RunMigrations(databaseURL, migrationsPath string) error`. Use `github.com/jackc/pgx/v5/pgxpool` and `github.com/golang-migrate/migrate/v4`.

- [ ] **Step 4: Add test database helper**

Create `api/internal/database/testdb.go` with `RequireTestDB(t *testing.T) *pgxpool.Pool` that reads `TEST_DATABASE_URL`. If the variable is absent, call `t.Skip("TEST_DATABASE_URL is not set")`.

- [ ] **Step 5: Run database package tests and commit**

Run: `cd api && go test ./internal/database ./internal/models -v`

Expected: PASS or SKIP only for integration helpers when `TEST_DATABASE_URL` is not set.

Run: `git add api && git commit -m "feat: add database schema"`

---

### Task 3: Auth, Invite Registration, And Admin Bootstrap

**Files:**
- Create: `api/internal/auth/service.go`
- Create: `api/internal/auth/service_test.go`
- Create: `api/internal/auth/middleware.go`
- Create: `api/internal/auth/handlers.go`

- [ ] **Step 1: Write auth service tests**

Create `api/internal/auth/service_test.go` with these exact tests and assertions:

- `TestRegisterConsumesInviteAndGrantsCredits`: creates an unused invite with 5 credits, registers `alice`, asserts the invite is `used`, the new user balance is 5, and one `invite_grant` ledger row exists.
- `TestRegisterRejectsUsedInvite`: creates an invite already linked to a user, attempts registration with the same code, and asserts registration fails without creating a second user.
- `TestLoginRejectsDisabledUser`: creates a disabled user with a bcrypt password, attempts login, and asserts the returned error maps to forbidden login.
- `TestEnsureAdminCreatesConfiguredAdminOnce`: calls `EnsureAdmin` twice with the same configured username and asserts only one admin row exists.

Each test uses `database.RequireTestDB(t)`, runs migrations, creates required fixture rows, and asserts database state directly.

- [ ] **Step 2: Run auth tests to verify they fail**

Run: `cd api && go test ./internal/auth -run TestRegister -v`

Expected: FAIL because auth service does not exist.

- [ ] **Step 3: Implement service**

Create `api/internal/auth/service.go` with:

```go
type Service struct {
	DB *pgxpool.Pool
}

type User struct {
	ID string
	Username string
	Role string
	Status string
	CreditBalance int
}

func (s Service) Register(ctx context.Context, username, password, inviteCode string) (User, error)
func (s Service) Login(ctx context.Context, username, password string) (User, error)
func (s Service) EnsureAdmin(ctx context.Context, username, password string) error
```

Use bcrypt for passwords. Wrap registration in a transaction that locks the invite row with `FOR UPDATE`, creates the user, marks the invite as used, sets `credit_balance`, and inserts an `invite_grant` ledger row.

- [ ] **Step 4: Implement session middleware and handlers**

Create `api/internal/auth/middleware.go` with cookie session helpers:

```go
func WithUser(service Service) func(http.Handler) http.Handler
func RequireUser(next http.Handler) http.Handler
func RequireAdmin(next http.Handler) http.Handler
func CurrentUser(r *http.Request) (User, bool)
```

Create `api/internal/auth/handlers.go` with handlers for:

```text
POST /api/auth/register
POST /api/auth/login
POST /api/auth/logout
GET /api/auth/me
```

Use an `HttpOnly`, `SameSite=Lax`, secure-in-production cookie.

- [ ] **Step 5: Run tests and commit**

Run: `cd api && go test ./internal/auth -v`

Expected: PASS, with integration tests skipped if `TEST_DATABASE_URL` is absent.

Run: `git add api/internal/auth && git commit -m "feat: add invite auth"`

---

### Task 4: Credits And Generation Task Service

**Files:**
- Create: `api/internal/credits/service.go`
- Create: `api/internal/credits/service_test.go`
- Create: `api/internal/generations/service.go`
- Create: `api/internal/generations/service_test.go`

- [ ] **Step 1: Write service tests**

Create tests with these exact names and assertions:

- `TestCreateTaskDebitsOneCredit`: active user starts with 3 credits; creating a task returns `queued`, balance becomes 2, and a `generation_debit` ledger row exists.
- `TestCreateTaskRejectsInsufficientCredits`: active user starts with 0 credits; creating a task fails and no task row is inserted.
- `TestCreateTaskRejectsSecondActiveTask`: user already has a `running` task; creating another task fails and balance is unchanged.
- `TestFailTaskRefundsCredit`: failed task transitions to `failed`, balance increases by 1, and a `generation_refund` ledger row exists.
- `TestSucceedTaskDoesNotRefundCredit`: succeeded task transitions to `succeeded`, image path is saved, and no refund row is inserted.

Use a migrated test database, create an active user with known balance, call service methods, and assert `users.credit_balance`, `generation_tasks.status`, and `credit_ledger` rows.

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd api && go test ./internal/credits ./internal/generations -run TestCreateTask -v`

Expected: FAIL because services do not exist.

- [ ] **Step 3: Implement credit service**

Create `api/internal/credits/service.go`:

```go
type Service struct {
	DB *pgxpool.Pool
}

func (s Service) Adjust(ctx context.Context, userID string, amount int, reason string, actorUserID string) error
func (s Service) RefundGeneration(ctx context.Context, tx pgx.Tx, userID string, taskID string, reason string) error
```

`Adjust` is for admin changes. `RefundGeneration` is used inside task failure transactions.

- [ ] **Step 4: Implement generation service**

Create `api/internal/generations/service.go`:

```go
type Service struct {
	DB *pgxpool.Pool
	Model string
	SizePresets map[string]string
}

type CreateTaskInput struct {
	UserID string
	Prompt string
	Ratio string
}

type Task struct {
	ID string
	UserID string
	Prompt string
	Size string
	Status string
	ImagePath string
	ErrorCode string
	ErrorMessage string
}

func (s Service) CreateTask(ctx context.Context, input CreateTaskInput) (Task, error)
func (s Service) GetTaskForUser(ctx context.Context, userID, taskID string) (Task, error)
func (s Service) ListTasksForUser(ctx context.Context, userID string) ([]Task, error)
func (s Service) DeleteTaskForUser(ctx context.Context, userID, taskID string) error
func (s Service) MarkSucceeded(ctx context.Context, taskID, requestID, imagePath string, latencyMS int) error
func (s Service) MarkFailedAndRefund(ctx context.Context, taskID, code, message string, latencyMS int) error
```

Validate prompt length between 1 and 2000 runes. Convert ratio to size with `SizePresets`. Create tasks and debit credits in one transaction.

- [ ] **Step 5: Run tests and commit**

Run: `cd api && go test ./internal/credits ./internal/generations -v`

Expected: PASS, with integration tests skipped if `TEST_DATABASE_URL` is absent.

Run: `git add api/internal/credits api/internal/generations && git commit -m "feat: add credit-backed generation tasks"`

---

### Task 5: Upstream Adapter And Local Image Storage

**Files:**
- Create: `api/internal/upstream/client.go`
- Create: `api/internal/upstream/client_test.go`
- Create: `api/internal/generations/storage.go`

- [ ] **Step 1: Write upstream client tests**

Create tests with these exact names and assertions:

- `TestGenerateImageSendsExpectedRequest`: fake server receives `POST /v1/images/generations`, bearer auth, and JSON body containing `model`, `prompt`, `n`, `size`, `quality`, `output_format`, and `background`.
- `TestGenerateImageMapsContentRejection`: fake server returns a 400 policy response; client returns `content_rejected`.
- `TestGenerateImageMapsTimeout`: request context expires; client returns `timeout`.
- `TestGenerateImageDoesNotExposeAPIKeyInErrors`: fake server returns 500; error string does not contain the configured API key.

Use `httptest.Server` to assert the path `/v1/images/generations`, the `Authorization: Bearer` header, and JSON body fields `model`, `prompt`, `n`, `size`, `quality`, `output_format`, and `background`.

- [ ] **Step 2: Run upstream tests to verify they fail**

Run: `cd api && go test ./internal/upstream -v`

Expected: FAIL because the upstream package does not exist.

- [ ] **Step 3: Implement upstream client**

Create `api/internal/upstream/client.go`:

```go
type Client struct {
	BaseURL string
	APIKey string
	Model string
	HTTPClient *http.Client
}

type Result struct {
	RequestID string
	ImageBytes []byte
	ErrorCode string
	ErrorMessage string
}

func (c Client) GenerateImage(ctx context.Context, prompt, size string) (Result, error)
```

Support base64 JSON responses. Classify 400/403 policy-like responses as `content_rejected`, 429 as `rate_limited`, context deadline as `timeout`, and other non-2xx responses as `upstream_error`. Return sanitized errors that never include `APIKey`.

- [ ] **Step 4: Implement local storage**

Create `api/internal/generations/storage.go`:

```go
type ImageStorage struct {
	Root string
}

func (s ImageStorage) Save(ctx context.Context, taskID string, data []byte, now time.Time) (string, error)
func (s ImageStorage) Open(ctx context.Context, relativePath string) (*os.File, error)
func (s ImageStorage) Delete(ctx context.Context, relativePath string) error
```

Use `storage/images/YYYY/MM/DD/<task-id>.png` under `Root`. Reject paths containing `..` in `Open` and `Delete`.

- [ ] **Step 5: Run tests and commit**

Run: `cd api && go test ./internal/upstream ./internal/generations -run 'TestGenerateImage|TestImageStorage' -v`

Expected: PASS.

Run: `git add api/internal/upstream api/internal/generations/storage.go && git commit -m "feat: add upstream image adapter"`

---

### Task 6: Worker Processing Loop

**Files:**
- Create: `api/internal/worker/worker.go`
- Create: `api/internal/worker/worker_test.go`

- [ ] **Step 1: Write worker tests**

Create tests with these exact names and assertions:

- `TestWorkerProcessesQueuedTaskSuccessfully`: a queued task is claimed, marked `running`, fake upstream returns PNG bytes, fake storage saves a path, and task becomes `succeeded`.
- `TestWorkerRefundsOnUpstreamFailure`: fake upstream returns `content_rejected`, task becomes `failed`, and one credit is refunded.
- `TestWorkerSkipsWhenNoQueuedTask`: empty queue returns `processed=false` and no error.

Use fake upstream and fake storage interfaces. Assert state transitions `queued` -> `running` -> `succeeded` or `failed`, image path saved on success, refund ledger created on failure.

- [ ] **Step 2: Run worker tests to verify they fail**

Run: `cd api && go test ./internal/worker -v`

Expected: FAIL because worker package does not exist.

- [ ] **Step 3: Implement worker**

Create `api/internal/worker/worker.go`:

```go
type Upstream interface {
	GenerateImage(ctx context.Context, prompt, size string) (upstream.Result, error)
}

type Storage interface {
	Save(ctx context.Context, taskID string, data []byte, now time.Time) (string, error)
}

type Worker struct {
	DB *pgxpool.Pool
	Generations generations.Service
	Upstream Upstream
	Storage Storage
	PollInterval time.Duration
}

func (w Worker) Run(ctx context.Context)
func (w Worker) ProcessOne(ctx context.Context) (bool, error)
```

`ProcessOne` should claim one queued task with `FOR UPDATE SKIP LOCKED`, mark it running, call upstream, save image on success, and mark failed with refund on failure.

- [ ] **Step 4: Run tests and commit**

Run: `cd api && go test ./internal/worker -v`

Expected: PASS, with integration tests skipped if `TEST_DATABASE_URL` is absent.

Run: `git add api/internal/worker && git commit -m "feat: add generation worker"`

---

### Task 7: User HTTP API And App Wiring

**Files:**
- Create: `api/internal/generations/handlers.go`
- Create: `api/internal/app/app.go`
- Create: `api/internal/app/routes.go`
- Modify: `api/cmd/server/main.go`

- [ ] **Step 1: Write handler tests**

Create handler tests with these exact names and assertions:

- `TestCreateGenerationReturnsQueuedTask`: authenticated user posts prompt and ratio, response status is 201, JSON status is `queued`, and no upstream key appears in the response.
- `TestImageEndpointRejectsOtherUser`: user B requests user A's succeeded task image and receives 404 or 403 without file bytes.
- `TestGenerationFailureMessageIsChinese`: failed task with `content_rejected` returns `提示词可能包含不支持生成的内容，请调整描述后重试。`.

Use `httptest` and authenticated request context.

- [ ] **Step 2: Run handler tests to verify they fail**

Run: `cd api && go test ./internal/generations -run TestCreateGeneration -v`

Expected: FAIL because handlers are missing.

- [ ] **Step 3: Implement user generation handlers**

Create handlers for:

```text
POST /api/generations
GET /api/generations
GET /api/generations/{id}
DELETE /api/generations/{id}
GET /api/generations/{id}/image
```

Request body for creation:

```json
{
  "prompt": "一张真实感照片",
  "ratio": "1:1"
}
```

Failure response includes a stable `error_code` and Chinese `message`.

- [ ] **Step 4: Wire app routes**

Create `api/internal/app/app.go` and `api/internal/app/routes.go` with Chi router setup:

```go
func New(cfg config.Config, db *pgxpool.Pool) (*App, error)
func (a *App) Routes() http.Handler
```

Modify `api/cmd/server/main.go` to connect DB, run migrations, ensure admin, start worker, and serve app routes.

- [ ] **Step 5: Run API tests and commit**

Run: `cd api && go test ./internal/... -v`

Expected: PASS, with database-backed tests skipped if `TEST_DATABASE_URL` is absent.

Run: `git add api && git commit -m "feat: expose user generation api"`

---

### Task 8: Admin HTTP API

**Files:**
- Create: `api/internal/admin/handlers.go`
- Create: `api/internal/admin/handlers_test.go`
- Modify: `api/internal/app/routes.go`

- [ ] **Step 1: Write admin handler tests**

Create tests with these exact names and assertions:

- `TestAdminCanCreateInvite`: admin creates an invite with initial credits; response contains code and status `unused`.
- `TestNonAdminCannotCreateInvite`: normal user posts to admin invite endpoint and receives 403.
- `TestAdminCanAdjustCredits`: admin adds 3 credits to a user; balance and `admin_adjustment` ledger row update.
- `TestAdminGenerationListDoesNotReturnImageURL`: admin task list includes prompt and status but does not include `image_path`, `imageUrl`, or `/api/generations/`.
- `TestAdminCanDisableUser`: admin disables a user; status becomes `disabled`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd api && go test ./internal/admin -v`

Expected: FAIL because admin package is missing.

- [ ] **Step 3: Implement admin handlers**

Create handlers for:

```text
GET /api/admin/users
PATCH /api/admin/users/{id}/status
POST /api/admin/users/{id}/credits
GET /api/admin/invites
POST /api/admin/invites
GET /api/admin/audit-logs
GET /api/admin/generation-tasks
```

Use `RequireAdmin`. On every mutation, insert an `audit_logs` row. For `GET /api/admin/generation-tasks`, return prompt, status, size, latency, error code, and error message, but never `image_path` or image URL.

- [ ] **Step 4: Wire admin routes and run tests**

Run: `cd api && go test ./internal/admin ./internal/app -v`

Expected: PASS, with database-backed tests skipped if `TEST_DATABASE_URL` is absent.

Run: `git add api/internal/admin api/internal/app && git commit -m "feat: add admin api"`

---

### Task 9: React App Shell, API Client, And Auth Pages

**Files:**
- Create: `web/tsconfig.json`
- Create: `web/vite.config.ts`
- Create: `web/src/api/client.ts`
- Create: `web/src/components/Layout.tsx`
- Create: `web/src/components/RequireAuth.tsx`
- Create: `web/src/pages/LoginPage.tsx`
- Create: `web/src/pages/RegisterPage.tsx`
- Create: `web/src/styles/app.css`
- Create: `web/src/test/setup.ts`
- Modify: `web/src/App.tsx`

- [ ] **Step 1: Write frontend auth tests**

Create page tests with React Testing Library:

- `renders Chinese login form`: render login page and expect `用户名`, `密码`, and `登录`.
- `submits username and password to login API`: mock `fetch`, fill username/password, click login, and assert `POST /api/auth/login`.
- `renders invite code on register page`: render register page and expect `邀请码`.

- [ ] **Step 2: Run frontend tests to verify they fail**

Run: `cd web && npm test -- --run`

Expected: FAIL until dependencies are installed and pages exist.

- [ ] **Step 3: Implement API client**

Create `web/src/api/client.ts`:

```ts
export type User = {
  id: string;
  username: string;
  role: "user" | "admin";
  status: "active" | "disabled";
  creditBalance: number;
};

export async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    credentials: "include",
    headers: { "Content-Type": "application/json", ...(init?.headers ?? {}) },
    ...init
  });
  if (!response.ok) {
    const body = await response.json().catch(() => ({ message: "请求失败" }));
    throw new Error(body.message ?? "请求失败");
  }
  return response.json() as Promise<T>;
}
```

- [ ] **Step 4: Implement layout and auth pages**

Build a restrained desktop-first Chinese UI. Use icon buttons from `lucide-react` where useful. Login and register forms should show server error messages and disable submit while loading.

- [ ] **Step 5: Run tests and commit**

Run: `cd web && npm test -- --run && npm run build`

Expected: PASS.

Run: `git add web && git commit -m "feat: add web auth shell"`

---

### Task 10: Workspace And History Pages

**Files:**
- Create: `web/src/pages/WorkspacePage.tsx`
- Create: `web/src/pages/HistoryPage.tsx`
- Modify: `web/src/api/client.ts`
- Modify: `web/src/App.tsx`
- Modify: `web/src/styles/app.css`

- [ ] **Step 1: Write workspace tests**

Create tests:

- `creates a generation with prompt and ratio`: submit a prompt with ratio `1:1` and assert `POST /api/generations` body contains `prompt` and `ratio`.
- `shows running state while polling`: mock an active task response and expect `生成中`.
- `shows Chinese failure message from API`: mock failed task response and expect the Chinese failure message from the API.
- `renders history without showing other users`: mock current user's task list and assert only returned tasks are rendered.

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd web && npm test -- --run Workspace History`

Expected: FAIL because pages are not implemented.

- [ ] **Step 3: Extend API client types**

Add:

```ts
export type GenerationTask = {
  id: string;
  prompt: string;
  ratio: string;
  size: string;
  status: "queued" | "running" | "succeeded" | "failed" | "canceled";
  imageUrl?: string;
  errorCode?: string;
  message?: string;
  createdAt: string;
  completedAt?: string;
};
```

- [ ] **Step 4: Implement workspace**

Workspace contains prompt textarea, ratio segmented control for `1:1`, `3:4`, `4:3`, `9:16`, `16:9`, balance display, generate button, current task panel, and result preview. Poll active task every 2 seconds until `succeeded` or `failed`.

- [ ] **Step 5: Implement history**

History lists 30-day tasks, status, prompt, ratio, created time, and user-owned image preview for succeeded tasks. Delete calls `DELETE /api/generations/{id}` and refreshes the list.

- [ ] **Step 6: Run tests and commit**

Run: `cd web && npm test -- --run && npm run build`

Expected: PASS.

Run: `git add web && git commit -m "feat: add generation workspace"`

---

### Task 11: Admin Console Pages

**Files:**
- Create: `web/src/pages/AdminPage.tsx`
- Modify: `web/src/api/client.ts`
- Modify: `web/src/App.tsx`
- Modify: `web/src/styles/app.css`

- [ ] **Step 1: Write admin page tests**

Create tests:

- `shows user management for admins`: mock admin user and users response, then expect the user table.
- `creates an invite with initial credits`: fill initial credits, submit, and assert `POST /api/admin/invites`.
- `adjusts user credits`: submit a credit adjustment and assert `POST /api/admin/users/{id}/credits`.
- `does not render image links in audit task table`: mock audit tasks with prompt and status and assert no image URL or preview is rendered.

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd web && npm test -- --run AdminPage`

Expected: FAIL because the admin page is missing.

- [ ] **Step 3: Implement admin page**

Build tabs for users, invites, credits, and audit. Use tables and compact forms. Keep the UI Chinese and operational: no marketing hero, no decorative cards, no image preview in audit.

- [ ] **Step 4: Run tests and commit**

Run: `cd web && npm test -- --run && npm run build`

Expected: PASS.

Run: `git add web && git commit -m "feat: add admin console"`

---

### Task 12: Docker Compose, Caddy, And Deployment Verification

**Files:**
- Create: `Dockerfile.api`
- Create: `Dockerfile.web`
- Create: `docker-compose.yml`
- Create: `Caddyfile`
- Create: `.env.example`
- Modify: `README.md`

- [ ] **Step 1: Write deployment files**

Create `.env.example`:

```env
APP_BASE_URL=https://img.example.com
ADMIN_USERNAME=admin
ADMIN_PASSWORD=change-me-before-deploy
DATABASE_URL=postgres://postgres:postgres@postgres:5432/imagecreate?sslmode=disable
SESSION_SECRET=change-me-to-a-long-random-string
OPENAI_BASE_URL=https://proxy.example.com
OPENAI_API_KEY=sk-change-me
OPENAI_IMAGE_MODEL=gpt-image-2
OPENAI_REQUEST_TIMEOUT_SECONDS=120
IMAGE_SIZE_PRESETS={"1:1":"1024x1024","3:4":"768x1024","4:3":"1024x768","9:16":"720x1280","16:9":"1280x720"}
IMAGE_STORAGE_DIR=/data/images
IMAGE_RETENTION_DAYS=30
DOMAIN=img.example.com
```

Create Dockerfiles for API and web builds. `docker-compose.yml` should define `caddy`, `web`, `api`, and `postgres` services plus volumes for Postgres data, Caddy data, and images.

- [ ] **Step 2: Add README deployment instructions**

Create `README.md` with:

```md
# AI 生图邀测 MVP

## Local development

1. Copy `.env.example` to `.env`.
2. Fill `OPENAI_BASE_URL`, `OPENAI_API_KEY`, `SESSION_SECRET`, and admin credentials.
3. Run `docker compose up --build`.
4. Visit `http://localhost` for local Caddy or the configured domain in production.

## Production

Point the selected subdomain to the VPS IP, copy `.env.example` to `.env`, set `DOMAIN`, then run `docker compose up -d --build`.
```

- [ ] **Step 3: Run compose config validation**

Run: `docker compose config`

Expected: Docker Compose prints the resolved configuration without errors.

- [ ] **Step 4: Run full verification**

Run: `cd api && go test ./...`

Expected: PASS or SKIP for tests requiring `TEST_DATABASE_URL`.

Run: `cd web && npm test -- --run && npm run build`

Expected: PASS.

Run: `docker compose build`

Expected: PASS.

- [ ] **Step 5: Commit deployment files**

Run: `git add . && git commit -m "chore: add docker deployment"`

---

### Task 13: End-To-End Smoke Test

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Start the stack**

Run: `docker compose up -d --build`

Expected: `api`, `web`, `postgres`, and `caddy` are running.

- [ ] **Step 2: Create an invite**

Log in as the admin initialized from `.env`. Create an invite with 5 credits.

Expected: invite appears as `unused` in admin invites.

- [ ] **Step 3: Register a user**

Open an incognito browser session, register with the invite, and log in.

Expected: user balance is 5.

- [ ] **Step 4: Generate one image**

Submit prompt `一张真实感照片，一杯热咖啡放在木桌上，清晨自然光` with ratio `1:1`.

Expected: task enters `queued` or `running`, then `succeeded`; balance becomes 4; image displays through `/api/generations/{id}/image`.

- [ ] **Step 5: Verify admin audit privacy**

Open admin task audit.

Expected: prompt, status, size, and latency are visible; image URL and image preview are not visible.

- [ ] **Step 6: Commit smoke-test documentation**

Update `README.md` with any exact deployment notes learned from the smoke test.

Run: `git add README.md && git commit -m "docs: add smoke test notes"`

---

## Self-Review Checklist

- Spec coverage: tasks cover invite auth, credits, async generation, local image storage, admin audit without images, Docker deployment, and later worker split readiness.
- Completeness: all named files, routes, commands, and default mappings are explicit.
- Type consistency: task statuses, roles, ledger types, API routes, and environment variable names match the approved spec.
