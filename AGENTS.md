# AGENTS.md

This file provides guidance to Codex (Codex.ai/code) when working with code in this repository.

## Overview

Monorepo for a student-community platform (personal academic management + peer collaboration; Spanish-language project, docs/comments are in Spanish). Flutter front (web/desktop/mobile) over several Go microservices sharing one Postgres. Deployed to a k8s cluster ("Pillán", ingress `ti3-brojas.dev.censei.cl`); images pushed to GHCR by `.github/workflows/build-images.yml` on pushes to `Development` (no auto-deploy; restart deployments manually). Cluster manifests live in `k8s/` (namespace `student-brojas`); `k8s/create-secrets.sh` is a template for the `ti3-secrets`/`ghcr-cred` secrets. Default branch for PRs is `main`; work happens on `Development`.

## Architecture

- `back/auth` (:8080) — identity: register/login/refresh/logout (refresh token rotation), password reset (SMTP), profile, Google Drive/Calendar OAuth connections, and `/internal/*` endpoints for other services.
- `back/notes` (:8082) — notes CRUD, likes, saves, sharing to groups, attachments. **Note content lives as Markdown in the author's Google Drive**; Postgres only stores metadata (`external_file_id`). Has a Drive reconcile job (`service/reconcile_drive.go`).
- `back/social` (:8083) — groups, members, todo/kanban, sprint sheets, meetings (Calendar + Discord notifiers), hours, Stream chat tokens. Uses **sqlc** (`db/queries` + `db/schema.sql` → `internal/repository/sqlc`).
- `back/infacadem` (HTTP :8081, gRPC :9081) — academic: curricula, subjects, grades (tree + weighted average), schedules, absences/attendance, requirements, overview. Registers `AcademicService` gRPC (stubs in `internal/gen/academicv1`).
- `back/proto` — buf modules, one per team (`common`, `infacadem`; `auth`/`notes`/`social`/`realtime` dirs exist). `back/ai` and `back/realtime` are mostly empty scaffolds, not wired into compose.
- `front` — Flutter; `lib/core/services/*` are HTTP clients per backend, `lib/features/*` screens, local-first Drift DB in `lib/core/database`. `front/nginx.conf` is the single-origin gateway: `/api-auth`, `/api-notes`, `/api-social`, `/api-academic` → service (prefix stripped); `/api-*/internal/` returns 404.

Conventions shared by the Go services:
- Each service is an **independent Go module** (`github.com/kurovoxx/tallerintegracion3/back/<svc>`, Go 1.26, gin + pgx); no `go.work`. Run Go commands from inside the service dir.
- Layering: `cmd/server/main.go` wires `handler/http → service → repository` by hand (`internal/{config,handler,middleware,model,repository,service,utils}`).
- Services **must not import each other**. Shared snippets (e.g. `social/internal/interceptor`, JWT middleware) are deliberately copied per service — keep copies identical.
- One Postgres, one schema per service: `identity`, `academic`, `notes`, `social`, `ai` (defined in `docker/postgres/init.sql`; per-service migrations in `back/<svc>/db/migrations`).
- Auth: user JWT (HS256, `JWT_SECRET`, falls back to `JWT` env) validated locally by each service; user id comes from the token, never from client input. Service-to-service calls use the `X-Internal-Key` header (gRPC metadata `x-internal-key`) = `INTERNAL_API_KEY`; Social/Notes call Auth `/internal/*` (calendar token, user lookup).
- `notes`/`social` have in-memory stores (`store_memory*.go`) used by tests; Postgres-backed ones are `store_pg.go` / repositories. External providers are switchable: `CALENDAR_MODE`, `STREAM_MODE` (`mock` locally), `STORAGE_MODE` (notes, `drive`).
- Env var names differ per service (`PORT` vs `SOCIAL_PORT`/`NOTES_PORT`); `PORT` is the fallback. See `.env.example`.

## Commands

Local stack (needs `.env`: `cp .env.example .env`; compose maps auth→8085, notes→8082, social→8083, infacadem→8087, front→8086, postgres→5432):
```bash
docker compose up -d --build            # or scripts/run-local.sh (rebuilds front w/ --no-cache)
curl http://localhost:8085/health       # auth; also :8082, :8083, :8087 /health
```
`scripts/run-deploy.sh` builds the front against the public domain (reads `.env.prod`).

Go (from `back/<svc>`):
```bash
go build ./...
go vet ./...
go test ./... -count=1
go test ./internal/service/ -run TestName -count=1     # single test
```
Integration tests are opt-in via env and skip otherwise: `AUTH_TEST_DATABASE_URL`, `PASSWORD_RESET_TEST_DATABASE_URL`, `NOTES_TEST_DATABASE_URL`, `SOCIAL_TEST_DATABASE_URL`, `ACADEMIC_TEST_DATABASE_URL`. Auth integration/repo tests create/drop schema `identity` — use a dedicated DB (CI uses `auth_test`) and run with `-p 1`. CI: `test-auth.yml` (auth; unit = `go test ./... -skip 'TestAuthIntegration_|TestE2E_|TestPasswordResetRepository'`, e2e = `-run 'TestE2E_'`), `test-social-groups.yml` (full social suite with Postgres, `-race -p 1`), `test-cross-services.yml` (`-tags=crossservice` E2E Auth+Notes+Social as processes, needs `CROSS_SERVICE_TEST_DATABASE_URL` ending in `_test`), `grpc-ci.yml` (buf generate + breaking check).

sqlc (social): `cd back/social && sqlc generate`.

Protobuf (never run `buf` from `back/proto` root — nondeterministic across nested modules; always inside a module):
```bash
make tools && make generate             # common module; installs buf into ./bin (needs docker, curl; Linux buf binary)
bash back/proto/infacadem/generate.sh   # infacadem module
```
Generated code is committed. Stubs land in `back/proto/gen/go/<svc>/v<N>` (common) or `back/infacadem/internal/gen` (infacadem). Proto layout: `back/proto/<svc>/v<N>/<resource>.proto`, `package <svc>.v<N>;` (buf lint enforces dir match), `go_package = "github.com/kurovoxx/tallerintegracion3/back/proto/gen/go/<svc>/v<N>;<pkg>"`. `social/internal/interceptor` (gRPC internal-key auth + logging) is wired into Social's gRPC server (`:9083`, `startGRPC`).

Flutter (from `front`):
```bash
flutter pub get
flutter test                            # single: flutter test test/api_config_test.dart
flutter analyze
dart run build_runner build             # regenerate Drift app_database.g.dart
flutter run -d chrome --dart-define=API_BASE_URL=http://localhost:8085 --dart-define=NOTES_BASE_URL=http://localhost:8082 --dart-define=SOCIAL_BASE_URL=http://localhost:8083 --dart-define=ACADEMIC_BASE_URL=http://localhost:8087
```
Front defaults point at the Pillán cluster, not localhost (`lib/core/services/api_config.dart`); override with `--dart-define` (see `front/.env.local.example`). Paths starting with `/` are resolved against the web origin (nginx proxy) or Pillán on native.

UI work in `front/lib` must follow `.skills/flutter_neobrutalism_ui.md` (canonical neobrutalist design spec; design tokens in `lib/core/theme/app_theme.dart`).

## Docs worth knowing

- `agentApiContract.md` / `agentApiContract2.md` — API contract; `endpointDoc/` — one file per endpoint; `documentation/reglas_negocio.md` — business rules (RN-* IDs, MoSCoW) referenced in code and tests; `agentMain.md` / `agentSql.md` — product master prompt and SQL schema notes; `documentation/doc<Name>.md` — per-developer task logs.
- `.env`, `.env.prod`, and `docker-compose.yml` contain real-looking dev credentials/defaults; `.env` is gitignored but `.env.prod` is tracked — don't add secrets there.
