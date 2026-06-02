# ADR-0002: Upstream API System & Transactional Outbox for Reliable Event Publishing
---

## Context

ADR-0001 describes how the notifier **consumes** subscriber lifecycle events from Kafka. This document describes the **upstream system** that produces those events.

The upstream system exposes three interfaces:
1. **Internal API** (`/internal/v1/*`) — serves the web UI for subscriber CRUD and API Key management; consumed only by first-party frontend engineers.
2. **Public API** (`/v1/*`) — serves partner and private integrations; supports two auth methods: OAuth2 (partner integrations) and API Key (private integrations).
3. **Webhook output** — every committed subscriber mutation publishes an event to Kafka, which the notifier (ADR-0001) delivers to partner webhooks.

**Core dual-write problem:** Subscriber data lives in PostgreSQL. Events must reach Kafka. These are two separate systems. Any crash between the two writes causes an inconsistency:
- DB written, Kafka publish fails → event silently lost; notifier never fires.
- Kafka written before DB commit, then DB rolls back → phantom event for a subscriber that doesn't exist.

The solution is the **Transactional Outbox Pattern**: the event is written to a PostgreSQL `outbox` table in the same transaction as the business write, then a separate publisher process forwards it to Kafka. The two systems are never written to in the same logical operation.

---

## High-Level Architecture

```
  ┌──────────────────────────────────────────────────────────────────────────┐
  │                      UPSTREAM SYSTEM OVERVIEW                            │
  └──────────────────────────────────────────────────────────────────────────┘

  ┌────────────┐   ┌─────────────────┐   ┌──────────────────────────┐
  │ Web Browser│   │ Partner (OAuth2)│   │ Private Integration      │
  │ (webapp)   │   │                 │   │ (API Key)                │
  └─────┬──────┘   └────────┬────────┘   └────────────┬─────────────┘
        │                   │                         │
        └───────────────────┴─────────────────────────┘
                            │  HTTPS
                            ▼
             ┌──────────────────────────────┐
             │  API Gateway / Load Balancer │
             │  TLS termination             │
             │  Rate limiting               │
             └──────────────┬───────────────┘
                            │
              ┌─────────────┴─────────────┐
              │                           │
        /internal/v1/*                  /v1/*
              │                           │
              ▼                           ▼
     ┌─────────────────┐      ┌───────────────────────┐
     │  Session / JWT  │      │  Auth Middleware      │
     │  middleware     │      │  ├─ OAuth2(Bearer)    │
     │  (webapp only)  │      │  └─ API Key(x-api-key)│
     └────────┬────────┘      └────────────┬──────────┘
              │                            │
              └──────────────┬─────────────┘
                             │  tenant_id + scopes injected into ctx
                             ▼
                 ┌───────────────────────────┐
                 │  Subscriber Service       │
                 │  CRUD handlers            │
                 │  API Key mgmt handlers    │
                 └──────────────┬────────────┘
                                │  BEGIN TRANSACTION
                                ▼
                 ┌──────────────────────────────────┐
                 │  PostgreSQL                      │
                 │  ├─ subscribers                  │
                 │  ├─ api_keys                     │
                 │  └─ outbox  ← event written here │
                 │               atomically with    │
                 │               business row       │
                 └──────────────┬───────────────────┘
                                │  COMMIT
                                │
                                │  (async — Outbox Publisher)
                                ▼
                 ┌──────────────────────────────────┐
                 │  Outbox Publisher                │
                 │  polls outbox WHERE status=      │
                 │  PENDING FOR UPDATE SKIP LOCKED  │
                 └──────────────┬───────────────────┘
                                │
                                ▼
                 ┌──────────────────────────────────┐
                 │  Kafka                           │
                 │  subscribers.events.small        │ ─▶ ADR-0001 notifier
                 │  subscribers.events.whale.<id>   │
                 └──────────────────────────────────┘
```

---

## Decision 1 — Authentication Architecture

### Internal API (webapp)

The internal API is never exposed to the public internet. The API gateway rejects any request to `/internal/v1/*` that does not originate from the webapp's session.

```
  Browser → POST /internal/v1/subscribers
  │  Cookie: session_token (HttpOnly; Secure; SameSite=Strict)
  ▼
  Session Middleware
  ├─ look up session_token in session store (Redis or DB)
  ├─ resolve → { user_id, tenant_id }
  └─ inject tenant_id into request context
  ▼
  Handler
  (tenant_id always comes from context — never from URL/body)
```

`tenant_id` is never accepted from the request payload; it is always resolved from the authenticated session. This prevents tenant-crossing attacks.

### Public API — OAuth2 (Partner Integrations)

```
  Partner → GET /v1/subscribers
  │  Authorization: Bearer <access_token>
  ▼
  OAuth2 Middleware
  ├─ decode JWT header → get kid
  ├─ fetch JWKS (cached, TTL 1h) → verify signature
  ├─ validate: exp, iss, aud
  ├─ extract scopes claim: subscribers:read / subscribers:write
  ├─ extract tenant_id from sub or custom claim
  └─ inject { tenant_id, scopes } into context
  ▼
  Handler
  (scope checked per endpoint via middleware or handler guard)
```

JWT verification is local (no network round-trip per request). JWKS is cached with a short TTL; key rotation is handled transparently.

### Public API — API Key (Private Integrations)

```
  Integration → POST /v1/subscribers
  │  x-api-key: xxxx_a1b2c3d4e5f6g7h8.s3cr3tR4nd0mStr1ng32ch
  ▼
  API Key Middleware
  ├─ split on "." → prefix="a1b2c3d4e5f6g7h8", secret="s3cr3t..."
  ├─ SELECT * FROM api_keys WHERE prefix = ? AND status = 'active'
  ├─ bcrypt.CompareHashAndPassword(stored_hash, secret)
  ├─ check: expires_at IS NULL OR expires_at > now()
  ├─ check: scopes cover the requested operation
  └─ inject { tenant_id, scopes } into context
  ▼
  Handler
```

**Trade-offs:**

| Dimension | Internal Session | OAuth2 | API Key |
|-----------|-----------------|--------|---------|
| Audience | First-party webapp | Third-party partners | Private/script integrations |
| Token lifetime | Session TTL (e.g. 24h) | Short-lived JWT (15min) | Long-lived (manual rotation) |
| Revocation | Delete session row | Await token expiry (or introspection) | Immediate (UPDATE status='revoked') |
| Latency | 1 Redis/DB lookup | Local JWT verify (0 network) | 1 DB lookup + bcrypt |
| Rotation | Re-login | OAuth2 refresh token flow | Manual; key shown once at creation |

---

## Decision 2 — API Key Design

### Key Format

```
xxxx_<16-char-prefix>.<32-char-secret>

Example:
xxxx_a1b2c3d4e5f6g7h8.xK9mP2nQ8rT5vW3yZ7cE4jL6bN1oA0dF
└──┘  └──────────────┘  └──────────────────────────────────┘
 tag      prefix              secret (random, URL-safe base64)
      (stored plaintext)    (hashed with bcrypt, never stored)
```

- **Prefix** — stored plaintext, indexed. Used to look up the row without a full-table scan. Safe to log (does not grant access).
- **Secret** — generated with `crypto/rand`, hashed with `bcrypt(cost=12)` or `argon2id`. Never stored. Shown to the user **once** at creation.
- **Tag** `xxxx_` — allows the key to be detected by secret-scanning tools (GitHub, GitGuardian).

### Database Schema

```sql
CREATE TABLE api_keys (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    TEXT        NOT NULL,
    name         TEXT        NOT NULL,              -- human label, e.g. "Zapier integration"
    prefix       TEXT        NOT NULL UNIQUE,       -- 16 chars, indexed
    secret_hash  TEXT        NOT NULL,              -- bcrypt / argon2id hash
    scopes       TEXT[]      NOT NULL,              -- e.g. {subscribers:read,subscribers:write}
    status       TEXT        NOT NULL DEFAULT 'active',  -- active | revoked
    last_used_at TIMESTAMPTZ,
    expires_at   TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at   TIMESTAMPTZ
);
CREATE INDEX ON api_keys (prefix);
CREATE INDEX ON api_keys (tenant_id);
```

### Key Lifecycle

```
  Create:  generate prefix + secret → bcrypt(secret) → store row → return full key ONCE
  Use:     split key → lookup by prefix → bcrypt.Compare → check status + expiry
  Revoke:  UPDATE api_keys SET status='revoked', revoked_at=now() WHERE id=?
  List:    SELECT id, name, prefix, scopes, status, last_used_at FROM api_keys
           WHERE tenant_id=? ORDER BY created_at DESC
           (secret_hash never returned)
```

### OpenAPI Spec — API Key Management

```yaml
openapi: "3.1.0"
info:
  title: API Key Management
  version: "1.0.0"

paths:
  /internal/v1/api-keys:
    get:
      summary: List API keys for the authenticated tenant
      security:
        - sessionAuth: []
      responses:
        "200":
          description: List of API keys (secret never included)
          content:
            application/json:
              schema:
                type: array
                items:
                  $ref: "#/components/schemas/ApiKey"

    post:
      summary: Create a new API key
      security:
        - sessionAuth: []
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: "#/components/schemas/CreateApiKeyRequest"
      responses:
        "201":
          description: Created — full key returned ONCE, never again
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/ApiKeyCreated"

  /internal/v1/api-keys/{id}:
    delete:
      summary: Revoke an API key immediately
      security:
        - sessionAuth: []
      parameters:
        - name: id
          in: path
          required: true
          schema:
            type: string
            format: uuid
      responses:
        "204":
          description: Revoked

components:
  securitySchemes:
    sessionAuth:
      type: apiKey
      in: cookie
      name: session_token

  schemas:
    ApiKey:
      type: object
      properties:
        id:          { type: string, format: uuid }
        name:        { type: string }
        prefix:      { type: string, example: "a1b2c3d4e5f6g7h8" }
        scopes:      { type: array, items: { type: string } }
        status:      { type: string, enum: [active, revoked] }
        last_used_at:{ type: string, format: date-time, nullable: true }
        expires_at:  { type: string, format: date-time, nullable: true }
        created_at:  { type: string, format: date-time }

    CreateApiKeyRequest:
      type: object
      required: [name, scopes]
      properties:
        name:       { type: string, example: "Zapier integration" }
        scopes:
          type: array
          items:
            type: string
            enum: [subscribers:read, subscribers:write, webhooks:manage]
        expires_at: { type: string, format: date-time, nullable: true }

    ApiKeyCreated:
      allOf:
        - $ref: "#/components/schemas/ApiKey"
        - type: object
          properties:
            key:
              type: string
              description: Full API key — shown once only, store securely
              example: "xxxx_a1b2c3d4e5f6g7h8.xK9mP2nQ8rT5vW3yZ7cE4jL6bN1oA0dF"
```

**Trade-offs:**

| Benefit                                                   | Cost |
|-----------------------------------------------------------|------|
| Prefix lookup is O(1) — no full key scan on every request | bcrypt adds ~100ms per verification (intentional, limits brute-force) |
| Secret never stored — breach of DB does not expose keys   | Key shown once: user must copy it; no recovery path |
| Immediate revocation via status flag                      | No automatic rotation — customers must manually rotate |
| `xxxx_` tag enables automated secret scanning             | Prefix leakage (e.g. in logs) is safe — grants no access alone |

---

## Decision 3 — Transactional Outbox Pattern

### Problem: The Dual-Write Race

A subscriber mutation touches two systems: PostgreSQL (source of truth) and Kafka (event bus). Writing to both non-atomically creates two failure modes:

```
  Failure mode A — event lost:
  ├─ INSERT INTO subscribers  ✓  committed
  └─ kafka.Write(...)         ✗  crash / network → event gone forever

  Failure mode B — phantom event:
  ├─ kafka.Write(...)         ✓  published
  └─ INSERT INTO subscribers  ✗  DB error → rolled back
     notifier fires for a subscriber that does not exist
```

Neither failure is acceptable. The outbox pattern eliminates both.

### Solution

Write the event to an `outbox` table **in the same database transaction** as the business row. A dedicated Outbox Publisher polls the table and forwards rows to Kafka. The two systems are never written to in one logical step.

### Outbox Flow

```
  POST /v1/subscribers  (create example)
  │
  ├─ BEGIN TRANSACTION
  │   ├─ INSERT INTO subscribers (id, tenant_id, email, ...)
  │   └─ INSERT INTO outbox (
  │         event_id   = gen_random_uuid(),   -- idempotency key
  │         tenant_id  = <from ctx>,
  │         topic      = tier_topic(tenant_id),  -- whale or small
  │         msg_key    = key_for_tier(tenant_id, subscriber_id),
  │         payload    = { EventId, Op:"c", TenantId, Subscriber, OccurredAt },
  │         status     = 'PENDING'
  │      )
  └─ COMMIT   ← both rows committed atomically
       │       if DB fails: both rolled back, no partial state
       │
       │  ┌──────────────────────────────────────── ┐
       │  │  Outbox Publisher  (background loop)    │
       │  │                                         │
       └──▶ SELECT id, topic, msg_key, payload      │
            FROM outbox                             │
            WHERE status = 'PENDING'                │
              AND (next_retry_at IS NULL            │
                   OR next_retry_at <= now())       │
            ORDER BY id ASC                         │
            LIMIT 100                               │
            FOR UPDATE SKIP LOCKED                  │ ← multiple instances safe
                 │                                  │
                 ├─ kafka.WriteMessages(batch)      │
                 │                                  │
                 ├─ success:                        │
                 │   UPDATE outbox                  │
                 │   SET status = 'PUBLISHED',      │
                 │       published_at = now()       │
                 │   WHERE id IN (...)              │
                 │                                  │
                 └─ failure:                        │
                     UPDATE outbox                  │
                     SET retry_count = retry_count+1│
                         last_error  = ?,           │
                         next_retry_at =            │
                           now() + backoff(n)       │
                     WHERE id IN (...)              │
            └───────────────────────────────────────┘

  Crash safety:
  ┌─────────────────────────────────────────────────────────────┐
  │ Scenario                        │ Outcome                   │
  ├─────────────────────────────────┼───────────────────────────┤
  │ Crash after COMMIT, before      │ Row stays PENDING →       │
  │ kafka.Write                     │ republished on restart    │
  ├─────────────────────────────────┼───────────────────────────┤
  │ Crash after kafka.Write, before │ Row stays PENDING →       │
  │ UPDATE status='PUBLISHED'       │ republished (duplicate)   │
  │                                 │ → notifier deduplicates   │
  │                                 │ on event_id (ADR-0001)    │
  ├─────────────────────────────────┼───────────────────────────┤
  │ Kafka unreachable               │ Rows accumulate as        │
  │                                 │ PENDING; published when   │
  │                                 │ Kafka recovers            │
  ├─────────────────────────────────┼───────────────────────────┤
  │ DB unreachable                  │ Business op fails with    │
  │                                 │ 500; no partial state     │
  └─────────────────────────────────┴───────────────────────────┘
```

### Outbox Table Schema

```sql
CREATE TABLE outbox (
    id             BIGSERIAL    PRIMARY KEY,
    event_id       UUID         NOT NULL UNIQUE,   -- idempotency key for notifier
    tenant_id      TEXT         NOT NULL,
    topic          TEXT         NOT NULL,           -- whale or small topic
    msg_key        TEXT         NOT NULL,           -- Kafka partition key
    payload        JSONB        NOT NULL,           -- NotifyEventPayload JSON
    status         TEXT         NOT NULL DEFAULT 'PENDING',
    --                                              PENDING | PUBLISHED | FAILED
    retry_count    INT          NOT NULL DEFAULT 0,
    last_error     TEXT,
    next_retry_at  TIMESTAMPTZ,
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT now(),
    published_at   TIMESTAMPTZ
);

-- Publisher reads only PENDING rows; partial index keeps it fast
CREATE INDEX idx_outbox_pending ON outbox (id)
    WHERE status = 'PENDING';

-- Cleanup job targets PUBLISHED rows older than retention window
CREATE INDEX idx_outbox_cleanup ON outbox (published_at)
    WHERE status = 'PUBLISHED';
```

### Outbox vs. Alternatives

| Approach | Atomicity | Complexity | Ops overhead |
|----------|-----------|------------|--------------|
| **Outbox (chosen)** | Full — same DB TX | Medium: poll loop + cleanup job | Low: runs alongside app |
| Kafka Transactions (`exactly_once`) | Full — Kafka idempotent producer + fencing | High: requires Kafka 2.5+, sticky sessions | Medium: tuning required |
| Change Data Capture (CDC / Debezium) | Full — reads WAL | High: Debezium cluster, connector config | High: separate Kafka Connect cluster |
| Two-phase commit (XA) | Full | Very high: rarely supported end-to-end in Go | High: DB + Kafka must support XA |

**Why not CDC?** CDC (Debezium reading PostgreSQL WAL) provides the same guarantees but adds a Kafka Connect cluster, Debezium connectors, schema registry, and WAL retention configuration. For a team not already running Kafka Connect, the outbox pattern delivers equivalent guarantees at a fraction of the operational cost.

**Trade-offs of the Outbox:**

| Benefit | Cost |
|---------|------|
| Zero event loss: event survives any crash after COMMIT | `outbox` table grows; requires a periodic cleanup job for PUBLISHED rows |
| Decouples business write latency from Kafka availability | Kafka publish is asynchronous — small delay (milliseconds to seconds) between COMMIT and Kafka delivery |
| Multiple publisher instances safe via `FOR UPDATE SKIP LOCKED` | Publisher must be monitored; a stuck publisher silently delays all events |
| At-least-once: notifier already deduplicates on `event_id` (ADR-0001) | Duplicate detection in notifier must be maintained as a contract |

---

## Consequences

- **No lost publish events.** Every committed subscriber mutation has an `outbox` row. Kafka publish is retried indefinitely until it succeeds.
- **At-least-once end-to-end.** The chain is: outbox publisher (at-least-once to Kafka) → notifier (at-least-once to partner). Partners must handle `EventId` idempotency.
- **Operational responsibilities:**
  - Outbox cleanup job: delete `status='PUBLISHED'` rows older than retention window (e.g. 7 days).
  - Publisher health alerting: alert if `COUNT(*) WHERE status='PENDING' AND created_at < now() - interval '5 minutes'` is non-zero.
  - API Key rotation: no automatic rotation — customers are responsible; provide `last_used_at` in the UI to aid auditing.
- **Auth contract:** `tenant_id` is always resolved server-side from the auth context, never accepted from client input. This applies to all three auth paths.
- **Connection to ADR-0001:** The `topic` and `msg_key` written to the outbox row must respect the D7 whale/small segregation rules (ADR-0001). The Outbox Publisher routes based on these pre-computed values, not by re-classifying the tenant at publish time.
