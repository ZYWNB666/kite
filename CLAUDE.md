# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Kite is a Kubernetes dashboard — a single Go binary that embeds a React frontend. It provides multi-cluster management, resource CRUD, real-time observability (Prometheus), enterprise auth (OAuth/LDAP/RBAC), and an AI assistant.

## Build & Run

```bash
# Install all dependencies (frontend + backend)
make deps

# Build everything (frontend → static/, then Go binary)
make build

# Run the server (defaults to :8080)
make run

# Development mode: backend on :8080 + Vite dev server on :5173
make dev

# Run all tests
make test

# Run a single Go test
go test -v -run TestName ./pkg/...

# Lint (golangci-lint + go vet + pnpm lint)
make lint

# Format code (go fmt + pnpm format)
make format

# Cross-compile for amd64/arm64 Linux
make cross-compile
```

### Key environment variables

- `PORT` — server port (default 8080)
- `DB_DSN` / `DB_TYPE` — database path and type (sqlite/mysql/postgres, default sqlite:`dev.db`)
- `JWT_SECRET` — JWT signing secret
- `KITE_ENCRYPT_KEY` — encryption key for sensitive DB fields
- `KITE_BASE` — subpath prefix for reverse proxy deployments (e.g., `/kite`)
- `CORS_ALLOWED_ORIGINS` — comma-separated CORS origins (for Vite dev: `http://localhost:5173`)
- `ANONYMOUS_USER_ENABLED=true` — allow unauthenticated access (dev only)
- `KITE_USERNAME` / `KITE_PASSWORD` — auto-create super user on first start
- `OAUTH_DEFAULT_ROLE` — role for auto-created OAuth users (default `viewer`)

## Architecture

### Startup flow

1. `main.go` — parse flags, call `initializeApp()`, build Gin engine, start HTTP server
2. `app.go:initializeApp()` — load envs, init DB, load general settings, init RBAC, init templates, create ClusterManager
3. `internal/load.go` — auto-creates super user from env vars, imports kubeconfig from file/env
4. `routes.go` — registers all API routes on the Gin router
5. `static.go` — serves the embedded React SPA (`//go:embed static`)

### Backend package map

| Package | Purpose |
|---|---|
| `pkg/common/` | Shared constants, env loading (`common.go`) |
| `pkg/cluster/` | Multi-cluster manager — each cluster has a `ClientSet` (K8s client + Prom client) |
| `pkg/kube/` | Kubernetes client wrapper (REST config, exec, logs, proxy, terminal) |
| `pkg/handlers/` | HTTP handlers — overview, logs, terminal, proxy, search, kubectl, GPU, image tags |
| `pkg/handlers/resources/` | Per-resource CRUD handlers using a generic `resourceHandler` interface |
| `pkg/auth/` | Auth handler, OAuth providers (OTel Dex, Feishu), LDAP, JWT + API key middleware |
| `pkg/middleware/` | RBAC enforcement, cluster injection, CORS, request logging, Prometheus metrics, static caching |
| `pkg/model/` | GORM models (User, Cluster, Role, Template, APIKey, AuditLog, etc.) + DB init |
| `pkg/rbac/` | RBAC authorization — role matching, CanAccess/CanProxy checks, role CRUD handlers |
| `pkg/ai/` | AI assistant — Anthropic/OpenAI clients, chat SSE, resource/prometheus/event tools |
| `pkg/prometheus/` | Prometheus client for resource usage metrics |
| `pkg/version/` | Build version info + GitHub release update checker |
| `pkg/utils/` | Misc utilities (encryption, search helpers, pod utilities) |
| `internal/` | Startup bootstrap — user creation, kubeconfig import |

### Middleware chain (order matters)

Metrics → GZIP → Recovery → Logger → CORS → [Auth: RequireAuth] → [ClusterMiddleware] → [RBACMiddleware]

### Resource handlers pattern

Resources are registered in `routes.go:RegisterRoutes()` using a `resourceHandler` interface (`handlers/resources/handler.go`). Most resources use `NewGenericResourceHandler[T, L]` with Go generics. Some resources (Pods, Nodes, Events) have custom handlers due to additional logic (restarts, metrics, related resources). All resource routes get RBAC-gated via `RBACMiddleware()`.

### Frontend

React 19 + TypeScript in `ui/`. Vite builds to `../static/`. Key tech: Tailwind CSS v4, Monaco editor, xterm.js, Recharts. Manual chunk splitting: Monaco editor, terminal (xterm), and log viewer are separate chunks.

- `ui/src/pages/` — one page per K8s resource type + login, settings, overview
- `ui/src/components/` — shared UI components (YAML editor, terminal, log viewer, AI chat, charts, etc.)
- `ui/src/contexts/` — React context providers (auth, cluster, theme, search, etc.)
- `ui/src/i18n/` — English + Chinese translations
- `ui/src/lib/` — API client, base-path normalization, utilities

### Database

GORM with SQLite (default), MySQL, or Postgres. Models: User, Cluster, Role, RoleAssignment, APIKey, Template, AuditLog, OAuthProvider, LDAPSetting, GeneralSetting, PendingSession, ResourceHistory. DB init runs auto-migration on startup.

## Git workflow

- Main branch: `main`
- Follow existing commit message style: `feat:` / `fix:` prefixes, Chinese descriptions used in recent commits but English is fine too
- Pre-commit: `make pre-commit` (format + lint)
