# Repository Guidelines

## Project Structure & Module Organization

Skyroom combines a Go API with a Next.js web client. `cmd/api/` contains the API entrypoint; business logic lives under `internal/`, with shared HTTP and PostgreSQL infrastructure in `internal/platform/`. The frontend is in `apps/web/app/` and reusable React components are in `apps/web/components/`. Keep API definitions in `contracts/`, embedded SQL migrations in `database/migrations/`, container and deployment files in `platform/`, and product or architecture notes in `docs/`. Treat `design-system/skyroom/MASTER.md` as the visual-system reference.

## Build, Test, and Development Commands

- `docker compose -f platform/deployment/compose.dev.yaml up -d` starts local PostgreSQL and Mailpit.
- `go run ./cmd/api` serves the API on the configured `HTTP_ADDR` (default `:8080`).
- `npm install` installs root workspace dependencies; `npm run dev:web` starts Next.js on port 3000.
- `go test -race ./cmd/... ./internal/...` runs the CI Go suite with race detection.
- `go vet ./cmd/... ./internal/...` performs Go static analysis.
- `npm run typecheck && npm run lint && npm run build:web` verifies strict TypeScript, ESLint, and the production web build.

## Coding Style & Naming Conventions

Follow `.editorconfig`: UTF-8, LF endings, final newline, two-space indentation, and tabs for Go. Format Go changes with `gofmt`; use idiomatic package names and exported `PascalCase` identifiers. TypeScript is strict. Name React components in `PascalCase`, functions and variables in `camelCase`, and component files in kebab case (for example, `service-status.tsx`). Preserve the `@/` frontend import alias. ESLint uses Next.js Core Web Vitals and TypeScript rules with zero warnings allowed.

## Testing Guidelines

Place Go tests beside their implementation as `*_test.go`, name cases `TestBehavior`, and use `t.Parallel()` when state is isolated. PostgreSQL integration tests read `SKYROOM_TEST_DATABASE_URL`; CI provides PostgreSQL 17. Add regression coverage for service, HTTP, storage, and migration behavior. No numeric coverage threshold is enforced, but new behavior should exercise success and failure paths. The frontend currently relies on type, lint, and build checks; introduce focused tests when adding a test runner.

## Commit & Pull Request Guidelines

Work on milestone branches named `feat/<milestone-name>`; do not commit directly to `staging` without explicit approval. History uses short, imperative Conventional Commit-style subjects such as `feat:`, `chore:`, `docs:`, and `ops:`. Keep commits scoped and explain migration or contract effects in the body. Pull requests should summarize the change, link the relevant issue, list verification commands, and include screenshots for UI changes. Call out environment variables, database migrations, deployment implications, or changes to `contracts/openapi.yaml` explicitly. Never commit secrets; derive local configuration from `.env.example` or `platform/deployment/production.env.example`.
