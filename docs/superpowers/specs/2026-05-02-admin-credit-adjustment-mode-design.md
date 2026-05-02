# Admin Credit Adjustment Mode Design

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let administrators either add to or subtract from a user's credit balance from the existing credit management table.

**Architecture:** Keep the current admin credit adjustment endpoint and ledger model, but make the UI expose an explicit mode switch for increase vs decrease. The frontend will convert the entered positive amount into a signed delta before submission. The backend will accept positive or negative deltas, keep the existing balance floor checks, and continue writing a single audit trail entry for every adjustment.

**Tech Stack:** Go backend, PostgreSQL, React + TypeScript frontend, existing admin API and test suites

---

## Behavior

- The admin credit table shows an `增加 / 扣减` switch for each user.
- The amount field stays positive-only in the UI.
- `增加` submits a positive `amount`.
- `扣减` submits the same amount as a negative `amount`.
- A zero amount remains invalid.
- A subtraction that would push the user's paid credit balance below zero is rejected and leaves the user unchanged.
- Successful adjustments still write the existing admin adjustment ledger record and admin audit log entry.

## Scope

This change only affects the admin credit adjustment flow.

It does not change:

- generation debit / refund behavior
- invite creation
- user status changes
- password reset flows
- credit ledger schema

## Files

- Modify: `web/src/pages/AdminPage.tsx`
- Modify: `web/src/pages/AdminPage.test.tsx`
- Modify: `api/internal/admin/handlers.go`
- Modify: `api/internal/admin/handlers_test.go`
- Modify: `api/internal/credits/service.go`
- Modify: `api/internal/credits/service_test.go`

## Acceptance Criteria

- Administrators can choose increase or decrease before submitting a credit adjustment.
- The frontend sends negative amounts for decrease mode.
- The backend applies negative adjustments correctly.
- The backend rejects adjustments that would overdraw the paid balance.
- The admin UI still refreshes the user row after a successful change.
- Existing adjustment and balance tests continue to pass.

