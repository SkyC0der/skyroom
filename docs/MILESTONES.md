# Skyroom MVP milestones and ticket plan

**Status:** Approved product direction; implementation started 2026-10-15  
**Initial beta:** Invite-only professional and cohort-based learning Communities with 20–200 Members  
**Activation:** Within 24 hours, a creator invites someone, the invitee joins, two Members exchange Messages in `general`, and both successfully return after reconnecting or starting a new session.

Tickets are vertical slices. Each ticket includes persistence, Go behavior, HTTP/realtime contracts, TypeScript UI, authorization, and automated tests where applicable.

## Milestone 1 — First Community, first Message

Goal: validate the complete activation path on one Go process and PostgreSQL.

### M1.1 — Verify an email and establish a session

**Mode:** AFK  
**Blocked by:** Nothing  
**Status:** Complete

- Request a six-digit code for any valid email.
- Expire codes after 10 minutes; enforce resend and attempt limits.
- Verify once, create or retrieve the User, and issue a revocable 30-day rolling session.
- Require fresh verification for future sensitive actions.
- Use Mailpit locally and a replaceable provider interface for Resend.
- Never return or log codes in production.

### M1.2 — Claim a unique global Handle

**Mode:** AFK  
**Blocked by:** M1.1
**Status:** Complete

- Require a Handle before Community participation.
- Normalize and enforce case-insensitive uniqueness.
- Keep the immutable User ID separate from the changeable Handle.
- Show useful conflict and validation errors.

### M1.3 — Create a Community atomically

**Mode:** AFK  
**Blocked by:** M1.2
**Status:** Complete

- Create the Community, Owner membership, non-removable Owner Role, implicit Everyone Role, and members-only `general` Room in one transaction.
- Limit beta accounts to three owned Communities.
- Enforce a real `CREATE_ROOM` capability even though only owners receive it initially.

### M1.4 — Invite and join one Member

**Mode:** AFK  
**Blocked by:** M1.3
**Status:** Complete

- Create a revocable, multi-use invitation that expires after seven days.
- Support an optional maximum-use count.
- Show Community name, description, member count, and inviter before joining.
- Join idempotently and assign Everyone membership atomically.

### M1.5 — Send the first durable Room Message

**Mode:** AFK  
**Blocked by:** M1.4
**Status:** Complete

- Post plain-text Messages through HTTP after current Room authorization.
- Commit the Message and outbox event together.
- Make retries idempotent with a client-generated key.
- Order and paginate history by a stable Conversation cursor.
- Show pending, acknowledged, failed, and retry states.

### M1.6 — Deliver, reconnect, and recover

**Mode:** AFK  
**Blocked by:** M1.5

**Status:** Complete

- Authorize WebSocket subscriptions.
- Fan out committed Message events through bounded connection queues.
- Reconnect with jittered backoff and recover cursor gaps through HTTP.
- Deduplicate optimistic Messages and repeated events by ID/idempotency key.
- Persist one last-read cursor per User and Conversation; show unread counts/dividers.

### M1.7 — Prove activation and deploy the slice

**Mode:** HITL for production secrets and final domain checks  
**Blocked by:** M1.6

**Status:** In progress — the PostgreSQL activation journey, structured request logs, process metrics, public smoke script, and logical-backup verification are implemented; staging deployment and final domain checks require production access.

- Exercise the activation journey in an end-to-end test.
- Add structured logs, basic latency/error metrics, backup verification, and a smoke test.
- Deploy web to `rooms.studioeco.co`, API/WebSocket to `api.rooms.studioeco.co`, and PostgreSQL to Neon.
- Verify session cookies, CORS/origin checks, TLS, reconnect behavior, and a new browser session.

## Milestone 2 — Reliable Community messaging

Goal: make Room conversations useful beyond the first Message.

1. **M2.1 — Switch Communities and accessible Rooms** — membership-aware navigation and deep links. Blocked by M1.7.
2. **M2.2 — Create and configure a text Room** — owner flow backed by `CREATE_ROOM`, quotas, and audit event. Blocked by M2.1.
3. **M2.3 — Edit and delete Messages** — revision history, authorization, optimistic reconciliation, and tombstones. Blocked by M1.6.
4. **M2.4 — React and reply in Threads** — one-level root Threads and idempotent reactions. Blocked by M2.3.
5. **M2.5 — Mention Members and notify them** — parsing, authorization-aware delivery, and preferences. Blocked by M2.1.
6. **M2.6 — Upload safe attachments** — signed upload, ownership, scan/quarantine state, and protected download. Blocked by M1.5.

## Milestone 3 — Direct Messages and privacy

Goal: provide safe one-to-one communication outside Community scope.

1. **M3.1 — Start or retrieve a one-to-one DM** — canonical participant pair and eligibility policy. Blocked by M1.6.
2. **M3.2 — Exchange and resume DM Messages** — shared Conversation/Message machinery, private authorization, and unread cursors. Blocked by M3.1.
3. **M3.3 — Handle Message requests** — Accept, Delete, and Block/Report without leaking read state. Blocked by M3.2.
4. **M3.4 — Block another User** — platform-wide block enforcement and existing-conversation behavior. Blocked by M3.3.
5. **M3.5 — Configure DM privacy** — recipient controls and abuse-resistant defaults. Blocked by M3.4.

## Milestone 4 — Delegated permissions

Goal: let owners safely distribute Community management.

1. **M4.1 — Create and order custom Roles** — hierarchy invariants and assignment limits. Blocked by M2.2.
2. **M4.2 — Edit capability grants** — impact warning, versioned permission cache invalidation, and audit records. Blocked by M4.1.
3. **M4.3 — Apply Room Overrides** — explicit Role/Member allow and deny with explainable effective access. Blocked by M4.2.
4. **M4.4 — Revoke active access** — remove HTTP/WebSocket subscriptions immediately after permission changes. Blocked by M4.3.

## Milestone 5 — Voice and video

Goal: move a permitted Room conversation into a managed LiveKit Call.

1. **M5.1 — Create and join a voice/video Room** — permission check, capacity check, short-lived RTC token, and join preview. Blocked by M4.4.
2. **M5.2 — Control devices and participant state** — microphone, camera, device selection, and distinct error states. Blocked by M5.1.
3. **M5.3 — Share a screen** — explicit publish control and browser compatibility guidance. Blocked by M5.2.
4. **M5.4 — Moderate an active Call** — disconnect/mute capability, ban propagation, and active RTC revocation. Blocked by M5.2.
5. **M5.5 — Meter media usage** — participant/session quotas, cost dashboards, and provider outage degradation. Blocked by M5.1.

## Milestone 6 — Governance and focused beta

Goal: operate the product safely with real Communities.

1. **M6.1 — Report content or a User** — evidence snapshot, reporter safety, and operator queue. Blocked by M2.3 and M3.4.
2. **M6.2 — Moderate a Member** — timeout, remove, ban, hierarchy checks, and audit history. Blocked by M4.2.
3. **M6.3 — Apply abuse controls** — layered rate limits for OTP, invitations, messaging, DMs, and RTC tokens. Blocked by all relevant flows.
4. **M6.4 — Observe service health** — service-level indicators, alerting, tracing, and privacy-safe operational dashboards. Blocked by M5.5.
5. **M6.5 — Run load and failure tests** — reconnect storms, hot Rooms, slow consumers, database interruption, and RTC outage. Blocked by M6.4.
6. **M6.6 — Pass the beta release gate** — accessibility, security, backup restore, incident runbooks, privacy copy, and activation analytics. Blocked by M6.1–M6.5.

## Approved first-slice decisions

- Any verified email may register; future Community policy may restrict invitation domains.
- Users have immutable IDs, unique global Handles, and optional Community display names.
- Beta Communities are invitation-only; each account may own up to three.
- Authentication uses six-digit codes valid for 10 minutes and 30-day rolling sessions.
- First Messages are plain text with durable ordering, idempotency, cursor pagination, realtime delivery, reconnect recovery, and minimal unread state.
- New Communities receive Owner, Everyone, and `general`; only owners initially receive `CREATE_ROOM`.
- Local services are PostgreSQL and Mailpit; hosted services are Neon and Resend.
- The first beta runs without Redis or NATS: one modular Go process plus PostgreSQL.
