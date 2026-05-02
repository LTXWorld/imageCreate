# Admin Daily Free Limit Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an admin control for changing a user's daily free credit limit without changing today's remaining free balance.

**Architecture:** Add one admin API endpoint that updates only `users.daily_free_credit_limit` inside the existing admin handler module and writes an audit log in the same transaction. Add one separate control in the existing admin credit table so free limit changes stay distinct from paid credit adjustments.

**Tech Stack:** Go, PostgreSQL, React, TypeScript, Vitest, Go test

---

### Task 1: Backend Endpoint

**Files:**
- Modify: `api/internal/admin/handlers.go`
- Modify: `api/internal/admin/handlers_test.go`
- Modify: `api/internal/app/routes.go`

- [ ] **Step 1: Write failing backend tests**

Add tests that call `PATCH /api/admin/users/{id}/daily-free-limit` and assert:

- `daily_free_credit_limit` changes.
- `daily_free_credit_balance`, `paid_credit_balance`, and `credit_balance` do not change.
- action `update_daily_free_credit_limit` is written to `audit_logs`.
- zero is accepted.
- negative values are rejected.

- [ ] **Step 2: Run backend admin tests**

Run: `go test ./internal/admin`

- [ ] **Step 3: Implement route and handler**

Add `UpdateDailyFreeCreditLimit` to `api/internal/admin/handlers.go`, wire it in `api/internal/app/routes.go`, validate non-negative integer values, update only `daily_free_credit_limit`, and return the existing `userResponse`.

- [ ] **Step 4: Re-run backend admin tests**

Run: `go test ./internal/admin`

### Task 2: Frontend Control

**Files:**
- Modify: `web/src/pages/AdminPage.tsx`
- Modify: `web/src/pages/AdminPage.test.tsx`

- [ ] **Step 1: Write failing frontend test**

Add a test that opens the credit tab, changes Alice's daily free limit input, submits it, and expects:

- request path `/api/admin/users/user-1/daily-free-limit`
- method `PATCH`
- body `{ "daily_free_credit_limit": 7 }`
- updated user row shows `3/7`

- [ ] **Step 2: Run AdminPage test**

Run: `npm test -- AdminPage.test.tsx`

- [ ] **Step 3: Implement UI state and submit handler**

Add per-user free-limit draft state, render a compact number input and submit button in the credit table, and update the user row from the response.

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

