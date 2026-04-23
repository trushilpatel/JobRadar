GIT_SHA ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo dev)
export GIT_SHA

.PHONY: dev dev-console dev-backend \
        build build-console build-backend clean-console \
        run-api run-crawler run-filter run-enricher run-scheduler \
        migrate scan-google db-truncate lint lint-go tsc \
        test test-short test-unit \
        test-api test-workers test-scheduler test-llm \
        fmt \
        test-adapters \
        test-adapter-greenhouse test-adapter-lever test-adapter-ashby \
        docker-up \
        up rebuild rebuild-backend rebuild-crawler rebuild-console down logs build-docker \
        install-hooks \
        help

# ── Dev ──────────────────────────────────────────────────────────────────────

dev: ## Start infra in Docker, then run console + API on host
	$(MAKE) docker-up
	cd backend && go run ./apps/migrate
	$(MAKE) -j2 dev-console dev-backend

dev-console: ## Start Vite dev server (ui/console)
	cd ui/console && npm run dev

dev-backend: ## Start API server (hot-reload via go run)
	cd backend && go run ./apps/api

# ── Build ─────────────────────────────────────────────────────────────────────

build: build-console build-backend ## Build everything

clean-console: ## Remove console build artifacts from host
	rm -rf ui/console/dist

build-console: clean-console ## Build console on host (local preview only — Docker builds its own via Dockerfile)
	cd ui/console && npm run build

build-backend: ## Compile all Go binaries
	cd backend && go build ./...

# ── Run individual workers ────────────────────────────────────────────────────

run-api: ## Run API server
	cd backend && go run ./apps/api

run-crawler: ## Run job listing crawler worker
	cd backend && go run ./apps/crawler

run-filter: ## Run LLM relevance filter worker
	cd backend && go run ./apps/filter

run-enricher: ## Run job post analyzer/enricher worker
	cd backend && go run ./apps/analyzer

run-scheduler: ## Run scan scheduler (enqueues due companies)
	cd backend && go run ./apps/scheduler

# ── Database ──────────────────────────────────────────────────────────────────

migrate: ## Run database migrations (goose up)
	cd backend && go run ./apps/migrate

scan-google: ## Manually trigger a Google careers scan (bypasses cooldown)
	curl -s -X POST http://localhost:8080/companies/google/scan | jq .

db-truncate: ## Truncate all tables except companies — requires psql + DB_URL env var
	psql "$$DB_URL" -c \
		"TRUNCATE TABLE job_pipeline_ledger, company_scan_queue, company_scan_log, job_application_tracker, job_dismissal_log, job_analytics, job_feed CASCADE;"

# ── Quality ───────────────────────────────────────────────────────────────────

lint: ## Run ESLint on console
	cd ui/console && npm run lint

lint-go: ## Run golangci-lint on backend (matches CI config)
	@command -v golangci-lint >/dev/null 2>&1 || { \
		echo "golangci-lint not found — install via: brew install golangci-lint"; \
		exit 1; \
	}
	cd backend && golangci-lint run --config ../.github/.golangci.yml --timeout 5m

fmt: ## Format all Go files with gofmt
	cd backend && go fmt ./...

lint-go-fast: ## Lint only files changed vs main branch
	@command -v golangci-lint >/dev/null 2>&1 || { \
		echo "golangci-lint not found — install via: brew install golangci-lint"; \
		exit 1; \
	}
	cd backend && golangci-lint run --config ../.github/.golangci.yml --new-from-rev=main --timeout 2m

tsc: ## Type-check console without emitting
	cd ui/console && npx tsc --noEmit

test: ## Run all Go tests (unit + integration, hits real APIs)
	cd backend && go test ./...

test-short: ## Run unit tests only — no real HTTP calls, fast
	cd backend && go test -short ./...

test-unit: test-scheduler test-workers test-api ## Run all unit tests (scheduler + workers + api) — no real connections

test-scheduler: ## Test company_scan_scheduler
	cd backend && go test -v ./apps/scheduler/...

test-workers: ## Test all pipeline workers (crawler, filter, enricher)
	cd backend && go test -v \
		./apps/crawler/... \
		./apps/crawler/... \
		./apps/filter/... \
		./apps/analyzer/...

test-api: ## Test API handlers and services
	cd backend && go test -v ./apps/api/... ./service/...

test-llm: ## Test LLM package
	cd backend && go test -v ./service/llm/...

test-adapters: ## Run all adapter integration tests (Greenhouse + Lever + Ashby hit real APIs)
	cd backend && go test -v -run "Integration" ./apps/crawler/ats/greenhouse/... ./apps/crawler/ats/lever/... ./apps/crawler/ats/ashby/...

test-adapter-greenhouse: ## Run Greenhouse integration tests only
	cd backend && go test -v -run "Integration" ./apps/crawler/ats/greenhouse/...

test-adapter-lever: ## Run Lever integration tests only
	cd backend && go test -v -run "Integration" ./apps/crawler/ats/lever/...

test-adapter-ashby: ## Run Ashby integration tests only
	cd backend && go test -v -run "Integration" ./apps/crawler/ats/ashby/...

vet: ## Run go vet
	cd backend && go vet ./...

# ── Docker (infra only — for local dev) ──────────────────────────────────────

docker-up: ## Start Kafka + Redis for local dev (DB is Supabase — no local postgres)
	docker-compose up -d kafka kafka-init redis

docker-logs: ## Tail docker-compose logs
	docker-compose logs -f

# ── Full stack ────────────────────────────────────────────────────────────────

up: ## Start the full stack using existing images (fast — no rebuild)
	docker-compose --profile workers up -d

rebuild: ## Rebuild all images then start (use after code changes)
	docker volume rm jobradar_kafkadata 2>/dev/null || true
	DOCKER_BUILDKIT=1 docker-compose --profile workers up -d --build

rebuild-backend: ## Rebuild backend image only (api/scheduler/filter/enricher)
	DOCKER_BUILDKIT=1 docker-compose up -d --build api scheduler filter enricher migrate

rebuild-crawler: ## Rebuild crawler image only
	DOCKER_BUILDKIT=1 docker-compose up -d --build crawler

rebuild-console: ## Rebuild console image only (build happens inside Docker)
	DOCKER_BUILDKIT=1 docker-compose up -d --build console

down: ## Stop and remove all containers (keeps volumes)
	docker-compose --profile workers down --remove-orphans

logs: ## Tail logs for all services (Ctrl-C to stop)
	docker-compose logs -f

build-docker: ## Build all images without starting (GIT_SHA=$(GIT_SHA) baked into labels)
	DOCKER_BUILDKIT=1 docker-compose build

# ── Setup ─────────────────────────────────────────────────────────────────────

install-hooks: ## Install Husky git hooks (run once after cloning)
	npm install

# ── Help ──────────────────────────────────────────────────────────────────────

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help
