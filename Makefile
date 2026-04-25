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
        k8s-obs-up k8s-obs-down k8s-obs-forward \
        k8s-logs k8s-logs-scheduler k8s-logs-crawler k8s-logs-filter k8s-logs-analyzer k8s-logs-api k8s-logs-console \
        k8s-restart \
        k8s-preflight k8s-nuke \
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
K8S_CACHE   ?= .k8s-build-cache

k8s-build-backend: ## Build backend image and import into k3d
	@mkdir -p $(K8S_CACHE)
	@DIGEST=$$(find backend -type f \( -name "*.go" -o -name "go.mod" -o -name "go.sum" \) ! -path "*/apps/crawler/*" | sort | xargs shasum 2>/dev/null | shasum | cut -c1-16); \
	CACHED=$$(cat $(K8S_CACHE)/backend 2>/dev/null || echo ""); \
	if [ "$$DIGEST" = "$$CACHED" ]; then \
		echo "  [skip] backend image is up to date"; \
	else \
		docker build -t jobradar-backend:local -f infra/Dockerfile.backend ./backend && \
		k3d image import jobradar-backend:local -c $(K3D_CLUSTER) --mode=direct && \
		printf '%s' "$$DIGEST" > $(K8S_CACHE)/backend; \
	fi

k8s-build-crawler: ## Build crawler image and import into k3d
	@mkdir -p $(K8S_CACHE)
	@DIGEST=$$(find backend -type f \( -name "*.go" -o -name "go.mod" -o -name "go.sum" \) | sort | xargs shasum 2>/dev/null | shasum | cut -c1-16); \
	CACHED=$$(cat $(K8S_CACHE)/crawler 2>/dev/null || echo ""); \
	if [ "$$DIGEST" = "$$CACHED" ]; then \
		echo "  [skip] crawler image is up to date"; \
	else \
		docker build -t jobradar-crawler:local -f infra/Dockerfile.crawler ./backend && \
		k3d image import jobradar-crawler:local -c $(K3D_CLUSTER) --mode=direct && \
		printf '%s' "$$DIGEST" > $(K8S_CACHE)/crawler; \
	fi

k8s-build-console: ## Build console image (bakes VITE_ vars from root .env) and import into k3d
	@mkdir -p $(K8S_CACHE)
	@DIGEST=$$(find ui/console/src ui/console/public ui/console/index.html ui/console/package-lock.json ui/console/Dockerfile -type f 2>/dev/null | sort | xargs shasum 2>/dev/null | shasum | cut -c1-16); \
	CACHED=$$(cat $(K8S_CACHE)/console 2>/dev/null || echo ""); \
	if [ "$$DIGEST" = "$$CACHED" ]; then \
		echo "  [skip] console image is up to date"; \
	else \
		set -a && . ./.env && set +a && \
		docker build \
			--build-arg VITE_SUPABASE_URL="$$VITE_SUPABASE_URL" \
			--build-arg VITE_SUPABASE_ANON_KEY="$$VITE_SUPABASE_ANON_KEY" \
			--build-arg VITE_APP_DOMAIN="$$VITE_APP_DOMAIN" \
			--build-arg VITE_ALLOWED_EMAIL="$$VITE_ALLOWED_EMAIL" \
			--build-arg GIT_SHA=$(GIT_SHA) \
			-t jobradar-console:local \
			-f ui/console/Dockerfile ./ui/console && \
		k3d image import jobradar-console:local -c $(K3D_CLUSTER) --mode=direct && \
		printf '%s' "$$DIGEST" > $(K8S_CACHE)/console; \
	fi

k8s-build: ## Build all images in parallel and import into k3d
	$(MAKE) -j3 k8s-build-backend k8s-build-crawler k8s-build-console

k8s-preflight: ## Verify kubectl context is k3d-jobradar before any deploy
	@CTX=$$(kubectl config current-context 2>/dev/null); \
	if [ "$$CTX" != "k3d-jobradar" ]; then \
		echo "ERROR: kubectl context is '$$CTX', expected 'k3d-jobradar'"; \
		echo "Run: kubectl config use-context k3d-jobradar"; \
		exit 1; \
	fi
	@echo "Context OK: k3d-jobradar"
	@kubectl get nodes --no-headers 2>/dev/null | grep -q "Ready" || (echo "ERROR: cluster not reachable"; exit 1)
	@echo "Cluster OK"
	@for svc in api scheduler crawler filter enricher migrate; do \
		kubectl get secret jobradar-$$svc-secret -n $(K8S_NS) >/dev/null 2>&1 || \
		echo "WARN: secret jobradar-$$svc-secret missing — pods will fail with CreateContainerConfigError"; \
	done

k8s-up: k8s-preflight ## Deploy app chart via Helm — reads DEV_TOKEN from backend/.env if set
	helm dependency update k8s/helm/jobradar 2>/dev/null || true
	@DEV_TOKEN=$$(grep -E '^DEV_TOKEN=' backend/.env 2>/dev/null | cut -d= -f2 | tr -d '[:space:]'); \
	if [ -n "$$DEV_TOKEN" ]; then \
		echo "DEV_TOKEN loaded from backend/.env"; \
		helm upgrade --install jobradar k8s/helm/jobradar -n $(K8S_NS) --create-namespace --set api.devToken="$$DEV_TOKEN"; \
	else \
		helm upgrade --install jobradar k8s/helm/jobradar -n $(K8S_NS) --create-namespace; \
	fi
	kubectl rollout status statefulset/kafka -n $(K8S_NS) --timeout=180s
	kubectl rollout status statefulset/redis -n $(K8S_NS) --timeout=60s
	kubectl get pods -n $(K8S_NS)

k8s-nuke: ## Fully wipe namespace + PVCs — use when storage class or StatefulSet spec changed
	helm uninstall jobradar -n $(K8S_NS) --ignore-not-found
	kubectl delete pvc --all -n $(K8S_NS) --ignore-not-found 2>/dev/null || true
	kubectl delete namespace $(K8S_NS) --ignore-not-found
	@echo "Namespace wiped. Run make k8s-up to redeploy."

k8s-down: ## Uninstall app Helm release (secrets and PVCs preserved)
	helm uninstall jobradar -n $(K8S_NS) --ignore-not-found

k8s-obs-up: ## Deploy observability chart (Prometheus + Grafana + Loki + Jaeger + audit-sync)
	helm dependency update k8s/helm/observability
	helm upgrade --install obs k8s/helm/observability -n $(K8S_NS) --create-namespace

k8s-obs-down: ## Uninstall observability Helm release
	helm uninstall obs -n $(K8S_NS) --ignore-not-found

k8s-migrate: ## Re-run database migrations via Helm hook (deletes prior job first)
	kubectl delete job jobradar-migrate -n $(K8S_NS) --ignore-not-found
	helm upgrade jobradar k8s/helm/jobradar -n $(K8S_NS)
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

k8s-forward: ## Port-forward console :3000 and API :8080 (when cluster has no NodePort mappings)
	kubectl port-forward svc/jobradar-console -n $(K8S_NS) 3000:8080 &
	kubectl port-forward svc/jobradar-api     -n $(K8S_NS) 8080:8080 &
	@echo "Console → http://localhost:3000"
	@echo "API     → http://localhost:8080"

k8s-obs-forward: ## Port-forward Grafana :3001, Jaeger :16686, Prometheus :9090
	kubectl port-forward svc/obs-grafana                              -n $(K8S_NS) 3001:80    &
	kubectl port-forward svc/obs-jaeger                               -n $(K8S_NS) 16686:16686 &
	kubectl port-forward svc/obs-kube-prometheus-stack-prometheus     -n $(K8S_NS) 9090:9090   &
	@echo "Grafana    → http://localhost:3001  (admin / admin)"
	@echo "Jaeger     → http://localhost:16686"
	@echo "Prometheus → http://localhost:9090"

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
