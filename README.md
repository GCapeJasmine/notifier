# Subscribers

A Go service that consumes subscriber lifecycle events from Kafka and notifies external partner services via HTTP webhook. Built with a composable processor pipeline, exponential-backoff retries, and a PostgreSQL dead-letter queue with replay capability.

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│  Flow A: Event Streaming (main service)                         │
│                                                                 │
│  Kafka topic                                                    │
│  subscribers.events.*                                           │
│       │                                                         │
│       ▼                                                         │
│  [SegmentIOSource]  ──── fetch batch ──────────────────────┐   │
│       │                                                     │   │
│       ▼                                                     │   │
│  [DeadLetterProcessor]  ◄── wraps entire chain             │   │
│       │   on error → save to PostgreSQL dead_letters        │   │
│       ▼                                                     │   │
│  [Retry]  exponential backoff (configurable)                │   │
│       │                                                     │   │
│       ▼                                                     │   │
│  [MaxWait]  per-message timeout (default 30s)               │   │
│       │                                                     │   │
│       ▼                                                     │   │
│  [NotifyEventParser]  unmarshal JSON, filter non-CRUD       │   │
│       │                                                     │   │
│       ▼                                                     │   │
│  [Notifier]  POST to partner service (op: c/u/d/a)          │   │
│       │                                                     │   │
│       └─────────────────────────────────────────────────────┘   │
│                            commit Kafka offset                  │
└─────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────┐
│  Flow B: Dead-Letter Replay (job)                               │
│                                                                 │
│  PostgreSQL dead_letters                                        │
│       │  (filter: ids / id range / tenant_id / workflow)        │
│       ▼                                                         │
│  [DeadLetterSource]  paginate records                           │
│       │                                                         │
│       ▼                                                         │
│  [DeadLetterDispatcher]                                         │
│       │  re-enter original workflow at step [Retry]             │
│       ├── success → DELETE dead_letters row                     │
│       └── failure → UPDATE retry_count + error                  │
└─────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────┐
│  Flow C: Publish Event (test utility)                           │
│                                                                 │
│  Config (tenants, subscribers, interval)                        │
│       │                                                         │
│       ▼                                                         │
│  [Publisher]  generate synthetic CRUD events → Kafka topic      │
└─────────────────────────────────────────────────────────────────┘
```

---

## Project Structure

```
subscribers/
├── cmd/
│   ├── event-streaming/          # Main service binary (webhook-notifier)
│   └── job/trivial/
│       ├── dead-letter-replay/   # Replay failed messages from PostgreSQL
│       ├── mock-partner/         # Local stub for the partner HTTP service
│       └── publish-event/        # Publish test events to Kafka
├── common/
│   ├── kafka/                    # kafka-go reader/writer wrappers + metrics
│   ├── log/                      # Zap logger setup
│   ├── postgres/                 # GORM connection helpers
│   └── utils/                    # Config loading, retry logic
├── config/                       # YAML config files for each binary
├── internal/
│   ├── adapter/deadletter/       # PostgreSQL implementation of DLQ repository
│   ├── application/
│   │   ├── event-streaming/      # Notify workflow: processors, config, wire
│   │   └── job/dead-letter-replay/ # Replay workflow: source, dispatcher, wire
│   ├── domain/                   # DeadLetter domain entity
│   └── port/persistent/          # Repository interface definitions
└── pkg/
    ├── partner/                  # HTTP client for partner webhook calls
    ├── telemetry/                # Prometheus metrics exporter
    ├── workflow/                 # Generic workflow engine (source + processor chain)
    ├── http/client/              # Resty wrapper with retry
    └── utils/                    # Application utilities
```

---

## Full Flows

### Flow A — Event Streaming

**Entry point:** `cmd/event-streaming/main.go`

The service runs a single continuous loop: fetch → process → commit.

| Step | Component | What it does |
|------|-----------|--------------|
| 1 | `SegmentIOSource` | Reads a batch of messages from the Kafka topic |
| 2 | `DeadLetterProcessor` | Wraps the remaining chain; on any error saves the raw message to `dead_letters` in PostgreSQL |
| 3 | `Retry` | Retries the downstream chain with exponential backoff before giving up and letting the dead-letter handler fire |
| 4 | `MaxWait` | Cancels context if a single message takes longer than `notifier_max_duration` |
| 5 | `NotifyEventParser` | Unmarshals Kafka key (`tenant_id`, `subscriber_id`) and value (event payload); drops messages with empty values |
| 6 | `Notifier` | Routes by `op` field and POSTs to the partner service via HTTP |
| 7 | (source) | Kafka offset committed only after all steps succeed |

**Operation codes** in the event payload:

| `op` | Meaning |
|------|---------|
| `c` | Create subscriber |
| `u` | Update subscriber |
| `d` | Delete subscriber |
| `a` | Add subscriber to segment |

**Kafka message shape:**

```json
// Key
{ "tenant_id": "t1", "subscriber_id": "s1" }

// Value
{
  "payload": {
    "event_id": "uuid",
    "op": "c",
    "tenant_id": "t1",
    "subscriber": { "subscriber_id": "s1" },
    "occurred_at": "2024-01-01T00:00:00Z"
  }
}
```

---

### Flow B — Dead-Letter Replay

**Entry point:** `cmd/job/trivial/dead-letter-replay/main.go`

Reads previously failed messages from PostgreSQL and re-runs them through the notify workflow (starting after `SegmentIOSource`, at the `Retry` step).

**Filter options** (set in `replay:` config section):

| Field | Description |
|-------|-------------|
| `ids` | Replay specific dead-letter IDs |
| `from_id` / `to_id` | Replay an ID range |
| `tenant_id` | Replay all failures for a tenant |
| *(none)* | Defaults to workflow-based pagination |

**Outcome per letter:**
- Success → row deleted from `dead_letters`
- Failure → `retry_count` incremented, `error` updated

---

### Flow C — Publish Event (Test Utility)

**Entry point:** `cmd/job/trivial/publish-event/main.go`

Generates and publishes synthetic subscriber events to Kafka on a configurable interval. Used to seed local development or load-test the pipeline.

---

## Configuration

All binaries accept a `--config <path>` flag pointing to a YAML file. Environment variables override any YAML field using `__` as the nesting separator (Viper convention), e.g.:

```
KAFKA_READER__AUTH__URL=broker:9092
PARTNER__KEY=secret
```

### Event Streaming (`notify-webhook-local.yaml`)

```yaml
workflow_name: "notify-workflow"

kafka_reader:
  auth:
    url: "localhost:9092"
  group_id: "webhook-notifier"
  topic: "subscribers.events"
  start_offset: -2           # -2 = latest, -1 = earliest

postgres:
  host: "localhost"
  port: 5432
  username: "postgres"
  password: "postgres"
  database: "subscribers"
  ssl_mode: "disable"

metric:
  enable: true
  port: 6067                 # Prometheus scrape port

notifier_max_duration: "30s"

retry_option:
  enabled: true
  initial_interval: "200ms"
  randomization_factor: 0.5
  multiplier: 2.0
  max_interval: "2s"
  max_elapsed_time: "10s"
  max_retries: 3

partner:
  base_url: "http://localhost:8080"
  key: "local-api-key"
  client_config:
    retry_count: 1
    retry_interval: "200ms"
    retry_max_wait: "2s"
```

### Dead-Letter Replay (`dead-letter-replay-local.yaml`)

```yaml
replay:
  tenant_id: "tenant-002"   # or: ids: [1,2,3] / from_id+to_id

event_streaming:             # same structure as above
  workflow_name: "notify-workflow"
  kafka_reader: { ... }
  postgres: { ... }
  partner: { ... }
```

### Publish Event (`publish-event-local.yaml`)

```yaml
kafka_writer:
  auth:
    url: "localhost:9092"
  topic: "subscribers.events"

event:
  interval: "2s"
  tenant_ids: ["tenant-001", "tenant-002", "tenant-003"]
  subscribers:
    - subscriber_id: "sub-001"
      data: '{"subscriber_id":"sub-001"}'
```

Sample configs for all variants (small / whale) are in the `config/` directory.

---

## Database

**Table: `dead_letters`** (auto-migrated by GORM on service startup)

| Column | Type | Notes |
|--------|------|-------|
| `id` | BIGSERIAL | Primary key |
| `tenant_id` | VARCHAR | Indexed |
| `workflow_name` | VARCHAR | Indexed (e.g., `notify-workflow`) |
| `error` | TEXT | Last error message |
| `data` | JSONB | Serialized `KafkaMessage` items |
| `retry_count` | INT | Number of replay attempts |
| `created_at` | TIMESTAMP | |
| `updated_at` | TIMESTAMP | |

**DSN example:** `host=localhost port=5432 user=postgres password=postgres dbname=subscribers sslmode=disable`

---

## Partner Webhook API

The service calls the partner endpoint for every event it processes:

```
POST {partner.base_url}
x-api-key: {partner.key}
Content-Type: application/json

{
  "event_id": "uuid",
  "op": "c",
  "tenant_id": "t1",
  "subscriber": { ... },
  "occurred_at": "2024-01-01T00:00:00Z"
}
```

Non-2xx responses are treated as errors and trigger the retry/dead-letter path.

---

## Local Development

**Prerequisites:** Docker, Go 1.25+

```bash
# 1. Start infrastructure (Zookeeper, Kafka, PostgreSQL)
docker-compose up -d

# 2. Start the mock partner service (simulates the webhook receiver)
go run ./cmd/job/trivial/mock-partner

# 3. Start the main event-streaming service
go run ./cmd/event-streaming --config config/notify-webhook-local.yaml

# 4. Publish test events to Kafka
go run ./cmd/job/trivial/publish-event --config config/publish-event-local.yaml
```

Infrastructure ports:

| Service | Port |
|---------|------|
| Kafka | 9092 |
| PostgreSQL | 5432 |
| Prometheus metrics | 6067 |

---

## Dead-Letter Replay Operation

Edit `config/dead-letter-replay-local.yaml` to choose which records to replay, then run:

```bash
go run ./cmd/job/trivial/dead-letter-replay \
  --config config/dead-letter-replay-local.yaml
```

**Replay filter examples** (set one in the `replay:` block):

```yaml
# By specific IDs
replay:
  ids: [42, 43, 44]

# By ID range
replay:
  from_id: 100
  to_id: 200

# By tenant
replay:
  tenant_id: "tenant-001"
```

---

## Observability

**Prometheus metrics** — scraped at `http://localhost:6067/metrics`

| Metric | Description |
|--------|-------------|
| `fetch_message_latency` | Kafka fetch duration |
| `fetch_message_lag` | Age of message at fetch time |
| `commit_message_latency` | Kafka commit duration |
| `commit_message_lag` | End-to-end processing lag |
| `write_message_latency` | Kafka write duration (publisher) |

**Structured logging** via `go.uber.org/zap`. All log lines include workflow name, tenant ID, and offset where applicable.

---

## Running Tests

```bash
go test ./...
```

Key test packages:
- `internal/application/event-streaming/util/` — parser and dead-letter processor
- `internal/application/job/dead-letter-replay/` — source readers and dispatcher
- `common/kafka/` — batch reader behavior

---

## Key Source Files

| File | Purpose |
|------|---------|
| `cmd/event-streaming/main.go` | Main service entry point |
| `pkg/workflow/workflow.go` | Generic workflow engine |
| `internal/application/event-streaming/util/notification.go` | Event parser |
| `internal/application/event-streaming/util/dead_letter_processor.go` | DLQ handler |
| `pkg/partner/client.go` | HTTP partner client |
| `internal/adapter/deadletter/dead_letter_repository.go` | PostgreSQL DLQ repository |
| `internal/application/event-streaming/wire/wire.go` | Dependency injection setup |
| `config/notify-webhook-local.yaml` | Reference config for local dev |
