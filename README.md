# Worker — Job Queue System

Event-driven job processing with Go, RabbitMQ, and Postgres. An API creates jobs, workers process them asynchronously, and a reaper recovers stuck jobs.

## Features

- **API** — Create jobs via REST, poll status
- **Worker** — Consume from RabbitMQ, process jobs, persist results
- **Reaper** — Recover jobs stuck in processing (worker crash)
- **Crash safety** — Manual acks; jobs reach terminal state even if workers die
- **Poison job handling** — Bounded retries, max 5 attempts
- **Idempotency** — Safe under duplicate message delivery

---

## Quick Start

### Run with Docker Compose

```bash
docker compose up -d --build
```
or (to view logs)
```bash
docker compose up --build
```

Then:

```bash
# Create a job
curl -X POST http://localhost:8080/jobs \
  -H "Content-Type: application/json" \
  -d '{"type":"hash","payload":{"hello":"world"}}'

# Response: {"job_id":"..."}
```

---

## Project Structure

```
worker/
├── cmd/
│   ├── api/main.go          # HTTP API entrypoint
│   └── worker/main.go       # Worker + reaper entrypoint
├── internal/
│   ├── api/
│   │   ├── handlers.go      # HTTP handlers (CreateJob, GetJob, Healthz)
│   │   └── router.go        # Routes
│   ├── config/
│   │   └── config.go        # Env-based config
│   ├── db/
│   │   ├── db.go            # Postgres connection
│   │   ├── migrate.go       # Embedded migrations
│   │   └── migrations/
│   │       ├── 001_init.sql # Jobs table
│   │       └── 002_triggers.sql
│   ├── jobs/
│   │   ├── model.go         # Job struct, status constants
│   │   ├── repository.go    # Postgres persistence
│   │   └── service.go       # Create job + publish
│   ├── queue/
│   │   ├── messages.go      # JobMessage type
│   │   └── rabbitmq.go      # Connect, publish, consume
│   └── worker/
│       ├── consumer.go      # Consume messages, ACK/NACK
│       ├── processor.go     # Process job (hash/fail logic)
│       └── reaper.go        # Requeue stuck jobs
├── test/
│   └── e2e/
│       ├── client.go        # API client
│       ├── *_test.go        # E2E tests
│       └── README.md
├── docker/
│   ├── api.Dockerfile
│   └── worker.Dockerfile
├── docker-compose.yml
├── docker-compose.test.yml  # Faster config for tests
├── DESIGN.md                # Design write-up
├── Makefile
└── go.mod
```

---

## How to Run

### Option 1: Docker Compose (recommended)

```bash
# Start all services (detached)
docker compose up -d --build

# Or foreground (see logs)
docker compose up --build
```

Services:

| Service   | Port  | Description                    |
|-----------|-------|--------------------------------|
| API       | 8080  | HTTP server                    |
| Postgres  | 5432  | Database                       |
| RabbitMQ  | 5672  | AMQP                           |
| RabbitMQ  | 15672 | Management UI                  |

### Option 2: Local development

Run infrastructure only:

```bash
docker compose up -d postgres rabbitmq
```

Then run API and worker locally:

```bash
# Terminal 1 — API
export DATABASE_URL="postgres://postgres:postgres@localhost:5432/jobs?sslmode=disable"
export RABBIT_URL="amqp://guest:guest@localhost:5672/"
go run ./cmd/api

# Terminal 2 — Worker
go run ./cmd/worker
```

### Option 3: Makefile

```bash
make up          # Start services
make down        # Stop
make test-e2e    # Run E2E tests (services must be up)
```

---

## API

### POST /jobs

Create a job.

**Request:**
```json
{
  "type": "hash",
  "payload": { "key": "value" }
}
```

- `type` (required): Job type. `hash` = compute SHA256; `fail` = always fail (for testing poison jobs).
- `payload` (optional): JSON object. If `{"fail": true}`, job fails (for testing).

**Response:** `202 Accepted`
```json
{
  "job_id": "uuid"
}
```

### GET /jobs/:id

Get job status and result.

**Response:** `200 OK`
```json
{
  "id": "uuid",
  "type": "hash",
  "status": "done",
  "attempts": 0,
  "max_attempts": 5,
  "result": { "sha256": "..." },
  "created_at": "...",
  "updated_at": "..."
}
```

- `status`: `queued` | `processing` | `done` | `failed`
- `result`: Present when done.
- `last_error`: Present when failed.

### GET /healthz

Liveness probe. Returns `200 OK` with `{"ok": true}`.

---

## Sample cURL Commands

```bash
# Create a normal job (will succeed)
curl -X POST http://localhost:8080/jobs \
  -H "Content-Type: application/json" \
  -d '{"type":"hash","payload":{"test": true}}'

# Create a poison job (will fail after 5 attempts)
curl -X POST http://localhost:8080/jobs \
  -H "Content-Type: application/json" \
  -d '{"type":"fail","payload":{}}'

# Poll job status (replace ID with job_id from above)
curl http://localhost:8080/jobs/<job_id>

# Health check
curl http://localhost:8080/healthz
```

---

## Configuration

Environment variables:

| Variable              | Default                        | Description                    |
|-----------------------|--------------------------------|--------------------------------|
| `HTTP_PORT`           | `8080`                         | API port                       |
| `DATABASE_URL`        | (required)                     | Postgres connection string     |
| `RABBIT_URL`          | (required)                     | RabbitMQ URL                   |
| `QUEUE_NAME`          | `jobs`                         | RabbitMQ queue name            |
| `MAX_ATTEMPTS`        | `5`                            | Max retries before failed      |
| `PREFETCH`            | `1`                            | Unacked messages per worker    |
| `PROCESSING_TIMEOUT`  | `60s`                          | Reaper: stuck job threshold    |
| `WORK_DURATION`       | `15s`                          | Simulated work time per job    |

---

## Testing

### E2E tests

Requires services running. Use test override for faster runs:

```bash
docker compose -f docker-compose.yml -f docker-compose.test.yml up -d --build
sleep 15
E2E=1 go test -v -count=1 ./test/e2e/...
```

Or:

```bash
make up-test
sleep 15
make test-e2e
```

See [test/e2e/README.md](test/e2e/README.md) for details.

### Test cases

| Test                           | Description                                  |
|--------------------------------|----------------------------------------------|
| `TestHappyPath`                | Create job → becomes `done`                  |
| `TestPoisonJob`                | Poison job → reaches `failed`                |
| `TestTwoWorkers`               | 2 workers share load                         |
| `TestWorkerCrash`              | Kill worker mid-job → job recovers           |
| `TestDuplicateDeliveryIdempotency` | Duplicate message → no corruption        |

---

## Design

See [DESIGN.md](DESIGN.md) for:

- Delivery guarantee (at-least-once)
- When we ACK messages
- Bounded retries and poison jobs
- Idempotency approach
- Worker crash recovery

---

## Tech Stack

- **Go 1.25**
- **PostgreSQL 16** — Job state
- **RabbitMQ 3** — Message queue
- **pgx** — Postgres driver
- **amqp091-go** — RabbitMQ client
