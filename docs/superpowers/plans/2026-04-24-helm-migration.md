# Helm Migration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace all raw k8s manifests with two Helm charts — `jobradar` (app services) and `observability` (Prometheus, Grafana, Loki, Promtail, Jaeger, supabase-audit-sync) — running in k3d on namespace `jobradar-local`.

**Architecture:** Two independent Helm charts installed in the same namespace. `jobradar` is a fully custom chart replacing existing raw manifests. `observability` is a thin wrapper chart that pulls community charts (kube-prometheus-stack, loki-stack, jaeger) as dependencies and adds a custom supabase-audit-sync Deployment. Prometheus scrapes app metrics via ClusterIP Services added to each worker. Grafana dashboards are provisioned from existing JSON files in `infra/grafana/dashboards/`.

**Tech Stack:** Helm 3, k3d (k3s), kubectl, kube-prometheus-stack, grafana/loki-stack, jaegertracing/jaeger.

---

## Phase 1: jobradar Chart

### Task 1: Scaffold jobradar chart

**Files:**
- Create: `k8s/helm/jobradar/Chart.yaml`
- Create: `k8s/helm/jobradar/values.yaml`
- Create: `k8s/helm/jobradar/templates/_helpers.tpl`
- Create: `k8s/helm/jobradar/templates/namespace.yaml`

- [ ] **Step 1: Add Helm repo (none needed for custom chart) and create chart skeleton**

```bash
mkdir -p k8s/helm/jobradar/templates/{kafka,redis,api,console,scheduler,crawler,filter,enricher,migrate}
```

- [ ] **Step 2: Create Chart.yaml**

Create `k8s/helm/jobradar/Chart.yaml`:
```yaml
apiVersion: v2
name: jobradar
description: JobRadar application services
type: application
version: 0.1.0
appVersion: "local"
```

- [ ] **Step 3: Create values.yaml**

Create `k8s/helm/jobradar/values.yaml`:
```yaml
namespace: jobradar-local
environment: local

images:
  backend:
    repository: jobradar-backend
    tag: local
    pullPolicy: IfNotPresent
  crawler:
    repository: jobradar-crawler
    tag: local
    pullPolicy: IfNotPresent
  console:
    repository: jobradar-console
    tag: local
    pullPolicy: IfNotPresent
  kafka:
    repository: apache/kafka
    tag: "3.9.0"
    pullPolicy: IfNotPresent
  redis:
    repository: redis
    tag: "7-alpine"
    pullPolicy: IfNotPresent

api:
  nodePort: 30080
  metricsAddr: ":9100"
  logLevel: info
  logFormat: json
  jaegerEndpoint: ""
  dbSslmode: require
  dbPort: "5432"
  supabaseProjectUrl: "https://gnwgoffnrxydkrkahhie.supabase.co"
  allowedEmail: "trushilpatel9898@gmail.com"
  jwtClockSkewSeconds: "60"
  jwksCacheTtlSeconds: "600"
  apiMaxBodyBytes: "1048576"

console:
  nodePort: 30300

scheduler:
  scanIntervalMinutes: "1440"
  checkIntervalSeconds: "60"
  stuckTimeoutMinutes: "30"
  logLevel: info
  logFormat: json
  metricsAddr: ":9101"
  jaegerEndpoint: ""
  dbSslmode: require
  dbPort: "5432"

crawler:
  kafkaBrokers: "kafka:9092"
  pollIntervalSeconds: "60"
  stuckTimeoutSeconds: "120"
  heartbeatIntervalSeconds: "30"
  scanCompanyDelaySeconds: "30"
  googleMinDelay: "55000"
  googleMaxDelay: "75000"
  googleTimeoutMs: "18000000"
  googleMaxPages: "0"
  googleEnrichMin: "5000"
  googleEnrichMax: "10000"
  googleMaxDetailJobs: "0"
  logLevel: info
  logFormat: json
  metricsAddr: ":9101"
  jaegerEndpoint: ""
  dbSslmode: require
  dbPort: "5432"

filter:
  kafkaBrokers: "kafka:9092"
  redisAddr: "redis:6379"
  redisDb: "0"
  batchSize: "50"
  flushTimeoutSeconds: "30"
  llmCallIntervalSeconds: "5"
  llmProvider: gemini
  cooldownBaseSeconds: "60"
  cooldownMaxSeconds: "3600"
  cooldownJitter: "0.2"
  logLevel: info
  logFormat: json
  metricsAddr: ":9101"
  jaegerEndpoint: ""
  dbSslmode: require
  dbPort: "5432"

enricher:
  kafkaBrokers: "kafka:9092"
  redisAddr: "redis:6379"
  redisDb: "0"
  fetchConcurrency: "15"
  llmBatchSize: "5"
  flushTimeoutSeconds: "120"
  llmCallIntervalSeconds: "5"
  defaultRps: "2"
  defaultBurst: "3"
  jitterMs: "200"
  discardJobWithExpGt: "0"
  llmProvider: gemini
  cooldownBaseSeconds: "60"
  cooldownMaxSeconds: "3600"
  cooldownJitter: "0.2"
  logLevel: info
  logFormat: json
  metricsAddr: ":9101"
  jaegerEndpoint: ""
  dbSslmode: require
  dbPort: "5432"

kafka:
  storage: 5Gi
  storageClass: local-path
  logRetentionHours: "168"
  numPartitions: "1"

redis:
  storage: 1Gi
  storageClass: local-path
```

- [ ] **Step 4: Create _helpers.tpl**

Create `k8s/helm/jobradar/templates/_helpers.tpl`:
```
{{/*
Common labels applied to all resources.
*/}}
{{- define "jobradar.labels" -}}
app: jobradar
environment: {{ .Values.environment }}
{{- end }}

{{/*
Selector labels for a given component.
Usage: include "jobradar.selectorLabels" (dict "component" "api")
*/}}
{{- define "jobradar.selectorLabels" -}}
app: jobradar
component: {{ .component }}
{{- end }}

{{/*
Standard security context for Go worker containers (non-root).
*/}}
{{- define "jobradar.workerSecurityContext" -}}
runAsNonRoot: true
runAsUser: 65532
runAsGroup: 65532
readOnlyRootFilesystem: true
allowPrivilegeEscalation: false
capabilities:
  drop: ["ALL"]
{{- end }}
```

- [ ] **Step 5: Create namespace.yaml**

Create `k8s/helm/jobradar/templates/namespace.yaml`:
```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: {{ .Values.namespace }}
  labels:
    {{- include "jobradar.labels" . | nindent 4 }}
```

- [ ] **Step 6: Validate chart renders without errors**

```bash
helm template jobradar ./k8s/helm/jobradar -n jobradar-local
```

Expected: namespace YAML printed, no errors.

- [ ] **Step 7: Commit**

```bash
git add k8s/helm/jobradar/
git commit -m "feat(helm): scaffold jobradar chart skeleton"
```

---

### Task 2: Kafka templates

**Files:**
- Create: `k8s/helm/jobradar/templates/kafka/configmap.yaml`
- Create: `k8s/helm/jobradar/templates/kafka/statefulset.yaml`
- Create: `k8s/helm/jobradar/templates/kafka/services.yaml`

- [ ] **Step 1: Create kafka/configmap.yaml**

Create `k8s/helm/jobradar/templates/kafka/configmap.yaml`:
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: jobradar-kafka-config
  namespace: {{ .Values.namespace }}
  labels:
    {{- include "jobradar.labels" . | nindent 4 }}
    component: kafka
data:
  KAFKA_NODE_ID: "1"
  KAFKA_PROCESS_ROLES: "broker,controller"
  KAFKA_CONTROLLER_QUORUM_VOTERS: "1@localhost:9093"
  KAFKA_LISTENERS: "PLAINTEXT://:9092,CONTROLLER://:9093"
  KAFKA_ADVERTISED_LISTENERS: "PLAINTEXT://kafka:9092"
  KAFKA_LISTENER_SECURITY_PROTOCOL_MAP: "CONTROLLER:PLAINTEXT,PLAINTEXT:PLAINTEXT"
  KAFKA_CONTROLLER_LISTENER_NAMES: "CONTROLLER"
  KAFKA_INTER_BROKER_LISTENER_NAME: "PLAINTEXT"
  KAFKA_OFFSETS_TOPIC_REPLICATION_FACTOR: "1"
  KAFKA_TRANSACTION_STATE_LOG_REPLICATION_FACTOR: "1"
  KAFKA_TRANSACTION_STATE_LOG_MIN_ISR: "1"
  KAFKA_AUTO_CREATE_TOPICS_ENABLE: "true"
  KAFKA_LOG_DIRS: "/var/lib/kafka/data"
  KAFKA_LOG_RETENTION_HOURS: {{ .Values.kafka.logRetentionHours | quote }}
  KAFKA_NUM_PARTITIONS: {{ .Values.kafka.numPartitions | quote }}
```

- [ ] **Step 2: Create kafka/statefulset.yaml**

Create `k8s/helm/jobradar/templates/kafka/statefulset.yaml`:
```yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: kafka
  namespace: {{ .Values.namespace }}
  labels:
    {{- include "jobradar.labels" . | nindent 4 }}
    component: kafka
spec:
  serviceName: kafka-headless
  replicas: 1
  selector:
    matchLabels:
      {{- include "jobradar.selectorLabels" (dict "component" "kafka") | nindent 6 }}
  template:
    metadata:
      labels:
        {{- include "jobradar.labels" . | nindent 8 }}
        component: kafka
    spec:
      containers:
        - name: kafka
          image: {{ .Values.images.kafka.repository }}:{{ .Values.images.kafka.tag }}
          imagePullPolicy: {{ .Values.images.kafka.pullPolicy }}
          ports:
            - containerPort: 9092
              name: broker
            - containerPort: 9093
              name: controller
          envFrom:
            - configMapRef:
                name: jobradar-kafka-config
          resources:
            requests:
              memory: "256Mi"
              cpu: "250m"
            limits:
              memory: "1Gi"
              cpu: "1000m"
          readinessProbe:
            tcpSocket:
              port: 9092
            initialDelaySeconds: 20
            periodSeconds: 10
            failureThreshold: 5
          livenessProbe:
            tcpSocket:
              port: 9092
            initialDelaySeconds: 30
            periodSeconds: 15
            failureThreshold: 3
          volumeMounts:
            - name: kafka-data
              mountPath: /var/lib/kafka/data
          securityContext:
            allowPrivilegeEscalation: false
            capabilities:
              drop: ["ALL"]
      securityContext:
        runAsNonRoot: true
        runAsUser: 1000
        runAsGroup: 1000
        fsGroup: 1000
  volumeClaimTemplates:
    - metadata:
        name: kafka-data
        labels:
          {{- include "jobradar.labels" . | nindent 10 }}
          component: kafka
      spec:
        accessModes: ["ReadWriteOnce"]
        storageClassName: {{ .Values.kafka.storageClass }}
        resources:
          requests:
            storage: {{ .Values.kafka.storage }}
```

- [ ] **Step 3: Create kafka/services.yaml**

Create `k8s/helm/jobradar/templates/kafka/services.yaml`:
```yaml
apiVersion: v1
kind: Service
metadata:
  name: kafka-headless
  namespace: {{ .Values.namespace }}
  labels:
    {{- include "jobradar.labels" . | nindent 4 }}
    component: kafka
spec:
  clusterIP: None
  ports:
    - name: broker
      port: 9092
      targetPort: 9092
    - name: controller
      port: 9093
      targetPort: 9093
  selector:
    {{- include "jobradar.selectorLabels" (dict "component" "kafka") | nindent 4 }}
---
apiVersion: v1
kind: Service
metadata:
  name: kafka
  namespace: {{ .Values.namespace }}
  labels:
    {{- include "jobradar.labels" . | nindent 4 }}
    component: kafka
spec:
  type: ClusterIP
  ports:
    - name: broker
      port: 9092
      targetPort: 9092
  selector:
    {{- include "jobradar.selectorLabels" (dict "component" "kafka") | nindent 4 }}
```

- [ ] **Step 4: Validate**

```bash
helm template jobradar ./k8s/helm/jobradar -n jobradar-local -s templates/kafka/statefulset.yaml
```

Expected: StatefulSet YAML printed with `storageClassName: local-path`.

- [ ] **Step 5: Commit**

```bash
git add k8s/helm/jobradar/templates/kafka/
git commit -m "feat(helm): add kafka templates"
```

---

### Task 3: Redis templates

**Files:**
- Create: `k8s/helm/jobradar/templates/redis/statefulset.yaml`
- Create: `k8s/helm/jobradar/templates/redis/service.yaml`

- [ ] **Step 1: Create redis/statefulset.yaml**

Create `k8s/helm/jobradar/templates/redis/statefulset.yaml`:
```yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: redis
  namespace: {{ .Values.namespace }}
  labels:
    {{- include "jobradar.labels" . | nindent 4 }}
    component: redis
spec:
  serviceName: redis
  replicas: 1
  selector:
    matchLabels:
      {{- include "jobradar.selectorLabels" (dict "component" "redis") | nindent 6 }}
  template:
    metadata:
      labels:
        {{- include "jobradar.labels" . | nindent 8 }}
        component: redis
    spec:
      containers:
        - name: redis
          image: {{ .Values.images.redis.repository }}:{{ .Values.images.redis.tag }}
          imagePullPolicy: {{ .Values.images.redis.pullPolicy }}
          command: ["redis-server", "--save", "60", "1", "--loglevel", "warning"]
          ports:
            - containerPort: 6379
              name: redis
          resources:
            requests:
              memory: "64Mi"
              cpu: "50m"
            limits:
              memory: "256Mi"
              cpu: "200m"
          readinessProbe:
            exec:
              command: ["redis-cli", "ping"]
            initialDelaySeconds: 5
            periodSeconds: 10
            failureThreshold: 3
          livenessProbe:
            exec:
              command: ["redis-cli", "ping"]
            initialDelaySeconds: 10
            periodSeconds: 15
            failureThreshold: 3
          volumeMounts:
            - name: redis-data
              mountPath: /data
          securityContext:
            runAsNonRoot: true
            runAsUser: 999
            runAsGroup: 999
            readOnlyRootFilesystem: true
            allowPrivilegeEscalation: false
            capabilities:
              drop: ["ALL"]
      securityContext:
        runAsNonRoot: true
        fsGroup: 999
  volumeClaimTemplates:
    - metadata:
        name: redis-data
        labels:
          {{- include "jobradar.labels" . | nindent 10 }}
          component: redis
      spec:
        accessModes: ["ReadWriteOnce"]
        storageClassName: {{ .Values.redis.storageClass }}
        resources:
          requests:
            storage: {{ .Values.redis.storage }}
```

- [ ] **Step 2: Create redis/service.yaml**

Create `k8s/helm/jobradar/templates/redis/service.yaml`:
```yaml
apiVersion: v1
kind: Service
metadata:
  name: redis
  namespace: {{ .Values.namespace }}
  labels:
    {{- include "jobradar.labels" . | nindent 4 }}
    component: redis
spec:
  type: ClusterIP
  ports:
    - name: redis
      port: 6379
      targetPort: 6379
  selector:
    {{- include "jobradar.selectorLabels" (dict "component" "redis") | nindent 4 }}
```

- [ ] **Step 3: Validate**

```bash
helm template jobradar ./k8s/helm/jobradar -n jobradar-local -s templates/redis/statefulset.yaml
```

Expected: StatefulSet YAML with `storageClassName: local-path`.

- [ ] **Step 4: Commit**

```bash
git add k8s/helm/jobradar/templates/redis/
git commit -m "feat(helm): add redis templates"
```

---

### Task 4: API templates

**Files:**
- Create: `k8s/helm/jobradar/templates/api/configmap.yaml`
- Create: `k8s/helm/jobradar/templates/api/deployment.yaml`
- Create: `k8s/helm/jobradar/templates/api/service.yaml`
- Create: `k8s/helm/jobradar/templates/api/secret.yaml.example`

- [ ] **Step 1: Create api/configmap.yaml**

Create `k8s/helm/jobradar/templates/api/configmap.yaml`:
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: jobradar-api-config
  namespace: {{ .Values.namespace }}
  labels:
    {{- include "jobradar.labels" . | nindent 4 }}
    component: api
data:
  METRICS_ADDR: {{ .Values.api.metricsAddr | quote }}
  LOG_LEVEL: {{ .Values.api.logLevel | quote }}
  LOG_FORMAT: {{ .Values.api.logFormat | quote }}
  JAEGER_ENDPOINT: {{ .Values.api.jaegerEndpoint | quote }}
  DB_SSLMODE: {{ .Values.api.dbSslmode | quote }}
  DB_PORT: {{ .Values.api.dbPort | quote }}
  SUPABASE_PROJECT_URL: {{ .Values.api.supabaseProjectUrl | quote }}
  ALLOWED_EMAIL: {{ .Values.api.allowedEmail | quote }}
  JWT_CLOCK_SKEW_SECONDS: {{ .Values.api.jwtClockSkewSeconds | quote }}
  JWKS_CACHE_TTL_SECONDS: {{ .Values.api.jwksCacheTtlSeconds | quote }}
  API_MAX_BODY_BYTES: {{ .Values.api.apiMaxBodyBytes | quote }}
```

- [ ] **Step 2: Create api/deployment.yaml**

Create `k8s/helm/jobradar/templates/api/deployment.yaml`:
```yaml
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
    spec:
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
              memory: "256Mi"
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
            failureThreshold: 3
          securityContext:
            {{- include "jobradar.workerSecurityContext" . | nindent 12 }}
      securityContext:
        runAsNonRoot: true
```

- [ ] **Step 3: Create api/service.yaml**

Create `k8s/helm/jobradar/templates/api/service.yaml`:
```yaml
apiVersion: v1
kind: Service
metadata:
  name: jobradar-api
  namespace: {{ .Values.namespace }}
  labels:
    {{- include "jobradar.labels" . | nindent 4 }}
    component: api
spec:
  type: NodePort
  ports:
    - name: http
      port: 8080
      targetPort: 8080
      nodePort: {{ .Values.api.nodePort }}
    - name: metrics
      port: 9100
      targetPort: 9100
  selector:
    {{- include "jobradar.selectorLabels" (dict "component" "api") | nindent 4 }}
```

- [ ] **Step 4: Create api/secret.yaml.example**

Create `k8s/helm/jobradar/templates/api/secret.yaml.example`:
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: jobradar-api-secret
  namespace: jobradar-local
type: Opaque
stringData:
  DB_HOST: "your-supabase-host.supabase.co"
  DB_USER: "postgres"
  DB_PASSWORD: "your-db-password"
  DB_NAME: "postgres"
```

- [ ] **Step 5: Validate**

```bash
helm template jobradar ./k8s/helm/jobradar -n jobradar-local -s templates/api/deployment.yaml
```

Expected: Deployment with port 9100 named `metrics`, command `/app/api`.

- [ ] **Step 6: Commit**

```bash
git add k8s/helm/jobradar/templates/api/
git commit -m "feat(helm): add api templates"
```

---

### Task 5: Console templates

**Files:**
- Create: `k8s/helm/jobradar/templates/console/configmap.yaml`
- Create: `k8s/helm/jobradar/templates/console/deployment.yaml`
- Create: `k8s/helm/jobradar/templates/console/service.yaml`

- [ ] **Step 1: Create console/configmap.yaml**

Create `k8s/helm/jobradar/templates/console/configmap.yaml`:
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: jobradar-console-nginx
  namespace: {{ .Values.namespace }}
  labels:
    {{- include "jobradar.labels" . | nindent 4 }}
    component: console
data:
  default.conf: |
    server {
        listen 8080;
        root /usr/share/nginx/html;
        index index.html;

        location ~* \.(js|css|svg|png|jpg|jpeg|gif|ico|woff|woff2|ttf)$ {
            expires 30d;
            add_header Cache-Control "public, immutable";
        }

        location = /robots.txt {
            try_files $uri =404;
        }

        location /api/ {
            proxy_pass         http://jobradar-api:8080/;
            proxy_http_version 1.1;
            proxy_set_header   Host $host;
            proxy_set_header   X-Real-IP $remote_addr;
        }

        location / {
            try_files $uri $uri/ /index.html;
        }
    }
```

- [ ] **Step 2: Create console/deployment.yaml**

Create `k8s/helm/jobradar/templates/console/deployment.yaml`:
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: jobradar-console
  namespace: {{ .Values.namespace }}
  labels:
    {{- include "jobradar.labels" . | nindent 4 }}
    component: console
spec:
  replicas: 1
  selector:
    matchLabels:
      {{- include "jobradar.selectorLabels" (dict "component" "console") | nindent 6 }}
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxSurge: 1
      maxUnavailable: 0
  template:
    metadata:
      labels:
        {{- include "jobradar.labels" . | nindent 8 }}
        component: console
    spec:
      containers:
        - name: console
          image: {{ .Values.images.console.repository }}:{{ .Values.images.console.tag }}
          imagePullPolicy: {{ .Values.images.console.pullPolicy }}
          ports:
            - containerPort: 8080
              name: http
          resources:
            requests:
              memory: "32Mi"
              cpu: "25m"
            limits:
              memory: "64Mi"
              cpu: "100m"
          livenessProbe:
            httpGet:
              path: /
              port: 8080
            initialDelaySeconds: 5
            periodSeconds: 30
            failureThreshold: 3
          readinessProbe:
            httpGet:
              path: /
              port: 8080
            initialDelaySeconds: 3
            periodSeconds: 10
            failureThreshold: 3
          volumeMounts:
            - name: nginx-conf
              mountPath: /etc/nginx/conf.d/default.conf
              subPath: default.conf
          securityContext:
            runAsNonRoot: true
            runAsUser: 101
            runAsGroup: 101
            readOnlyRootFilesystem: false
            allowPrivilegeEscalation: false
            capabilities:
              drop: ["ALL"]
      volumes:
        - name: nginx-conf
          configMap:
            name: jobradar-console-nginx
      securityContext:
        runAsNonRoot: true
```

- [ ] **Step 3: Create console/service.yaml**

Create `k8s/helm/jobradar/templates/console/service.yaml`:
```yaml
apiVersion: v1
kind: Service
metadata:
  name: jobradar-console
  namespace: {{ .Values.namespace }}
  labels:
    {{- include "jobradar.labels" . | nindent 4 }}
    component: console
spec:
  type: NodePort
  ports:
    - name: http
      port: 8080
      targetPort: 8080
      nodePort: {{ .Values.console.nodePort }}
  selector:
    {{- include "jobradar.selectorLabels" (dict "component" "console") | nindent 4 }}
```

- [ ] **Step 4: Validate**

```bash
helm template jobradar ./k8s/helm/jobradar -n jobradar-local -s templates/console/service.yaml
```

Expected: NodePort service with `nodePort: 30300`.

- [ ] **Step 5: Commit**

```bash
git add k8s/helm/jobradar/templates/console/
git commit -m "feat(helm): add console templates"
```

---

### Task 6: Scheduler templates

**Files:**
- Create: `k8s/helm/jobradar/templates/scheduler/configmap.yaml`
- Create: `k8s/helm/jobradar/templates/scheduler/deployment.yaml`
- Create: `k8s/helm/jobradar/templates/scheduler/service.yaml`
- Create: `k8s/helm/jobradar/templates/scheduler/secret.yaml.example`

- [ ] **Step 1: Create scheduler/configmap.yaml**

Create `k8s/helm/jobradar/templates/scheduler/configmap.yaml`:
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: jobradar-scheduler-config
  namespace: {{ .Values.namespace }}
  labels:
    {{- include "jobradar.labels" . | nindent 4 }}
    component: scheduler
data:
  SCAN_INTERVAL_MINUTES: {{ .Values.scheduler.scanIntervalMinutes | quote }}
  SCHEDULER_CHECK_INTERVAL_SECONDS: {{ .Values.scheduler.checkIntervalSeconds | quote }}
  SCHEDULER_STUCK_TIMEOUT_MINUTES: {{ .Values.scheduler.stuckTimeoutMinutes | quote }}
  LOG_LEVEL: {{ .Values.scheduler.logLevel | quote }}
  LOG_FORMAT: {{ .Values.scheduler.logFormat | quote }}
  METRICS_ADDR: {{ .Values.scheduler.metricsAddr | quote }}
  JAEGER_ENDPOINT: {{ .Values.scheduler.jaegerEndpoint | quote }}
  DB_SSLMODE: {{ .Values.scheduler.dbSslmode | quote }}
  DB_PORT: {{ .Values.scheduler.dbPort | quote }}
```

- [ ] **Step 2: Create scheduler/deployment.yaml**

Create `k8s/helm/jobradar/templates/scheduler/deployment.yaml`:
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: jobradar-scheduler
  namespace: {{ .Values.namespace }}
  labels:
    {{- include "jobradar.labels" . | nindent 4 }}
    component: scheduler
spec:
  replicas: 1
  selector:
    matchLabels:
      {{- include "jobradar.selectorLabels" (dict "component" "scheduler") | nindent 6 }}
  strategy:
    type: Recreate
  template:
    metadata:
      labels:
        {{- include "jobradar.labels" . | nindent 8 }}
        component: scheduler
    spec:
      containers:
        - name: scheduler
          image: {{ .Values.images.backend.repository }}:{{ .Values.images.backend.tag }}
          imagePullPolicy: {{ .Values.images.backend.pullPolicy }}
          command: ["/app/scheduler"]
          envFrom:
            - configMapRef:
                name: jobradar-scheduler-config
            - secretRef:
                name: jobradar-scheduler-secret
          ports:
            - containerPort: 9101
              name: metrics
          resources:
            requests:
              memory: "32Mi"
              cpu: "50m"
            limits:
              memory: "128Mi"
              cpu: "200m"
          livenessProbe:
            httpGet:
              path: /health
              port: 9101
            initialDelaySeconds: 10
            periodSeconds: 30
            failureThreshold: 3
          readinessProbe:
            httpGet:
              path: /ready
              port: 9101
            initialDelaySeconds: 5
            periodSeconds: 10
            failureThreshold: 3
          securityContext:
            {{- include "jobradar.workerSecurityContext" . | nindent 12 }}
      securityContext:
        runAsNonRoot: true
```

- [ ] **Step 3: Create scheduler/service.yaml**

Create `k8s/helm/jobradar/templates/scheduler/service.yaml`:
```yaml
apiVersion: v1
kind: Service
metadata:
  name: jobradar-scheduler
  namespace: {{ .Values.namespace }}
  labels:
    {{- include "jobradar.labels" . | nindent 4 }}
    component: scheduler
spec:
  type: ClusterIP
  ports:
    - name: metrics
      port: 9101
      targetPort: 9101
  selector:
    {{- include "jobradar.selectorLabels" (dict "component" "scheduler") | nindent 4 }}
```

- [ ] **Step 4: Create scheduler/secret.yaml.example**

Create `k8s/helm/jobradar/templates/scheduler/secret.yaml.example`:
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: jobradar-scheduler-secret
  namespace: jobradar-local
type: Opaque
stringData:
  DB_HOST: "your-supabase-host.supabase.co"
  DB_USER: "postgres"
  DB_PASSWORD: "your-db-password"
  DB_NAME: "postgres"
```

- [ ] **Step 5: Validate**

```bash
helm template jobradar ./k8s/helm/jobradar -n jobradar-local -s templates/scheduler/deployment.yaml
```

Expected: Deployment with `command: ["/app/scheduler"]` and metrics port 9101.

- [ ] **Step 6: Commit**

```bash
git add k8s/helm/jobradar/templates/scheduler/
git commit -m "feat(helm): add scheduler templates"
```

---

### Task 7: Crawler templates

The crawler is special: it needs `SYS_ADMIN` capability and a `/dev/shm` tmpfs volume for Chrome.

**Files:**
- Create: `k8s/helm/jobradar/templates/crawler/configmap.yaml`
- Create: `k8s/helm/jobradar/templates/crawler/deployment.yaml`
- Create: `k8s/helm/jobradar/templates/crawler/service.yaml`
- Create: `k8s/helm/jobradar/templates/crawler/secret.yaml.example`

- [ ] **Step 1: Create crawler/configmap.yaml**

Create `k8s/helm/jobradar/templates/crawler/configmap.yaml`:
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: jobradar-crawler-config
  namespace: {{ .Values.namespace }}
  labels:
    {{- include "jobradar.labels" . | nindent 4 }}
    component: crawler
data:
  KAFKA_BROKERS: {{ .Values.crawler.kafkaBrokers | quote }}
  SCAN_COMPANY_DELAY_SECONDS: {{ .Values.crawler.scanCompanyDelaySeconds | quote }}
  CRAWLER_POLL_INTERVAL_SECONDS: {{ .Values.crawler.pollIntervalSeconds | quote }}
  CRAWLER_STUCK_TIMEOUT_SECONDS: {{ .Values.crawler.stuckTimeoutSeconds | quote }}
  CRAWLER_HEARTBEAT_INTERVAL_SECONDS: {{ .Values.crawler.heartbeatIntervalSeconds | quote }}
  GOOGLE_SCRAPER_MIN_DELAY: {{ .Values.crawler.googleMinDelay | quote }}
  GOOGLE_SCRAPER_MAX_DELAY: {{ .Values.crawler.googleMaxDelay | quote }}
  GOOGLE_SCRAPER_TIMEOUT_MS: {{ .Values.crawler.googleTimeoutMs | quote }}
  GOOGLE_SCRAPER_MAX_PAGES: {{ .Values.crawler.googleMaxPages | quote }}
  GOOGLE_SCRAPER_ENRICH_MIN: {{ .Values.crawler.googleEnrichMin | quote }}
  GOOGLE_SCRAPER_ENRICH_MAX: {{ .Values.crawler.googleEnrichMax | quote }}
  GOOGLE_SCRAPER_MAX_DETAIL_JOBS: {{ .Values.crawler.googleMaxDetailJobs | quote }}
  LOG_LEVEL: {{ .Values.crawler.logLevel | quote }}
  LOG_FORMAT: {{ .Values.crawler.logFormat | quote }}
  METRICS_ADDR: {{ .Values.crawler.metricsAddr | quote }}
  JAEGER_ENDPOINT: {{ .Values.crawler.jaegerEndpoint | quote }}
  DB_SSLMODE: {{ .Values.crawler.dbSslmode | quote }}
  DB_PORT: {{ .Values.crawler.dbPort | quote }}
```

- [ ] **Step 2: Create crawler/deployment.yaml**

Create `k8s/helm/jobradar/templates/crawler/deployment.yaml`:
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: jobradar-crawler
  namespace: {{ .Values.namespace }}
  labels:
    {{- include "jobradar.labels" . | nindent 4 }}
    component: crawler
spec:
  replicas: 1
  selector:
    matchLabels:
      {{- include "jobradar.selectorLabels" (dict "component" "crawler") | nindent 6 }}
  strategy:
    type: Recreate
  template:
    metadata:
      labels:
        {{- include "jobradar.labels" . | nindent 8 }}
        component: crawler
    spec:
      containers:
        - name: crawler
          image: {{ .Values.images.crawler.repository }}:{{ .Values.images.crawler.tag }}
          imagePullPolicy: {{ .Values.images.crawler.pullPolicy }}
          envFrom:
            - configMapRef:
                name: jobradar-crawler-config
            - secretRef:
                name: jobradar-crawler-secret
          ports:
            - containerPort: 9101
              name: metrics
          resources:
            requests:
              memory: "1Gi"
              cpu: "250m"
            limits:
              memory: "3Gi"
              cpu: "1000m"
          livenessProbe:
            httpGet:
              path: /health
              port: 9101
            initialDelaySeconds: 15
            periodSeconds: 30
            failureThreshold: 3
          readinessProbe:
            httpGet:
              path: /ready
              port: 9101
            initialDelaySeconds: 10
            periodSeconds: 15
            failureThreshold: 3
          securityContext:
            # Chrome (rod) needs SYS_ADMIN for user namespace sandboxing and
            # write access to /tmp for its temp profile.
            runAsNonRoot: false
            readOnlyRootFilesystem: false
            allowPrivilegeEscalation: false
            capabilities:
              drop: ["ALL"]
              add: ["SYS_ADMIN"]
          volumeMounts:
            - name: dshm
              mountPath: /dev/shm
      volumes:
        # Chrome uses /dev/shm. Default 64Mi causes renderer crashes.
        - name: dshm
          emptyDir:
            medium: Memory
            sizeLimit: "512Mi"
      securityContext:
        runAsNonRoot: false
```

- [ ] **Step 3: Create crawler/service.yaml**

Create `k8s/helm/jobradar/templates/crawler/service.yaml`:
```yaml
apiVersion: v1
kind: Service
metadata:
  name: jobradar-crawler
  namespace: {{ .Values.namespace }}
  labels:
    {{- include "jobradar.labels" . | nindent 4 }}
    component: crawler
spec:
  type: ClusterIP
  ports:
    - name: metrics
      port: 9101
      targetPort: 9101
  selector:
    {{- include "jobradar.selectorLabels" (dict "component" "crawler") | nindent 4 }}
```

- [ ] **Step 4: Create crawler/secret.yaml.example**

Create `k8s/helm/jobradar/templates/crawler/secret.yaml.example`:
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: jobradar-crawler-secret
  namespace: jobradar-local
type: Opaque
stringData:
  DB_HOST: "your-supabase-host.supabase.co"
  DB_USER: "postgres"
  DB_PASSWORD: "your-db-password"
  DB_NAME: "postgres"
```

- [ ] **Step 5: Validate**

```bash
helm template jobradar ./k8s/helm/jobradar -n jobradar-local -s templates/crawler/deployment.yaml
```

Expected: Deployment with `SYS_ADMIN` capability, `dshm` emptyDir volume, image `jobradar-crawler:local`.

- [ ] **Step 6: Commit**

```bash
git add k8s/helm/jobradar/templates/crawler/
git commit -m "feat(helm): add crawler templates"
```

---

### Task 8: Filter templates

**Files:**
- Create: `k8s/helm/jobradar/templates/filter/configmap.yaml`
- Create: `k8s/helm/jobradar/templates/filter/deployment.yaml`
- Create: `k8s/helm/jobradar/templates/filter/service.yaml`
- Create: `k8s/helm/jobradar/templates/filter/secret.yaml.example`

- [ ] **Step 1: Create filter/configmap.yaml**

Create `k8s/helm/jobradar/templates/filter/configmap.yaml`:
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: jobradar-filter-config
  namespace: {{ .Values.namespace }}
  labels:
    {{- include "jobradar.labels" . | nindent 4 }}
    component: filter
data:
  KAFKA_BROKERS: {{ .Values.filter.kafkaBrokers | quote }}
  REDIS_ADDR: {{ .Values.filter.redisAddr | quote }}
  REDIS_DB: {{ .Values.filter.redisDb | quote }}
  FILTER_BATCH_SIZE: {{ .Values.filter.batchSize | quote }}
  FILTER_FLUSH_TIMEOUT_SECONDS: {{ .Values.filter.flushTimeoutSeconds | quote }}
  FILTER_LLM_CALL_INTERVAL_SECONDS: {{ .Values.filter.llmCallIntervalSeconds | quote }}
  LLM_PROVIDER: {{ .Values.filter.llmProvider | quote }}
  LLM_COOLDOWN_BASE_SECONDS: {{ .Values.filter.cooldownBaseSeconds | quote }}
  LLM_COOLDOWN_MAX_SECONDS: {{ .Values.filter.cooldownMaxSeconds | quote }}
  LLM_COOLDOWN_JITTER: {{ .Values.filter.cooldownJitter | quote }}
  LOG_LEVEL: {{ .Values.filter.logLevel | quote }}
  LOG_FORMAT: {{ .Values.filter.logFormat | quote }}
  METRICS_ADDR: {{ .Values.filter.metricsAddr | quote }}
  JAEGER_ENDPOINT: {{ .Values.filter.jaegerEndpoint | quote }}
  DB_SSLMODE: {{ .Values.filter.dbSslmode | quote }}
  DB_PORT: {{ .Values.filter.dbPort | quote }}
```

- [ ] **Step 2: Create filter/deployment.yaml**

Create `k8s/helm/jobradar/templates/filter/deployment.yaml`:
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: jobradar-filter
  namespace: {{ .Values.namespace }}
  labels:
    {{- include "jobradar.labels" . | nindent 4 }}
    component: filter
spec:
  replicas: 1
  selector:
    matchLabels:
      {{- include "jobradar.selectorLabels" (dict "component" "filter") | nindent 6 }}
  strategy:
    type: Recreate
  template:
    metadata:
      labels:
        {{- include "jobradar.labels" . | nindent 8 }}
        component: filter
    spec:
      containers:
        - name: filter
          image: {{ .Values.images.backend.repository }}:{{ .Values.images.backend.tag }}
          imagePullPolicy: {{ .Values.images.backend.pullPolicy }}
          command: ["/app/filter"]
          envFrom:
            - configMapRef:
                name: jobradar-filter-config
            - secretRef:
                name: jobradar-filter-secret
          ports:
            - containerPort: 9101
              name: metrics
          resources:
            requests:
              memory: "64Mi"
              cpu: "100m"
            limits:
              memory: "256Mi"
              cpu: "500m"
          livenessProbe:
            httpGet:
              path: /health
              port: 9101
            initialDelaySeconds: 10
            periodSeconds: 30
            failureThreshold: 3
          readinessProbe:
            httpGet:
              path: /ready
              port: 9101
            initialDelaySeconds: 5
            periodSeconds: 10
            failureThreshold: 3
          securityContext:
            {{- include "jobradar.workerSecurityContext" . | nindent 12 }}
      securityContext:
        runAsNonRoot: true
```

- [ ] **Step 3: Create filter/service.yaml**

Create `k8s/helm/jobradar/templates/filter/service.yaml`:
```yaml
apiVersion: v1
kind: Service
metadata:
  name: jobradar-filter
  namespace: {{ .Values.namespace }}
  labels:
    {{- include "jobradar.labels" . | nindent 4 }}
    component: filter
spec:
  type: ClusterIP
  ports:
    - name: metrics
      port: 9101
      targetPort: 9101
  selector:
    {{- include "jobradar.selectorLabels" (dict "component" "filter") | nindent 4 }}
```

- [ ] **Step 4: Create filter/secret.yaml.example**

Create `k8s/helm/jobradar/templates/filter/secret.yaml.example`:
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: jobradar-filter-secret
  namespace: jobradar-local
type: Opaque
stringData:
  DB_HOST: "your-supabase-host.supabase.co"
  DB_USER: "postgres"
  DB_PASSWORD: "your-db-password"
  DB_NAME: "postgres"
  GEMINI_API_KEY: "your-gemini-api-key"
```

- [ ] **Step 5: Validate**

```bash
helm template jobradar ./k8s/helm/jobradar -n jobradar-local -s templates/filter/deployment.yaml
```

Expected: Deployment with `command: ["/app/filter"]`.

- [ ] **Step 6: Commit**

```bash
git add k8s/helm/jobradar/templates/filter/
git commit -m "feat(helm): add filter templates"
```

---

### Task 9: Enricher templates

**Files:**
- Create: `k8s/helm/jobradar/templates/enricher/configmap.yaml`
- Create: `k8s/helm/jobradar/templates/enricher/deployment.yaml`
- Create: `k8s/helm/jobradar/templates/enricher/service.yaml`
- Create: `k8s/helm/jobradar/templates/enricher/secret.yaml.example`

- [ ] **Step 1: Create enricher/configmap.yaml**

Create `k8s/helm/jobradar/templates/enricher/configmap.yaml`:
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: jobradar-enricher-config
  namespace: {{ .Values.namespace }}
  labels:
    {{- include "jobradar.labels" . | nindent 4 }}
    component: enricher
data:
  KAFKA_BROKERS: {{ .Values.enricher.kafkaBrokers | quote }}
  REDIS_ADDR: {{ .Values.enricher.redisAddr | quote }}
  REDIS_DB: {{ .Values.enricher.redisDb | quote }}
  ENRICHER_FETCH_CONCURRENCY: {{ .Values.enricher.fetchConcurrency | quote }}
  ENRICHER_LLM_BATCH_SIZE: {{ .Values.enricher.llmBatchSize | quote }}
  ENRICHER_FLUSH_TIMEOUT_SECONDS: {{ .Values.enricher.flushTimeoutSeconds | quote }}
  ENRICHER_LLM_CALL_INTERVAL_SECONDS: {{ .Values.enricher.llmCallIntervalSeconds | quote }}
  ENRICHER_DEFAULT_RPS: {{ .Values.enricher.defaultRps | quote }}
  ENRICHER_DEFAULT_BURST: {{ .Values.enricher.defaultBurst | quote }}
  ENRICHER_JITTER_MS: {{ .Values.enricher.jitterMs | quote }}
  DISCARD_JOB_WITH_EXP_GT: {{ .Values.enricher.discardJobWithExpGt | quote }}
  LLM_PROVIDER: {{ .Values.enricher.llmProvider | quote }}
  LLM_COOLDOWN_BASE_SECONDS: {{ .Values.enricher.cooldownBaseSeconds | quote }}
  LLM_COOLDOWN_MAX_SECONDS: {{ .Values.enricher.cooldownMaxSeconds | quote }}
  LLM_COOLDOWN_JITTER: {{ .Values.enricher.cooldownJitter | quote }}
  LOG_LEVEL: {{ .Values.enricher.logLevel | quote }}
  LOG_FORMAT: {{ .Values.enricher.logFormat | quote }}
  METRICS_ADDR: {{ .Values.enricher.metricsAddr | quote }}
  JAEGER_ENDPOINT: {{ .Values.enricher.jaegerEndpoint | quote }}
  DB_SSLMODE: {{ .Values.enricher.dbSslmode | quote }}
  DB_PORT: {{ .Values.enricher.dbPort | quote }}
```

- [ ] **Step 2: Create enricher/deployment.yaml**

Create `k8s/helm/jobradar/templates/enricher/deployment.yaml`:
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: jobradar-enricher
  namespace: {{ .Values.namespace }}
  labels:
    {{- include "jobradar.labels" . | nindent 4 }}
    component: enricher
spec:
  replicas: 1
  selector:
    matchLabels:
      {{- include "jobradar.selectorLabels" (dict "component" "enricher") | nindent 6 }}
  strategy:
    type: Recreate
  template:
    metadata:
      labels:
        {{- include "jobradar.labels" . | nindent 8 }}
        component: enricher
    spec:
      containers:
        - name: enricher
          image: {{ .Values.images.backend.repository }}:{{ .Values.images.backend.tag }}
          imagePullPolicy: {{ .Values.images.backend.pullPolicy }}
          command: ["/app/enricher"]
          envFrom:
            - configMapRef:
                name: jobradar-enricher-config
            - secretRef:
                name: jobradar-enricher-secret
          ports:
            - containerPort: 9101
              name: metrics
          resources:
            requests:
              memory: "64Mi"
              cpu: "100m"
            limits:
              memory: "256Mi"
              cpu: "500m"
          livenessProbe:
            httpGet:
              path: /health
              port: 9101
            initialDelaySeconds: 10
            periodSeconds: 30
            failureThreshold: 3
          readinessProbe:
            httpGet:
              path: /ready
              port: 9101
            initialDelaySeconds: 5
            periodSeconds: 10
            failureThreshold: 3
          securityContext:
            {{- include "jobradar.workerSecurityContext" . | nindent 12 }}
      securityContext:
        runAsNonRoot: true
```

- [ ] **Step 3: Create enricher/service.yaml**

Create `k8s/helm/jobradar/templates/enricher/service.yaml`:
```yaml
apiVersion: v1
kind: Service
metadata:
  name: jobradar-enricher
  namespace: {{ .Values.namespace }}
  labels:
    {{- include "jobradar.labels" . | nindent 4 }}
    component: enricher
spec:
  type: ClusterIP
  ports:
    - name: metrics
      port: 9101
      targetPort: 9101
  selector:
    {{- include "jobradar.selectorLabels" (dict "component" "enricher") | nindent 4 }}
```

- [ ] **Step 4: Create enricher/secret.yaml.example**

Create `k8s/helm/jobradar/templates/enricher/secret.yaml.example`:
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: jobradar-enricher-secret
  namespace: jobradar-local
type: Opaque
stringData:
  DB_HOST: "your-supabase-host.supabase.co"
  DB_USER: "postgres"
  DB_PASSWORD: "your-db-password"
  DB_NAME: "postgres"
  GEMINI_API_KEY: "your-gemini-api-key"
```

- [ ] **Step 5: Validate**

```bash
helm template jobradar ./k8s/helm/jobradar -n jobradar-local -s templates/enricher/deployment.yaml
```

Expected: Deployment with `command: ["/app/enricher"]`, name `jobradar-enricher`.

- [ ] **Step 6: Commit**

```bash
git add k8s/helm/jobradar/templates/enricher/
git commit -m "feat(helm): add enricher templates"
```

---

### Task 10: Migrate Job template

**Files:**
- Create: `k8s/helm/jobradar/templates/migrate/job.yaml`

- [ ] **Step 1: Create migrate/job.yaml**

Create `k8s/helm/jobradar/templates/migrate/job.yaml`:
```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: jobradar-migrate
  namespace: {{ .Values.namespace }}
  labels:
    {{- include "jobradar.labels" . | nindent 4 }}
    component: migrate
spec:
  ttlSecondsAfterFinished: 600
  backoffLimit: 3
  template:
    metadata:
      labels:
        {{- include "jobradar.labels" . | nindent 8 }}
        component: migrate
    spec:
      restartPolicy: Never
      containers:
        - name: migrate
          image: {{ .Values.images.backend.repository }}:{{ .Values.images.backend.tag }}
          imagePullPolicy: {{ .Values.images.backend.pullPolicy }}
          command: ["/app/migrate", "/app/migrations"]
          envFrom:
            - secretRef:
                name: jobradar-migrate-secret
          env:
            - name: DB_SSLMODE
              value: "require"
            - name: DB_PORT
              value: "5432"
            - name: LOG_FORMAT
              value: "json"
          resources:
            requests:
              memory: "32Mi"
              cpu: "50m"
            limits:
              memory: "128Mi"
              cpu: "200m"
          securityContext:
            {{- include "jobradar.workerSecurityContext" . | nindent 12 }}
      securityContext:
        runAsNonRoot: true
```

- [ ] **Step 2: Validate full chart renders cleanly**

```bash
helm template jobradar ./k8s/helm/jobradar -n jobradar-local | kubectl apply --dry-run=client -f -
```

Expected: All resources validated, no errors. Output lists ~20 objects.

- [ ] **Step 3: Commit**

```bash
git add k8s/helm/jobradar/templates/migrate/
git commit -m "feat(helm): add migrate job template"
```

---

### Task 11: Deploy jobradar chart, validate, remove old manifests

This task swaps the cluster from raw manifests to the Helm chart. PVCs are preserved (not deleted) so Kafka and Redis data survives.

- [ ] **Step 1: Tear down existing raw-manifest resources (keep PVCs)**

```bash
kubectl delete deployment,statefulset,service,configmap,job \
  -l app=jobradar -n jobradar-local \
  --ignore-not-found=true
```

Expected: All existing Deployments, StatefulSets, Services, ConfigMaps, and Jobs deleted. PVCs (`kafka-data-kafka-0`, `redis-data-redis-0`) still exist.

- [ ] **Step 2: Confirm PVCs survived**

```bash
kubectl get pvc -n jobradar-local
```

Expected: `kafka-data-kafka-0` and `redis-data-redis-0` still listed.

- [ ] **Step 3: Install jobradar Helm chart**

```bash
helm install jobradar ./k8s/helm/jobradar -n jobradar-local
```

Expected: `STATUS: deployed` and a list of created resources.

- [ ] **Step 4: Wait for all pods to become Ready**

```bash
kubectl rollout status deployment/jobradar-api -n jobradar-local --timeout=120s
kubectl rollout status deployment/jobradar-console -n jobradar-local --timeout=120s
kubectl rollout status deployment/jobradar-scheduler -n jobradar-local --timeout=120s
kubectl rollout status deployment/jobradar-crawler -n jobradar-local --timeout=120s
kubectl rollout status deployment/jobradar-filter -n jobradar-local --timeout=120s
kubectl rollout status deployment/jobradar-enricher -n jobradar-local --timeout=120s
kubectl rollout status statefulset/kafka -n jobradar-local --timeout=120s
kubectl rollout status statefulset/redis -n jobradar-local --timeout=120s
```

Expected: All report `successfully rolled out`.

- [ ] **Step 5: Verify API is reachable**

```bash
curl -s http://localhost:30080/health
```

Expected: HTTP 200 response.

- [ ] **Step 6: Delete old raw manifest files**

```bash
rm -rf k8s/analyzer k8s/api k8s/console k8s/crawler k8s/filter k8s/kafka k8s/migrate k8s/redis k8s/scheduler
```

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "feat(helm): deploy jobradar chart, remove legacy raw manifests"
```

---

## Phase 2: observability Chart

### Task 12: Scaffold observability chart

**Files:**
- Create: `k8s/helm/observability/Chart.yaml`
- Create: `k8s/helm/observability/values.yaml`
- Create: `k8s/helm/observability/templates/_helpers.tpl`

- [ ] **Step 1: Add required Helm repos**

```bash
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo add grafana https://grafana.github.io/helm-charts
helm repo add jaegertracing https://jaegertracing.github.io/helm-charts
helm repo update
```

Expected: `Update Complete. ⎈Happy Helming!⎈`

- [ ] **Step 2: Create observability/Chart.yaml**

Create `k8s/helm/observability/Chart.yaml`:
```yaml
apiVersion: v2
name: observability
description: JobRadar observability stack — Prometheus, Grafana, Loki, Jaeger
type: application
version: 0.1.0
dependencies:
  - name: kube-prometheus-stack
    version: "58.x"
    repository: https://prometheus-community.github.io/helm-charts
  - name: loki-stack
    version: "2.x"
    repository: https://grafana.github.io/helm-charts
  - name: jaeger
    version: "3.x"
    repository: https://jaegertracing.github.io/helm-charts
```

- [ ] **Step 3: Fetch exact latest chart versions and pin them**

```bash
helm search repo prometheus-community/kube-prometheus-stack --versions | head -3
helm search repo grafana/loki-stack --versions | head -3
helm search repo jaegertracing/jaeger --versions | head -3
```

Update `Chart.yaml` with the exact `x.y.z` versions returned (replace `58.x`, `2.x`, `3.x` with the actual latest patch versions).

- [ ] **Step 4: Create observability/templates/_helpers.tpl**

Create `k8s/helm/observability/templates/_helpers.tpl`:
```
{{- define "observability.labels" -}}
app: jobradar-obs
environment: local
{{- end }}
```

- [ ] **Step 5: Pull chart dependencies**

```bash
cd k8s/helm/observability && helm dependency update && cd -
```

Expected: `charts/` directory created with three `.tgz` files.

- [ ] **Step 6: Commit**

```bash
git add k8s/helm/observability/
git commit -m "feat(helm): scaffold observability chart with dependencies"
```

---

### Task 13: Configure kube-prometheus-stack values

This configures Prometheus (scrape targets) and Grafana (Loki datasource, dashboard provisioning, NodePort access).

**Files:**
- Modify: `k8s/helm/observability/values.yaml`

- [ ] **Step 1: Create values.yaml with kube-prometheus-stack config**

Create `k8s/helm/observability/values.yaml`:
```yaml
namespace: jobradar-local

kube-prometheus-stack:
  # Disable heavyweight components not needed for local dev.
  alertmanager:
    enabled: false
  nodeExporter:
    enabled: false
  kubeStateMetrics:
    enabled: true
  prometheusOperator:
    enabled: true

  prometheus:
    prometheusSpec:
      # Scrape app metrics endpoints via static configs.
      additionalScrapeConfigs:
        - job_name: jobradar_api
          static_configs:
            - targets: ['jobradar-api.jobradar-local.svc.cluster.local:9100']
        - job_name: jobradar_scheduler
          static_configs:
            - targets: ['jobradar-scheduler.jobradar-local.svc.cluster.local:9101']
        - job_name: jobradar_crawler
          static_configs:
            - targets: ['jobradar-crawler.jobradar-local.svc.cluster.local:9101']
        - job_name: jobradar_filter
          static_configs:
            - targets: ['jobradar-filter.jobradar-local.svc.cluster.local:9101']
        - job_name: jobradar_enricher
          static_configs:
            - targets: ['jobradar-enricher.jobradar-local.svc.cluster.local:9101']
      storageSpec:
        volumeClaimTemplate:
          spec:
            storageClassName: local-path
            accessModes: ["ReadWriteOnce"]
            resources:
              requests:
                storage: 10Gi

  grafana:
    service:
      type: NodePort
      nodePort: 3001
    # Loki datasource — must be wired manually (not auto-wired like Prometheus).
    additionalDataSources:
      - name: Loki
        type: loki
        url: http://observability-loki-stack:3100
        access: proxy
        isDefault: false
    # Dashboard provisioning from ConfigMap built in Task 14.
    dashboardProviders:
      dashboardproviders.yaml:
        apiVersion: 1
        providers:
          - name: jobradar
            orgId: 1
            folder: JobRadar
            type: file
            disableDeletion: false
            options:
              path: /var/lib/grafana/dashboards/jobradar
    dashboardsConfigMaps:
      jobradar: jobradar-grafana-dashboards
    adminPassword: "admin"
    grafana.ini:
      users:
        allow_sign_up: false

loki-stack:
  loki:
    enabled: true
    persistence:
      enabled: true
      storageClassName: local-path
      size: 5Gi
    config:
      limits_config:
        retention_period: 72h
  promtail:
    enabled: true
    config:
      clients:
        - url: http://observability-loki-stack:3100/loki/api/v1/push

jaeger:
  allInOne:
    enabled: true
  storage:
    type: memory
  agent:
    enabled: false
  collector:
    enabled: false
  query:
    enabled: false
  provisionDataStore:
    cassandra: false
  ingress:
    enabled: false
  allInOne:
    enabled: true
    extraEnv:
      - name: COLLECTOR_OTLP_ENABLED
        value: "true"
  service:
    type: NodePort
    nodePort: 16686
    otlpGrpc:
      port: 4317
    otlpHttp:
      port: 4318

supabaseAuditSync:
  image:
    repository: jobradar-supabase-audit-sync
    tag: local
    pullPolicy: IfNotPresent
  lokiUrl: "http://observability-loki-stack:3100/loki/api/v1/push"
  intervalSeconds: "900"
  stateFile: "/var/lib/audit-sync/state.json"
  storage: 100Mi
  storageClass: local-path
```

- [ ] **Step 2: Validate chart templates render**

```bash
helm template obs ./k8s/helm/observability -n jobradar-local | head -50
```

Expected: YAML output, no template errors.

- [ ] **Step 3: Commit**

```bash
git add k8s/helm/observability/values.yaml
git commit -m "feat(helm): add observability values — prometheus, grafana, loki, jaeger config"
```

---

### Task 14: Grafana dashboards ConfigMap template

The four dashboard JSON files in `infra/grafana/dashboards/` are embedded into a ConfigMap so Grafana auto-provisions them on startup.

**Files:**
- Create: `k8s/helm/observability/templates/grafana-dashboards.yaml`

- [ ] **Step 1: Create grafana-dashboards.yaml**

Create `k8s/helm/observability/templates/grafana-dashboards.yaml`:
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: jobradar-grafana-dashboards
  namespace: {{ .Values.namespace }}
  labels:
    {{- include "observability.labels" . | nindent 4 }}
    grafana_dashboard: "1"
data:
  auth-events.json: |
{{ .Files.Get "dashboards/auth-events.json" | indent 4 }}
  jobradar-overview.json: |
{{ .Files.Get "dashboards/jobradar-overview.json" | indent 4 }}
  security-posture.json: |
{{ .Files.Get "dashboards/security-posture.json" | indent 4 }}
  tls-health.json: |
{{ .Files.Get "dashboards/tls-health.json" | indent 4 }}
```

- [ ] **Step 2: Copy dashboard JSON files into the chart**

Helm's `.Files.Get` can only access files within the chart directory. Copy the dashboards in:

```bash
mkdir -p k8s/helm/observability/dashboards
cp infra/grafana/dashboards/*.json k8s/helm/observability/dashboards/
```

- [ ] **Step 3: Validate ConfigMap renders with dashboard content**

```bash
helm template obs ./k8s/helm/observability -n jobradar-local \
  -s templates/grafana-dashboards.yaml | head -30
```

Expected: ConfigMap with `data:` keys containing JSON content.

- [ ] **Step 4: Commit**

```bash
git add k8s/helm/observability/templates/grafana-dashboards.yaml k8s/helm/observability/dashboards/
git commit -m "feat(helm): provision Grafana dashboards via ConfigMap"
```

---

### Task 15: supabase-audit-sync templates

**Files:**
- Create: `k8s/helm/observability/templates/supabase-audit-sync/deployment.yaml`
- Create: `k8s/helm/observability/templates/supabase-audit-sync/secret.yaml.example`

- [ ] **Step 1: Create supabase-audit-sync/deployment.yaml**

Create `k8s/helm/observability/templates/supabase-audit-sync/deployment.yaml`:
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: supabase-audit-sync
  namespace: {{ .Values.namespace }}
  labels:
    {{- include "observability.labels" . | nindent 4 }}
    component: supabase-audit-sync
spec:
  replicas: 1
  selector:
    matchLabels:
      app: jobradar-obs
      component: supabase-audit-sync
  strategy:
    type: Recreate
  template:
    metadata:
      labels:
        app: jobradar-obs
        component: supabase-audit-sync
    spec:
      containers:
        - name: supabase-audit-sync
          image: {{ .Values.supabaseAuditSync.image.repository }}:{{ .Values.supabaseAuditSync.image.tag }}
          imagePullPolicy: {{ .Values.supabaseAuditSync.image.pullPolicy }}
          env:
            - name: SUPABASE_DB_URL
              valueFrom:
                secretKeyRef:
                  name: supabase-audit-sync-secret
                  key: SUPABASE_DB_URL
            - name: LOKI_URL
              value: {{ .Values.supabaseAuditSync.lokiUrl | quote }}
            - name: AUDIT_SYNC_INTERVAL_SECONDS
              value: {{ .Values.supabaseAuditSync.intervalSeconds | quote }}
            - name: AUDIT_SYNC_STATE_FILE
              value: {{ .Values.supabaseAuditSync.stateFile | quote }}
          volumeMounts:
            - name: audit-state
              mountPath: /var/lib/audit-sync
          resources:
            requests:
              memory: "32Mi"
              cpu: "25m"
            limits:
              memory: "64Mi"
              cpu: "100m"
          securityContext:
            runAsNonRoot: true
            runAsUser: 65532
            runAsGroup: 65532
            readOnlyRootFilesystem: true
            allowPrivilegeEscalation: false
            capabilities:
              drop: ["ALL"]
      volumes:
        - name: audit-state
          persistentVolumeClaim:
            claimName: supabase-audit-sync-state
      securityContext:
        runAsNonRoot: true
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: supabase-audit-sync-state
  namespace: {{ .Values.namespace }}
  labels:
    {{- include "observability.labels" . | nindent 4 }}
    component: supabase-audit-sync
spec:
  accessModes: ["ReadWriteOnce"]
  storageClassName: {{ .Values.supabaseAuditSync.storageClass }}
  resources:
    requests:
      storage: {{ .Values.supabaseAuditSync.storage }}
```

- [ ] **Step 2: Create supabase-audit-sync/secret.yaml.example**

Create `k8s/helm/observability/templates/supabase-audit-sync/secret.yaml.example`:
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: supabase-audit-sync-secret
  namespace: jobradar-local
type: Opaque
stringData:
  SUPABASE_DB_URL: "postgres://postgres:your-password@your-supabase-host.supabase.co:5432/postgres"
```

- [ ] **Step 3: Apply the secret (before helm install)**

Copy the example and fill in your real Supabase DB URL:

```bash
cp k8s/helm/observability/templates/supabase-audit-sync/secret.yaml.example /tmp/audit-secret.yaml
# Edit /tmp/audit-secret.yaml with real credentials
kubectl apply -f /tmp/audit-secret.yaml -n jobradar-local
```

- [ ] **Step 4: Validate template**

```bash
helm template obs ./k8s/helm/observability -n jobradar-local \
  -s templates/supabase-audit-sync/deployment.yaml
```

Expected: Deployment + PVC YAML with `supabase-audit-sync-secret` secretKeyRef.

- [ ] **Step 5: Commit**

```bash
git add k8s/helm/observability/templates/supabase-audit-sync/
git commit -m "feat(helm): add supabase-audit-sync deployment template"
```

---

### Task 16: Deploy observability chart and validate

- [ ] **Step 1: Install observability chart**

```bash
helm install obs ./k8s/helm/observability -n jobradar-local
```

Expected: `STATUS: deployed`.

- [ ] **Step 2: Wait for Grafana and Prometheus to become Ready**

```bash
kubectl rollout status deployment/obs-grafana -n jobradar-local --timeout=180s
kubectl rollout status statefulset/prometheus-obs-kube-prometheus-stack-prometheus -n jobradar-local --timeout=180s
```

Expected: Both report `successfully rolled out`.

- [ ] **Step 3: Wait for Loki**

```bash
kubectl rollout status statefulset/obs-loki-stack -n jobradar-local --timeout=120s
```

- [ ] **Step 4: Verify Grafana is reachable**

Open `http://localhost:3001` in your browser. Login with `admin` / `admin`.

Expected: Grafana UI loads. Left sidebar → Dashboards → JobRadar folder shows 4 dashboards.

- [ ] **Step 5: Verify Prometheus scrapes app targets**

In Grafana: Explore → Prometheus → run `up{job=~"jobradar.*"}`.

Expected: 5 series returned (api, scheduler, crawler, filter, enricher), all with value `1`.

- [ ] **Step 6: Verify Loki receives logs**

In Grafana: Explore → Loki → run `{namespace="jobradar-local"}`.

Expected: Log lines from jobradar pods appear.

- [ ] **Step 7: Verify Jaeger UI**

Open `http://localhost:16686`. Select a service from the dropdown.

Expected: Jaeger UI loads and shows services.

- [ ] **Step 8: Commit final state**

```bash
git add -A
git commit -m "feat(helm): deploy full observability stack — prometheus, grafana, loki, jaeger, audit-sync"
```
