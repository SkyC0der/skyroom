# Skyroom

Skyroom is a web-first communication platform for private communities. It combines passwordless identity, invitation-based membership, durable text rooms, realtime delivery, unread state, and optimistic message workflows in a deliberately small deployment footprint.

The project is under active development. The current build supports the complete first-community activation path and the core reliable-room workflow; direct messages, delegated moderation, calls, and attachments remain future work.

## What works today

- Passwordless email verification with expiring six-digit codes, resend limits, attempt limits, and revocable rolling sessions.
- Unique, case-insensitive global handles.
- Atomic Community creation with Owner and Everyone roles plus a default `general` Room.
- Revocable, expiring, multi-use invitation links with optional use limits.
- Membership-aware Community and Room navigation with deep links.
- Owner-authorized text Room creation, quotas, and audit records.
- Durable plain-text Messages with idempotent posting and stable cursor pagination.
- Author-only Message edits and deletion using revision checks and durable tombstones.
- Immutable Message revision records and transactional outbox events.
- Authorized WebSocket delivery, bounded connection queues, reconnect recovery, and event deduplication.
- Durable per-Member read cursors, unread counts, and unread dividers.
- Optimistic send, retry, edit, and delete states in the web client.
- Structured request logging, process-local service metrics, health endpoints, smoke checks, and backup verification tooling.

## Architecture

Skyroom is a modular monolith with a statically rendered web client:

```text
Browser
  ├── Next.js 16 / React 19 web client
  └── HTTP + WebSocket
          │
          ▼
Go API
  ├── identity       email codes, sessions, Handles
  ├── community      Communities, Rooms, invitations
  ├── permission     capability checks
  ├── conversation   Messages, revisions, cursors, realtime events
  └── platform       HTTP, PostgreSQL, migrations, observability
          │
          ├── PostgreSQL 17   durable state and transactional outbox
          └── SMTP            verification-code delivery
```

The API owns authorization and durable state. The browser uses optimistic state only for responsiveness and reconciles every mutation against the server-assigned Message ID and revision. Message rows retain their original ordering identity when edited or deleted, while an append-only revision table preserves mutation history.

The development server can use in-memory stores when `DATABASE_URL` is absent. PostgreSQL is required outside development and is the authoritative production path.

## Technology

| Area | Stack |
| --- | --- |
| API | Go 1.25, `net/http`, pgx |
| Web | Next.js 16, React 19, strict TypeScript |
| Database | PostgreSQL 17 with embedded SQL migrations |
| Realtime | Authorized WebSockets via `coder/websocket` |
| Local email | Mailpit |
| Edge/deployment | Caddy, Docker Compose, Vercel |
| Contracts | OpenAPI 3.1 and JSON Schema event envelopes |

## Repository layout

```text
apps/web/                   Next.js application and React components
cmd/api/                    API process entrypoint
contracts/                  OpenAPI and realtime event schemas
database/migrations/        Ordered, embedded PostgreSQL migrations
internal/community/         Community, Room, invitation, and membership logic
internal/conversation/      Messages, revisions, history, read state, and broker
internal/identity/          Verification codes, sessions, and Handles
internal/permission/        Room capability authorization
internal/platform/          HTTP server, IDs, PostgreSQL runner, and test helpers
platform/containers/        Production container definitions
platform/deployment/        Compose, Caddy, deploy, smoke, and backup tooling
```

Private product notes and design working files live locally under `docs/` and `design-system/`; both directories are intentionally excluded from Git.

## Requirements

- Go 1.25 or newer
- Node.js 22 or newer
- npm 10 or newer
- Docker with Compose for local PostgreSQL and Mailpit

## Quick start

1. Install JavaScript dependencies:

   ```bash
   npm install
   ```

2. Start PostgreSQL and Mailpit:

   ```bash
   docker compose -f platform/deployment/compose.dev.yaml up -d
   ```

   The default host ports are PostgreSQL `5434`, SMTP `1026`, and the Mailpit UI `8026`.

3. Copy the example environment file and replace the development auth secret:

   ```bash
   cp .env.example .env
   ```

4. Export the API variables and start the server:

   ```bash
   set -a
   source .env
   set +a
   go run ./cmd/api
   ```

5. In a second terminal, start the web client:

   ```bash
   npm run dev:web
   ```

Open `http://localhost:3000`. Verification emails appear in Mailpit at `http://localhost:8026`.

The web client calls `NEXT_PUBLIC_API_BASE_URL` in local development. Production leaves it empty because Caddy serves `/api` from the same origin.

## Configuration

| Variable | Required | Purpose |
| --- | --- | --- |
| `APP_ENV` | No | Runtime mode; defaults to `development`. |
| `HTTP_ADDR` | No | API listen address; defaults to `:8080`. |
| `WEB_ORIGIN` | Yes in production | Allowed browser origin and WebSocket origin. |
| `AUTH_HASH_SECRET` | Yes outside development | Secret used to protect authentication material; must be at least 32 bytes outside development. |
| `DATABASE_URL` | Yes outside development | PostgreSQL connection string. Embedded migrations run on API startup. |
| `SMTP_ADDR` | No | SMTP host and port; defaults to local Mailpit at `localhost:1026`. |
| `EMAIL_FROM` | No | Sender address for verification messages. |
| `NEXT_PUBLIC_API_BASE_URL` | Web development only | Browser-visible API base URL, normally `http://localhost:8080`. |

Provider variables for Resend, LiveKit, and Cloudflare R2 are documented in `.env.example`; their production adapters belong to later milestones.

## API and realtime contracts

[`contracts/openapi.yaml`](contracts/openapi.yaml) is the HTTP source contract. Current endpoint groups cover:

- authentication codes, session inspection, and sign-out;
- Handle claiming;
- Community listing and creation;
- accessible Room listing and authorized Room creation;
- invitation creation, preview, revocation, and idempotent joining;
- Message history, posting, editing, deletion, and read state;
- Room WebSocket events;
- health, liveness, and aggregate process metrics.

Realtime envelopes are defined under [`contracts/events/`](contracts/events/). A Message create, update, or delete is committed with its outbox event in the same PostgreSQL transaction. WebSocket delivery is intentionally ephemeral: reconnecting clients recover created-Message gaps through cursor history and reconcile current revisions from Message history.

## Data and consistency model

- IDs are UUIDs generated by the application.
- Message order is `(created_at, id)` and does not change after edits.
- Posting is idempotent per author and client-generated key.
- Edits and deletes require the caller's expected revision to prevent lost updates.
- Deletes retain a tombstone so cursors and future references remain valid.
- Every committed Message mutation creates an outbox event atomically.
- Community and Room access is checked server-side for HTTP and WebSocket operations.
- Read cursors only move forward.

## Verification

Run the standard checks from the repository root:

```bash
go test -race ./cmd/... ./internal/...
go vet ./cmd/... ./internal/...
npm run typecheck
npm run lint
npm run build:web
```

PostgreSQL integration tests use isolated temporary schemas and run only when `SKYROOM_TEST_DATABASE_URL` points to a disposable database:

```bash
SKYROOM_TEST_DATABASE_URL='postgres://skyroom:skyroom-local@localhost:5434/skyroom?sslmode=disable' \
  go test -race ./cmd/... ./internal/...
```

CI runs the Go race suite, static analysis, frontend checks, and the production web build.

## Deployment

The production-shaped stack is defined in `platform/deployment/compose.yaml`:

```bash
docker compose -f platform/deployment/compose.yaml up --build
```

Copy `platform/deployment/production.env.example` to a deployment-local environment file and replace every placeholder. Never commit deployment credentials.

The staging workflow in `.github/workflows/deploy-staging.yml` runs CI before deployment. The API deploy uses an SSH forced-command wrapper that accepts only a commit from the staging branch; the web application is built separately by Vercel. Supporting scripts provide public smoke checks and logical PostgreSQL backup verification.

## Security notes

- Session cookies are secure outside development and API responses disable caching.
- Browser origins are checked for state-changing HTTP calls and WebSocket upgrades.
- Verification codes expire, are attempt-limited, and are never returned or logged in production.
- Authentication secrets and database credentials belong in environment variables or the deployment platform's secret store.
- `.env`, `.env.*`, local Vercel state, logs, build artifacts, and private product/design directories are ignored.

If you discover a vulnerability, avoid filing a public issue containing exploit details or sensitive data. Contact the repository owner privately first.

## Contributing

Keep changes scoped and include regression coverage for new behavior. Go tests belong beside their implementation; TypeScript must remain strict and ESLint must pass with zero warnings. Use short imperative Conventional Commit subjects such as `feat:`, `fix:`, `docs:`, and `chore:`.

Before opening a pull request, run the verification commands above and call out changes to migrations, environment variables, deployment behavior, or `contracts/openapi.yaml`.

## License

No software license has been added yet. Public source availability alone does not grant permission to copy, modify, or redistribute the project.
