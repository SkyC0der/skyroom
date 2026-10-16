# Skyroom

Skyroom is a low-friction, web-first communications platform for persistent communities. Members verify an email, choose a unique global Handle, join Communities and Rooms, exchange durable Messages and DMs, and participate in live audio or video.

The initial deployment is intentionally small: a static Next.js client and one Go application behind Caddy. Managed PostgreSQL, LiveKit, Resend, and Cloudflare R2 keep durable data and media outside the VPS. The module boundaries in `docs/SYSTEM_DESIGN.md` preserve a path to split realtime and worker processes when measured load requires it.

## Repository layout

```text
apps/web/                 Next.js and TypeScript web client
cmd/api/                  Go application entrypoint
internal/community/       Community and invitation domain logic
internal/permission/      Capability authorization
internal/platform/        HTTP and runtime infrastructure
contracts/                HTTP and realtime source contracts
platform/containers/      Production container definitions
platform/deployment/      Caddy and Compose deployment
docs/                     Product and system-design documents
```

## Requirements

- Go 1.25+
- Node.js 22+
- npm 10+
- Docker with Compose for the production-shaped local stack

## Local development

Start local PostgreSQL and Mailpit:

```bash
docker compose -f platform/deployment/compose.dev.yaml up -d
```

The stack uses host ports 5434 (PostgreSQL), 1026 (SMTP), and 8026 (Mailpit) to avoid common local conflicts. Override them with `SKYROOM_POSTGRES_PORT`, `SKYROOM_SMTP_PORT`, or `SKYROOM_MAILPIT_PORT`. Mailpit's inbox is available at `http://localhost:8026`. Then run the API:

```bash
DATABASE_URL='postgres://skyroom:skyroom-local@localhost:5434/skyroom?sslmode=disable' go run ./cmd/api
```

In another terminal, run the web client:

```bash
npm install
npm run dev:web
```

Open `http://localhost:3000`. The development client reads `NEXT_PUBLIC_API_BASE_URL`; production uses same-origin `/api` through Caddy.

## Verification

```bash
go test ./cmd/... ./internal/...
go vet ./cmd/... ./internal/...
npm run typecheck
npm run lint
npm run build:web
```

The HTTP endpoints are defined in `contracts/openapi.yaml`. They cover identity, Community creation, invitation previews, revocation, and idempotent joins. PostgreSQL is used whenever `DATABASE_URL` is set; development falls back to memory only when it is absent. The API applies embedded migrations at startup.

## Deployment

The production-shaped stack builds with:

```bash
docker compose -f platform/deployment/compose.yaml up --build
```

The web client will use `rooms.studioeco.co`; the VPS API will use `api.rooms.studioeco.co`. Copy `platform/deployment/production.env.example` to `platform/deployment/.env` on the VPS and replace every placeholder before starting Compose. Secrets belong in VPS environment configuration and must never be committed.

### Staging pipeline

`staging` is currently the only hosted environment. Every push to `staging` runs CI (`.github/workflows/deploy-staging.yml` calls `ci.yml`) and, if it passes, deploys:

- **API** — GitHub Actions connects over SSH with a CI-only key that is pinned in `authorized_keys` to the forced command `/home/skyroom/bin/skyroom-deploy` (a copy of `platform/deployment/vps-deploy-wrapper.sh`). It accepts only `deploy <sha>` for commits on `origin/staging`, checks that commit out, and runs `platform/deployment/deploy.sh`, which rebuilds and health-checks the stack.
- **Web** — Vercel builds `apps/web` from the `staging` branch and serves it on `rooms.studioeco.co`.

Repository secrets: `VPS_HOST`, `VPS_SSH_KEY` (CI private key), `VPS_KNOWN_HOSTS` (pinned host key). If the wrapper changes, reinstall it on the VPS:

```bash
install -m 755 platform/deployment/vps-deploy-wrapper.sh /home/skyroom/bin/skyroom-deploy
```
