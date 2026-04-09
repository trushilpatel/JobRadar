.PHONY: dev dev-frontend dev-backend \
        build build-frontend build-backend \
        run-api run-scan run-scorer run-enricher \
        migrate lint tsc \
        test test-short test-unit \
        test-api test-workers test-scheduler test-llm \
        test-adapters \
        test-adapter-greenhouse test-adapter-lever test-adapter-ashby \
        docker-up docker-down \
        help

# ── Dev ──────────────────────────────────────────────────────────────────────

dev: ## Start frontend + API in parallel (requires tmux or runs sequentially)
	$(MAKE) -j2 dev-frontend dev-backend

dev-frontend: ## Start Vite dev server
	cd frontend && npm run dev

dev-backend: ## Start API server (hot-reload via go run)
	cd backend && go run ./cmd/api

# ── Build ─────────────────────────────────────────────────────────────────────

build: build-frontend build-backend ## Build everything

build-frontend: ## Build frontend (tsc + vite)
	cd frontend && npm run build

build-backend: ## Compile all Go binaries
	cd backend && go build ./...

# ── Run individual workers ────────────────────────────────────────────────────

run-api: ## Run API server
	cd backend && go run ./cmd/api

run-scan: ## Run scan worker
	cd backend && go run ./cmd/scan

run-scorer: ## Run LLM scorer worker
	cd backend && go run ./cmd/scorer

run-enricher: ## Run enricher worker
	cd backend && go run ./cmd/enricher

run-scheduler: ## Run scan scheduler (enqueues due companies)
	cd backend && go run ./cmd/scheduler

# ── Database ──────────────────────────────────────────────────────────────────

migrate: ## Run database migrations (goose up)
	cd backend && go run ./cmd/migrate

# ── Quality ───────────────────────────────────────────────────────────────────

lint: ## Run ESLint on frontend
	cd frontend && npm run lint

tsc: ## Type-check frontend without emitting
	cd frontend && npx tsc --noEmit

test: ## Run all Go tests (unit + integration, hits real APIs)
	cd backend && go test ./...

test-short: ## Run unit tests only — no real HTTP calls, fast
	cd backend && go test -short ./...

test-unit: test-scheduler test-workers test-api ## Run all unit tests (scheduler + workers + api) — no real connections

test-scheduler: ## Test company_scan_scheduler
	cd backend && go test -v ./cmd/company_scan_scheduler/...

test-workers: ## Test all pipeline workers (crawler, relevance filter, enricher)
	cd backend && go test -v \
		./internal/worker/job_listing_crawler/... \
		./internal/worker/job_listing_relevance_filter/... \
		./internal/worker/job_post_analyzer/...

test-api: ## Test API handlers
	cd backend && go test -v ./internal/api/handler/...

test-llm: ## Test LLM package
	cd backend && go test -v ./internal/llm/...

test-adapters: ## Run all adapter integration tests (Greenhouse + Lever + Ashby hit real APIs)
	cd backend && go test -v -run "Integration" ./internal/ats_fetcher/greenhouse/... ./internal/ats_fetcher/lever/... ./internal/ats_fetcher/ashby/...

test-adapter-greenhouse: ## Run Greenhouse integration tests only
	cd backend && go test -v -run "Integration" ./internal/ats_fetcher/greenhouse/...

test-adapter-lever: ## Run Lever integration tests only
	cd backend && go test -v -run "Integration" ./internal/ats_fetcher/lever/...

test-adapter-ashby: ## Run Ashby integration tests only
	cd backend && go test -v -run "Integration" ./internal/ats_fetcher/ashby/...

vet: ## Run go vet
	cd backend && go vet ./...

# ── Docker ────────────────────────────────────────────────────────────────────

docker-up: ## Start Kafka + Postgres via docker-compose
	docker-compose up -d

docker-down: ## Stop docker-compose services
	docker-compose down

docker-logs: ## Tail docker-compose logs
	docker-compose logs -f

# ── Help ──────────────────────────────────────────────────────────────────────

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help
