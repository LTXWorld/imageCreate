# Admin Credit Adjustment Mode Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an explicit increase/decrease mode to admin credit adjustments.

**Architecture:** The existing admin credit endpoint remains the single backend path. Frontend state records a per-user mode and positive amount, then submits a signed delta. Backend tests confirm negative deltas are supported and protected by existing balance guards.

**Tech Stack:** Go, PostgreSQL, React, TypeScript, Vitest, Go test

---

### Task 1: Backend Negative Adjustment Coverage

**Files:**
- Modify: `api/internal/admin/handlers_test.go`

- [ ] **Step 1: Write failing handler tests**

Add tests covering `POST /api/admin/users/{id}/credits` with a negative amount and with an overdraft attempt.

- [ ] **Step 2: Run backend admin tests**

Run: `go test ./internal/admin`

- [ ] **Step 3: Keep or adjust implementation**

If the tests reveal missing behavior, update `api/internal/admin/handlers.go` so negative non-zero amounts are valid, paid balance cannot become negative, and ledger/audit records keep the signed amount.

- [ ] **Step 4: Re-run backend admin tests**

Run: `go test ./internal/admin`

### Task 2: Frontend Adjustment Mode

**Files:**
- Modify: `web/src/pages/AdminPage.tsx`
- Modify: `web/src/pages/AdminPage.test.tsx`

- [ ] **Step 1: Write failing UI test**

Add a test that switches Alice's credit row to `扣减`, enters a positive amount, submits, and expects the fetch body to contain a negative `amount`.

- [ ] **Step 2: Run the AdminPage test**

Run: `npm test -- AdminPage.test.tsx`

- [ ] **Step 3: Implement UI state and controls**

Add a per-user `mode` to `CreditDraft`, default it to `increase`, render an `增加 / 扣减` select, set the amount input `min="1"`, and submit the signed amount.

- [ ] **Step 4: Re-run the AdminPage test**

Run: `npm test -- AdminPage.test.tsx`

### Task 3: Full Verification

**Files:**
- No production files expected

- [ ] **Step 1: Run focused backend tests**

Run: `go test ./internal/admin ./internal/credits`

- [ ] **Step 2: Run focused frontend tests**

Run: `npm test -- AdminPage.test.tsx`

- [ ] **Step 3: Check worktree diff**

Run: `git diff --stat`

