# Phase 1 — Shared contracts

Every Phase 1 bucket must honor these contracts EXACTLY. They are the integration boundary between buckets running in parallel.

---

## Identity (Bucket 2 — `backend/common/identity/identity.go`)

```go
package identity

type Identity struct {
    UserID      string
    Email       string
    Roles       []string
    Status      string  // "active" | "suspended"
    DisplayName string
}

func (i Identity) HasRole(role string) bool
func (i Identity) IsAdmin() bool  // shortcut for HasRole("admin")

// Context helpers
func FromContext(ctx context.Context) (Identity, bool)
func MustFromContext(ctx context.Context) Identity      // panics if missing — handlers only
func UserIDFrom(ctx context.Context) string             // empty string if missing
func IsAdmin(ctx context.Context) bool
func HasRole(ctx context.Context, role string) bool
func NewContext(ctx context.Context, id Identity) context.Context

// Test helper
func WithTestIdentity(ctx context.Context, userID string, roles ...string) context.Context
```

---

## User model (Bucket 1 — `backend/service/user/models/user.go`)

```go
package usermodel

type User struct {
    ID          string     `gorm:"primaryKey;type:uuid"`
    Email       string     `gorm:"uniqueIndex;type:citext;not null"`
    DisplayName *string
    Status      string     `gorm:"not null;default:active"`
    CreatedAt   time.Time  `gorm:"not null;default:now()"`
    UpdatedAt   time.Time  `gorm:"not null;default:now()"`
}

func (User) TableName() string { return "users" }

const (
    StatusActive    = "active"
    StatusSuspended = "suspended"
)
```

---

## Role models (Bucket 1 — `backend/service/role/models/role.go`)

```go
package rolemodel

type Role struct {
    ID          string     `gorm:"primaryKey;type:text"`
    Name        string     `gorm:"not null"`
    Description *string
    CreatedAt   time.Time
    UpdatedAt   time.Time
}

func (Role) TableName() string { return "roles" }

type UserRole struct {
    UserID    string     `gorm:"primaryKey;type:uuid"`
    RoleID    string     `gorm:"primaryKey;type:text"`
    GrantedAt time.Time
    GrantedBy *string    `gorm:"type:uuid"`
}

func (UserRole) TableName() string { return "user_roles" }

const (
    RoleUser  = "user"
    RoleAdmin = "admin"
)
```

---

## Audit model (Bucket 1 — `backend/service/audit/models/audit.go`)

```go
package auditmodel

type Entry struct {
    ID          string         `gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
    ActorUserID *string        `gorm:"type:uuid"`
    Action      string         `gorm:"not null"`
    TargetType  *string
    TargetID    *string
    Payload     datatypes.JSON
    CreatedAt   time.Time
}

func (Entry) TableName() string { return "admin_audit_log" }

// Action constants for now (extend in later phases)
const (
    ActionUserDelete       = "user.delete"
    ActionUserRoleGrant    = "user.role.grant"
    ActionUserRoleRevoke   = "user.role.revoke"
    ActionUserStatusChange = "user.status.change"
)
```

---

## Cache (Bucket 2 — `backend/platform/cache/`)

```go
package cache

type Cache interface {
    Get(ctx context.Context, key string, dest any) (hit bool, err error)
    Set(ctx context.Context, key string, val any, ttl time.Duration) error
    Delete(ctx context.Context, keys ...string) error
}

// Single-flight wrapper, no stampede
func GetOrLoad[T any](
    ctx context.Context,
    c Cache,
    key string,
    ttl time.Duration,
    loader func(ctx context.Context) (T, error),
) (T, error)

// Implementations
// - RedisCache (production) — backed by github.com/redis/go-redis/v9
// - MemoryCache (tests)     — sync.Map + TTL
// - NoopCache (disable)     — always miss
```

---

## Rate limit (Bucket 2 — `backend/platform/ratelimit/`)

```go
package ratelimit

type Scope string

const (
    ScopeRead  Scope = "read"
    ScopeWrite Scope = "write"
    ScopeAdmin Scope = "admin"
)

type Limiter interface {
    Allow(ctx context.Context, key string, scope Scope) (allowed bool, retryAfter time.Duration, err error)
}

// Implementations
// - RedisLimiter (production) — Lua sliding window or token bucket
// - MemoryLimiter (tests)
```

---

## Supabase admin (Bucket 2 — `backend/platform/supabaseadmin/`)

```go
package supabaseadmin

type Client interface {
    DeleteUser(ctx context.Context, userID string) error
}

// Single implementation in v1: REST call to Supabase admin API
// Reads SUPABASE_URL + SUPABASE_SERVICE_ROLE_KEY from env at construction
```

---

## Service interfaces (Bucket 4)

### `backend/service/user/UserService.go`

```go
type Service interface {
    Get(ctx context.Context, id string) (*usermodel.User, error)
    GetByEmail(ctx context.Context, email string) (*usermodel.User, error)
    Provision(ctx context.Context, id, email string) (*usermodel.User, error)  // idempotent on conflict
    UpdateProfile(ctx context.Context, id string, displayName string) error
    Delete(ctx context.Context, id string) error  // hard delete + Supabase admin call
}
```

### `backend/service/role/RoleService.go`

```go
type Service interface {
    ListForUser(ctx context.Context, userID string) ([]string, error)
    Grant(ctx context.Context, userID, roleID string, grantedBy *string) error
    Revoke(ctx context.Context, userID, roleID string) error
    ListAll(ctx context.Context) ([]rolemodel.Role, error)
}
```

### `backend/service/audit/AuditService.go`

```go
type ListFilter struct {
    ActorUserID *string
    Action      *string
    Limit       int
    Offset      int
}

type Service interface {
    Append(ctx context.Context, entry auditmodel.Entry) error
    List(ctx context.Context, filter ListFilter) (entries []auditmodel.Entry, total int64, err error)
}
```

---

## Middleware (Bucket 3 — `backend/apps/api/middleware/`)

```go
// Extends existing middleware/auth.go
// After JWT validation:
//   1. Look up users by JWT.sub via UserService.Get (auto-provision if missing)
//   2. If first time AND email in ADMIN_EMAILS env var → grant 'admin' role
//   3. Load roles via RoleService.ListForUser
//   4. Build Identity{UserID, Email, Roles, Status, DisplayName}
//   5. Cache via UserCacheService (60s TTL)
//   6. Reject if Status == 'suspended'
//   7. Attach identity to ctx via identity.NewContext

func RequireRole(role string) func(http.Handler) http.Handler
// Reads identity.HasRole from ctx; returns 403 if missing role.

func RateLimit(limiter ratelimit.Limiter) func(http.Handler) http.Handler
// Derives Scope from r.Method (GET/HEAD → Read; others → Write)
// Key: identity.UserIDFrom(ctx); fall through to remote_addr if unauthenticated.
// On block: return 429 with Retry-After header.

func AuditAdminMutation(service audit.Service) func(http.Handler) http.Handler
// Wraps admin mutating handlers; on 2xx response, append to admin_audit_log
// action = derived from route + method (e.g. "company.create")
```

---

## Env vars (Bucket 5 / integration — `backend/apps/api/main.go`)

```
ADMIN_EMAILS=alice@x.com,bob@y.com
SUPABASE_URL=...
SUPABASE_SERVICE_ROLE_KEY=...
RATE_LIMIT_READ_RPM=30
RATE_LIMIT_WRITE_RPM=20
RATE_LIMIT_ADMIN_RPM=120
CACHE_TTL_USER_SECONDS=60
CACHE_TTL_COMPANY_SECONDS=60
CACHE_NEGATIVE_TTL_SECONDS=10
```

---

## Endpoints (Bucket 4)

```
GET    /api/me        → returns Identity bundle as JSON
PATCH  /api/me        → body {display_name}; updates user
DELETE /api/me        → hard delete (Supabase admin first, then DB cascade)
```

Use existing `handler.Respond[T]` / `handler.RespondError` helpers.

---

## File ownership map (no overlap)

| Bucket | Owns |
|---|---|
| **B1: Migrations + models** | `backend/migrations/<NNN>_phase1_*.sql`; `backend/service/{user,role,audit}/models/` |
| **B2: Platform packages** | `backend/common/identity/`; `backend/platform/{cache,ratelimit,supabaseadmin}/` |
| **B3: Middleware** | `backend/apps/api/middleware/{role,ratelimit,audit}.go` (new); extends `auth.go` |
| **B4: Services + handlers** | `backend/service/{user,role,audit}/` (everything except `models/`); `/me` handler; router edits to register `/api/me` and apply global middleware |
| **B5: Integration QA (post)** | nothing — read-only review |

No bucket commits. Orchestrator commits after QA passes.
