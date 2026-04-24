# Dev Auth Bypass Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an `X-Dev-Token` header bypass to the API auth middleware so any HTTP client (LLM, script, curl) can hit all backend endpoints locally without Supabase magic-link auth.

**Architecture:** A `DEV_TOKEN` env var is added to `config.APIConfig`. When non-empty, the existing `middleware.Auth` function checks the `X-Dev-Token` request header first; if it matches, it injects a synthetic `Identity` (skipping JWT validation entirely) and calls the next handler. When `DEV_TOKEN` is empty (production default), the code path is completely unreachable. No separate middleware, no new handler — a single short-circuit inside the existing auth middleware.

**Tech Stack:** Go, `net/http`, `backend/apps/api/middleware/auth.go`, `backend/common/config/config.go`

---

## Files

| File | Change |
|---|---|
| `backend/common/config/config.go` | Add `DevToken string` to `APIConfig`; load from `DEV_TOKEN` env var |
| `backend/apps/api/middleware/auth.go` | Add `devToken string` param to `Auth()`; short-circuit before JWT when it matches |
| `backend/apps/api/main.go` | Pass `cfg.API.DevToken` to `middleware.Auth` |
| `backend/apps/api/middleware/auth_test.go` | Tests for bypass on/off/wrong-token, existing JWT path untouched |
| `k8s/helm/jobradar/values.yaml` | Add `api.devToken: ""` (empty = disabled) |
| `k8s/helm/jobradar/templates/api/configmap.yaml` | Emit `DEV_TOKEN` from values |
| `backend/apps/api/configmap.yaml` (k8s example) | Document `DEV_TOKEN` in example configmap |

---

## Task 1: Add `DevToken` to config

**Files:**
- Modify: `backend/common/config/config.go`

- [ ] **Step 1: Read the file to find `APIConfig` and its `Load` block**

```bash
grep -n "APIConfig\|MaxBodyBytes\|API_MAX" backend/common/config/config.go
```

Expected: lines showing `APIConfig` struct definition and `getEnvInt("API_MAX_BODY_BYTES"...)` in `Load()`.

- [ ] **Step 2: Add `DevToken` field to `APIConfig`**

Find the `APIConfig` struct and add the field:

```go
type APIConfig struct {
	MaxBodyBytes int64
	DevToken     string
}
```

- [ ] **Step 3: Load `DEV_TOKEN` from env in `config.Load()`**

Find the `API: APIConfig{...}` block inside `Load()` and add the field:

```go
API: APIConfig{
    MaxBodyBytes: int64(getEnvInt("API_MAX_BODY_BYTES", 1048576)),
    DevToken:     os.Getenv("DEV_TOKEN"),
},
```

- [ ] **Step 4: Verify the project still compiles**

```bash
cd backend && go build ./...
```

Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add backend/common/config/config.go
git commit -m "feat(config): add DevToken field to APIConfig loaded from DEV_TOKEN env"
```

---

## Task 2: Add dev bypass to auth middleware

**Files:**
- Modify: `backend/apps/api/middleware/auth.go`
- Modify: `backend/apps/api/main.go`

- [ ] **Step 1: Read auth.go to find the `Auth` function signature and its return handler**

```bash
grep -n "^func Auth\|func(next\|extractBearer\|toContext\|AllowedEmail" backend/apps/api/middleware/auth.go | head -20
```

Note the exact signature and where the inner `http.HandlerFunc` begins.

- [ ] **Step 2: Add `devToken string` parameter to `Auth()`**

Change the function signature from:

```go
func Auth(cfg config.AuthConfig, log logger.Logger) (func(http.Handler) http.Handler, error) {
```

to:

```go
func Auth(cfg config.AuthConfig, devToken string, log logger.Logger) (func(http.Handler) http.Handler, error) {
```

- [ ] **Step 3: Add the bypass check at the top of the inner handler**

Inside the returned `http.HandlerFunc`, before any existing JWT logic, add:

```go
return func(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Dev bypass — only active when DEV_TOKEN env var is set.
        if devToken != "" && r.Header.Get("X-Dev-Token") == devToken {
            ctx := toContext(r.Context(), Identity{
                UserID: "dev-bypass",
                Email:  cfg.AllowedEmail,
            })
            next.ServeHTTP(w, r.WithContext(ctx))
            return
        }
        // ... existing JWT validation continues unchanged below ...
```

Make sure the closing brace of the `HandlerFunc` is correct after the existing logic.

- [ ] **Step 4: Update `main.go` to pass `cfg.API.DevToken`**

Find the call to `middleware.Auth` in `backend/apps/api/main.go`:

```bash
grep -n "middleware.Auth" backend/apps/api/main.go
```

Change it from:

```go
authMW, err := middleware.Auth(cfg.Auth, log)
```

to:

```go
authMW, err := middleware.Auth(cfg.Auth, cfg.API.DevToken, log)
```

- [ ] **Step 5: Verify it compiles**

```bash
cd backend && go build ./...
```

Expected: no errors.

- [ ] **Step 6: Commit**

```bash
git add backend/apps/api/middleware/auth.go backend/apps/api/main.go
git commit -m "feat(api): add X-Dev-Token bypass to auth middleware — disabled when DEV_TOKEN is unset"
```

---

## Task 3: Write tests

**Files:**
- Modify: `backend/apps/api/middleware/auth_test.go`

- [ ] **Step 1: Check if auth_test.go exists and read its structure**

```bash
ls backend/apps/api/middleware/auth_test.go 2>/dev/null && head -30 backend/apps/api/middleware/auth_test.go
```

Note the package name and any existing test helpers.

- [ ] **Step 2: Write the failing tests**

Add these test cases to `auth_test.go` (adjust package name if needed — use the existing one):

```go
func TestDevBypass(t *testing.T) {
    const devToken = "test-dev-token-abc123"
    const allowedEmail = "dev@example.com"

    cfg := config.AuthConfig{
        AllowedEmail: allowedEmail,
        // SupabaseJWTSecret or JWKS must be set for Auth() to initialise.
        // Use HS256 with a dummy secret so no network call is needed.
        SupabaseJWTSecret: "dummy-secret-for-test-only",
    }

    mw, err := Auth(cfg, devToken, logger.NewNop())
    if err != nil {
        t.Fatalf("Auth() error: %v", err)
    }

    // Sentinel handler that records whether it was called and what identity it saw.
    var gotIdent Identity
    var handlerCalled bool
    next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        handlerCalled = true
        gotIdent, _ = FromContext(r.Context())
        w.WriteHeader(http.StatusOK)
    })

    handler := mw(next)

    t.Run("bypass active when token matches", func(t *testing.T) {
        handlerCalled = false
        gotIdent = Identity{}

        req := httptest.NewRequest(http.MethodGet, "/jobs", nil)
        req.Header.Set("X-Dev-Token", devToken)
        rr := httptest.NewRecorder()

        handler.ServeHTTP(rr, req)

        if !handlerCalled {
            t.Fatal("next handler was not called")
        }
        if rr.Code != http.StatusOK {
            t.Fatalf("expected 200, got %d", rr.Code)
        }
        if gotIdent.UserID != "dev-bypass" {
            t.Fatalf("expected UserID dev-bypass, got %q", gotIdent.UserID)
        }
        if gotIdent.Email != allowedEmail {
            t.Fatalf("expected Email %q, got %q", allowedEmail, gotIdent.Email)
        }
    })

    t.Run("bypass skipped when token is wrong", func(t *testing.T) {
        handlerCalled = false

        req := httptest.NewRequest(http.MethodGet, "/jobs", nil)
        req.Header.Set("X-Dev-Token", "wrong-token")
        rr := httptest.NewRecorder()

        handler.ServeHTTP(rr, req)

        // No valid JWT supplied, so auth should reject with 401.
        if rr.Code != http.StatusUnauthorized {
            t.Fatalf("expected 401, got %d", rr.Code)
        }
        if handlerCalled {
            t.Fatal("next handler should not have been called")
        }
    })

    t.Run("bypass skipped when header absent", func(t *testing.T) {
        handlerCalled = false

        req := httptest.NewRequest(http.MethodGet, "/jobs", nil)
        rr := httptest.NewRecorder()

        handler.ServeHTTP(rr, req)

        if rr.Code != http.StatusUnauthorized {
            t.Fatalf("expected 401, got %d", rr.Code)
        }
        if handlerCalled {
            t.Fatal("next handler should not have been called")
        }
    })
}

func TestDevBypassDisabledWhenTokenEmpty(t *testing.T) {
    cfg := config.AuthConfig{
        AllowedEmail:      "dev@example.com",
        SupabaseJWTSecret: "dummy-secret-for-test-only",
    }

    // devToken = "" means bypass is completely off.
    mw, err := Auth(cfg, "", logger.NewNop())
    if err != nil {
        t.Fatalf("Auth() error: %v", err)
    }

    var handlerCalled bool
    next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        handlerCalled = true
        w.WriteHeader(http.StatusOK)
    })

    handler := mw(next)

    // Even if someone sends the header, it must not bypass auth when devToken == "".
    req := httptest.NewRequest(http.MethodGet, "/jobs", nil)
    req.Header.Set("X-Dev-Token", "anything")
    rr := httptest.NewRecorder()

    handler.ServeHTTP(rr, req)

    if handlerCalled {
        t.Fatal("next handler must not be called when devToken is empty")
    }
    if rr.Code != http.StatusUnauthorized {
        t.Fatalf("expected 401, got %d", rr.Code)
    }
}
```

- [ ] **Step 3: Run tests — expect them to fail or error (missing logger.NewNop or similar)**

```bash
cd backend && go test ./apps/api/middleware/... -run "TestDevBypass" -v 2>&1 | head -40
```

Note any compilation errors or missing helpers. If `logger.NewNop()` doesn't exist, use whatever nop/discard logger the package uses in other tests.

- [ ] **Step 4: Fix any compilation issues in the test file, then run again**

```bash
cd backend && go test ./apps/api/middleware/... -run "TestDevBypass" -v
```

Expected:
```
--- PASS: TestDevBypass/bypass_active_when_token_matches
--- PASS: TestDevBypass/bypass_skipped_when_token_is_wrong
--- PASS: TestDevBypass/bypass_skipped_when_header_absent
--- PASS: TestDevBypassDisabledWhenTokenEmpty
PASS
```

- [ ] **Step 5: Run the full middleware test suite to confirm nothing regressed**

```bash
cd backend && go test ./apps/api/middleware/... -v
```

Expected: all tests pass.

- [ ] **Step 6: Commit**

```bash
git add backend/apps/api/middleware/auth_test.go
git commit -m "test(api): add dev bypass tests — enabled/disabled/wrong-token cases"
```

---

## Task 4: Wire into Kubernetes config

**Files:**
- Modify: `k8s/helm/jobradar/values.yaml`
- Modify: `k8s/helm/jobradar/templates/api/configmap.yaml`

- [ ] **Step 1: Add `devToken` to values.yaml under the `api` section**

Open `k8s/helm/jobradar/values.yaml` and add to the `api:` block:

```yaml
api:
  nodePort: 30080
  metricsAddr: ":9100"
  devToken: ""          # Set to a secret string for local dev only. Leave empty in production.
  # ... rest of existing api fields ...
```

- [ ] **Step 2: Emit `DEV_TOKEN` in the API configmap template**

Open `k8s/helm/jobradar/templates/api/configmap.yaml` and add:

```yaml
data:
  # ... existing keys ...
  DEV_TOKEN: {{ .Values.api.devToken | quote }}
```

- [ ] **Step 3: Validate the Helm template renders correctly**

```bash
helm template jobradar k8s/helm/jobradar -n jobradar-local -s templates/api/configmap.yaml | grep DEV_TOKEN
```

Expected:
```
  DEV_TOKEN: ""
```

- [ ] **Step 4: Commit**

```bash
git add k8s/helm/jobradar/values.yaml k8s/helm/jobradar/templates/api/configmap.yaml
git commit -m "feat(helm): add DEV_TOKEN to api configmap — empty by default, set locally to enable bypass"
```

---

## Task 5: End-to-end verification

- [ ] **Step 1: Set a dev token in the live cluster**

```bash
helm upgrade jobradar k8s/helm/jobradar -n jobradar-local \
  --set api.devToken="local-dev-$(openssl rand -hex 8)"
```

Copy the generated token from the command output (or read it back):

```bash
kubectl get configmap jobradar-api-config -n jobradar-local -o jsonpath='{.data.DEV_TOKEN}'
```

- [ ] **Step 2: Wait for the API pod to pick up the new config**

```bash
kubectl rollout status deployment/jobradar-api -n jobradar-local --timeout=60s
```

- [ ] **Step 3: Hit a protected endpoint without auth — expect 401**

```bash
curl -s -o /dev/null -w "%{http_code}" http://localhost:30080/jobs
```

Expected: `401`

- [ ] **Step 4: Hit the same endpoint with the dev token — expect 200**

```bash
TOKEN=$(kubectl get configmap jobradar-api-config -n jobradar-local -o jsonpath='{.data.DEV_TOKEN}')
curl -s -o /dev/null -w "%{http_code}" -H "X-Dev-Token: $TOKEN" http://localhost:30080/jobs
```

Expected: `200`

- [ ] **Step 5: Hit with wrong token — expect 401**

```bash
curl -s -o /dev/null -w "%{http_code}" -H "X-Dev-Token: wrong" http://localhost:30080/jobs
```

Expected: `401`

- [ ] **Step 6: Test a few more endpoints to confirm the identity flows through correctly**

```bash
TOKEN=$(kubectl get configmap jobradar-api-config -n jobradar-local -o jsonpath='{.data.DEV_TOKEN}')

# Dashboard stats
curl -s -H "X-Dev-Token: $TOKEN" http://localhost:30080/dashboard/stats | head -c 200

# Companies list
curl -s -H "X-Dev-Token: $TOKEN" http://localhost:30080/companies | head -c 200
```

Expected: JSON responses, not 401.

- [ ] **Step 7: Commit and update README with usage**

```bash
git add -A
git commit -m "docs: document DEV_TOKEN local dev bypass usage"
```
