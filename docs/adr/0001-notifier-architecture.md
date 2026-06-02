# ADR-0001: Notifier Service Architecture

---

## Context

The notifier service must reliably propagate subscriber lifecycle events — create (`c`), update (`u`), delete (`d`), add-to-segment (`a`) — to external partner services via HTTP webhooks. Events originate upstream and are consumed from Kafka; the notifier owns delivery to partners.

Three core challenges drive the architectural decisions in this document:

1. **Reliability** — partner webhooks fail transiently or permanently; crashes must not lose events.
2. **Scalability** — worker capacity must grow horizontally as event volume fluctuates, with no code changes.
3. **Fairness** — whale accounts (hundreds of thousands of subscribers) must not starve small-account customers sharing the same infrastructure.

---

## System Challenges

### Challenge 1 — Reliability

**Problem:** A partner webhook endpoint may fail transiently (network blip, HTTP 429/502/503) or permanently (bad payload, endpoint removed). A consumer crash mid-processing must not silently drop the event.

**Solution — 3-layer safety net:**

```
  Event enters Workflow
        │
        ▼
  ┌─────────────────────────────────────────────────────────────┐
  │  Layer 1 — HTTP client retry  (pkg/http/client/resty.go)    │
  │  Fast, in-process, configurable retry_count + interval      │
  │  Handles: connection timeout, transient 5xx                 │
  └────────────────────────────┬────────────────────────────────┘
                               │ still failing?
                               ▼
  ┌─────────────────────────────────────────────────────────────┐
  │  Layer 2 — Exponential backoff retry  (D4 / Retry processor)│
  │  cenkalti/backoff: initial_interval → max_elapsed_time      │
  │  Handles: sustained partner degradation, rate limiting      │
  └────────────────────────────┬────────────────────────────────┘
                               │ max retries exhausted?
                               ▼
  ┌─────────────────────────────────────────────────────────────┐
  │  Layer 3 — Dead-Letter Queue  (D3 / DeadLetterProcessor)    │
  │  Serialize raw KafkaMessage bytes → PostgreSQL dead_letters │
  │  Handles: persistent failures, bad payloads, partner outage │
  └────────────────────────────┬────────────────────────────────┘
                               │ operator triggers replay
                               ▼
                    Dead-Letter Replay Job
                    re-enters at Layer 2

  ──────────────────────────────────────────────────────────────
  Kafka offset committed ONLY after layer 1 succeeds
  OR after layer 3 saves to DLQ → zero silent data loss
```

Decisions: **D3**, **D4**

---

### Challenge 2 — Scalability

**Problem:** Subscriber event volume fluctuates — a product launch or bulk import can spike throughput by orders of magnitude. Worker capacity must expand horizontally without redeploying configuration or changing code.

**Solution — Kafka partition parallelism + per-tier independent scaling:**

```
  subscribers.events.small  (shared topic, N partitions)
  ┌──────────────────────────────────────────────────────────┐
  │  Partition 0 ──▶ Worker instance A  ┐                    │
  │  Partition 1 ──▶ Worker instance B  │ consumer group:    │
  │  Partition 2 ──▶ Worker instance C  │ notifier-small     │
  │  Partition N ──▶ Worker instance D  ┘                    │
  │                                                          │
  │  Add more worker instances → Kafka rebalances            │
  │  partitions automatically. No config change needed.      │
  │  Ceiling: max_parallel_workers = partition_count         │
  └──────────────────────────────────────────────────────────┘

  subscribers.events.whale.<tenant_id>  (dedicated, M partitions)
  ┌──────────────────────────────────────────────────────────┐
  │  Partition 0 ──▶ Worker instance X  ┐                    │
  │  Partition 1 ──▶ Worker instance Y  │ consumer group:    │
  │  Partition M ──▶ Worker instance Z  ┘ notifier-whale-    │
  │                                        <tenant_id>       │
  │  Each whale tenant scales independently of others        │
  │  and independently of the small-account tier             │
  └──────────────────────────────────────────────────────────┘

  To scale further: increase partition count on the topic,
  then add worker instances → Kafka redistributes.
```

Decisions: **D1**, **D7**

---

### Challenge 3 — Fairness

**Problem:** A whale account processing a 200k-subscriber bulk import emits hundreds of thousands of events simultaneously. These events will flood the shared queue and cause small-account customers — who triggered just a handful of events — to wait minutes instead of milliseconds.

**Solution — Queue segregation by tenant tier.** See **D7** for the full decision, flow diagram, and trade-offs.

Summary:
- Whale → own dedicated topic, own consumer group → isolated from all other tenants.
- Small → shared topic, shared consumer group → whale bursts cannot bleed into this lane.
- Per-subscriber ordering preserved in both tiers via message key design.

---

## Full System Flow

```
 ┌──────────────────────────────────────────────────────────────────────────────┐
 │                         NOTIFIER — SYSTEM OVERVIEW                           │
 └──────────────────────────────────────────────────────────────────────────────┘

  ┌─────────────┐       ┌───────────────────────────────┐
  │  Upstream   │       │  Kafka Cluster                │
  │  Producer   │──────▶│  Topic: subscribers.events.*  │
  │  (external) │       │  Partitions: N                │
  └─────────────┘       └───────────────┬───────────────┘
                                        │  batch fetch
                                        ▼
                         ┌──────────────────────────────┐
                         │  SegmentIOSource             │
                         │  kafka-go batch reader       │
                         │  group: webhook-notifier     │
                         └──────────────┬───────────────┘
                                        │  []KafkaMessage
                                        ▼
                         ┌──────────────────────────────┐
                         │  Workflow[KafkaMessage]      │◀─── cmd/event-streaming
                         │  (fetch → process → commit)  │
                         └──────────────┬───────────────┘
                                        │
                    ┌───────────────────┴───────────────────┐
                    │                                       │
              success ▼                               error ▼
     ┌─────────────────────────┐         ┌────────────────────────────┐
     │  Kafka offset committed │         │  PostgreSQL dead_letters   │
     │  (at-least-once)        │         │  (raw bytes preserved)     │
     └─────────────────────────┘         └────────────────┬───────────┘
                                                          │
                                         ┌────────────────▼───────────┐
                                         │  Dead-Letter Replay Job    │◀─── cmd/job/trivial/
                                         │  (re-enters workflow at    │     dead-letter-replay
                                         │   Retry step)              │
                                         └────────────────────────────┘


 ┌──────────────────────────────────────────────────────────────────────────────┐
 │                           PROCESSOR CHAIN DETAIL                             │ 
 └──────────────────────────────────────────────────────────────────────────────┘

  []KafkaMessage
       │
       ▼
  ┌────────────────────────────────────────────────────────────────┐
  │  DeadLetterProcessor  (outermost — wraps entire chain)         │
  │  • normal flow: error → save to PostgreSQL, suppress error     │
  │  • replay flow: error → increment retry_count, update DB       │
  │                                                                │
  │  ┌──────────────────────────────────────────────────────────┐  │
  │  │  Retry  (exponential backoff via cenkalti/backoff)       │  │
  │  │  • initial_interval / max_elapsed_time / max_retries     │  │
  │  │                                                          │  │
  │  │  ┌────────────────────────────────────────────────────┐  │  │
  │  │  │  MaxWait  (per-message timeout, default 30s)       │  │  │
  │  │  │  • cancels ctx on exceeded deadline                │  │  │
  │  │  │                                                    │  │  │
  │  │  │  ┌─────────────────────────────────────────────┐   │  │  │
  │  │  │  │  NotifyEventParser                          │   │  │  │
  │  │  │  │  • JSON unmarshal KafkaMessage → NotifyEvent│   │  │  │
  │  │  │  │  • filter: skip non-CRUD ops silently       │   │  │  │
  │  │  │  │                                             │   │  │  │
  │  │  │  │  ┌───────────────────────────────────────┐  │   │  │  │
  │  │  │  │  │  Notifier  (leaf processor)           │  │   │  │  │
  │  │  │  │  │  • route by Op: c / u / d / a         │  │   │  │  │
  │  │  │  │  │  • POST /notify to partner service    │  │   │  │  │
  │  │  │  │  │  • non-2xx → return error             │  │   │  │  │
  │  │  │  │  └───────────────────────────────────────┘  │   │  │  │
  │  │  │  └─────────────────────────────────────────────┘   │  │  │
  │  │  └────────────────────────────────────────────────────┘  │  │
  │  └──────────────────────────────────────────────────────────┘  │
  └────────────────────────────────────────────────────────────────┘
       │
       ▼
  Kafka offset committed  OR  dead_letters row written


 ┌──────────────────────────────────────────────────────────────────────────────┐
 │                           DEAD-LETTER REPLAY FLOW                            │
 └──────────────────────────────────────────────────────────────────────────────┘

  PostgreSQL dead_letters
       │
       │  DeadLetterSource (cursor-based readers)
       │  ├─ IdsReader        — replay specific row IDs
       │  ├─ RangeReader      — replay ID range
       │  ├─ TenantReader     — replay by tenant_id
       │  └─ WorkflowReader   — replay by workflow name
       │
       ▼
  DeadLetterDispatcher
  ├─ deserialize Data (json.RawMessage) → []KafkaMessage
  ├─ set WorkflowContext.Attr[ReplayLetterId] = id
  ├─ set WorkflowContext.Attr[ReplayRetryCount] = retry_count
  └─ call workflowRoot.Process(ctx, items)   ← enters at Retry step
       │
       ├─ success → DELETE FROM dead_letters WHERE id = ?
       └─ failure → UPDATE dead_letters SET retry_count++, error = ?


 ┌──────────────────────────────────────────────────────────────────────────────┐
 │                         DATA SHAPE THROUGH WORKFLOW                          │
 └──────────────────────────────────────────────────────────────────────────────┘

  KafkaMessage { Key []byte, Value []byte, Topic, Partition, Offset }
       │
       │  NotifyEventParser
       ▼
  NotifyEvent {
    Key:   { TenantId, SubscriberId }
    Value: { Payload: {
        EventId     string       // UUID — idempotency key for partner
        Op          string       // "c" | "u" | "d" | "a"
        TenantId    string
        Subscriber  json.RawMessage
        OccurredAt  time.Time
    }}
  }
       │
       │  Notifier  →  POST partner/notify
       ▼
  Request body: NotifyEventPayload { EventId, Op, TenantId, Subscriber, OccurredAt }
  Auth header:  x-api-key: <partner.key>
```

---

## Architectural Decisions

### D1 — Kafka as Message Broker

**Decision:** Consume subscriber events from Kafka topic `subscribers.events.*` using `segmentio/kafka-go`. Commit offsets only after the full processor chain succeeds.

**Rationale:** Kafka provides durable, ordered, replayable event storage. Deferring offset commit ensures no event is silently dropped on failure.

**Trade-offs:**

| Benefit | Cost |
|---------|------|
| At-least-once delivery with crash safety | Requires Zookeeper + Kafka cluster (ops overhead) |
| Per-partition ordering preserved | Cross-partition ordering not guaranteed |
| Consumer group parallelism via partitions | Duplicate delivery possible on restart — partners must handle `EventId` idempotency |
| Offset rewind available for backfill | Kafka lag alerting needed in production |

---

### D2 — Generic Workflow Engine

**Decision:** Use a generic `Workflow[T]` that drives a `fetch → process → commit` loop. Processors implement a `Processor` interface and chain via `SetNext()`. `WorkflowBuilder` assembles the chain.

```
pkg/workflow/
  workflow.go   — Workflow[T], WorkflowBuilder[T]
  processor.go  — Processor interface, Retry, MaxWait
  source.go     — EventSource[T] interface, SegmentIOSource
```

**Rationale:** Separates the looping/committing concern from business logic. The same engine is reused for both the streaming service and the replay job without duplication.

### Why Chain of Responsibility

The central structural requirement is that `DeadLetterProcessor` must wrap the **entire remaining chain** and intercept any error regardless of which inner processor produced it — "outermost catches all":

```
  DeadLetterProcessor.Process(ctx, items)
    └─ next.Process(ctx, items)   ← resolves to the entire remaining chain
         any error from Retry / MaxWait / Parser / Notifier bubbles up here
    on error → save raw bytes to DLQ, suppress error (Kafka offset still commits)
```

This "wrap everything below me" semantics is the natural expression of Chain of Responsibility. It is awkward or impossible in flat alternatives:

| Pattern | Why not chosen |
|---------|----------------|
| **Sequential function calls** | No abstraction boundary — no single location catches errors from all downstream steps; adding a step requires editing the loop body |
| **Go middleware stack** (`net/http` style) | Designed for one-shot request/response; processors here must pass a mutable `[]WorkflowItem` *forward and receive a modified slice back* — two-way data flow middleware does not naturally express |
| **Channel pipeline** (goroutine per step) | Concurrent overhead for a workload that is sequential per message; error propagation across goroutines requires extra synchronisation (`errgroup`, channel select), making "abort chain on first error" complex |
| **Event bus / pub-sub** | Fully decoupled — no ordering guarantee; error in one handler does not abort others, making "stop on first error, dead-letter the message" impossible without a separate coordination layer |
| **Decorator pattern** | Each decorator wraps a *single specific next* — cannot express "wrap everything below me" with a single generic interface without the chain's `SetNext` indirection |

Three properties of this problem make Chain of Responsibility the right fit:

1. **Wrapping semantics** — `DeadLetterProcessor` and `Retry` both need to catch errors from everything further down the chain, not just their immediate successor.
2. **Open/closed extensibility** — adding a new step (schema validation, rate limiting) requires only a new type implementing `Processor` and one line in `bind.go`; no existing processor is modified.
3. **Symmetric reuse** — `Retry` and `MaxWait` run unchanged in both the streaming path and the dead-letter replay path; the chain is assembled at wire time, not hard-coded per flow.

**Trade-offs:**

| Benefit | Cost |
|---------|------|
| Each processor independently testable | Chain assembly order is critical — wrong order causes subtle bugs |
| Add new middleware by inserting a processor in `bind.go` | Type erasure via `WorkflowItem` wrapper adds wrapping/unwrapping boilerplate |
| Same engine for streaming and replay | Misplaced processor can bypass retry or dead-lettering |

---

### D3 — PostgreSQL Dead-Letter Queue + Replay Job

**Decision:** Wrap the entire processor chain in `DeadLetterProcessor`. On final failure it serializes the raw `KafkaMessage` bytes into the `dead_letters` table (JSONB). A separate one-shot job replays rows back through the workflow.

```
internal/
  adapter/deadletter/dead_letter_repository.go   — GORM repository
  domain/dead_letter.go                          — DeadLetter entity
  port/persistent/dead_letter_repository.go      — interface
  application/event-streaming/util/
    dead_letter_processor.go                     — wraps chain
  application/job/dead-letter-replay/
    dead_letter_source.go                        — cursor readers
    dead_letter_dispatcher.go                    — re-enter workflow
```

**Replay signaling:** `WorkflowContext.Attr[ReplayLetterId]` flags an item as a replay so `DeadLetterProcessor` routes it to `handleRetryResult()` instead of writing a new row.

**Trade-offs:**

| Benefit | Cost |
|---------|------|
| No event permanently lost — always replayable | PostgreSQL must be available for every message processing step |
| Original raw bytes preserved — zero data loss | dead_letters table grows unbounded without a TTL/archival policy |
| Flexible replay filtering (by ID, range, tenant, workflow) | Replay is a manual operator job, not automatic |
| Retry count tracked across replays | Multiple replay attempts can pile up retry_count without a circuit breaker |

---

### D4 — Exponential Backoff Retry

**Decision:** Use `cenkalti/backoff/v4` between `DeadLetterProcessor` and `MaxWait`. Configuration: `initial_interval`, `max_elapsed_time`, `max_retries` (all tunable per environment).

**Rationale:** Transient partner errors (429, 502, 503) should not immediately dead-letter. Backoff gives the partner time to recover without hammering it.

**Trade-offs:**

| Benefit | Cost |
|---------|------|
| Absorbs transient partner failures automatically | Increases per-message latency during retries |
| Configurable per environment (local vs. prod) | Long `max_elapsed_time` can stall Kafka partition progress |
| Works identically in replay path | Retry + MaxWait interaction must be tuned so timeout > one retry cycle |

---

### D5 — Google Wire for Dependency Injection

**Decision:** Use compile-time DI via `google/wire`. Provider sets defined in `bind.go` per application. Generated code lives in `wire_gen.go`.

```
internal/application/event-streaming/
  bind.go          — NotifyWorkflowProviderSet
  wire/wire.go     — WireExporter, GetWorkflow
  wire/wire_gen.go — generated

internal/application/job/dead-letter-replay/
  bind.go          — DeadLetterReplayProviderSet
  wire/...
```

**Trade-offs:**

| Benefit | Cost |
|---------|------|
| Dependency graph verified at compile time | `wire` CLI must be run after adding/removing providers |
| Auto-generated wiring is readable and debuggable | `wire_gen.go` checked into VCS; drift if not regenerated |
| No runtime reflection overhead | Learning curve for new contributors unfamiliar with Wire |

---

### D6 — Observability: OpenTelemetry + Prometheus + Zap

**Decision:** Expose Prometheus metrics via OpenTelemetry SDK on a configurable port (default `6067`). Use `go.uber.org/zap` for structured logging throughout.

**Metrics tracked:**

```
fetch_message_latency    — Kafka fetch duration
fetch_message_lag        — message age at fetch (producer → consumer)
commit_message_latency   — offset commit duration
commit_message_lag       — end-to-end processing lag
write_message_latency    — Kafka write duration (publisher job)
```

**Trade-offs:**

| Benefit | Cost |
|---------|------|
| Production-ready visibility out of the box | OTel SDK adds ~10ms startup overhead |
| Lag metrics expose backpressure and slow consumers | Metrics cardinality must be managed (avoid high-dimensional labels) |
| Zap fields (tenant_id, offset) enable log correlation | Verbose DEBUG logging from kafka-go adapter must be suppressed in prod |

---

### D7 — Queue Segregation for Fairness

**Problem:** Whale accounts (subscribers in the hundreds of thousands) and small accounts produce events simultaneously. Without segregation, a whale's burst floods the shared queue and causes long wait times for small-account customers.

**Decision:** Route events at publish time based on tenant tier:

- **Whale accounts** → dedicated topic per tenant: `subscribers.events.whale.<tenant_id>`
  - Message key = `subscriber_id`
- **Small accounts** → shared topic: `subscribers.events.small`
  - Message key = `tenant_id:subscriber_id`

**Flow:**

```
  Upstream Producer
        │
        ▼
  ┌──────────────────────┐
  │     Event Router     │  classify tenant as whale or small
  │  (tier flag / count  │  (e.g. subscriber_count > threshold,
  │   threshold check)   │   or explicit config whitelist)
  └──────────┬───────────┘
             │
      ┌──────┴──────────────────┐
      │                         │
   whale                      small
      │                         │
      ▼                         ▼
 subscribers.             subscribers.
 events.whale.            events.small
 <tenant_id>              (shared topic)
 key = subscriber_id      key = tenant_id:subscriber_id
      │                         │
      ▼                         ▼
 Dedicated consumer        Shared consumer
 group per whale tenant    group
 (1 group per whale)       (1 group, all small tenants)
      │                         │
      └──────────┬──────────────┘
                 ▼
       Workflow[KafkaMessage]
       (same processor chain — D2)
       DeadLetterProcessor → Retry → MaxWait
       → NotifyEventParser → Notifier
```

**Ordering guarantees:**

- **Whale:** `key = subscriber_id` → all events for one subscriber hash to the same partition → in-order delivery per subscriber guaranteed.
- **Small:** `key = tenant_id:subscriber_id` → Kafka hashes the composite key to a partition → all events for one subscriber within one tenant stay on one partition (ordered); different tenants spread naturally across partitions.

**Trade-offs:**

| Benefit | Cost |
|---------|------|
| Whale bursts cannot starve small-account processing | Whale classification logic must be maintained (tier flag or subscriber-count threshold) |
| Per-subscriber ordering preserved for both tiers | Topic proliferation — one topic per whale tenant; requires dynamic topic provisioning |
| Consumer groups scale independently per tier | Ops must monitor each whale topic and provision consumer capacity per whale tenant |
| Small accounts experience consistent, predictable latency | Tier reclassification (small → whale promotion) requires a topic migration and consumer restart |
| Zero code changes to processor chain — only routing differs | A sudden new whale tenant needs a topic + consumer group created before it can be processed |

---

## Consequences

| Challenge | Guarantee | Caveat |
|-----------|-----------|--------|
| **Reliability** | Three-layer safety net (HTTP retry → backoff → DLQ) ensures no silent event loss. Kafka offset committed only on success. | DLQ replay is a manual operator step; automation (cron, alerting on `retry_count`) is the deployment layer's responsibility. |
| **Scalability** | Horizontal scaling by adding consumer instances + Kafka partition count. Small and whale tiers scale independently. No code changes required. | `max_parallel_workers = partition_count` — over-provisioning consumers beyond partition count wastes resources; partition count must be planned upfront. |
| **Fairness** | Whale bursts are fully isolated to per-tenant topics; small accounts always have dedicated queue capacity. Per-subscriber ordering preserved in both tiers. | Whale tier promotion (small → whale) requires topic migration and consumer restart. Dynamic topic provisioning must be automated in production. |

**Additional consequences:**
- Partners must treat `EventId` as an idempotency key — at-least-once delivery means duplicates are possible on consumer restart.
- Extensibility: new event operations or notification targets require only a new `Processor` implementation inserted into `bind.go` — no changes to the workflow engine.
- Local development: `docker-compose.yml` provides Kafka + ZooKeeper + PostgreSQL; `mock-partner` and `publish-event` binaries cover full end-to-end testing without a real partner.
