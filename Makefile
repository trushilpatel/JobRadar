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
        local local-down prod prod-down \
        up rebuild rebuild-backend rebuild-crawler rebuild-console down logs build-docker \
        k8s-build k8s-build-backend k8s-build-crawler k8s-build-console \
        k8s-up k8s-down k8s-migrate k8s-status k8s-forward k8s-unforward \
        k8s-logs k8s-logs-scheduler k8s-logs-crawler k8s-logs-filter k8s-logs-analyzer k8s-logs-api k8s-logs-console \
        k8s-restart \
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

local: ## Start full local stack in Docker (everything except Caddy) — API :8080, console :3000
	docker compose --profile infra --profile app --profile workers --profile obs up -d --build

local-down: ## Stop local stack
	docker compose --profile infra --profile app --profile workers --profile obs down --remove-orphans

prod: ## Start full production stack (all profiles)
	docker compose --profile infra --profile app --profile workers --profile obs --profile proxy up -d

prod-down: ## Stop production stack
	docker compose --profile infra --profile app --profile workers --profile obs --profile proxy down --remove-orphans

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

docker-up: ## Start infra only in Docker (Kafka + Redis) — for use with make dev
	docker compose --profile infra up -d

docker-logs: ## Tail docker-compose logs
	docker-compose logs -f

# ── Full stack ────────────────────────────────────────────────────────────────

up: ## Start the full stack using existing images (fast — no rebuild)
	docker-compose --profile workers up -d

rebuild: ## Rebuild all images then start full local stack
	docker volume rm jobradar_redpandadata 2>/dev/null || true
	DOCKER_BUILDKIT=1 docker compose --profile infra --profile app --profile obs up -d --build

rebuild-backend: ## Rebuild backend image only (api/scheduler/filter/enricher)
	DOCKER_BUILDKIT=1 docker-compose up -d --build api scheduler filter enricher migrate

rebuild-crawler: ## Rebuild crawler image only
	DOCKER_BUILDKIT=1 docker-compose up -d --build crawler

rebuild-console: ## Rebuild console image only (build happens inside Docker)
	DOCKER_BUILDKIT=1 docker-compose up -d --build console

down: ## Stop all containers (keeps volumes)
	docker compose --profile infra --profile app --profile workers --profile obs --profile proxy down --remove-orphans

logs: ## Tail logs for all services (Ctrl-C to stop)
	docker-compose logs -f

build-docker: ## Build all images without starting (GIT_SHA=$(GIT_SHA) baked into labels)
	DOCKER_BUILDKIT=1 docker-compose build

# ── Kubernetes (local k3d) ────────────────────────────────────────────────────

K3D_CLUSTER ?= jobradar
K8S_NS      ?= jobradar-local

k8s-build-backend: ## Build backend image and import into k3d
	docker build -t jobradar-backend:local -f infra/Dockerfile.backend ./backend
	k3d image import jobradar-backend:local -c $(K3D_CLUSTER)

k8s-build-crawler: ## Build crawler image and import into k3d
	docker build -t jobradar-crawler:local -f infra/Dockerfile.crawler ./backend
	k3d image import jobradar-crawler:local -c $(K3D_CLUSTER)

k8s-build-console: ## Build console image (bakes VITE_ vars from root .env) and import into k3d
	@set -a && . ./.env && set +a && \
	docker build \
		--build-arg VITE_SUPABASE_URL="$$VITE_SUPABASE_URL" \
		--build-arg VITE_SUPABASE_ANON_KEY="$$VITE_SUPABASE_ANON_KEY" \
		--build-arg VITE_APP_DOMAIN="$$VITE_APP_DOMAIN" \
		--build-arg VITE_ALLOWED_EMAIL="$$VITE_ALLOWED_EMAIL" \
		--build-arg GIT_SHA=$(GIT_SHA) \
		-t jobradar-console:local \
		-f ui/console/Dockerfile ./ui/console
	k3d image import jobradar-console:local -c $(K3D_CLUSTER)

k8s-build: k8s-build-backend k8s-build-crawler k8s-build-console ## Build all images and import into k3d

k8s-up: ## Deploy full stack to k3d (infra → migrate → workers → api → console)
	kubectl apply -f k8s/scheduler/namespace.yaml
	kubectl apply \
		-f k8s/kafka/configmap.yaml \
		-f k8s/kafka/service.yaml \
		-f k8s/kafka/statefulset.yaml
	kubectl apply \
		-f k8s/redis/statefulset.yaml \
		-f k8s/redis/service.yaml
	kubectl wait --for=condition=ready pod/kafka-0 -n $(K8S_NS) --timeout=120s
	kubectl wait --for=condition=ready pod/redis-0 -n $(K8S_NS) --timeout=60s
	kubectl delete job jobradar-migrate -n $(K8S_NS) --ignore-not-found
	kubectl apply -f k8s/migrate/job.yaml
	kubectl wait --for=condition=complete job/jobradar-migrate -n $(K8S_NS) --timeout=120s
	kubectl apply \
		-f k8s/scheduler/configmap.yaml  -f k8s/scheduler/deployment.yaml \
		-f k8s/crawler/configmap.yaml    -f k8s/crawler/deployment.yaml \
		-f k8s/filter/configmap.yaml     -f k8s/filter/deployment.yaml \
		-f k8s/analyzer/configmap.yaml   -f k8s/analyzer/deployment.yaml \
		-f k8s/api/configmap.yaml        -f k8s/api/deployment.yaml       -f k8s/api/service.yaml \
		-f k8s/console/configmap.yaml    -f k8s/console/deployment.yaml   -f k8s/console/service.yaml

k8s-down: ## Tear down all k8s workloads (keeps secrets intact)
	kubectl delete deployments,statefulsets,jobs,services,configmaps \
		-l app=jobradar -n $(K8S_NS) --ignore-not-found

k8s-migrate: ## Re-run database migrations (deletes prior job then waits for completion)
	kubectl delete job jobradar-migrate -n $(K8S_NS) --ignore-not-found
	kubectl apply -f k8s/migrate/job.yaml
	kubectl wait --for=condition=complete job/jobradar-migrate -n $(K8S_NS) --timeout=120s
	kubectl logs -l app=jobradar,component=migrate -n $(K8S_NS) --tail=20

k8s-status: ## Show pod statuses and restart counts
	kubectl get pods -n $(K8S_NS) -o wide

k8s-logs: ## Tail logs for all jobradar services
	kubectl logs -f -l app=jobradar -n $(K8S_NS) --max-log-requests=10 --prefix

k8s-logs-scheduler: ## Tail scheduler logs
	kubectl logs -f -l app=jobradar,component=scheduler -n $(K8S_NS)

k8s-logs-crawler: ## Tail crawler logs
	kubectl logs -f -l app=jobradar,component=crawler -n $(K8S_NS)

k8s-logs-filter: ## Tail filter logs
	kubectl logs -f -l app=jobradar,component=filter -n $(K8S_NS)

k8s-logs-analyzer: ## Tail analyzer logs
	kubectl logs -f -l app=jobradar,component=analyzer -n $(K8S_NS)

k8s-logs-api: ## Tail api logs
	kubectl logs -f -l app=jobradar,component=api -n $(K8S_NS)

k8s-logs-console: ## Tail console logs
	kubectl logs -f -l app=jobradar,component=console -n $(K8S_NS)

k8s-forward: ## Port-forward console :3000 and API :8080 to localhost (runs in background)
	kubectl port-forward svc/jobradar-console -n $(K8S_NS) 3000:8080 &
	kubectl port-forward svc/jobradar-api     -n $(K8S_NS) 8080:8080 &
	@echo "Console → http://localhost:3000"
	@echo "API     → http://localhost:8080"

k8s-unforward: ## Stop all kubectl port-forward processes
	@pkill -f "kubectl port-forward" || true

k8s-restart: ## Rolling restart all jobradar deployments
	kubectl rollout restart deployment -l app=jobradar -n $(K8S_NS)

k8s-restart-%: ## Rolling restart one service  e.g. make k8s-restart-api
	kubectl rollout restart deployment/jobradar-$* -n $(K8S_NS)

# ── Setup ─────────────────────────────────────────────────────────────────────

install-hooks: ## Install Husky git hooks (run once after cloning)
	npm install

# ── Help ──────────────────────────────────────────────────────────────────────

help: ## Show this help
	@grep -E '^[a-zA-Z0-9_%/-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-24s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help
