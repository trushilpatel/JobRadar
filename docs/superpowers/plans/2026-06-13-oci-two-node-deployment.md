# OCI Two-Node Deployment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deploy JobRadar across two OCI ARM A1 instances (Node 1: API+Console, Node 2: Pipeline) with a GitHub Actions workflow that rebuilds only changed images, deploys only affected nodes, and restarts only pods whose image tag changed.

**Architecture:** Eight Helm deployment templates gain `enabled` guards and per-image deploy annotations. Two committed prod values files (`values.prod-node1.yaml`, `values.prod-node2.yaml`) control which services run on each node. A single `deploy.yml` workflow detects path-level changes, builds only the affected images in parallel, then deploys only the affected nodes — reading current Helm release state to carry forward unchanged image tags.

**Tech Stack:** GitHub Actions, `dorny/paths-filter@v3`, `docker/build-push-action@v6` (multi-arch `linux/arm64`), GHCR, Helm 3, k3s (on OCI), SSH via `appleboy/ssh-action`.

---

## File map

| Action | Path                                                       | What changes                                                                         |
| ------ | ---------------------------------------------------------- | ------------------------------------------------------------------------------------ |
| Modify | `k8s/helm/jobradar/values.yaml`                            | `enabled: true` defaults for all services; `imagePullSecret: ""`; GHCR repo comments |
| Modify | `k8s/helm/jobradar/templates/api/deployment.yaml`          | `enabled` guard + `deploy/backend-sha` annotation + `imagePullSecrets`               |
| Modify | `k8s/helm/jobradar/templates/scheduler/deployment.yaml`    | Same as api                                                                          |
| Modify | `k8s/helm/jobradar/templates/filter/deployment.yaml`       | Same as api                                                                          |
| Modify | `k8s/helm/jobradar/templates/enricher/deployment.yaml`     | Same as api                                                                          |
| Modify | `k8s/helm/jobradar/templates/crawler/deployment.yaml`      | `enabled` guard + `deploy/crawler-sha` annotation + `imagePullSecrets`               |
| Modify | `k8s/helm/jobradar/templates/orchestrator/deployment.yaml` | `enabled` guard + `deploy/orchestrator-sha` annotation + `imagePullSecrets`          |
| Modify | `k8s/helm/jobradar/templates/crawler-node/deployment.yaml` | `enabled` guard + `deploy/crawler-node-sha` annotation + `imagePullSecrets`          |
| Modify | `k8s/helm/jobradar/templates/console/deployment.yaml`      | `enabled` guard + `deploy/console-sha` annotation + `imagePullSecrets`               |
| Create | `k8s/helm/jobradar/values.prod-node1.yaml`                 | Serving-only overrides (tag-agnostic)                                                |
| Create | `k8s/helm/jobradar/values.prod-node2.yaml`                 | Pipeline-only overrides (tag-agnostic)                                               |
| Create | `.github/workflows/deploy.yml`                             | Change detection + conditional builds + parallel node deploys                        |
| Modify | `infra/CLAUDE.md`                                          | OCI bootstrap steps + prod deploy workflow                                           |

---

## Task 1: Update `values.yaml` with `enabled` flags and `imagePullSecret`

**Files:** `k8s/helm/jobradar/values.yaml`

- [ ] **Step 1: Add `enabled: true` to every service block and add `imagePullSecret`**

  Open `k8s/helm/jobradar/values.yaml`. Add `enabled: true` to the top of each service's block and add a root-level `imagePullSecret` key. The diff below shows every location:

  ```yaml
  # Add at the very top of the file (before namespace:)
  imagePullSecret: '' # set in prod values: ghcr-pull-secret
  ```

  Then add `enabled: true` to each service block. For example, `api:` becomes:

  ```yaml
  api:
    enabled: true
    nodePort: 30080
    # ... rest unchanged
  ```

  Repeat for: `console`, `scheduler`, `crawler`, `orchestrator`, `crawlerNode`, `filter`, `enricher`.

  Also add `enabled: true` to `nats:` so the in-cluster NATS can be disabled in prod:

  ```yaml
  nats:
    enabled: true
    external: false
    # ... rest unchanged
  ```

- [ ] **Step 2: Also guard the NATS deployment template**

  Open `k8s/helm/jobradar/templates/nats/deployment.yaml`. Wrap the entire file:

  ```yaml
  {{- if and .Values.nats.enabled (not .Values.nats.external) }}
  apiVersion: apps/v1
  kind: Deployment
  # ... existing content unchanged ...
  {{- end }}
  ```

  Check if the nats template already has an `external` guard — if it does, keep that logic and just add the `enabled` layer on top.

- [ ] **Step 3: Verify the yaml is valid**

  ```bash
  cd /Users/trushil/Projects/JobRadar
  helm template jobradar k8s/helm/jobradar -f k8s/helm/jobradar/values.local.yaml > /dev/null
  echo "exit: $?"
  ```

  Expected: `exit: 0` with no errors.

- [ ] **Step 4: Commit**

  ```bash
  git add k8s/helm/jobradar/values.yaml k8s/helm/jobradar/templates/nats/deployment.yaml
  git commit -m "feat(helm): add enabled flags and imagePullSecret to values"
  ```

---

## Task 2: Add `enabled` guard and deploy annotation to backend-image deployments

**Files:** `templates/api/deployment.yaml`, `templates/scheduler/deployment.yaml`, `templates/filter/deployment.yaml`, `templates/enricher/deployment.yaml`

All four files use `images.backend` and get the same `deploy/backend-sha` annotation.

- [ ] **Step 1: Patch `api/deployment.yaml`**

  Wrap the entire file with an `enabled` guard and add the annotation + `imagePullSecrets` to the pod spec:

  ```yaml
  {{- if .Values.api.enabled }}
  apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: jobradar-api
    namespace: {{ .Values.namespace }}
    labels:
      {{- include "jobradar.labels" . | nindent 4 }}
      component: api
  spec:
    replicas: 1
    selector:
      matchLabels:
        {{- include "jobradar.selectorLabels" (dict "component" "api") | nindent 6 }}
    strategy:
      type: RollingUpdate
      rollingUpdate:
        maxSurge: 1
        maxUnavailable: 0
    template:
      metadata:
        labels:
          {{- include "jobradar.labels" . | nindent 8 }}
          component: api
        annotations:
          deploy/backend-sha: {{ .Values.images.backend.tag | quote }}
      spec:
        {{- with .Values.imagePullSecret }}
        imagePullSecrets:
          - name: {{ . }}
        {{- end }}
        containers:
          - name: api
            image: {{ .Values.images.backend.repository }}:{{ .Values.images.backend.tag }}
            imagePullPolicy: {{ .Values.images.backend.pullPolicy }}
            command: ["/app/api"]
            envFrom:
              - configMapRef:
                  name: jobradar-api-config
              - secretRef:
                  name: jobradar-api-secret
            ports:
              - containerPort: 8080
                name: http
              - containerPort: 9100
                name: metrics
            resources:
              requests:
                memory: "64Mi"
                cpu: "100m"
              limits:
                memory: "128Mi"
                cpu: "500m"
            livenessProbe:
              httpGet:
                path: /health
                port: 9100
              initialDelaySeconds: 10
              periodSeconds: 30
              failureThreshold: 3
            readinessProbe:
              httpGet:
                path: /ready
                port: 9100
              initialDelaySeconds: 5
              periodSeconds: 10
              timeoutSeconds: 3
              failureThreshold: 5
            securityContext:
              {{- include "jobradar.workerSecurityContext" . | nindent 12 }}
        securityContext:
          runAsNonRoot: true
  {{- end }}
  ```

- [ ] **Step 2: Patch `scheduler/deployment.yaml`**

  Same pattern — add `{{- if .Values.scheduler.enabled }}` / `{{- end }}` wrapper, add the `annotations` block under `template.metadata`, add `imagePullSecrets` under `spec`. The annotation is `deploy/backend-sha: {{ .Values.images.backend.tag | quote }}`. Everything else stays identical to the current file.

- [ ] **Step 3: Patch `filter/deployment.yaml`**

  Same pattern. Annotation: `deploy/backend-sha: {{ .Values.images.backend.tag | quote }}`. Guard: `{{- if .Values.filter.enabled }}`.

- [ ] **Step 4: Patch `enricher/deployment.yaml`**

  Same pattern. Annotation: `deploy/backend-sha: {{ .Values.images.backend.tag | quote }}`. Guard: `{{- if .Values.enricher.enabled }}`.

- [ ] **Step 5: Verify locally**

  ```bash
  helm template jobradar k8s/helm/jobradar -f k8s/helm/jobradar/values.local.yaml \
    | grep -A3 "deploy/backend-sha"
  ```

  Expected: four blocks showing `deploy/backend-sha: "local"` (one per backend-image deployment).

  Then verify disabling works:

  ```bash
  helm template jobradar k8s/helm/jobradar -f k8s/helm/jobradar/values.local.yaml \
    --set api.enabled=false \
    | grep "name: jobradar-api" || echo "api correctly disabled"
  ```

  Expected: `api correctly disabled`

- [ ] **Step 6: Commit**

  ```bash
  git add k8s/helm/jobradar/templates/api/deployment.yaml \
          k8s/helm/jobradar/templates/scheduler/deployment.yaml \
          k8s/helm/jobradar/templates/filter/deployment.yaml \
          k8s/helm/jobradar/templates/enricher/deployment.yaml
  git commit -m "feat(helm): add enabled guard and deploy annotation to backend-image deployments"
  ```

---

## Task 3: Add `enabled` guard and deploy annotation to crawler, orchestrator, crawler-node

**Files:** `templates/crawler/deployment.yaml`, `templates/orchestrator/deployment.yaml`, `templates/crawler-node/deployment.yaml`

- [ ] **Step 1: Patch `crawler/deployment.yaml`**

  Add `{{- if .Values.crawler.enabled }}` / `{{- end }}` wrapper. Add under `template.metadata`:

  ```yaml
  annotations:
    deploy/crawler-sha: { { .Values.images.crawler.tag | quote } }
  ```

  Add under `spec:` (before `volumes:`):

  ```yaml
        {{- with .Values.imagePullSecret }}
        imagePullSecrets:
          - name: {{ . }}
        {{- end }}
  ```

- [ ] **Step 2: Patch `orchestrator/deployment.yaml`**

  Guard: `{{- if .Values.orchestrator.enabled }}`. Annotation: `deploy/orchestrator-sha: {{ .Values.images.orchestrator.tag | quote }}`. Same `imagePullSecrets` block under `spec:`.

- [ ] **Step 3: Patch `crawler-node/deployment.yaml`**

  Guard: `{{- if .Values.crawlerNode.enabled }}`. Annotation: `deploy/crawler-node-sha: {{ .Values.images.crawlerNode.tag | quote }}`. Same `imagePullSecrets` block under `spec:`.

- [ ] **Step 4: Verify**

  ```bash
  helm template jobradar k8s/helm/jobradar -f k8s/helm/jobradar/values.local.yaml \
    | grep -E "deploy/(crawler|orchestrator|crawler-node)-sha"
  ```

  Expected: three lines, one per deployment.

  ```bash
  helm template jobradar k8s/helm/jobradar -f k8s/helm/jobradar/values.local.yaml \
    --set crawlerNode.enabled=false \
    | grep "name: jobradar-crawler-node" || echo "crawler-node correctly disabled"
  ```

  Expected: `crawler-node correctly disabled`

- [ ] **Step 5: Commit**

  ```bash
  git add k8s/helm/jobradar/templates/crawler/deployment.yaml \
          k8s/helm/jobradar/templates/orchestrator/deployment.yaml \
          k8s/helm/jobradar/templates/crawler-node/deployment.yaml
  git commit -m "feat(helm): add enabled guard and deploy annotation to pipeline deployments"
  ```

---

## Task 4: Add `enabled` guard and deploy annotation to console deployment

**Files:** `templates/console/deployment.yaml`

The console template is different — it has volumes and a custom security context. Be careful to insert `imagePullSecrets` before `containers:` and after any volume declarations under `spec:`.

- [ ] **Step 1: Patch `console/deployment.yaml`**

  Add `{{- if .Values.console.enabled }}` at the top and `{{- end }}` at the bottom.

  Add under `template.metadata` (after the `labels:` block):

  ```yaml
  annotations:
    deploy/console-sha: { { .Values.images.console.tag | quote } }
  ```

  Add under `spec:` just before the `containers:` key (after the `volumes:` block):

  ```yaml
        {{- with .Values.imagePullSecret }}
        imagePullSecrets:
          - name: {{ . }}
        {{- end }}
  ```

- [ ] **Step 2: Verify**

  ```bash
  helm template jobradar k8s/helm/jobradar -f k8s/helm/jobradar/values.local.yaml \
    | grep "deploy/console-sha"
  ```

  Expected: one line: `deploy/console-sha: "local"`

  ```bash
  helm template jobradar k8s/helm/jobradar -f k8s/helm/jobradar/values.local.yaml \
    --set console.enabled=false \
    | grep "name: jobradar-console" || echo "console correctly disabled"
  ```

  Expected: `console correctly disabled`

- [ ] **Step 3: Full local smoke test — all services render with local values**

  ```bash
  helm template jobradar k8s/helm/jobradar -f k8s/helm/jobradar/values.local.yaml \
    | grep "^kind:" | sort | uniq -c
  ```

  Expected: you should see `Deployment` appearing 8 times (api, console, scheduler, crawler, orchestrator, crawler-node, filter, enricher) plus any Services and ConfigMaps.

- [ ] **Step 4: Commit**

  ```bash
  git add k8s/helm/jobradar/templates/console/deployment.yaml
  git commit -m "feat(helm): add enabled guard and deploy annotation to console deployment"
  ```

---

## Task 5: Create prod values files

**Files:** `k8s/helm/jobradar/values.prod-node1.yaml`, `k8s/helm/jobradar/values.prod-node2.yaml`

- [ ] **Step 1: Create `values.prod-node1.yaml`**

  ```yaml
  # Node 1 — Serving only. Pipeline workers disabled.
  # Image tags are NOT set here — passed at deploy time via --set images.<name>.tag=<sha>

  nats:
    enabled: false # no in-cluster NATS; prod uses Synadia Cloud (external: true in values.yaml)
    external: true

  scheduler:
    enabled: false
  crawler:
    enabled: false
  orchestrator:
    enabled: false
  crawlerNode:
    enabled: false
  filter:
    enabled: false
  enricher:
    enabled: false

  imagePullSecret: ghcr-pull-secret

  images:
    backend:
      repository: ghcr.io/trushilpatel/jobradar-backend
      tag: latest # overridden at deploy time
      pullPolicy: IfNotPresent
    console:
      repository: ghcr.io/trushilpatel/jobradar-console
      tag: latest # overridden at deploy time
      pullPolicy: IfNotPresent
  ```

- [ ] **Step 2: Create `values.prod-node2.yaml`**

  ```yaml
  # Node 2 — Pipeline only. Serving layer disabled.
  # Image tags are NOT set here — passed at deploy time via --set images.<name>.tag=<sha>

  nats:
    enabled: false # no in-cluster NATS; prod uses Synadia Cloud (external: true in values.yaml)
    external: true

  api:
    enabled: false
  console:
    enabled: false

  crawlerNode:
    replicas: 3

  imagePullSecret: ghcr-pull-secret

  images:
    backend:
      repository: ghcr.io/trushilpatel/jobradar-backend
      tag: latest # overridden at deploy time
      pullPolicy: IfNotPresent
    crawler:
      repository: ghcr.io/trushilpatel/jobradar-crawler
      tag: latest # overridden at deploy time
      pullPolicy: IfNotPresent
    orchestrator:
      repository: ghcr.io/trushilpatel/jobradar-orchestrator
      tag: latest # overridden at deploy time
      pullPolicy: IfNotPresent
    crawlerNode:
      repository: ghcr.io/trushilpatel/jobradar-crawler-node
      tag: latest # overridden at deploy time
      pullPolicy: IfNotPresent
  ```

- [ ] **Step 3: Verify Node 1 renders only API + Console deployments**

  ```bash
  helm template jobradar k8s/helm/jobradar \
    -f k8s/helm/jobradar/values.local.yaml \
    -f k8s/helm/jobradar/values.prod-node1.yaml \
    | grep "^  name: jobradar-"
  ```

  Expected output contains only `jobradar-api` and `jobradar-console` Deployment names. No pipeline services.

- [ ] **Step 4: Verify Node 2 renders only pipeline deployments**

  ```bash
  helm template jobradar k8s/helm/jobradar \
    -f k8s/helm/jobradar/values.local.yaml \
    -f k8s/helm/jobradar/values.prod-node2.yaml \
    | grep "^  name: jobradar-"
  ```

  Expected: only `jobradar-scheduler`, `jobradar-crawler`, `jobradar-orchestrator`, `jobradar-crawler-node`, `jobradar-filter`, `jobradar-enricher`. No api or console.

- [ ] **Step 5: Verify crawler-node replicas = 3 on Node 2**

  ```bash
  helm template jobradar k8s/helm/jobradar \
    -f k8s/helm/jobradar/values.local.yaml \
    -f k8s/helm/jobradar/values.prod-node2.yaml \
    | grep -A2 "name: jobradar-crawler-node" | grep "replicas"
  ```

  Expected: `replicas: 3`

- [ ] **Step 6: Commit**

  ```bash
  git add k8s/helm/jobradar/values.prod-node1.yaml k8s/helm/jobradar/values.prod-node2.yaml
  git commit -m "feat(helm): add prod values files for Node 1 (serving) and Node 2 (pipeline)"
  ```

---

## Task 6: Create `.github/workflows/deploy.yml`

**Files:** `.github/workflows/deploy.yml`

This is the core of the optimized CI/CD. One workflow, seven jobs.

- [ ] **Step 1: Create `.github/workflows/deploy.yml`**

  ```yaml
  name: Deploy

  on:
    push:
      branches: [main]

  concurrency:
    group: deploy-${{ github.ref }}
    cancel-in-progress: true

  env:
    REGISTRY: ghcr.io
    OWNER: trushilpatel

  jobs:
    # ── 1. Detect which source paths changed ──────────────────────────────────
    detect-changes:
      name: detect changes
      runs-on: ubuntu-latest
      outputs:
        backend_changed: ${{ steps.filter.outputs.backend }}
        crawler_changed: ${{ steps.filter.outputs.crawler }}
        orch_changed: ${{ steps.filter.outputs.orchestrator }}
        node_changed: ${{ steps.filter.outputs.crawler_node }}
        console_changed: ${{ steps.filter.outputs.console }}
        node1_changed: ${{ steps.flags.outputs.node1 }}
        node2_changed: ${{ steps.flags.outputs.node2 }}
      steps:
        - uses: actions/checkout@v4

        - uses: dorny/paths-filter@v3
          id: filter
          with:
            filters: |
              backend:
                - 'backend/apps/api/**'
                - 'backend/apps/filter/**'
                - 'backend/apps/enricher/**'
                - 'backend/apps/scheduler/**'
                - 'backend/apps/migrate/**'
                - 'backend/service/**'
                - 'backend/platform/**'
                - 'backend/common/**'
                - 'backend/go.mod'
                - 'backend/go.sum'
                - 'infra/Dockerfile.backend'
              crawler:
                - 'backend/apps/crawler/**'
                - 'backend/service/**'
                - 'backend/platform/**'
                - 'backend/common/**'
                - 'backend/go.mod'
                - 'backend/go.sum'
                - 'infra/Dockerfile.crawler'
              orchestrator:
                - 'backend/apps/orchestrator/**'
                - 'backend/service/**'
                - 'backend/platform/**'
                - 'backend/common/**'
                - 'backend/go.mod'
                - 'backend/go.sum'
                - 'infra/Dockerfile.orchestrator'
              crawler_node:
                - 'backend/apps/crawler-node/**'
                - 'backend/service/**'
                - 'backend/platform/**'
                - 'backend/common/**'
                - 'backend/go.mod'
                - 'backend/go.sum'
                - 'infra/Dockerfile.crawler-node'
              console:
                - 'frontend/**'
                - 'frontend/Dockerfile'

        - name: Derive node-level flags
          id: flags
          run: |
            NODE1=$( [ "${{ steps.filter.outputs.backend }}" == "true" ] || \
                     [ "${{ steps.filter.outputs.console }}"  == "true" ] && echo "true" || echo "false" )
            NODE2=$( [ "${{ steps.filter.outputs.backend }}"      == "true" ] || \
                     [ "${{ steps.filter.outputs.crawler }}"      == "true" ] || \
                     [ "${{ steps.filter.outputs.orchestrator }}" == "true" ] || \
                     [ "${{ steps.filter.outputs.crawler_node }}" == "true" ] && echo "true" || echo "false" )
            echo "node1=$NODE1" >> $GITHUB_OUTPUT
            echo "node2=$NODE2" >> $GITHUB_OUTPUT

    # ── 2–6. Conditional parallel image builds ────────────────────────────────
    build-backend:
      name: build backend image
      needs: detect-changes
      if: needs.detect-changes.outputs.backend_changed == 'true'
      runs-on: ubuntu-latest
      steps:
        - uses: actions/checkout@v4
        - uses: docker/setup-qemu-action@v3
        - uses: docker/setup-buildx-action@v3
        - uses: docker/login-action@v3
          with:
            registry: ${{ env.REGISTRY }}
            username: ${{ github.actor }}
            password: ${{ secrets.GITHUB_TOKEN }}
        - name: Build and push
          uses: docker/build-push-action@v6
          with:
            context: backend
            file: infra/Dockerfile.backend
            platforms: linux/arm64
            push: true
            tags: |
              ${{ env.REGISTRY }}/${{ env.OWNER }}/jobradar-backend:${{ github.sha && github.sha[:7] || 'latest' }}
              ${{ env.REGISTRY }}/${{ env.OWNER }}/jobradar-backend:${{ github.sha }}
              ${{ env.REGISTRY }}/${{ env.OWNER }}/jobradar-backend:latest
            cache-from: type=gha,scope=backend
            cache-to: type=gha,scope=backend,mode=max

    build-crawler:
      name: build crawler image
      needs: detect-changes
      if: needs.detect-changes.outputs.crawler_changed == 'true'
      runs-on: ubuntu-latest
      steps:
        - uses: actions/checkout@v4
        - uses: docker/setup-qemu-action@v3
        - uses: docker/setup-buildx-action@v3
        - uses: docker/login-action@v3
          with:
            registry: ${{ env.REGISTRY }}
            username: ${{ github.actor }}
            password: ${{ secrets.GITHUB_TOKEN }}
        - name: Build and push
          uses: docker/build-push-action@v6
          with:
            context: backend
            file: infra/Dockerfile.crawler
            platforms: linux/arm64
            push: true
            tags: |
              ${{ env.REGISTRY }}/${{ env.OWNER }}/jobradar-crawler:${{ github.sha }}
              ${{ env.REGISTRY }}/${{ env.OWNER }}/jobradar-crawler:latest
            cache-from: type=gha,scope=crawler
            cache-to: type=gha,scope=crawler,mode=max

    build-orchestrator:
      name: build orchestrator image
      needs: detect-changes
      if: needs.detect-changes.outputs.orch_changed == 'true'
      runs-on: ubuntu-latest
      steps:
        - uses: actions/checkout@v4
        - uses: docker/setup-qemu-action@v3
        - uses: docker/setup-buildx-action@v3
        - uses: docker/login-action@v3
          with:
            registry: ${{ env.REGISTRY }}
            username: ${{ github.actor }}
            password: ${{ secrets.GITHUB_TOKEN }}
        - name: Build and push
          uses: docker/build-push-action@v6
          with:
            context: backend
            file: infra/Dockerfile.orchestrator
            platforms: linux/arm64
            push: true
            tags: |
              ${{ env.REGISTRY }}/${{ env.OWNER }}/jobradar-orchestrator:${{ github.sha }}
              ${{ env.REGISTRY }}/${{ env.OWNER }}/jobradar-orchestrator:latest
            cache-from: type=gha,scope=orchestrator
            cache-to: type=gha,scope=orchestrator,mode=max

    build-crawler-node:
      name: build crawler-node image
      needs: detect-changes
      if: needs.detect-changes.outputs.node_changed == 'true'
      runs-on: ubuntu-latest
      steps:
        - uses: actions/checkout@v4
        - uses: docker/setup-qemu-action@v3
        - uses: docker/setup-buildx-action@v3
        - uses: docker/login-action@v3
          with:
            registry: ${{ env.REGISTRY }}
            username: ${{ github.actor }}
            password: ${{ secrets.GITHUB_TOKEN }}
        - name: Build and push
          uses: docker/build-push-action@v6
          with:
            context: backend
            file: infra/Dockerfile.crawler-node
            platforms: linux/arm64
            push: true
            tags: |
              ${{ env.REGISTRY }}/${{ env.OWNER }}/jobradar-crawler-node:${{ github.sha }}
              ${{ env.REGISTRY }}/${{ env.OWNER }}/jobradar-crawler-node:latest
            cache-from: type=gha,scope=crawler-node
            cache-to: type=gha,scope=crawler-node,mode=max

    build-console:
      name: build console image
      needs: detect-changes
      if: needs.detect-changes.outputs.console_changed == 'true'
      runs-on: ubuntu-latest
      steps:
        - uses: actions/checkout@v4
        - uses: docker/setup-qemu-action@v3
        - uses: docker/setup-buildx-action@v3
        - uses: docker/login-action@v3
          with:
            registry: ${{ env.REGISTRY }}
            username: ${{ github.actor }}
            password: ${{ secrets.GITHUB_TOKEN }}
        - name: Build and push
          uses: docker/build-push-action@v6
          with:
            context: frontend
            file: frontend/Dockerfile
            platforms: linux/arm64
            push: true
            build-args: |
              VITE_SUPABASE_URL=${{ secrets.VITE_SUPABASE_URL }}
              VITE_SUPABASE_ANON_KEY=${{ secrets.VITE_SUPABASE_ANON_KEY }}
            tags: |
              ${{ env.REGISTRY }}/${{ env.OWNER }}/jobradar-console:${{ github.sha }}
              ${{ env.REGISTRY }}/${{ env.OWNER }}/jobradar-console:latest
            cache-from: type=gha,scope=console
            cache-to: type=gha,scope=console,mode=max

    # ── 7. Deploy Node 1 (API + Console) ─────────────────────────────────────
    deploy-node1:
      name: deploy node1 (serving)
      needs:
        - detect-changes
        - build-backend
        - build-console
      # always() lets this run even when build jobs were skipped (image unchanged)
      if: |
        always() &&
        needs.detect-changes.outputs.node1_changed == 'true' &&
        !contains(needs.*.result, 'failure') &&
        !contains(needs.*.result, 'cancelled')
      runs-on: ubuntu-latest
      steps:
        - uses: actions/checkout@v4

        - name: Setup SSH key
          run: |
            mkdir -p ~/.ssh
            echo "${{ secrets.OCI_SSH_KEY }}" > ~/.ssh/id_ed25519
            chmod 600 ~/.ssh/id_ed25519
            ssh-keyscan -H "${{ secrets.OCI_NODE1_HOST }}" >> ~/.ssh/known_hosts

        - name: Resolve image tags for Node 1
          id: tags
          env:
            HOST: ${{ secrets.OCI_NODE1_HOST }}
            USER: ${{ secrets.OCI_SSH_USER }}
          run: |
            SHORT_SHA="${GITHUB_SHA:0:7}"

            # For each image: use new SHA if rebuilt this run, else read from current helm release
            if [ "${{ needs.build-backend.result }}" == "success" ]; then
              BACKEND_TAG="$SHORT_SHA"
            else
              BACKEND_TAG=$(ssh -i ~/.ssh/id_ed25519 "$USER@$HOST" \
                "KUBECONFIG=/etc/rancher/k3s/k3s.yaml helm get values jobradar --all -o json 2>/dev/null \
                | jq -r '.images.backend.tag // \"latest\"'")
            fi

            if [ "${{ needs.build-console.result }}" == "success" ]; then
              CONSOLE_TAG="$SHORT_SHA"
            else
              CONSOLE_TAG=$(ssh -i ~/.ssh/id_ed25519 "$USER@$HOST" \
                "KUBECONFIG=/etc/rancher/k3s/k3s.yaml helm get values jobradar --all -o json 2>/dev/null \
                | jq -r '.images.console.tag // \"latest\"'")
            fi

            echo "backend=$BACKEND_TAG" >> $GITHUB_OUTPUT
            echo "console=$CONSOLE_TAG" >> $GITHUB_OUTPUT

        - name: Copy chart and values to Node 1
          env:
            HOST: ${{ secrets.OCI_NODE1_HOST }}
            USER: ${{ secrets.OCI_SSH_USER }}
          run: |
            scp -i ~/.ssh/id_ed25519 -r k8s/helm/jobradar "$USER@$HOST:/tmp/jobradar-chart"
            scp -i ~/.ssh/id_ed25519 k8s/helm/jobradar/values.prod-node1.yaml "$USER@$HOST:/tmp/"

        - name: Helm upgrade Node 1
          env:
            HOST: ${{ secrets.OCI_NODE1_HOST }}
            USER: ${{ secrets.OCI_SSH_USER }}
          run: |
            ssh -i ~/.ssh/id_ed25519 "$USER@$HOST" \
              "KUBECONFIG=/etc/rancher/k3s/k3s.yaml helm upgrade --install jobradar /tmp/jobradar-chart \
                --namespace jobradar-local \
                --create-namespace \
                -f /tmp/values.prod-node1.yaml \
                --set images.backend.tag=${{ steps.tags.outputs.backend }} \
                --set images.console.tag=${{ steps.tags.outputs.console }} \
                --wait --timeout 5m"

    # ── 8. Deploy Node 2 (Pipeline) ───────────────────────────────────────────
    deploy-node2:
      name: deploy node2 (pipeline)
      needs:
        - detect-changes
        - build-backend
        - build-crawler
        - build-orchestrator
        - build-crawler-node
      if: |
        always() &&
        needs.detect-changes.outputs.node2_changed == 'true' &&
        !contains(needs.*.result, 'failure') &&
        !contains(needs.*.result, 'cancelled')
      runs-on: ubuntu-latest
      steps:
        - uses: actions/checkout@v4

        - name: Setup SSH key
          run: |
            mkdir -p ~/.ssh
            echo "${{ secrets.OCI_SSH_KEY }}" > ~/.ssh/id_ed25519
            chmod 600 ~/.ssh/id_ed25519
            ssh-keyscan -H "${{ secrets.OCI_NODE2_HOST }}" >> ~/.ssh/known_hosts

        - name: Resolve image tags for Node 2
          id: tags
          env:
            HOST: ${{ secrets.OCI_NODE2_HOST }}
            USER: ${{ secrets.OCI_SSH_USER }}
          run: |
            SHORT_SHA="${GITHUB_SHA:0:7}"

            get_tag() {
              local result="$1"
              local key="$2"
              if [ "$result" == "success" ]; then
                echo "$SHORT_SHA"
              else
                ssh -i ~/.ssh/id_ed25519 "$USER@$HOST" \
                  "KUBECONFIG=/etc/rancher/k3s/k3s.yaml helm get values jobradar --all -o json 2>/dev/null \
                  | jq -r \".images.${key}.tag // \\\"latest\\\"\""
              fi
            }

            echo "backend=$(get_tag '${{ needs.build-backend.result }}' backend)" >> $GITHUB_OUTPUT
            echo "crawler=$(get_tag '${{ needs.build-crawler.result }}' crawler)" >> $GITHUB_OUTPUT
            echo "orchestrator=$(get_tag '${{ needs.build-orchestrator.result }}' orchestrator)" >> $GITHUB_OUTPUT
            echo "crawler_node=$(get_tag '${{ needs.build-crawler-node.result }}' crawlerNode)" >> $GITHUB_OUTPUT

        - name: Copy chart and values to Node 2
          env:
            HOST: ${{ secrets.OCI_NODE2_HOST }}
            USER: ${{ secrets.OCI_SSH_USER }}
          run: |
            scp -i ~/.ssh/id_ed25519 -r k8s/helm/jobradar "$USER@$HOST:/tmp/jobradar-chart"
            scp -i ~/.ssh/id_ed25519 k8s/helm/jobradar/values.prod-node2.yaml "$USER@$HOST:/tmp/"

        - name: Helm upgrade Node 2
          env:
            HOST: ${{ secrets.OCI_NODE2_HOST }}
            USER: ${{ secrets.OCI_SSH_USER }}
          run: |
            ssh -i ~/.ssh/id_ed25519 "$USER@$HOST" \
              "KUBECONFIG=/etc/rancher/k3s/k3s.yaml helm upgrade --install jobradar /tmp/jobradar-chart \
                --namespace jobradar-local \
                --create-namespace \
                -f /tmp/values.prod-node2.yaml \
                --set images.backend.tag=${{ steps.tags.outputs.backend }} \
                --set images.crawler.tag=${{ steps.tags.outputs.crawler }} \
                --set images.orchestrator.tag=${{ steps.tags.outputs.orchestrator }} \
                --set images.crawlerNode.tag=${{ steps.tags.outputs.crawler_node }} \
                --wait --timeout 5m"
  ```

- [ ] **Step 2: Fix the node-level flag derivation**

  The shell conditional in `derive node-level flags` needs correct bash syntax. Replace that step's `run:` block with:

  ```bash
  if [[ "${{ steps.filter.outputs.backend }}" == "true" || "${{ steps.filter.outputs.console }}" == "true" ]]; then
    echo "node1=true" >> $GITHUB_OUTPUT
  else
    echo "node1=false" >> $GITHUB_OUTPUT
  fi

  if [[ "${{ steps.filter.outputs.backend }}"      == "true" || \
        "${{ steps.filter.outputs.crawler }}"      == "true" || \
        "${{ steps.filter.outputs.orchestrator }}" == "true" || \
        "${{ steps.filter.outputs.crawler_node }}" == "true" ]]; then
    echo "node2=true" >> $GITHUB_OUTPUT
  else
    echo "node2=false" >> $GITHUB_OUTPUT
  fi
  ```

- [ ] **Step 3: Fix the SHA tag expression**

  GitHub Actions expressions don't support string slicing like `${{ github.sha[:7] }}`. Replace every `${{ github.sha && github.sha[:7] || 'latest' }}` tag with a two-step approach — use `github.sha` for the full SHA tag and compute the short SHA in a `run:` step that sets `SHORT_SHA` env var:

  For each build job, add before the `Build and push` step:

  ```yaml
  - name: Set short SHA
    run: echo "SHORT_SHA=${GITHUB_SHA:0:7}" >> $GITHUB_ENV
  ```

  Then in `tags:` use `${{ env.SHORT_SHA }}` instead of the broken expression:

  ```yaml
  tags: |
    ${{ env.REGISTRY }}/${{ env.OWNER }}/jobradar-backend:${{ env.SHORT_SHA }}
    ${{ env.REGISTRY }}/${{ env.OWNER }}/jobradar-backend:${{ github.sha }}
    ${{ env.REGISTRY }}/${{ env.OWNER }}/jobradar-backend:latest
  ```

- [ ] **Step 4: Validate YAML syntax**

  ```bash
  python3 -c "import yaml; yaml.safe_load(open('.github/workflows/deploy.yml'))" && echo "valid yaml"
  ```

  Expected: `valid yaml`

- [ ] **Step 5: Commit**

  ```bash
  git add .github/workflows/deploy.yml
  git commit -m "feat(ci): add optimized deploy workflow with change detection and parallel builds"
  ```

---

## Task 7: Update `infra/CLAUDE.md` with OCI bootstrap and prod deploy notes

**Files:** `infra/CLAUDE.md`

- [ ] **Step 1: Add OCI Production section to `infra/CLAUDE.md`**

  Append the following section to the end of `infra/CLAUDE.md`:

  ````markdown
  ## OCI Production Deployment

  Two Oracle Cloud Free Tier A1 ARM instances (1 GB each). Deployed automatically on push to `main`
  via `.github/workflows/deploy.yml`. Only changed images are rebuilt; only affected nodes are
  deployed; only pods whose image tag changed are restarted.

  **Node 1 — Serving:** API + Console  
  **Node 2 — Pipeline:** Scheduler + Orchestrator + CrawlerNode (×3) + Filter + Enricher

  ### One-time bootstrap (run once per OCI instance over SSH)

  ```bash
  # 1. Install k3s
  curl -sfL https://get.k3s.io | sh -

  # 2. Install helm (k3s does not include helm CLI)
  curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash

  # 3. Install jq (used by deploy workflow to read current image tags)
  apt-get install -y jq        # Ubuntu
  # or: yum install -y jq      # Oracle Linux

  # 4. Install Doppler CLI and authenticate
  curl -Ls https://cli.doppler.com/install.sh | sh
  doppler login
  doppler setup   # select job-radar / prod

  # 5. Create GHCR imagePullSecret (create a GitHub PAT with read:packages scope first)
  KUBECONFIG=/etc/rancher/k3s/k3s.yaml kubectl create namespace jobradar-local
  KUBECONFIG=/etc/rancher/k3s/k3s.yaml kubectl create secret docker-registry ghcr-pull-secret \
    --docker-server=ghcr.io \
    --docker-username=<your-github-username> \
    --docker-password=<PAT-with-read:packages> \
    --namespace jobradar-local

  # 6. Open OCI Security List ports
  #    Node 1: 22 (SSH), 30080 (API), 30300 (Console)
  #    Node 2: 22 (SSH) only

  # 7. Run first deploy manually (subsequent deploys are automated via GitHub Actions)
  #    On Node 1:
  KUBECONFIG=/etc/rancher/k3s/k3s.yaml doppler run -- helm upgrade --install jobradar k8s/helm/jobradar \
    -f k8s/helm/jobradar/values.prod-node1.yaml \
    --set images.backend.tag=<sha> \
    --set images.console.tag=<sha> \
    --namespace jobradar-local --create-namespace
  ```
  ````

  ### GitHub secrets required

  Add these in GitHub repo → Settings → Secrets and variables → Actions:

  | Secret                   | Value                                                      |
  | ------------------------ | ---------------------------------------------------------- |
  | `OCI_NODE1_HOST`         | Public IP of Node 1                                        |
  | `OCI_NODE2_HOST`         | Public IP of Node 2                                        |
  | `OCI_SSH_USER`           | SSH username (`ubuntu` for Ubuntu, `opc` for Oracle Linux) |
  | `OCI_SSH_KEY`            | Content of the SSH private key (Ed25519)                   |
  | `VITE_SUPABASE_URL`      | Value of `VITE_SUPABASE_URL` from Doppler                  |
  | `VITE_SUPABASE_ANON_KEY` | Value of `VITE_SUPABASE_ANON_KEY` from Doppler             |

  `GITHUB_TOKEN` for GHCR push is injected automatically — no extra secret needed.

  ### Observability on OCI

  Deploy the existing obs chart on each node after bootstrapping:

  ```bash
  KUBECONFIG=/etc/rancher/k3s/k3s.yaml doppler run -- helm upgrade --install obs k8s/helm/obs \
    --namespace jobradar-local
  ```

  ```

  ```

- [ ] **Step 2: Commit**

  ```bash
  git add infra/CLAUDE.md
  git commit -m "docs(infra): add OCI bootstrap and prod deploy workflow documentation"
  ```

---

## Task 8: Push to PR

- [ ] **Step 1: Push the feature branch**

  ```bash
  git push -u origin feat/oci-two-node-deployment
  ```

  The output will include the URL to create the PR. Copy it.

- [ ] **Step 2: Open the PR**

  Use the URL printed by `git push`. Target branch: `dev`. Title: `feat: OCI two-node production deployment with optimized CI/CD`.

  PR description checklist:
  - Node 1 runs API + Console, Node 2 runs all pipeline workers
  - Only changed images are rebuilt (dorny/paths-filter per Dockerfile path)
  - Only affected nodes deploy (node1/node2 flags derived from image change flags)
  - Only pods whose image annotation changed restart (deploy/\*-sha pod annotations)
  - Deploy reads current Helm release for unchanged image tags via `helm get values`
  - GHCR with linux/arm64 builds via buildx + QEMU
  - GHA cache per image scope — Go compile ~20s on hit
  - Bootstrap steps documented in `infra/CLAUDE.md`
  - `values.prod-node1.yaml` and `values.prod-node2.yaml` are tag-agnostic

---

## Self-review notes

- The `deploy-node2` `get_tag` function uses shell quoting around the `needs.*` expression — test in GHA; if it fails, inline the four `if` blocks individually
- The `helm get values` SSH command requires `helm` + `jq` on the OCI instance — both are in the bootstrap steps above
- `KUBECONFIG=/etc/rancher/k3s/k3s.yaml` is required on OCI since k3s writes its kubeconfig there (not `~/.kube/config`)
- The obs chart deploy is manual for now — can be added to the workflow as a future iteration
