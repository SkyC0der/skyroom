# Skyroom

Skyroom is a low-friction, web-first communications platform for conferences and professional communities. Members verify a work email, choose a community-specific username, join rooms, exchange durable messages, and participate in live audio or video.

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

Run the API:

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

The first endpoint is `GET /api/v1/health`, defined in `contracts/openapi.yaml`.

## Deployment

The production-shaped stack builds with:

```bash
docker compose -f platform/deployment/compose.yaml up --build
```

The public deployment will use `rooms.studioeco.co`. Secrets belong in VPS environment configuration and must never be committed.
