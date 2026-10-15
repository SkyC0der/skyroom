# Skyroom

Skyroom is a low-friction, web-first communications platform for persistent communities. Members verify an email, choose a unique global Handle, join Communities and Rooms, exchange durable Messages and DMs, and participate in live audio or video.

The initial deployment is intentionally small: a static Next.js client and one Go application behind Caddy. Managed PostgreSQL, LiveKit, Resend, and Cloudflare R2 keep durable data and media outside the VPS. The module boundaries in `docs/SYSTEM_DESIGN.md` preserve a path to split realtime and worker processes when measured load requires it.

## Repository layout

```text
apps/web/                 Next.js and TypeScript web client
cmd/api/                  Go application entrypoint
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

Mailpit's local inbox is available at `http://localhost:8025`. Then run the API:

```bash
go run ./cmd/api
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

The first functional endpoints are `POST /api/v1/auth/codes` and `POST /api/v1/auth/verify`, defined in `contracts/openapi.yaml`. The current authentication store is in memory while the PostgreSQL adapter is connected; the migration establishing its durable schema is in `database/migrations/000001_identity.up.sql`.

## Deployment

The production-shaped stack builds with:

```bash
docker compose -f platform/deployment/compose.yaml up --build
```

The public deployment will use `rooms.studioeco.co`. Secrets belong in VPS environment configuration and must never be committed.
