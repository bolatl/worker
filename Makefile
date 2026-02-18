.PHONY: up down test test-e2e test-e2e-fast

# Start services
up:
	docker compose up --build

# Start with test overrides (faster WORK_DURATION for e2e)
up-test:
	docker compose -f docker-compose.yml -f docker-compose.test.yml up --build

down:
	docker compose down

# Unit tests (if any)
test:
	go test ./...

# E2E tests - require services running (run 'make up-test' first)
test-e2e:
	E2E=1 go test -v -timeout 5m ./test/e2e/...

# Quick e2e: up with test config, wait, run tests
test-e2e-fast: up-test
	sleep 15
	E2E=1 go test -v -timeout 5m ./test/e2e/...
