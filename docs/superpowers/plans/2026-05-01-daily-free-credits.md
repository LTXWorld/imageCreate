# Daily Free Credits Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a commercial-ready two-wallet credit model where each user receives refreshed daily free credits while paid credits remain untouched.

**Architecture:** Add wallet columns to `users`, wallet metadata to `credit_ledger`, and keep `credit_balance` as a synced compatibility total. Centralize daily refresh and wallet constants in the API, then update registration, session lookup, generation debit/refund, admin adjustments, and React normalization/display.

**Tech Stack:** Go 1.22, pgx, PostgreSQL migrations via golang-migrate, React + TypeScript + Vite, Vitest.

---

## File Structure

- Create `api/migrations/000002_daily_free_credits.up.sql`: add wallet columns, ledger wallet metadata, expanded ledger type constraint, and refresh idempotency index.
- Modify `api/internal/models/models.go`: add wallet and ledger constants.
- Modify `api/internal/credits/service.go`: add daily refresh helpers and update refund/adjustment logic for wallet-aware balances.
- Modify `api/internal/credits/service_test.go`: test refresh idempotency and wallet refund behavior.
- Modify `api/internal/auth/service.go`: extend `User`, initialize wallets on registration/admin bootstrap, refresh users on login/session lookup, and return wallet fields.
- Modify `api/internal/auth/service_test.go`: verify registration initializes wallets and login refreshes stale free credits.
- Modify `api/internal/generations/service.go`: refresh before debit, debit free before paid, write wallet-specific debit ledger, and keep compatibility balance synced.
- Modify `api/internal/generations/service_test.go`: verify free-first debit, paid fallback, insufficient total balance, and wallet-specific refund.
- Modify `api/internal/admin/handlers.go`: return split wallet fields and adjust paid credits by default.
- Modify `api/internal/admin/handlers_test.go`: verify admin adjustment affects paid credits and admin list returns wallet fields.
- Modify `api/cmd/server/main.go`: start a daily background refresh loop after migrations and admin bootstrap.
- Modify `web/src/api/client.ts`: normalize wallet fields while keeping `creditBalance` as total fallback.
- Modify `web/src/pages/WorkspacePage.tsx` and `web/src/pages/AdminPage.tsx`: display daily free and paid credits.
- Modify matching frontend tests in `web/src/pages/*test.tsx` and `web/src/api/client.test.ts`.

## Task 1: Database Migration And Constants

**Files:**
- Create: `api/migrations/000002_daily_free_credits.up.sql`
- Modify: `api/internal/models/models.go`
- Test: `api/internal/database/testdb_test.go`

- [ ] **Step 1: Write the failing migration test**

Add this test to `api/internal/database/testdb_test.go`:

```go
func TestMigrationsAddDailyFreeCreditWalletColumns(t *testing.T) {
	ctx := context.Background()
	db := RequireTestDB(t)
	databaseURL := os.Getenv("TEST_DATABASE_URL")

	if err := RunMigrations(databaseURL, filepath.Join("..", "..", "migrations")); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	var userID string
	if err := db.QueryRow(ctx, `
		INSERT INTO users (username, password_hash, role, status, credit_balance)
		VALUES ('wallet-migration-user', 'hash', 'user', 'active', 4)
		RETURNING id::text
	`).Scan(&userID); err != nil {
		t.Fatalf("insert migrated user shape: %v", err)
	}

	var freeLimit, freeBalance, paidBalance, total int
	if err := db.QueryRow(ctx, `
		SELECT daily_free_credit_limit, daily_free_credit_balance, paid_credit_balance, credit_balance
		FROM users
		WHERE id = $1::uuid
	`, userID).Scan(&freeLimit, &freeBalance, &paidBalance, &total); err != nil {
		t.Fatalf("query wallet columns: %v", err)
	}
	if freeLimit != 0 || freeBalance != 0 || paidBalance != 0 || total != 4 {
		t.Fatalf("new user defaults freeLimit=%d freeBalance=%d paidBalance=%d total=%d, want 0,0,0,4", freeLimit, freeBalance, paidBalance, total)
	}
}
```

- [ ] **Step 2: Run the migration test to verify it fails**

Run:

```bash
cd api && go test ./internal/database -run TestMigrationsAddDailyFreeCreditWalletColumns -count=1
```

Expected: FAIL with an error mentioning `daily_free_credit_limit` does not exist.

- [ ] **Step 3: Add the migration**

Create `api/migrations/000002_daily_free_credits.up.sql`:

```sql
ALTER TABLE users
  ADD COLUMN daily_free_credit_limit INTEGER NOT NULL DEFAULT 0 CHECK (daily_free_credit_limit >= 0),
  ADD COLUMN daily_free_credit_balance INTEGER NOT NULL DEFAULT 0 CHECK (daily_free_credit_balance >= 0),
  ADD COLUMN paid_credit_balance INTEGER NOT NULL DEFAULT 0 CHECK (paid_credit_balance >= 0),
  ADD COLUMN last_daily_free_credit_refreshed_on DATE NOT NULL DEFAULT CURRENT_DATE;

UPDATE users
SET daily_free_credit_limit = credit_balance,
    daily_free_credit_balance = credit_balance,
    paid_credit_balance = 0,
    last_daily_free_credit_refreshed_on = CURRENT_DATE;

ALTER TABLE credit_ledger
  ADD COLUMN wallet_type TEXT CHECK (wallet_type IN ('daily_free', 'paid')),
  ADD COLUMN business_date DATE;

ALTER TABLE credit_ledger
  DROP CONSTRAINT credit_ledger_type_check,
  ADD CONSTRAINT credit_ledger_type_check CHECK (
    type IN (
      'invite_grant',
      'admin_adjustment',
      'generation_debit',
      'generation_refund',
      'daily_free_refresh',
      'daily_free_generation_debit',
      'daily_free_generation_refund',
      'paid_generation_debit',
      'paid_generation_refund',
      'paid_admin_adjustment'
    )
  );

CREATE UNIQUE INDEX credit_ledger_one_daily_free_refresh_per_user_date
ON credit_ledger(user_id, business_date)
WHERE type = 'daily_free_refresh';
```

- [ ] **Step 4: Add model constants**

Update `api/internal/models/models.go`:

```go
const (
	WalletDailyFree = "daily_free"
	WalletPaid      = "paid"
)

const (
	LedgerInviteGrant               = "invite_grant"
	LedgerAdminAdjustment           = "admin_adjustment"
	LedgerGenerationDebit           = "generation_debit"
	LedgerGenerationRefund          = "generation_refund"
	LedgerDailyFreeRefresh          = "daily_free_refresh"
	LedgerDailyFreeGenerationDebit  = "daily_free_generation_debit"
	LedgerDailyFreeGenerationRefund = "daily_free_generation_refund"
	LedgerPaidGenerationDebit       = "paid_generation_debit"
	LedgerPaidGenerationRefund      = "paid_generation_refund"
	LedgerPaidAdminAdjustment       = "paid_admin_adjustment"
)
```

Keep the existing role and task constants unchanged.

- [ ] **Step 5: Run migration tests**

Run:

```bash
cd api && go test ./internal/database -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add api/migrations/000002_daily_free_credits.up.sql api/internal/models/models.go api/internal/database/testdb_test.go
git commit -m "feat: add wallet credit schema"
```

## Task 2: Daily Refresh Service

**Files:**
- Modify: `api/internal/credits/service.go`
- Modify: `api/internal/credits/service_test.go`

- [ ] **Step 1: Write failing refresh tests**

Add tests to `api/internal/credits/service_test.go`:

```go
func TestRefreshDailyFreeCreditsRestoresFreeBalanceOnlyOnce(t *testing.T) {
	ctx, db := setupCreditTestDB(t)
	userID := insertCreditTestUser(t, ctx, db, "refresh-alice", models.RoleUser, 0)
	_, err := db.Exec(ctx, `
		UPDATE users
		SET daily_free_credit_limit = 5,
			daily_free_credit_balance = 1,
			paid_credit_balance = 9,
			credit_balance = 10,
			last_daily_free_credit_refreshed_on = CURRENT_DATE - 1
		WHERE id = $1::uuid
	`, userID)
	if err != nil {
		t.Fatalf("seed stale wallet: %v", err)
	}

	service := Service{DB: db}
	refreshed, err := service.RefreshDailyFreeCredits(ctx, userID)
	if err != nil {
		t.Fatalf("refresh daily free credits: %v", err)
	}
	if !refreshed {
		t.Fatal("refreshed = false, want true")
	}

	refreshed, err = service.RefreshDailyFreeCredits(ctx, userID)
	if err != nil {
		t.Fatalf("second refresh daily free credits: %v", err)
	}
	if refreshed {
		t.Fatal("second refreshed = true, want false")
	}

	var freeBalance, paidBalance, total, ledgerRows int
	if err := db.QueryRow(ctx, `
		SELECT daily_free_credit_balance, paid_credit_balance, credit_balance
		FROM users
		WHERE id = $1::uuid
	`, userID).Scan(&freeBalance, &paidBalance, &total); err != nil {
		t.Fatalf("query wallet balances: %v", err)
	}
	if err := db.QueryRow(ctx, `
		SELECT count(*)
		FROM credit_ledger
		WHERE user_id = $1::uuid
			AND type = $2
			AND wallet_type = $3
			AND business_date = CURRENT_DATE
	`, userID, models.LedgerDailyFreeRefresh, models.WalletDailyFree).Scan(&ledgerRows); err != nil {
		t.Fatalf("count refresh ledger: %v", err)
	}
	if freeBalance != 5 || paidBalance != 9 || total != 14 || ledgerRows != 1 {
		t.Fatalf("free=%d paid=%d total=%d ledgerRows=%d, want 5,9,14,1", freeBalance, paidBalance, total, ledgerRows)
	}
}
```

- [ ] **Step 2: Run the refresh test to verify it fails**

Run:

```bash
cd api && go test ./internal/credits -run TestRefreshDailyFreeCreditsRestoresFreeBalanceOnlyOnce -count=1
```

Expected: FAIL because `RefreshDailyFreeCredits` is undefined.

- [ ] **Step 3: Implement refresh methods**

Add to `api/internal/credits/service.go`:

```go
func (s Service) RefreshDailyFreeCredits(ctx context.Context, userID string) (bool, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin daily free refresh: %w", err)
	}
	defer tx.Rollback(ctx)

	refreshed, err := s.RefreshDailyFreeCreditsTx(ctx, tx, userID)
	if err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit daily free refresh: %w", err)
	}
	return refreshed, nil
}

func (s Service) RefreshDailyFreeCreditsTx(ctx context.Context, tx pgx.Tx, userID string) (bool, error) {
	var freeLimit, totalAfter int
	err := tx.QueryRow(ctx, `
		UPDATE users
		SET daily_free_credit_balance = daily_free_credit_limit,
			credit_balance = daily_free_credit_limit + paid_credit_balance,
			last_daily_free_credit_refreshed_on = CURRENT_DATE,
			updated_at = now()
		WHERE id = $1::uuid
			AND status = $2
			AND last_daily_free_credit_refreshed_on < CURRENT_DATE
		RETURNING daily_free_credit_limit, credit_balance
	`, userID, models.UserStatusActive).Scan(&freeLimit, &totalAfter)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("refresh daily free credits: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO credit_ledger (user_id, type, wallet_type, amount, balance_after, reason, business_date)
		VALUES ($1::uuid, $2, $3, $4, $5, $6, CURRENT_DATE)
		ON CONFLICT DO NOTHING
	`, userID, models.LedgerDailyFreeRefresh, models.WalletDailyFree, freeLimit, totalAfter, "daily free credits refreshed"); err != nil {
		return false, fmt.Errorf("insert daily free refresh ledger: %w", err)
	}
	return true, nil
}
```

- [ ] **Step 4: Run credits tests**

Run:

```bash
cd api && go test ./internal/credits -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add api/internal/credits/service.go api/internal/credits/service_test.go
git commit -m "feat: refresh daily free credits"
```

## Task 3: Auth Registration And Session Wallets

**Files:**
- Modify: `api/internal/auth/service.go`
- Modify: `api/internal/auth/service_test.go`
- Modify: `api/internal/auth/middleware.go`

- [ ] **Step 1: Write failing auth tests**

Add to `api/internal/auth/service_test.go`:

```go
func TestRegisterInitializesDailyFreeWallet(t *testing.T) {
	ctx, db := setupAuthTestDB(t)
	createInvite(t, ctx, db, "daily-free-register", 6)
	service := Service{DB: db}

	user, err := service.Register(ctx, "daily-free-user", "secret1", "daily-free-register")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if user.CreditBalance != 6 || user.DailyFreeCreditLimit != 6 || user.DailyFreeCreditBalance != 6 || user.PaidCreditBalance != 0 {
		t.Fatalf("user wallets total=%d free=%d/%d paid=%d, want total=6 free=6/6 paid=0",
			user.CreditBalance, user.DailyFreeCreditBalance, user.DailyFreeCreditLimit, user.PaidCreditBalance)
	}
}

func TestLoginRefreshesStaleDailyFreeWallet(t *testing.T) {
	ctx, db := setupAuthTestDB(t)
	userID := insertAuthTestUser(t, ctx, db, "stale-login", "secret1", models.RoleUser, models.UserStatusActive, 0)
	_, err := db.Exec(ctx, `
		UPDATE users
		SET daily_free_credit_limit = 3,
			daily_free_credit_balance = 0,
			paid_credit_balance = 2,
			credit_balance = 2,
			last_daily_free_credit_refreshed_on = CURRENT_DATE - 1
		WHERE id = $1::uuid
	`, userID)
	if err != nil {
		t.Fatalf("seed stale wallet: %v", err)
	}

	user, err := Service{DB: db}.Login(ctx, "stale-login", "secret1")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if user.CreditBalance != 5 || user.DailyFreeCreditBalance != 3 || user.PaidCreditBalance != 2 {
		t.Fatalf("wallets total=%d free=%d paid=%d, want 5,3,2", user.CreditBalance, user.DailyFreeCreditBalance, user.PaidCreditBalance)
	}
}
```

- [ ] **Step 2: Run auth tests to verify they fail**

Run:

```bash
cd api && go test ./internal/auth -run 'TestRegisterInitializesDailyFreeWallet|TestLoginRefreshesStaleDailyFreeWallet' -count=1
```

Expected: FAIL because `User` does not expose wallet fields and registration does not initialize them.

- [ ] **Step 3: Extend the user shape and scans**

In `api/internal/auth/service.go`, update `User`:

```go
type User struct {
	ID                     string `json:"id"`
	Username               string `json:"username"`
	Role                   string `json:"role"`
	Status                 string `json:"status"`
	CreditBalance          int    `json:"credit_balance"`
	DailyFreeCreditLimit   int    `json:"daily_free_credit_limit"`
	DailyFreeCreditBalance int    `json:"daily_free_credit_balance"`
	PaidCreditBalance      int    `json:"paid_credit_balance"`
}
```

Add a helper:

```go
func scanUser(row pgx.Row) (User, error) {
	var user User
	err := row.Scan(
		&user.ID,
		&user.Username,
		&user.Role,
		&user.Status,
		&user.CreditBalance,
		&user.DailyFreeCreditLimit,
		&user.DailyFreeCreditBalance,
		&user.PaidCreditBalance,
	)
	return user, err
}
```

Update user `SELECT` and `RETURNING` clauses to select:

```sql
id::text, username, role, status, credit_balance,
daily_free_credit_limit, daily_free_credit_balance, paid_credit_balance
```

- [ ] **Step 4: Initialize wallets during registration and admin bootstrap**

Change registration insert:

```sql
INSERT INTO users (
	username, password_hash, role, status, credit_balance,
	daily_free_credit_limit, daily_free_credit_balance, paid_credit_balance,
	last_daily_free_credit_refreshed_on
)
VALUES ($1, $2, $3, $4, $5, $5, $5, 0, CURRENT_DATE)
RETURNING id::text, username, role, status, credit_balance,
	daily_free_credit_limit, daily_free_credit_balance, paid_credit_balance
```

Change admin bootstrap insert:

```sql
INSERT INTO users (
	username, password_hash, role, status, credit_balance,
	daily_free_credit_limit, daily_free_credit_balance, paid_credit_balance,
	last_daily_free_credit_refreshed_on
)
VALUES ($1, $2, $3, $4, 0, 0, 0, 0, CURRENT_DATE)
ON CONFLICT (username) DO NOTHING
```

- [ ] **Step 5: Refresh before returning login/session users**

Import `imagecreate/api/internal/credits` in `api/internal/auth/service.go`. At the start of `Login`, after password and status checks but before returning, call:

```go
if _, err := (credits.Service{DB: s.DB}).RefreshDailyFreeCredits(ctx, user.ID); err != nil {
	return User{}, err
}
return s.userByID(ctx, user.ID)
```

In `userByID`, call refresh before the final user scan only when the first scan finds an active user. Keep disabled users rejected.

- [ ] **Step 6: Run auth tests**

Run:

```bash
cd api && go test ./internal/auth -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add api/internal/auth/service.go api/internal/auth/middleware.go api/internal/auth/service_test.go
git commit -m "feat: return daily free credit wallets"
```

## Task 4: Wallet-Aware Generation Debit And Refund

**Files:**
- Modify: `api/internal/generations/service.go`
- Modify: `api/internal/generations/service_test.go`
- Modify: `api/internal/credits/service.go`
- Modify: `api/internal/credits/service_test.go`

- [ ] **Step 1: Write failing generation debit tests**

Add to `api/internal/generations/service_test.go`:

```go
func TestCreateTaskDebitsDailyFreeCreditsBeforePaidCredits(t *testing.T) {
	ctx, db := setupGenerationTestDB(t)
	service := testGenerationService(db)
	userID := insertGenerationTestUser(t, ctx, db, "free-first", 0)
	_, err := db.Exec(ctx, `
		UPDATE users
		SET daily_free_credit_limit = 2,
			daily_free_credit_balance = 2,
			paid_credit_balance = 4,
			credit_balance = 6
		WHERE id = $1::uuid
	`, userID)
	if err != nil {
		t.Fatalf("seed wallets: %v", err)
	}

	task, err := service.CreateTask(ctx, CreateTaskInput{UserID: userID, Prompt: "a quiet lake", Ratio: "1:1"})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	var freeBalance, paidBalance, total int
	if err := db.QueryRow(ctx, `
		SELECT daily_free_credit_balance, paid_credit_balance, credit_balance
		FROM users
		WHERE id = $1::uuid
	`, userID).Scan(&freeBalance, &paidBalance, &total); err != nil {
		t.Fatalf("query wallets: %v", err)
	}
	if freeBalance != 1 || paidBalance != 4 || total != 5 {
		t.Fatalf("wallets free=%d paid=%d total=%d, want 1,4,5", freeBalance, paidBalance, total)
	}
	if rows := countLedgerRows(t, ctx, db, userID, task.ID, models.LedgerDailyFreeGenerationDebit); rows != 1 {
		t.Fatalf("daily free debit ledger rows = %d, want 1", rows)
	}
}

func TestCreateTaskDebitsPaidCreditsWhenDailyFreeIsEmpty(t *testing.T) {
	ctx, db := setupGenerationTestDB(t)
	service := testGenerationService(db)
	userID := insertGenerationTestUser(t, ctx, db, "paid-fallback", 0)
	_, err := db.Exec(ctx, `
		UPDATE users
		SET daily_free_credit_limit = 2,
			daily_free_credit_balance = 0,
			paid_credit_balance = 3,
			credit_balance = 3
		WHERE id = $1::uuid
	`, userID)
	if err != nil {
		t.Fatalf("seed wallets: %v", err)
	}

	task, err := service.CreateTask(ctx, CreateTaskInput{UserID: userID, Prompt: "a bright studio", Ratio: "1:1"})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	var freeBalance, paidBalance, total int
	if err := db.QueryRow(ctx, `
		SELECT daily_free_credit_balance, paid_credit_balance, credit_balance
		FROM users
		WHERE id = $1::uuid
	`, userID).Scan(&freeBalance, &paidBalance, &total); err != nil {
		t.Fatalf("query wallets: %v", err)
	}
	if freeBalance != 0 || paidBalance != 2 || total != 2 {
		t.Fatalf("wallets free=%d paid=%d total=%d, want 0,2,2", freeBalance, paidBalance, total)
	}
	if rows := countLedgerRows(t, ctx, db, userID, task.ID, models.LedgerPaidGenerationDebit); rows != 1 {
		t.Fatalf("paid debit ledger rows = %d, want 1", rows)
	}
}
```

- [ ] **Step 2: Run generation tests to verify they fail**

Run:

```bash
cd api && go test ./internal/generations -run 'TestCreateTaskDebitsDailyFreeCreditsBeforePaidCredits|TestCreateTaskDebitsPaidCreditsWhenDailyFreeIsEmpty' -count=1
```

Expected: FAIL because existing debit only uses `credit_balance` and old ledger types.

- [ ] **Step 3: Implement wallet debit**

In `api/internal/generations/service.go`, change `debitGenerationCredit` to return `balanceAfter, walletType, ledgerType`:

```go
func debitGenerationCredit(ctx context.Context, tx pgx.Tx, userID string) (int, string, string, error) {
	var balanceAfter int
	err := tx.QueryRow(ctx, `
		UPDATE users
		SET daily_free_credit_balance = daily_free_credit_balance - 1,
			credit_balance = daily_free_credit_balance - 1 + paid_credit_balance,
			updated_at = now()
		WHERE id = $1::uuid
			AND status = $2
			AND daily_free_credit_balance >= 1
		RETURNING credit_balance
	`, userID, models.UserStatusActive).Scan(&balanceAfter)
	if err == nil {
		return balanceAfter, models.WalletDailyFree, models.LedgerDailyFreeGenerationDebit, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, "", "", fmt.Errorf("debit daily free generation credit: %w", err)
	}

	err = tx.QueryRow(ctx, `
		UPDATE users
		SET paid_credit_balance = paid_credit_balance - 1,
			credit_balance = daily_free_credit_balance + paid_credit_balance - 1,
			updated_at = now()
		WHERE id = $1::uuid
			AND status = $2
			AND paid_credit_balance >= 1
		RETURNING credit_balance
	`, userID, models.UserStatusActive).Scan(&balanceAfter)
	if err == nil {
		return balanceAfter, models.WalletPaid, models.LedgerPaidGenerationDebit, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, "", "", fmt.Errorf("debit paid generation credit: %w", err)
	}

	var status string
	var total int
	err = tx.QueryRow(ctx, `SELECT status, credit_balance FROM users WHERE id = $1::uuid`, userID).Scan(&status, &total)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, "", "", ErrNotFound
	}
	if err != nil {
		return 0, "", "", fmt.Errorf("inspect user credit state: %w", err)
	}
	if status != models.UserStatusActive {
		return 0, "", "", ErrDisabledUser
	}
	return 0, "", "", ErrInsufficientCredits
}
```

Before calling debit inside `CreateTask`, call:

```go
if _, err := (credits.Service{DB: s.DB}).RefreshDailyFreeCreditsTx(ctx, tx, input.UserID); err != nil {
	return Task{}, err
}
```

Write the debit ledger with wallet fields:

```go
INSERT INTO credit_ledger (user_id, task_id, type, wallet_type, amount, balance_after, reason)
VALUES ($1::uuid, $2::uuid, $3, $4, -1, $5, $6)
```

- [ ] **Step 4: Write failing refund tests**

Add to `api/internal/credits/service_test.go`:

```go
func TestRefundGenerationReturnsCreditToOriginalPaidWallet(t *testing.T) {
	ctx, db := setupCreditTestDB(t)
	service := Service{DB: db}
	userID := insertCreditTestUser(t, ctx, db, "paid-refund", models.RoleUser, 0)
	_, err := db.Exec(ctx, `
		UPDATE users
		SET daily_free_credit_limit = 1,
			daily_free_credit_balance = 0,
			paid_credit_balance = 1,
			credit_balance = 1
		WHERE id = $1::uuid
	`, userID)
	if err != nil {
		t.Fatalf("seed wallets: %v", err)
	}

	var taskID string
	if err := db.QueryRow(ctx, `
		INSERT INTO generation_tasks (user_id, prompt, size, status, upstream_model)
		VALUES ($1, 'prompt', '1024x1024', $2, 'test-model')
		RETURNING id::text
	`, userID, models.TaskFailed).Scan(&taskID); err != nil {
		t.Fatalf("insert task: %v", err)
	}
	if _, err := db.Exec(ctx, `
		INSERT INTO credit_ledger (user_id, task_id, type, wallet_type, amount, balance_after, reason)
		VALUES ($1::uuid, $2::uuid, $3, $4, -1, 0, 'generation task created')
	`, userID, taskID, models.LedgerPaidGenerationDebit, models.WalletPaid); err != nil {
		t.Fatalf("insert debit ledger: %v", err)
	}

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	if err := service.RefundGeneration(ctx, tx, userID, taskID, "provider failed"); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("refund generation: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit tx: %v", err)
	}

	var freeBalance, paidBalance, total int
	if err := db.QueryRow(ctx, `
		SELECT daily_free_credit_balance, paid_credit_balance, credit_balance
		FROM users
		WHERE id = $1::uuid
	`, userID).Scan(&freeBalance, &paidBalance, &total); err != nil {
		t.Fatalf("query wallets: %v", err)
	}
	if freeBalance != 0 || paidBalance != 2 || total != 2 {
		t.Fatalf("wallets free=%d paid=%d total=%d, want 0,2,2", freeBalance, paidBalance, total)
	}
}
```

- [ ] **Step 5: Implement wallet-aware refund**

In `api/internal/credits/service.go`, keep the task lock and replace the old refund duplicate check with:

```go
var alreadyRefunded bool
if err := tx.QueryRow(ctx, `
	SELECT EXISTS (
		SELECT 1
		FROM credit_ledger
		WHERE task_id = $1::uuid
			AND type IN ($2, $3)
	)
`, taskID, models.LedgerDailyFreeGenerationRefund, models.LedgerPaidGenerationRefund).Scan(&alreadyRefunded); err != nil {
	return fmt.Errorf("check existing generation refund: %w", err)
}
if alreadyRefunded {
	return nil
}
```

Then find the original debit wallet:

```go
var debitType, walletType string
if err := tx.QueryRow(ctx, `
	SELECT type, wallet_type
	FROM credit_ledger
	WHERE task_id = $1::uuid
		AND type IN ($2, $3)
	ORDER BY created_at ASC
	LIMIT 1
`, taskID, models.LedgerDailyFreeGenerationDebit, models.LedgerPaidGenerationDebit).Scan(&debitType, &walletType); err != nil {
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("generation debit ledger not found")
	}
	return fmt.Errorf("find generation debit wallet: %w", err)
}

refundType := models.LedgerDailyFreeGenerationRefund
walletColumn := "daily_free_credit_balance"
if debitType == models.LedgerPaidGenerationDebit || walletType == models.WalletPaid {
	refundType = models.LedgerPaidGenerationRefund
	walletColumn = "paid_credit_balance"
}
```

Use a small switch instead of interpolating unchecked SQL:

```go
updateSQL := `
	UPDATE users
	SET daily_free_credit_balance = daily_free_credit_balance + 1,
		credit_balance = daily_free_credit_balance + 1 + paid_credit_balance,
		updated_at = now()
	WHERE id = $1::uuid
	RETURNING credit_balance
`
if walletColumn == "paid_credit_balance" {
	updateSQL = `
		UPDATE users
		SET paid_credit_balance = paid_credit_balance + 1,
			credit_balance = daily_free_credit_balance + paid_credit_balance + 1,
			updated_at = now()
		WHERE id = $1::uuid
		RETURNING credit_balance
	`
}
```

Insert refund ledger with:

```sql
INSERT INTO credit_ledger (user_id, task_id, type, wallet_type, amount, balance_after, reason)
VALUES ($1::uuid, $2::uuid, $3, $4, 1, $5, $6)
```

using `refundType` and `walletType`.

Update the existing refund tests in `api/internal/credits/service_test.go` so each task has a matching debit ledger before `RefundGeneration` is called:

```go
if _, err := db.Exec(ctx, `
	INSERT INTO credit_ledger (user_id, task_id, type, wallet_type, amount, balance_after, reason)
	VALUES ($1::uuid, $2::uuid, $3, $4, -1, 0, 'generation task created')
`, userID, taskID, models.LedgerDailyFreeGenerationDebit, models.WalletDailyFree); err != nil {
	t.Fatalf("insert debit ledger: %v", err)
}
```

Change those tests to count `models.LedgerDailyFreeGenerationRefund` or `models.LedgerPaidGenerationRefund` instead of `models.LedgerGenerationRefund`.

- [ ] **Step 6: Run generation and credits tests**

Run:

```bash
cd api && go test ./internal/generations ./internal/credits -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add api/internal/generations/service.go api/internal/generations/service_test.go api/internal/credits/service.go api/internal/credits/service_test.go
git commit -m "feat: debit and refund wallet credits"
```

## Task 5: Admin Paid Credit Adjustments And API Shapes

**Files:**
- Modify: `api/internal/admin/handlers.go`
- Modify: `api/internal/admin/handlers_test.go`
- Modify: `api/internal/credits/service.go`
- Modify: `api/internal/credits/service_test.go`

- [ ] **Step 1: Write failing admin tests**

Add to `api/internal/admin/handlers_test.go`:

```go
func TestAdjustCreditsUpdatesPaidWalletAndTotal(t *testing.T) {
	ctx, db := setupAdminTestDB(t)
	adminID := insertAdminTestUser(t, ctx, db, "admin-paid-adjust", models.RoleAdmin, models.UserStatusActive, 0)
	userID := insertAdminTestUser(t, ctx, db, "paid-adjust-target", models.RoleUser, models.UserStatusActive, 0)
	_, err := db.Exec(ctx, `
		UPDATE users
		SET daily_free_credit_limit = 2,
			daily_free_credit_balance = 1,
			paid_credit_balance = 3,
			credit_balance = 4
		WHERE id = $1::uuid
	`, userID)
	if err != nil {
		t.Fatalf("seed wallets: %v", err)
	}

	req := authenticatedAdminJSONRequest(t, http.MethodPost, "/api/admin/users/"+userID+"/credits", `{"amount":2,"reason":"paid top-up"}`, adminID)
	rr := httptest.NewRecorder()
	testAdminRouter(db).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200", rr.Code, rr.Body.String())
	}

	var freeBalance, paidBalance, total, ledgerRows int
	if err := db.QueryRow(ctx, `
		SELECT daily_free_credit_balance, paid_credit_balance, credit_balance
		FROM users
		WHERE id = $1::uuid
	`, userID).Scan(&freeBalance, &paidBalance, &total); err != nil {
		t.Fatalf("query wallets: %v", err)
	}
	if err := db.QueryRow(ctx, `
		SELECT count(*)
		FROM credit_ledger
		WHERE user_id = $1::uuid
			AND type = $2
			AND wallet_type = $3
	`, userID, models.LedgerPaidAdminAdjustment, models.WalletPaid).Scan(&ledgerRows); err != nil {
		t.Fatalf("count paid admin ledger: %v", err)
	}
	if freeBalance != 1 || paidBalance != 5 || total != 6 || ledgerRows != 1 {
		t.Fatalf("free=%d paid=%d total=%d ledgerRows=%d, want 1,5,6,1", freeBalance, paidBalance, total, ledgerRows)
	}
}
```

- [ ] **Step 2: Run admin test to verify it fails**

Run:

```bash
cd api && go test ./internal/admin -run TestAdjustCreditsUpdatesPaidWalletAndTotal -count=1
```

Expected: FAIL because admin adjustment only updates `credit_balance`.

- [ ] **Step 3: Extend admin user response and list query**

In `api/internal/admin/handlers.go`, extend `userResponse`:

```go
DailyFreeCreditLimit   int `json:"daily_free_credit_limit"`
DailyFreeCreditBalance int `json:"daily_free_credit_balance"`
PaidCreditBalance      int `json:"paid_credit_balance"`
```

Update all user `SELECT` and `RETURNING` clauses to include:

```sql
daily_free_credit_limit, daily_free_credit_balance, paid_credit_balance
```

Update scans in `ListUsers`, `updateUserStatus`, and `adjustCredits`.

- [ ] **Step 4: Change admin adjustment to paid wallet**

Update `adjustCredits` SQL:

```sql
UPDATE users
SET paid_credit_balance = (paid_credit_balance::bigint + $2::bigint)::integer,
	credit_balance = (daily_free_credit_balance::bigint + paid_credit_balance::bigint + $2::bigint)::integer,
	updated_at = now()
WHERE id = $1::uuid
	AND paid_credit_balance::bigint + $2::bigint BETWEEN 0 AND 2147483647
RETURNING id::text, username, role, status, credit_balance,
	daily_free_credit_limit, daily_free_credit_balance, paid_credit_balance,
	created_at, updated_at
```

Write ledger:

```sql
INSERT INTO credit_ledger (user_id, type, wallet_type, amount, balance_after, reason, actor_user_id)
VALUES ($1::uuid, $2, $3, $4, $5, $6, $7::uuid)
```

with `models.LedgerPaidAdminAdjustment` and `models.WalletPaid`.

- [ ] **Step 5: Run admin and credits tests**

Run:

```bash
cd api && go test ./internal/admin ./internal/credits -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add api/internal/admin/handlers.go api/internal/admin/handlers_test.go api/internal/credits/service.go api/internal/credits/service_test.go
git commit -m "feat: adjust paid credit wallet"
```

## Task 6: Background Daily Refresh Loop

**Files:**
- Modify: `api/internal/credits/service.go`
- Modify: `api/internal/credits/service_test.go`
- Modify: `api/cmd/server/main.go`

- [ ] **Step 1: Write failing batch refresh test**

Add to `api/internal/credits/service_test.go`:

```go
func TestRefreshAllDailyFreeCreditsRefreshesOnlyActiveStaleUsers(t *testing.T) {
	ctx, db := setupCreditTestDB(t)
	activeID := insertCreditTestUser(t, ctx, db, "active-stale", models.RoleUser, 0)
	disabledID := insertCreditTestUser(t, ctx, db, "disabled-stale", models.RoleUser, 0)
	_, err := db.Exec(ctx, `
		UPDATE users
		SET daily_free_credit_limit = 4,
			daily_free_credit_balance = 0,
			paid_credit_balance = 1,
			credit_balance = 1,
			last_daily_free_credit_refreshed_on = CURRENT_DATE - 1
		WHERE id IN ($1::uuid, $2::uuid)
	`, activeID, disabledID)
	if err != nil {
		t.Fatalf("seed stale wallets: %v", err)
	}
	if _, err := db.Exec(ctx, `UPDATE users SET status = $2 WHERE id = $1::uuid`, disabledID, models.UserStatusDisabled); err != nil {
		t.Fatalf("disable user: %v", err)
	}

	count, err := Service{DB: db}.RefreshAllDailyFreeCredits(ctx)
	if err != nil {
		t.Fatalf("refresh all: %v", err)
	}
	if count != 1 {
		t.Fatalf("refreshed count = %d, want 1", count)
	}
	if got := creditTestBalance(t, ctx, db, activeID); got != 5 {
		t.Fatalf("active total = %d, want 5", got)
	}
	if got := creditTestBalance(t, ctx, db, disabledID); got != 1 {
		t.Fatalf("disabled total = %d, want 1", got)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run:

```bash
cd api && go test ./internal/credits -run TestRefreshAllDailyFreeCreditsRefreshesOnlyActiveStaleUsers -count=1
```

Expected: FAIL because `RefreshAllDailyFreeCredits` is undefined.

- [ ] **Step 3: Implement batch refresh**

Add to `api/internal/credits/service.go`:

```go
func (s Service) RefreshAllDailyFreeCredits(ctx context.Context) (int, error) {
	rows, err := s.DB.Query(ctx, `
		SELECT id::text
		FROM users
		WHERE status = $1
			AND last_daily_free_credit_refreshed_on < CURRENT_DATE
		ORDER BY id
	`, models.UserStatusActive)
	if err != nil {
		return 0, fmt.Errorf("list users for daily free refresh: %w", err)
	}
	defer rows.Close()

	refreshed := 0
	for rows.Next() {
		var userID string
		if err := rows.Scan(&userID); err != nil {
			return refreshed, fmt.Errorf("scan user for daily free refresh: %w", err)
		}
		ok, err := s.RefreshDailyFreeCredits(ctx, userID)
		if err != nil {
			return refreshed, err
		}
		if ok {
			refreshed++
		}
	}
	if err := rows.Err(); err != nil {
		return refreshed, fmt.Errorf("iterate users for daily free refresh: %w", err)
	}
	return refreshed, nil
}
```

- [ ] **Step 4: Start the refresh loop in the server**

In `api/cmd/server/main.go`, import `imagecreate/api/internal/credits` and add after worker start:

```go
refreshCtx, cancelRefresh := context.WithCancel(context.Background())
defer cancelRefresh()
refreshDone := runDailyFreeCreditRefreshLoop(refreshCtx, credits.Service{DB: db})
```

Add helper functions:

```go
type dailyFreeCreditRefresher interface {
	RefreshAllDailyFreeCredits(context.Context) (int, error)
}

func runDailyFreeCreditRefreshLoop(ctx context.Context, refresher dailyFreeCreditRefresher) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		timer := time.NewTimer(time.Until(nextLocalMidnight()))
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				refreshCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
				count, err := refresher.RefreshAllDailyFreeCredits(refreshCtx)
				cancel()
				if err != nil {
					log.Printf("refresh daily free credits: %v", err)
				} else {
					log.Printf("refreshed daily free credits for %d users", count)
				}
				timer.Reset(time.Until(nextLocalMidnight()))
			}
		}
	}()
	return done
}

func nextLocalMidnight() time.Time {
	now := time.Now()
	next := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 5, 0, now.Location())
	return next
}
```

On shutdown, call `cancelRefresh()` and wait with existing `waitForWorker(refreshDone, cfg.OpenAIRequestTimeout)`.

- [ ] **Step 5: Run tests**

Run:

```bash
cd api && go test ./internal/credits ./cmd/server -count=1
```

Expected: PASS. If `./cmd/server` has no tests, Go should still compile it successfully.

- [ ] **Step 6: Commit**

```bash
git add api/internal/credits/service.go api/internal/credits/service_test.go api/cmd/server/main.go
git commit -m "feat: run daily free credit refresh"
```

## Task 7: Frontend Normalization And Display

**Files:**
- Modify: `web/src/api/client.ts`
- Modify: `web/src/api/client.test.ts`
- Modify: `web/src/pages/WorkspacePage.tsx`
- Modify: `web/src/pages/WorkspacePage.test.tsx`
- Modify: `web/src/pages/AdminPage.tsx`
- Modify: `web/src/pages/AdminPage.test.tsx`

- [ ] **Step 1: Write failing client normalization test**

Add to `web/src/api/client.test.ts`:

```ts
it("normalizes split credit wallet fields", () => {
  const { user } = normalizeAuthResponse({
    user: {
      id: "user-1",
      username: "alice",
      role: "user",
      status: "active",
      credit_balance: 7,
      daily_free_credit_limit: 5,
      daily_free_credit_balance: 2,
      paid_credit_balance: 5,
    },
  });

  expect(user.creditBalance).toBe(7);
  expect(user.dailyFreeCreditLimit).toBe(5);
  expect(user.dailyFreeCreditBalance).toBe(2);
  expect(user.paidCreditBalance).toBe(5);
});
```

- [ ] **Step 2: Run the client test to verify it fails**

Run:

```bash
cd web && npm test -- --run src/api/client.test.ts
```

Expected: FAIL because `User` does not expose wallet fields.

- [ ] **Step 3: Extend frontend user types and normalizers**

In `web/src/api/client.ts`, update `User`:

```ts
export type User = {
  id: string;
  username: string;
  role: "user" | "admin";
  status: "active" | "disabled";
  creditBalance: number;
  dailyFreeCreditLimit: number;
  dailyFreeCreditBalance: number;
  paidCreditBalance: number;
};
```

Update `ApiUser`:

```ts
type ApiUser = User & {
  credit_balance?: number;
  daily_free_credit_limit?: number;
  dailyFreeCreditLimit?: number;
  daily_free_credit_balance?: number;
  dailyFreeCreditBalance?: number;
  paid_credit_balance?: number;
  paidCreditBalance?: number;
  created_at?: string;
  createdAt?: string;
  updated_at?: string;
  updatedAt?: string;
};
```

Update `normalizeUser`:

```ts
const dailyFreeCreditLimit = user.dailyFreeCreditLimit ?? user.daily_free_credit_limit ?? 0;
const dailyFreeCreditBalance = user.dailyFreeCreditBalance ?? user.daily_free_credit_balance ?? 0;
const paidCreditBalance = user.paidCreditBalance ?? user.paid_credit_balance ?? 0;
const creditBalance = user.creditBalance ?? user.credit_balance ?? dailyFreeCreditBalance + paidCreditBalance;

return {
  id: user.id,
  username: user.username,
  role: user.role,
  status: user.status,
  creditBalance,
  dailyFreeCreditLimit,
  dailyFreeCreditBalance,
  paidCreditBalance,
};
```

- [ ] **Step 4: Update workspace display**

In `web/src/pages/WorkspacePage.tsx`, find the balance display and add text using the current user object:

```tsx
<span>今日免费额度 {user.dailyFreeCreditBalance}/{user.dailyFreeCreditLimit}</span>
<span>付费额度 {user.paidCreditBalance}</span>
```

Keep the existing total balance display if present.

- [ ] **Step 5: Update admin display**

In `web/src/pages/AdminPage.tsx`, add columns or compact cells for:

```tsx
<td>{user.dailyFreeCreditBalance}/{user.dailyFreeCreditLimit}</td>
<td>{user.paidCreditBalance}</td>
<td>{user.creditBalance}</td>
```

Use existing table styling and Chinese labels:

```tsx
<th>今日免费</th>
<th>付费额度</th>
<th>合计</th>
```

- [ ] **Step 6: Update frontend tests**

Update test fixtures that create users to include:

```ts
daily_free_credit_limit: 5,
daily_free_credit_balance: 5,
paid_credit_balance: 0,
```

Add expectations:

```ts
expect(screen.getByText(/今日免费额度 5\/5/)).toBeInTheDocument();
expect(screen.getByText(/付费额度 0/)).toBeInTheDocument();
```

- [ ] **Step 7: Run frontend tests**

Run:

```bash
cd web && npm test -- --run src/api/client.test.ts src/pages/WorkspacePage.test.tsx src/pages/AdminPage.test.tsx
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add web/src/api/client.ts web/src/api/client.test.ts web/src/pages/WorkspacePage.tsx web/src/pages/WorkspacePage.test.tsx web/src/pages/AdminPage.tsx web/src/pages/AdminPage.test.tsx
git commit -m "feat: show split credit wallets"
```

## Task 8: Full Verification

**Files:**
- Verify only; no planned edits.

- [ ] **Step 1: Run all Go tests**

Run:

```bash
cd api && go test ./... -count=1
```

Expected: PASS.

- [ ] **Step 2: Run all frontend tests**

Run:

```bash
cd web && npm test -- --run
```

Expected: PASS.

- [ ] **Step 3: Inspect git status**

Run:

```bash
git status --short
```

Expected: no unstaged or uncommitted implementation changes.

## Self-Review

- Spec coverage: the plan covers schema migration, free daily refresh, paid wallet preservation, free-first debit, paid fallback, original-wallet refunds, admin paid adjustments, compatibility total, API fields, UI display, and tests.
- Placeholder scan: no `TBD`, `TODO`, or open-ended implementation steps remain.
- Type consistency: Go fields use `DailyFreeCreditLimit`, `DailyFreeCreditBalance`, `PaidCreditBalance`; JSON uses `daily_free_credit_limit`, `daily_free_credit_balance`, `paid_credit_balance`; frontend uses `dailyFreeCreditLimit`, `dailyFreeCreditBalance`, `paidCreditBalance`.
