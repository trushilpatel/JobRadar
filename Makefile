GIT_SHA ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo dev)
export GIT_SHA

.PHONY: dev dev-console dev-backend dev-stop dev-check dev-open \
        build build-console build-backend clean-console \
        run-crawler run-filter run-enricher run-scheduler \
        migrate scan-google db-truncate lint lint-go tsc \
        test test-short test-adapters test-watch test-watch-backend \
        coverage coverage-frontend \
        fmt \
        k8s-build k8s-build-backend k8s-build-crawler k8s-build-orchestrator k8s-build-crawler-node k8s-build-console \
        k8s-up k8s-down k8s-migrate k8s-status k8s-forward k8s-unforward \
        k8s-obs-up k8s-obs-down \
        k8s-logs k8s-logs-nats k8s-logs-filter k8s-logs-enricher k8s-logs-api k8s-logs-console k8s-logs-crawler k8s-logs-scheduler k8s-logs-orchestrator k8s-logs-crawler-node \
        k8s-restart k8s-restart-orchestrator k8s-restart-crawler-node \
        k8s-bootstrap k8s-preflight k8s-nuke \
        install-hooks \
        help

# ── Dev (host) ────────────────────────────────────────────────────────────────
# Local dev: API on :8080 (air hot-reload) + Vite HMR on :5173.
# Secrets come from Doppler dev — no .env files, no Docker, no k3d needed.
# API_PROXY_TARGET=http://localhost:8080 is set in Doppler dev config.
# For full pipeline workers (crawler/filter/enricher) use make k8s-up instead.

dev: dev-stop ## Migrate + start API (air hot-reload on :8080) + Vite HMR (:5173)
	doppler run -- sh -c 'cd backend && go run ./apps/migrate'
	$(MAKE) -j2 dev-console dev-backend

dev-console: ## Start Vite dev server on :5173 (HMR, proxies /api → :8080)
	cd frontend && doppler run -- npm run dev

dev-backend: ## Start API on :8080 with air hot-reload (recompiles on .go save)
	cd backend && doppler run -- air

dev-stop: ## Kill local dev processes (API :8080 + Vite :5173)
	@-lsof -ti:8080 | xargs kill 2>/dev/null && echo "  killed API  (:8080)" || true
	@-lsof -ti:5173 | xargs kill 2>/dev/null && echo "  killed Vite (:5173)" || true

dev-check: ## Check whether local dev services are up
	@printf "  API  :8080  "; curl -sf http://localhost:9100/health > /dev/null && echo "UP" || echo "DOWN (make dev-backend)"
	@printf "  Vite :5173  "; curl -sf http://localhost:5173/  > /dev/null && echo "UP" || echo "DOWN (make dev-console)"

dev-open: ## Open app in browser at localhost:5173
	open http://localhost:5173/

# ── Build ─────────────────────────────────────────────────────────────────────

build: build-console build-backend ## Build everything

clean-console: ## Remove console build artifacts from host
	rm -rf frontend/dist

build-console: clean-console ## Build console on host (local preview only — Docker builds its own via Dockerfile)
	cd frontend && npm run build

build-backend: ## Compile all Go binaries
	cd backend && go build ./...

# ── Run individual workers ────────────────────────────────────────────────────

run-crawler: ## Run job listing crawler worker
	cd backend && doppler run -- go run ./apps/crawler

run-filter: ## Run LLM relevance filter worker
	cd backend && doppler run -- go run ./apps/filter

run-enricher: ## Run job post analyzer/enricher worker
	cd backend && doppler run -- go run ./apps/analyzer

run-scheduler: ## Run scan scheduler (enqueues due companies)
	cd backend && doppler run -- go run ./apps/scheduler

# ── Database ──────────────────────────────────────────────────────────────────

migrate: ## Run database migrations (goose up)
	cd backend && doppler run -- go run ./apps/migrate

scan-google: ## Manually trigger a Google careers scan (bypasses cooldown)
	curl -s -X POST http://localhost:8080/companies/google/scan | jq .

db-truncate: ## Truncate all tables except companies — requires psql + DB_URL env var
	psql "$$DB_URL" -c \
		"TRUNCATE TABLE job_pipeline_ledger, company_scan_queue, company_scan_log, job_application_tracker, job_dismissal_log, job_analytics, job_feed CASCADE;"

# ── Quality ───────────────────────────────────────────────────────────────────

lint: ## Run ESLint on console
	cd frontend && npm run lint

lint-go: ## Run golangci-lint on backend (matches CI config)
	@command -v golangci-lint >/dev/null 2>&1 || { \
		echo "golangci-lint not found — install via: brew install golangci-lint"; \
		exit 1; \
	}
	cd backend && golangci-lint run --config ../.github/.golangci.yml --timeout 5m

fmt: ## Format all Go files with gofmt
	cd backend && go fmt ./...

tsc: ## Type-check console without emitting
	cd frontend && npx tsc --noEmit

test: ## Run all Go tests
	cd backend && go test ./...

test-short: ## Run unit tests only — no real HTTP calls, fast
	cd backend && go test -short ./...

test-adapters: ## Run all adapter integration tests (Greenhouse + Lever + Ashby hit real APIs)
	cd backend && go test -v -run "Integration" ./apps/crawler/ats/greenhouse/... ./apps/crawler/ats/lever/... ./apps/crawler/ats/ashby/...

test-watch: ## Run frontend tests in watch mode (vitest)
	cd frontend && npx vitest --watch

test-watch-backend: ## Re-run backend tests on every .go save (requires entr: brew install entr)
	@command -v entr >/dev/null 2>&1 || { echo "install entr: brew install entr"; exit 1; }
	find backend -name '*.go' | entr -c sh -c 'cd backend && go test -short -count=1 ./...'

coverage: ## Run backend unit tests with coverage and enforce 95% per-package gate
	cd backend && go test -short -count=1 -coverpkg=./... -coverprofile=/tmp/cov.out ./... && ../scripts/check-coverage.sh /tmp/cov.out 95

coverage-frontend: ## Run frontend tests with coverage and enforce 90% statement gate
	cd frontend && npm run test:coverage

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
	@DIGEST=$$(find backend -type f \( -name "*.go" -o -name "go.mod" -o -name "go.sum" \) \( -path "*/apps/crawler/*" -o -path "*/common/*" \) | sort | xargs shasum 2>/dev/null | shasum | cut -c1-16); \
	CACHED=$$(cat $(K8S_CACHE)/crawler 2>/dev/null || echo ""); \
	if [ "$$DIGEST" = "$$CACHED" ]; then \
		echo "  [skip] crawler image is up to date"; \
	else \
		docker build -t jobradar-crawler:local --build-arg GIT_SHA=$(GIT_SHA) -f infra/Dockerfile.crawler ./backend && \
		k3d image import jobradar-crawler:local -c $(K3D_CLUSTER) --mode=direct && \
		printf '%s' "$$DIGEST" > $(K8S_CACHE)/crawler; \
	fi

k8s-build-console: ## Build console image (bakes VITE_ vars from Doppler) and import into k3d
	@mkdir -p $(K8S_CACHE)
	@DIGEST=$$(find frontend/src frontend/public frontend/index.html frontend/package-lock.json frontend/Dockerfile -type f 2>/dev/null | sort | xargs shasum 2>/dev/null | shasum | cut -c1-16); \
	CACHED=$$(cat $(K8S_CACHE)/console 2>/dev/null || echo ""); \
	if [ "$$DIGEST" = "$$CACHED" ]; then \
		echo "  [skip] console image is up to date"; \
	else \
		doppler run -- sh -c ' \
			docker build \
				--build-arg VITE_SUPABASE_URL="$$VITE_SUPABASE_URL" \
				--build-arg VITE_SUPABASE_ANON_KEY="$$VITE_SUPABASE_ANON_KEY" \
				--build-arg GIT_SHA=$(GIT_SHA) \
				-t jobradar-console:local \
				-f frontend/Dockerfile ./frontend' && \
		k3d image import jobradar-console:local -c $(K3D_CLUSTER) --mode=direct && \
		printf '%s' "$$DIGEST" > $(K8S_CACHE)/console; \
	fi

k8s-build-orchestrator: ## Build orchestrator image and import into k3d
	@mkdir -p $(K8S_CACHE)
	@DIGEST=$$(find backend -type f \( -name "*.go" -o -name "go.mod" -o -name "go.sum" \) \( -path "*/apps/orchestrator/*" -o -path "*/service/crawlernode/*" -o -path "*/common/*" \) | sort | xargs shasum 2>/dev/null | shasum | cut -c1-16); \
	CACHED=$$(cat $(K8S_CACHE)/orchestrator 2>/dev/null || echo ""); \
	if [ "$$DIGEST" = "$$CACHED" ]; then \
		echo "  [skip] orchestrator image is up to date"; \
	else \
		docker build -t jobradar-orchestrator:local --build-arg GIT_SHA=$(GIT_SHA) -f infra/Dockerfile.orchestrator ./backend && \
		k3d image import jobradar-orchestrator:local -c $(K3D_CLUSTER) --mode=direct && \
		printf '%s' "$$DIGEST" > $(K8S_CACHE)/orchestrator; \
	fi

k8s-build-crawler-node: ## Build crawler-node image and import into k3d
	@mkdir -p $(K8S_CACHE)
	@DIGEST=$$(find backend -type f \( -name "*.go" -o -name "go.mod" -o -name "go.sum" \) \( -path "*/apps/crawler-node/*" -o -path "*/apps/crawler/ats/*" -o -path "*/common/*" \) | sort | xargs shasum 2>/dev/null | shasum | cut -c1-16); \
	CACHED=$$(cat $(K8S_CACHE)/crawler-node 2>/dev/null || echo ""); \
	if [ "$$DIGEST" = "$$CACHED" ]; then \
		echo "  [skip] crawler-node image is up to date"; \
	else \
		docker build -t jobradar-crawler-node:local --build-arg GIT_SHA=$(GIT_SHA) -f infra/Dockerfile.crawler-node ./backend && \
		k3d image import jobradar-crawler-node:local -c $(K3D_CLUSTER) --mode=direct && \
		printf '%s' "$$DIGEST" > $(K8S_CACHE)/crawler-node; \
	fi

k8s-build: ## Build all images in parallel and import into k3d
	$(MAKE) -j5 k8s-build-backend k8s-build-crawler k8s-build-orchestrator k8s-build-crawler-node k8s-build-console

k8s-bootstrap: ## Fresh-machine setup: validate all files/env/creds, create cluster + all k8s secrets
	@bash scripts/k8s-bootstrap.sh

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
	@for svc in api filter enricher migrate; do \
		kubectl get secret jobradar-$$svc-secret -n $(K8S_NS) >/dev/null 2>&1 || \
		echo "WARN: secret jobradar-$$svc-secret missing — pods will fail with CreateContainerConfigError"; \
	done

k8s-up: k8s-preflight ## Deploy app chart via Helm — requires values.local.yaml; reads DEV_TOKEN from Doppler
	@if [ ! -f k8s/helm/jobradar/values.local.yaml ]; then \
		echo "ERROR: k8s/helm/jobradar/values.local.yaml not found"; \
		echo "Copy k8s/helm/jobradar/values.local.yaml.example and fill in supabaseProjectUrl (the chart aborts on an empty value)."; \
		echo "In a git worktree this file is absent (gitignored, lives only in the main checkout) — symlink it:"; \
		echo "  ln -s \$$(git rev-parse --git-common-dir | sed 's,/.git,,')/k8s/helm/jobradar/values.local.yaml k8s/helm/jobradar/values.local.yaml"; \
		exit 1; \
	fi
	helm dependency update k8s/helm/jobradar 2>/dev/null || true
	@DEV_TOKEN=$$(doppler secrets get DEV_TOKEN --plain --project job-radar --config dev 2>/dev/null || true); \
	if [ -n "$$DEV_TOKEN" ]; then \
		echo "DEV_TOKEN loaded from Doppler"; \
		helm upgrade --install jobradar k8s/helm/jobradar -n $(K8S_NS) --create-namespace -f k8s/helm/jobradar/values.local.yaml --set api.devToken="$$DEV_TOKEN"; \
	else \
		helm upgrade --install jobradar k8s/helm/jobradar -n $(K8S_NS) --create-namespace -f k8s/helm/jobradar/values.local.yaml; \
	fi
	kubectl get pods -n $(K8S_NS)

k8s-redeploy: k8s-build k8s-up ## Rebuild images, Helm-sync, then roll all app pods so same-tag :local images are actually picked up
	@echo "Rolling app deployments to pick up freshly imported :local images (same-tag rebuilds don't trigger a Helm rollout)..."
	kubectl rollout restart deployment -l app=jobradar,component!=nats -n $(K8S_NS)
	@for d in $$(kubectl get deployment -l app=jobradar,component!=nats -n $(K8S_NS) -o name); do \
		kubectl rollout status $$d -n $(K8S_NS) --timeout=180s; \
	done

k8s-nuke: ## Fully wipe namespace + PVCs — use when storage class or StatefulSet spec changed
	helm uninstall jobradar -n $(K8S_NS) --ignore-not-found
	kubectl delete pvc --all -n $(K8S_NS) --ignore-not-found 2>/dev/null || true
	kubectl delete namespace $(K8S_NS) --ignore-not-found
	@echo "Namespace wiped. Run make k8s-up to redeploy."

k8s-down: ## Uninstall app Helm release (secrets and PVCs preserved)
	helm uninstall jobradar -n $(K8S_NS) --ignore-not-found

k8s-obs-up: ## Deploy observability chart (Grafana Alloy + audit-sync, ships to Grafana Cloud)
	helm dependency update k8s/helm/observability
	@LOCAL_VALUES=$$([ -f k8s/helm/observability/values.local.yaml ] && echo "-f k8s/helm/observability/values.local.yaml" || echo ""); \
	if [ -z "$$LOCAL_VALUES" ]; then \
		echo "ERROR: k8s/helm/observability/values.local.yaml not found"; \
		echo "Copy k8s/helm/observability/values.local.yaml.example and fill in Grafana Cloud credentials"; \
		exit 1; \
	fi; \
	helm upgrade --install obs k8s/helm/observability -n $(K8S_NS) --create-namespace $$LOCAL_VALUES

k8s-obs-down: ## Uninstall observability Helm release
	helm uninstall obs -n $(K8S_NS) --ignore-not-found

k8s-migrate: ## Re-run database migrations via Helm hook (deletes prior job first)
	kubectl delete job jobradar-migrate -n $(K8S_NS) --ignore-not-found
	@LOCAL_VALUES=$$([ -f k8s/helm/jobradar/values.local.yaml ] && echo "-f k8s/helm/jobradar/values.local.yaml" || echo ""); \
	helm upgrade jobradar k8s/helm/jobradar -n $(K8S_NS) $$LOCAL_VALUES
	kubectl wait --for=condition=complete job/jobradar-migrate -n $(K8S_NS) --timeout=120s
	kubectl logs -l app=jobradar,component=migrate -n $(K8S_NS) --tail=20

k8s-status: ## Show pod statuses and restart counts
	kubectl get pods -n $(K8S_NS) -o wide

k8s-logs: ## Tail logs for all jobradar services
	kubectl logs -f -l app=jobradar -n $(K8S_NS) --max-log-requests=10 --prefix

k8s-logs-nats: ## Tail NATS logs
	kubectl logs -f -l app=jobradar,component=nats -n $(K8S_NS)

k8s-logs-filter: ## Tail filter logs
	kubectl logs -f -l app=jobradar,component=filter -n $(K8S_NS)

k8s-logs-enricher: ## Tail enricher logs
	kubectl logs -f -l app=jobradar,component=enricher -n $(K8S_NS)

k8s-logs-api: ## Tail api logs
	kubectl logs -f -l app=jobradar,component=api -n $(K8S_NS)

k8s-logs-console: ## Tail console logs
	kubectl logs -f -l app=jobradar,component=console -n $(K8S_NS)

k8s-logs-crawler: ## Tail crawler logs
	kubectl logs -f -l app=jobradar,component=crawler -n $(K8S_NS)

k8s-logs-scheduler: ## Tail scheduler logs
	kubectl logs -f -l app=jobradar,component=scheduler -n $(K8S_NS)

k8s-logs-orchestrator: ## Tail orchestrator logs
	kubectl logs -f -l app=jobradar,component=orchestrator -n $(K8S_NS)

k8s-logs-crawler-node: ## Tail all crawler-node replica logs (prefixed by pod name)
	kubectl logs -f -l app=jobradar,component=crawler-node -n $(K8S_NS) --max-log-requests=10 --prefix

k8s-restart-orchestrator: ## Rolling restart orchestrator deployment
	kubectl rollout restart deployment/jobradar-orchestrator -n $(K8S_NS)
	kubectl rollout status deployment/jobradar-orchestrator -n $(K8S_NS) --timeout=120s

k8s-restart-crawler-node: ## Rolling restart all crawler-node replicas
	kubectl rollout restart deployment/jobradar-crawler-node -n $(K8S_NS)
	kubectl rollout status deployment/jobradar-crawler-node -n $(K8S_NS) --timeout=120s

k8s-forward: ## Port-forward console :3000 and API :8080 (when cluster has no NodePort mappings)
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
