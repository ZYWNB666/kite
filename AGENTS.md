# AGENTS.md — Agent instructions for kite

## Build order matters

Frontend **must** build before backend. The Go binary uses `//go:embed static` in `static.go`, so `static/` must exist at compile time. A bare `go build .` on a fresh clone will fail.

```bash
make deps          # pnpm install (ui/) + go mod download
make build         # frontend → static/, then go build (correct order)
```

If only editing Go code and `static/` already exists, `make backend` is safe alone.

## Commands

| Task | Command |
|---|---|
| Dev server (backend + Vite) | `make dev` |
| Run all tests | `make test` |
| Single Go test | `go test -v -run TestName ./pkg/...` |
| Frontend tests only | `cd ui && pnpm run test` |
| Frontend watch mode | `cd ui && pnpm run test:watch` |
| Lint everything | `make lint` |
| Format everything | `make format` |
| Pre-commit check | `make pre-commit` (format + lint) |
| E2E tests | `make e2e-test` (requires kind + docker + kite binary) |

## Go testing quirks

- Uses `github.com/bytedance/mockey` for mocking — **not** testify/mock or gomock.
- Tests using mockey **must** set `MOCKEY_CHECK_GCFLAGS=false` (see `internal/load_test.go` init()).
- Two mockey styles exist: `mockey.PatchConvey(...)` and `mockey.Mock(fn).Return(val).Build()`.
- `testify/assert` is used sparingly (2 files); most tests use `t.Fatalf`/`t.Logf`.
- No external K8s cluster needed for unit tests — mockey handles client mocking.

## Frontend specifics

- **pnpm** only (not npm/yarn). Each of `ui/`, `e2e/`, `docs/` is an independent pnpm project — no workspace root.
- Node: `^20.19.0 || >=22.12.0` required.
- **Tailwind v4**: no `tailwind.config.*`. Uses `@tailwindcss/vite` plugin with CSS-first config.
- **Vite 8**: uses `rolldownOptions` (not `rollupOptions`).
- **Prettier**: no semicolons, single quotes, 2-space indent, trailing comma es5, imports sorted by `@ianvs/prettier-plugin-sort-imports`.
- **TypeScript strict**: `strict: true`, `noUnusedLocals`, `noUnusedParameters`, `erasableSyntaxOnly`.
- Path alias: `@` → `./src`.
- `global` aliased to `globalThis` in vite define (xterm.js compat).
- Build output is `../static/` (not `dist/`). That directory is gitignored and embedded by Go.

## Backend architecture

- **Gin** framework. Middleware chain order matters: `Metrics → GZIP → Recovery → Logger → CORS → Auth → ClusterMiddleware → RBACMiddleware`.
- **GORM** with AutoMigrate on startup — no migration scripts. To add a model, append it to the models slice in `model.go:InitDB()`.
- **CGO_ENABLED=0** always. Pure-Go SQLite driver (`glebarez/sqlite`). Cross-compilation is straightforward.
- **golangci-lint** is downloaded to `./bin/golangci-lint` by the Makefile (not globally installed).
- Version vars (`pkg/version.Version` etc.) default to "dev"/"unknown" and are set via `-ldflags` at build time.
- `common.LoadEnvs()` mutates package-level vars (not a config struct). Tests must save/restore manually.
- `/_all` in REST API means "all namespaces" (like `--all-namespaces` in kubectl).

## Runtime base path (`KITE_BASE`)

`index.html` contains a `__KITE_BASE__` placeholder. The Go server replaces it at serve-time via `utils.InjectKiteBase()`. The Vite build uses a custom `runtimeBaseHtmlPlugin` for this. Do not replace the placeholder with a hardcoded path.

## Git hooks

Manual install: `scripts/install-hooks.sh` (runs `make pre-commit`). No husky.

## E2E tests

Prerequisites: kind cluster (`make e2e-kind-up`), docker, kite binary. LDAP/Dex sidecars via `make e2e-setup-ldap` / `make e2e-setup-dex`. Filter specs with `SPEC=` env var. Single worker, 3min timeout, port 38080.
