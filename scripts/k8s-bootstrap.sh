#!/usr/bin/env bash
# k8s-bootstrap.sh — one-shot setup for a fresh machine or clean cluster.
# Validates all required files, env vars, and credential files, then creates
# the cluster and every k8s secret. Run once before "make k8s-build && make k8s-up".
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

NS="jobradar-local"
CLUSTER="jobradar"
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; BOLD='\033[1m'; NC='\033[0m'

ok()   { echo -e "${GREEN}  ✓${NC} $*"; }
warn() { echo -e "${YELLOW}  !${NC} $*"; }
fail() { echo -e "${RED}  ✗${NC} $*"; ERRORS=$((ERRORS + 1)); }

ERRORS=0

# ── 0. Fetch secrets from Doppler ─────────────────────────────────────────────
# All secrets live in Doppler (job-radar / dev). We download them once to a
# temp file and clean up on exit — nothing sensitive ever touches disk long-term.
DOPPLER_CONFIG="${DOPPLER_CONFIG:-dev}"
DOPPLER_ENV="$(mktemp)"
trap "rm -f '$DOPPLER_ENV'" EXIT

echo -e "\n${BOLD}[0/5] Fetching secrets from Doppler (job-radar / $DOPPLER_CONFIG)${NC}"
if ! command -v doppler >/dev/null 2>&1; then
  echo -e "${RED}  ✗ doppler CLI not found.${NC} Install: https://docs.doppler.com/docs/install-cli"
  exit 1
fi
if ! doppler secrets download \
    --project job-radar \
    --config "$DOPPLER_CONFIG" \
    --format env-no-quotes \
    --no-file > "$DOPPLER_ENV" 2>/dev/null; then
  echo -e "${RED}  ✗ Doppler fetch failed.${NC} Run: doppler login"
  exit 1
fi
ok "Secrets downloaded ($(wc -l < "$DOPPLER_ENV") vars)"

# Read a value from the Doppler env snapshot.
env_val() {
  local key="$1"
  grep -E "^${key}=" "$DOPPLER_ENV" 2>/dev/null | head -1 | cut -d= -f2-
}

# ── 1. Required files ─────────────────────────────────────────────────────────
echo -e "\n${BOLD}[1/5] Checking required files${NC}"

CREDS_FILE="$REPO_ROOT/secrets/jobradar_NATS.creds"
VALUES_LOCAL="$REPO_ROOT/k8s/helm/jobradar/values.local.yaml"

check_file() {
  local file="$1" label="$2"
  if [ ! -f "$file" ]; then
    fail "$label not found: $file"
  elif [ ! -s "$file" ]; then
    fail "$label is empty: $file"
  else
    ok "$label"
  fi
}

check_file "$CREDS_FILE"   "secrets/jobradar_NATS.creds"
check_file "$VALUES_LOCAL" "k8s/helm/jobradar/values.local.yaml"

# ── 2. Required secrets in Doppler ────────────────────────────────────────────
echo -e "\n${BOLD}[2/5] Checking required secrets in Doppler${NC}"

REQUIRED_VARS=(
  DB_HOST
  DB_USER
  DB_PASSWORD
  DB_NAME
  REDIS_URL
  GEMINI_KEY
  GEMINI_KEY_JP
  CRAWLER_TOKEN_SECRET
  CRAWLER_TOKEN
  CLERK_JWKS_URL
  CLERK_ISSUER_URL
  CLERK_SECRET_KEY
  OCI_S3_ENDPOINT
  OCI_S3_REGION
  OCI_S3_BUCKET
  OCI_S3_ACCESS_KEY
  OCI_S3_SECRET_KEY
)

for var in "${REQUIRED_VARS[@]}"; do
  val="$(env_val "$var")"
  if [ -z "$val" ]; then
    fail "$var is not set in Doppler (job-radar / $DOPPLER_CONFIG)"
  else
    ok "$var"
  fi
done

# ── 3. Required values.local.yaml fields ─────────────────────────────────────
echo -e "\n${BOLD}[3/5] Checking k8s/helm/jobradar/values.local.yaml${NC}"

check_yaml_key() {
  local key="$1"
  if grep -q "$key" "$VALUES_LOCAL" 2>/dev/null; then
    ok "$key present"
  else
    fail "$key not set in k8s/helm/jobradar/values.local.yaml"
  fi
}

check_yaml_key "natsCredsFile"
check_yaml_key "clerkJwksUrl"

# ── 4. Abort on any failure ───────────────────────────────────────────────────
if [ "$ERRORS" -gt 0 ]; then
  echo -e "\n${RED}${BOLD}Bootstrap aborted — $ERRORS check(s) failed above.${NC}"
  echo -e "Fix the issues then re-run: ${BOLD}make k8s-bootstrap${NC}\n"
  exit 1
fi

echo -e "\n${GREEN}${BOLD}All checks passed.${NC}"

# ── 5. Cluster ────────────────────────────────────────────────────────────────
echo -e "\n${BOLD}[4/5] Cluster setup${NC}"

if k3d cluster list 2>/dev/null | grep -q "^$CLUSTER"; then
  ok "k3d cluster '$CLUSTER' already exists"
else
  echo "  Creating k3d cluster '$CLUSTER'..."
  k3d cluster create "$CLUSTER" \
    --port "30080:30080@loadbalancer" \
    --port "30300:30300@loadbalancer"
  ok "k3d cluster '$CLUSTER' created"
fi

kubectl config use-context "k3d-$CLUSTER" >/dev/null
ok "kubectl context → k3d-$CLUSTER"

# Ensure namespace exists and is Helm-managed
if ! kubectl get namespace "$NS" >/dev/null 2>&1; then
  kubectl create namespace "$NS" >/dev/null
  ok "Namespace '$NS' created"
fi

if ! kubectl get namespace "$NS" \
    -o jsonpath='{.metadata.labels.app\.kubernetes\.io/managed-by}' 2>/dev/null \
    | grep -q "Helm"; then
  kubectl label namespace "$NS" app.kubernetes.io/managed-by=Helm --overwrite >/dev/null
  kubectl annotate namespace "$NS" \
    meta.helm.sh/release-name=jobradar \
    meta.helm.sh/release-namespace="$NS" \
    --overwrite >/dev/null
  ok "Namespace '$NS' adopted into Helm"
else
  ok "Namespace '$NS' already Helm-managed"
fi

# ── 6. Secrets ────────────────────────────────────────────────────────────────
echo -e "\n${BOLD}[5/5] Creating k8s secrets${NC}"

apply_secret() {
  local name="$1"; shift
  kubectl create secret generic "$name" "$@" \
    -n "$NS" --dry-run=client -o yaml | kubectl apply -f - >/dev/null
  ok "secret/$name"
}

DB_HOST="$(env_val DB_HOST)"
DB_USER="$(env_val DB_USER)"
DB_PASSWORD="$(env_val DB_PASSWORD)"
DB_NAME="$(env_val DB_NAME)"
REDIS_URL="$(env_val REDIS_URL)"
GEMINI_KEY="$(env_val GEMINI_KEY)"
GEMINI_KEY_JP="$(env_val GEMINI_KEY_JP)"
CLERK_JWKS_URL="$(env_val CLERK_JWKS_URL)"
CLERK_ISSUER_URL="$(env_val CLERK_ISSUER_URL)"
CLERK_SECRET_KEY="$(env_val CLERK_SECRET_KEY)"
CRAWLER_TOKEN_SECRET="$(env_val CRAWLER_TOKEN_SECRET)"
CRAWLER_TOKEN="$(env_val CRAWLER_TOKEN)"
OCI_S3_ENDPOINT="$(env_val OCI_S3_ENDPOINT)"
OCI_S3_REGION="$(env_val OCI_S3_REGION)"
OCI_S3_BUCKET="$(env_val OCI_S3_BUCKET)"
OCI_S3_ACCESS_KEY="$(env_val OCI_S3_ACCESS_KEY)"
OCI_S3_SECRET_KEY="$(env_val OCI_S3_SECRET_KEY)"

# NATS credentials file
apply_secret nats-creds \
  --from-file="jobradar.creds=$CREDS_FILE"

# API secret
apply_secret jobradar-api-secret \
  --from-literal="DB_HOST=$DB_HOST" \
  --from-literal="DB_USER=$DB_USER" \
  --from-literal="DB_PASSWORD=$DB_PASSWORD" \
  --from-literal="DB_NAME=$DB_NAME" \
  --from-literal="REDIS_URL=$REDIS_URL" \
  --from-literal="CLERK_JWKS_URL=$CLERK_JWKS_URL" \
  --from-literal="CLERK_ISSUER_URL=$CLERK_ISSUER_URL" \
  --from-literal="CLERK_SECRET_KEY=$CLERK_SECRET_KEY" \
  --from-literal="OCI_S3_ENDPOINT=$OCI_S3_ENDPOINT" \
  --from-literal="OCI_S3_REGION=$OCI_S3_REGION" \
  --from-literal="OCI_S3_BUCKET=$OCI_S3_BUCKET" \
  --from-literal="OCI_S3_ACCESS_KEY=$OCI_S3_ACCESS_KEY" \
  --from-literal="OCI_S3_SECRET_KEY=$OCI_S3_SECRET_KEY"

# Migrate secret
apply_secret jobradar-migrate-secret \
  --from-literal="DB_HOST=$DB_HOST" \
  --from-literal="DB_USER=$DB_USER" \
  --from-literal="DB_PASSWORD=$DB_PASSWORD" \
  --from-literal="DB_NAME=$DB_NAME"

# Filter secret
apply_secret jobradar-filter-secret \
  --from-literal="DB_HOST=$DB_HOST" \
  --from-literal="DB_USER=$DB_USER" \
  --from-literal="DB_PASSWORD=$DB_PASSWORD" \
  --from-literal="DB_NAME=$DB_NAME" \
  --from-literal="REDIS_URL=$REDIS_URL" \
  --from-literal="GEMINI_KEY=$GEMINI_KEY" \
  --from-literal="GEMINI_KEY_JP=$GEMINI_KEY_JP"

# Enricher secret
apply_secret jobradar-enricher-secret \
  --from-literal="DB_HOST=$DB_HOST" \
  --from-literal="DB_USER=$DB_USER" \
  --from-literal="DB_PASSWORD=$DB_PASSWORD" \
  --from-literal="DB_NAME=$DB_NAME" \
  --from-literal="REDIS_URL=$REDIS_URL" \
  --from-literal="GEMINI_KEY=$GEMINI_KEY" \
  --from-literal="GEMINI_KEY_JP=$GEMINI_KEY_JP"

# Crawler secret
apply_secret jobradar-crawler-secret \
  --from-literal="DB_HOST=$DB_HOST" \
  --from-literal="DB_USER=$DB_USER" \
  --from-literal="DB_PASSWORD=$DB_PASSWORD" \
  --from-literal="DB_NAME=$DB_NAME" \
  --from-literal="REDIS_URL=$REDIS_URL"

# Orchestrator secret — needs DB + NATS + the HMAC signing key for node tokens
apply_secret jobradar-orchestrator-secret \
  --from-literal="DB_HOST=$DB_HOST" \
  --from-literal="DB_USER=$DB_USER" \
  --from-literal="DB_PASSWORD=$DB_PASSWORD" \
  --from-literal="DB_NAME=$DB_NAME" \
  --from-literal="CRAWLER_TOKEN_SECRET=$CRAWLER_TOKEN_SECRET"

# Crawler-node secret — only needs the bearer token to authenticate to the orchestrator
apply_secret jobradar-crawler-node-secret \
  --from-literal="CRAWLER_TOKEN=$CRAWLER_TOKEN"

# Scheduler secret
apply_secret jobradar-scheduler-secret \
  --from-literal="DB_HOST=$DB_HOST" \
  --from-literal="DB_USER=$DB_USER" \
  --from-literal="DB_PASSWORD=$DB_PASSWORD" \
  --from-literal="DB_NAME=$DB_NAME"

# ── Done ──────────────────────────────────────────────────────────────────────
echo -e "\n${GREEN}${BOLD}Bootstrap complete.${NC} Next steps:"
echo -e "  ${BOLD}make k8s-build${NC}   — build + import all images into k3d"
echo -e "  ${BOLD}make k8s-up${NC}      — deploy via Helm\n"
