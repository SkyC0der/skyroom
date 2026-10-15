# Skyroom handoff for Claude

**Prepared:** 2026-10-15  
**Repository:** `https://github.com/SkyC0der/skyroom` (private)  
**Local path:** `/Users/skycoder/Documents/sellercrowd/skyroom`  
**Branch:** `main`  
**Code baseline before this handoff document:** `a60a885e070a040aded3e179aaa838023d38ac4f`  
**Project owner:** Emmanuel Ezenwigbo

## 1. Critical operating instructions

- Never add Claude, Anthropic, an assistant, or a bot as author, committer, or co-author. Preserve the configured identity: `Emmanuel Ezenwigbo <emmanuelezenwigbo@gmail.com>`.
- Never request, print, commit, or paste VPS passwords, private SSH keys, API keys, session tokens, database URLs, or generated secrets.
- Existing changes belong to the user. Inspect before modifying or staging them.
- Use `apply_patch` for repository file edits. Run the proportional test suite before committing.
- Do not expose PostgreSQL, Redis, or internal service ports publicly. Docker-published ports can bypass UFW.
- Do not represent the authentication implementation as production-ready until the PostgreSQL and email-provider adapters are connected.

## 2. Product definition

Skyroom is a browser-first, general-purpose communication platform inspired by Discord. It supports persistent Communities, Rooms, text Messages, one-to-one DMs, configurable Roles and Room Overrides, and managed voice/video Calls.

The initial beta is deliberately narrower: professional and cohort-based learning Communities with approximately 20–200 Members. The expected examples are bootcamps, workshops, student leadership groups, professional clubs, and similar Communities that value low-friction browser access.

### Confirmed product decisions

- Web-first responsive PWA.
- TypeScript/Next.js client and Go backend.
- Any verified email may register; work email is not mandatory.
- Immutable internal User ID, unique changeable global Handle, and optional per-Community display name.
- Passwordless six-digit code valid for 10 minutes.
- Verification attempts and resends are limited.
- Rolling 30-day session; sensitive actions eventually require fresh verification.
- Initial Communities are invitation-only.
- A beta account may own at most three Communities.
- Default invitation is multi-use, expires after seven days, is revocable, and may have a maximum-use count.
- New Community creation atomically creates the owner membership, non-removable Owner Role, implicit Everyone Role, and members-only `general` text Room.
- Only owners initially receive `CREATE_ROOM`, but authorization must use the real capability system.
- First Messages are plain text with durable storage, stable ordering, cursor pagination, optimistic state, idempotent retries, WebSocket delivery, reconnect recovery, and minimal unread cursors.
- Message edits, deletion, reactions, Threads, mentions, and attachments follow after the reliable first-Message path.
- One-to-one DMs ship before the beta release; group DMs remain deferred.
- LiveKit or another managed SFU handles WebRTC media. The Go application authorizes Calls but never relays media packets.
- Local infrastructure: Docker PostgreSQL and Mailpit.
- Hosted infrastructure: Neon, Resend, LiveKit Cloud, Cloudflare R2, Vercel, and the VPS API.
- Initial beta architecture: one modular Go process plus PostgreSQL. No Redis or NATS until multiple application instances require shared state/event distribution.

### Activation definition

A Community is activated when, within 24 hours:

1. The creator invites at least one person.
2. At least one invitee joins.
3. Two distinct Members exchange Messages in `general`.
4. Both Members return successfully following a reconnect or new session.

## 3. Canonical project documents

- `docs/PRD.md` — supplied product blueprint. Currently untracked and contains malformed pasted Mermaid/CSS around its domain diagram; clean and review it before committing.
- `docs/MILESTONES.md` — approved six-milestone vertical-slice ticket plan.
- `docs/SYSTEM_DESIGN.md` — substantial system design, but portions still describe the earlier conference-first positioning. Reconcile it with the general-purpose PRD and platform-level DM Conversation model.
- `CONTEXT.md` — ubiquitous language. It also needs reconciliation: the current version still defines Username as Community-specific, while the approved model is global Handle plus optional Community display name.
- `design-system/skyroom/MASTER.md` — generated UI foundation. Currently untracked. Inspect before committing.

Do not create GitHub issues from `docs/MILESTONES.md` without the user's explicit approval of the final ticket granularity. The milestone structure is approved as a plan; publishing issues was intentionally deferred.

## 4. Repository and architecture

```text
apps/web/                  Next.js 16 / React 19 / TypeScript client
cmd/api/                   Go API entrypoint
internal/identity/         Authentication domain service and adapters
internal/platform/         HTTP server and middleware
contracts/openapi.yaml     HTTP contract
contracts/events/          Realtime event schemas
database/migrations/       PostgreSQL migrations
platform/containers/       Container images
platform/deployment/       Caddy and Compose deployment
docs/                      Product, milestone, and architecture documents
```

The target design is a modular monolith. HTTP handles durable commands and history. WebSockets carry committed changes. PostgreSQL is the source of truth. Messages and their outbox events commit together. Clients deduplicate at-least-once delivery and reconcile cursor gaps after reconnecting.

Conversation is the shared durable abstraction:

- A Room Conversation belongs to exactly one Room.
- A DM Conversation has exactly two distinct User participants and belongs to no Community.
- Every Message belongs to exactly one Conversation.
- Room authorization is Community/Role based.
- DM authorization is participant/block/privacy based.

## 5. Implemented functionality

### Initial scaffold — commit `e7da0f7`

- Next.js static client.
- Go HTTP process with health endpoints.
- CORS, request logging, panic recovery, graceful shutdown.
- OpenAPI and realtime event envelope scaffolds.
- Container and Caddy deployment scaffolds.

### Passwordless authentication foundation — commit `3715fe7`

- `POST /api/v1/auth/codes`.
- `POST /api/v1/auth/verify`.
- Email normalization and validation.
- Cryptographically generated six-digit codes.
- HMAC-hashed codes; raw codes are not stored.
- Ten-minute expiry, five-attempt limit, 60-second resend limit, and single consumption.
- Cryptographically random session token; only its SHA-256 hash is stored.
- HTTP-only, SameSite=Lax, 30-day cookie.
- New User response reports `needs_handle: true`.
- SMTP sender for local Mailpit.
- In-memory Store for tests and early local execution.
- PostgreSQL identity migration for Users, challenges, and sessions.
- Functional `/sign-in` two-step Next.js interface.
- Unit tests and HTTP flow tests.

### API-domain deployment correction — commit `a60a885`

- `rooms.studioeco.co` remains the web/Vercel origin.
- `api.rooms.studioeco.co` is the VPS/Caddy API endpoint.
- Caddy is API-only and reverse-proxies to the Go service.
- Production Compose requires `AUTH_HASH_SECRET`.
- Production environment template is `platform/deployment/production.env.example`.

## 6. Known authentication gaps

Ticket M1.1 is still **in progress**, not complete:

- The running application uses `MemoryStore`; sessions and challenges disappear after a restart.
- The PostgreSQL Store adapter has not been implemented.
- No migration runner is wired into deployment.
- A pgx dependency could not be downloaded in the previous sandbox because outbound package access was blocked. The intended driver is pgx v5.
- Resend is not implemented. Production currently has only the local SMTP adapter.
- There is no authenticated-session lookup middleware yet.
- There is no logout/session revocation endpoint yet.
- Rolling session renewal and fresh-auth enforcement are not implemented.
- The Handle-claiming endpoint/UI is M1.2 and has not begun.
- CSRF/origin enforcement for authenticated state-changing endpoints must be added before those endpoints exist.

The API health endpoint may be deployed now, but do not invite users or claim production authentication works.

## 7. Verification status

The following passed after commit `3715fe7`:

```bash
GOCACHE=/tmp/skyroom-go-cache go test ./cmd/... ./internal/...
GOCACHE=/tmp/skyroom-go-cache go vet ./cmd/... ./internal/...
npm run typecheck
npm run lint
npm run build:web
git diff --check
```

The Compose file also passed `docker compose config` with required environment variables supplied.

## 8. Git state at handoff

Before this handoff document is committed, `main` and `origin/main` both point to `a60a885`. Confirm the current head with `git log -1 --oneline` rather than assuming this remains the tip.

Untracked local paths intentionally not included in the last commit:

```text
design-system/
docs/PRD.md
```

Inspect and clean these before adding them. Do not discard them.

## 9. VPS state

### Host

- Provider hostname: `vps158464-bg5`
- Public IPv4: `23.94.174.220`
- OS: Debian GNU/Linux 12 (bookworm)
- Kernel after upgrade: `6.1.0-53-amd64`
- Resources: approximately 2 GB RAM, 2 GB swap, 30 GB storage
- Deployment user: `skyroom`

### Security baseline completed

- Root SSH login disabled.
- Password SSH login disabled.
- Keyboard-interactive SSH disabled.
- Public-key SSH enabled.
- `skyroom` has sudo access.
- UFW active at boot.
- Default incoming policy deny; default outgoing allow.
- Allowed inbound ports: `22/tcp`, `80/tcp`, `443/tcp`, and `443/udp`, including IPv6 rules.
- No secrets or private keys are recorded in this document.

### Installed software

- Docker Engine Community `29.9.1`.
- Docker Compose `v5.6.0`.
- Git `2.39.5`.
- Docker `hello-world` test succeeded.

### GitHub deployment access

- A repository-scoped read-only deploy key titled `Skyroom production VPS` is installed on the VPS.
- Direct authentication succeeded with `Hi SkyC0der/skyroom!`.
- Do not copy the private deploy key off the VPS or grant it write access.

### Clone status

The first clone attempt failed because `~` inside `GIT_SSH_COMMAND` was not expanded. A corrected clone command was supplied, but successful clone output has not yet been confirmed.

Use:

```bash
mkdir -p /home/skyroom/apps
git -c core.sshCommand="ssh -i /home/skyroom/.ssh/skyroom_github -o IdentitiesOnly=yes" \
  clone git@github.com:SkyC0der/skyroom.git /home/skyroom/apps/skyroom

cd /home/skyroom/apps/skyroom
git config core.sshCommand \
  "ssh -i /home/skyroom/.ssh/skyroom_github -o IdentitiesOnly=yes"
git log --oneline -3
```

Expected leading commits:

```text
a60a885 ops: separate api deployment domain
3715fe7 feat: start passwordless authentication slice
e7da0f7 chore: scaffold skyroom application
```

## 10. DNS and hosting state

- Domain: `studioeco.co`, managed in Cloudflare.
- Existing apex and other subdomain records belong to other projects and must not be changed.
- Intended web hostname: `rooms.studioeco.co` on Vercel.
- Intended API hostname: `api.rooms.studioeco.co` on the VPS.
- Required Cloudflare record: `A`, name `api.rooms`, value `23.94.174.220`, initially **DNS only**.
- Whether that record has been created is not yet confirmed.
- Vercel project/domain attachment is not yet confirmed.
- Neon, Resend, LiveKit, and R2 production resources are not yet connected.

## 11. Immediate next actions

### A. Confirm repository clone

Run the corrected clone commands in section 9 and verify the expected commits.

### B. Confirm DNS before starting Caddy

In Cloudflare, add or verify:

```text
Type: A
Name: api.rooms
Value: 23.94.174.220
Proxy: DNS only
```

Do not alter `studioeco.co`, `www`, `magic`, `ecobot`, MX, SPF, DKIM, DMARC, or Vercel verification records.

### C. Create production environment without displaying its secret

On the VPS:

```bash
cd /home/skyroom/apps/skyroom/platform/deployment
cp production.env.example .env
sed -i "s/generate-with-openssl-rand-hex-32/$(openssl rand -hex 32)/" .env
chmod 600 .env
```

Do not print `.env`.

### D. Deploy only the health scaffold

After DNS resolves to the VPS:

```bash
cd /home/skyroom/apps/skyroom
docker compose -f platform/deployment/compose.yaml up -d --build
docker compose -f platform/deployment/compose.yaml ps
docker compose -f platform/deployment/compose.yaml logs --tail=100 api caddy
```

Smoke test from the VPS:

```bash
curl -fsS http://127.0.0.1/api/v1/health \
  -H 'Host: api.rooms.studioeco.co'
```

Then from a separate machine:

```bash
curl -fsS https://api.rooms.studioeco.co/api/v1/health
```

Expected JSON contains `"status":"ok"` and an RFC3339 UTC timestamp.

### E. Complete M1.1 before production auth testing

1. Add pgx v5 and implement a PostgreSQL `identity.Store`.
2. Make challenge verification, attempt increment, consumption, User creation, and session creation transactionally correct under concurrency.
3. Add a migration command or controlled startup migration path.
4. Implement the Resend provider through the existing `CodeSender` interface.
5. Add authenticated session lookup, revocation, rolling renewal, and logout.
6. Add integration tests against a real disposable PostgreSQL instance.
7. Replace `MemoryStore` in production composition; keep it only for tests.
8. Perform the complete browser/Mailpit flow locally, then the Resend flow in a non-production hosted environment.

### F. Begin M1.2

After M1.1 is durable, implement unique global Handle claiming. Normalize case, reserve unsafe/system names, enforce uniqueness in PostgreSQL, and provide conflict-safe API/UI behavior.

## 12. Deployment update procedure

Once the repository is cloned and the first deployment exists:

```bash
cd /home/skyroom/apps/skyroom
git pull --ff-only
docker compose -f platform/deployment/compose.yaml up -d --build
docker compose -f platform/deployment/compose.yaml ps
docker compose -f platform/deployment/compose.yaml logs --tail=100 api caddy
```

Before each deployment:

- Read the diff and migration implications.
- Back up durable data once PostgreSQL is connected.
- Never run destructive Git commands or delete volumes to fix an application error.
- Confirm that only intended public ports are published.
- Keep Caddy certificates/data in their named volumes.

## 13. DSA and reliability intent

The user is studying DSA for Google/Meta SWE interviews and wants the project to exercise those ideas appropriately:

- Conversation Messages form an append-only ordered log.
- Cursor pagination uses `(conversation_id, message_id)` rather than large offsets.
- Hash maps/sets deduplicate optimistic Messages, retries, and repeated events.
- Idempotency keys prevent duplicate durable writes.
- The transactional outbox is a reliable queue between commits and fan-out.
- WebSocket clients receive bounded outgoing queues to prevent unbounded memory use.
- Reconnect uses a merge/reconciliation algorithm across local optimistic state, missed history, and repeated events.
- Permission grants may use bitsets, with explicit precedence rules for Room Overrides.
- Avoid premature sharding. Establish correct invariants and measured bottlenecks first.

## 14. Definition of the next successful handoff

The next handoff should report:

- Whether the repository cloned successfully on the VPS.
- Whether `api.rooms.studioeco.co` resolves and has a valid certificate.
- The health endpoint response from outside the VPS.
- PostgreSQL adapter and migration-runner status.
- Resend adapter and verified sending-domain status.
- Test commands and results.
- Exact commit hashes, with the user's identity as author/committer.
- Remaining blockers without claiming unfinished functionality is complete.
