# GitHub Actions Deploy Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add CI and production deployment workflows for `img.bfsmlt.top`.

**Architecture:** GitHub Actions runs the existing Go, web, and Docker Compose verification on every push and pull request. A separate deploy workflow waits for CI on `master`, then SSHes to the VPS, checks out or updates the repository, and runs Docker Compose using the VPS-local `.env`.

**Tech Stack:** GitHub Actions, Go 1.22, Node 22, npm, Docker Compose, SSH, Caddy.

---

### Task 1: Add CI Workflow

**Files:**
- Create: `.github/workflows/ci.yml`

- [x] **Step 1: Create a workflow that runs tests and builds**

Run Go tests from `api`, install web dependencies with `npm ci`, run Vitest and Vite build, then validate Compose with `docker compose config`.

### Task 2: Add Deploy Workflow

**Files:**
- Create: `.github/workflows/deploy.yml`

- [x] **Step 1: Deploy only after successful CI on `master`**

Use `workflow_run` so deployment is gated by CI success. Use repository secrets for SSH target and health check URL.

### Task 3: Document Setup

**Files:**
- Modify: `README.md`

- [x] **Step 1: Document DNS, VPS `.env`, and GitHub Secrets**

Describe `img.bfsmlt.top`, required VPS setup, required GitHub Secrets, and the deploy behavior.
