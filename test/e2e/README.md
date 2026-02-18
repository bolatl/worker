# E2E Tests

End-to-end tests for the tech-task scenarios. Require the full stack (API, workers, Postgres, RabbitMQ) to be running.

## Prerequisites

- Docker and Docker Compose
- Go 1.21+

## Quick Start

```bash
# 1. Start services (use test override for faster runs)
docker compose -f docker-compose.yml -f docker-compose.test.yml up -d --build

# 2. Wait for services to be ready
sleep 10

# 3. Run e2e tests
E2E=1 go test -v ./test/e2e/...
```

## Test Cases

| Test | Description |
|------|-------------|
| `TestHappyPath` | Create job → becomes `done` |
| `TestPoisonJob` | Submit fail job → reaches `failed`, no infinite loop |
| `TestTwoWorkers` | 2 workers + 8 jobs → both workers do work |
| `TestWorkerCrash` | Kill worker mid-job → restart → job completes or fails cleanly |
| `TestDuplicateDeliveryIdempotency` | Duplicate message after job done → state unchanged |

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `E2E` | (empty) | Set to `1` to run e2e tests (skipped otherwise) |
| `API_URL` | `http://localhost:8080` | Base URL of the API |
| `RABBIT_URL` | `amqp://guest:guest@localhost:5672/` | RabbitMQ URL (for idempotency test) |
| `QUEUE_NAME` | `jobs` | Queue name (for idempotency test) |

## Timing (with docker-compose.test.yml)

- **Happy path**: ~5s
- **Poison job**: ~15s (5 attempts × 2s)
- **Two workers**: ~15s (8 jobs, 2 workers, 2s each)
- **Worker crash**: ~90s (reaper timeout 60s + processing 2s)

## Without test override (default WORK_DURATION=15s)

Tests are slower but work the same. Happy path ~45s, poison ~2min.
