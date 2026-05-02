# Admin Daily Free Balance Top-Up Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an admin control for increasing a user's remaining free balance for today without changing the daily free limit.

**Architecture:** Add one admin API endpoint that updates only `users.daily_free_credit_balance` and keeps the existing daily limit and paid balance untouched. Add one compact top-up control in the existing admin credit table so the three admin actions stay separate: free limit, free balance top-up, and paid credit adjustment.

**Tech Stack:** Go, PostgreSQL, React, TypeScript, Vitest, Go test

---

### Task 1: Backend Endpoint

**Files:**
- Modify: `api/internal/admin/handlers.go`
- Modify: `api/internal/admin/handlers_test.go`
- Modify: `api/internal/app/routes.go`

- [ ] **Step 1: Write failing backend tests**

Add tests that call `PATCH /api/admin/users/{id}/daily-free-balance` and assert:

- `daily_free_credit_balance` increases by the submitted amount.
- `daily_free_credit_limit` does not change.
- `paid_credit_balance` does not change.
- `credit_balance` changes by the same amount as the free balance.
- zero and negative values are rejected.
- action `top_up_daily_free_credit_balance` is written to `audit_logs`.

- [ ] **Step 2: Run backend admin tests**

Run: `go test ./internal/admin`

- [ ] **Step 3: Implement route and handler**

Add `TopUpDailyFreeBalance` to `api/internal/admin/handlers.go`, wire it in `api/internal/app/routes.go`, validate a positive integer amount, update only `daily_free_credit_balance` and `credit_balance`, and return the existing `userResponse`.

- [ ] **Step 4: Re-run backend admin tests**

Run: `go test ./internal/admin`

### Task 2: Frontend Control

**Files:**
- Modify: `web/src/pages/AdminPage.tsx`
- Modify: `web/src/pages/AdminPage.test.tsx`

- [ ] **Step 1: Write failing frontend test**

Add a test that opens the credit tab, enters a top-up amount for Alice, submits it, and expects:

- request path `/api/admin/users/user-1/daily-free-balance`
- method `PATCH`
- body `{ "amount": 3 }`
- updated `今日免费 X/Y` is rendered after success

- [ ] **Step 2: Run AdminPage test**

Run: `npm test -- AdminPage.test.tsx`

- [ ] **Step 3: Implement UI state and submit handler**

Add per-user free-balance draft state, render a compact input and button for the top-up control, and update the user row from the response without changing the existing free-limit or paid-credit controls.

- [ ] **Step 4: Re-run AdminPage test**

Run: `npm test -- AdminPage.test.tsx`

### Task 3: Verification

**Files:**
- No production files expected

- [ ] **Step 1: Run backend focused tests**

Run: `go test ./internal/admin ./internal/credits`

- [ ] **Step 2: Run frontend focused tests**

Run: `npm test -- AdminPage.test.tsx`

- [ ] **Step 3: Inspect diff**

Run: `git diff --stat`

