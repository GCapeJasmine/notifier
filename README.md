# Notifier

A Go service that consumes subscriber lifecycle events from Kafka and notifies external partner services via HTTP webhook. Built with a composable processor pipeline, exponential-backoff retries, and a PostgreSQL dead-letter queue with replay capability.

## Documentation

Architecture decisions, full flow diagrams, and design trade-offs live in [`docs/adr/`](docs/adr/):

- [ADR-0001 — Notifier Architecture](docs/adr/0001-notifier-architecture.md) — Kafka consumer, workflow engine, dead-letter queue, retry strategy, observability, fairness
- [ADR-0002 — Upstream API & Outbox](docs/adr/0002-upstream-api-and-outbox.md) — API authentication, API key design, transactional outbox pattern

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

## Architecture

The service is structured around **Hexagonal Architecture** (Ports and Adapters). Dependency arrows point inward only — infrastructure knows about the domain; the domain knows nothing about infrastructure.

```
  ┌───────────────────────────────────────────────────────────────────┐
  │  External Systems                                                 │
  │  Kafka · PostgreSQL · HTTP Partner                                │
  │                                                                   │
  │  ┌─────────────────────────────────────────────────────────────┐  │
  │  │  Adapters  (internal/adapter/ · pkg/ · common/)             │  │
  │  │  Kafka reader/writer, GORM dead-letter repo, partner client │  │
  │  │                                                             │  │
  │  │  ┌───────────────────────────────────────────────────────┐  │  │
  │  │  │  Application  (internal/application/ · pkg/workflow/) │  │  │
  │  │  │  event-streaming workflow · dead-letter-replay job    │  │  │
  │  │  │                                                       │  │  │
  │  │  │  ┌─────────────────────────────────────────────────┐  │  │  │
  │  │  │  │  Ports  (internal/port/)                        │  │  │  │
  │  │  │  │  DeadLetterRepository  (interface)              │  │  │  │
  │  │  │  │                                                 │  │  │  │
  │  │  │  │  ┌───────────────────────────────────────────┐  │  │  │  │
  │  │  │  │  │  Domain  (internal/domain/)               │  │  │  │  │
  │  │  │  │  │  DeadLetter entity                        │  │  │  │  │
  │  │  │  │  └───────────────────────────────────────────┘  │  │  │  │
  │  │  │  └─────────────────────────────────────────────────┘  │  │  │
  │  │  └───────────────────────────────────────────────────────┘  │  │
  │  └─────────────────────────────────────────────────────────────┘  │
  └───────────────────────────────────────────────────────────────────┘
```

| Layer | Directory | Depends on |
|-------|-----------|------------|
| Domain | `internal/domain/` | nothing |
| Port | `internal/port/persistent/` | domain only |
| Application | `internal/application/` | domain + ports |
| Adapter | `internal/adapter/` | ports (implements them) |
| Infrastructure | `pkg/`, `common/` | nothing internal |
| Entry points | `cmd/` | application layer |

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
