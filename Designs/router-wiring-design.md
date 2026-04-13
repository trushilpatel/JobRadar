# Router Registration & API Wiring — Design

## Problem

- `NewRouter()` takes every handler as a param — grows with every new resource
- `main.go` manually constructs all 9 stores + 5 handlers — same problem

## Solution: 3 pieces

```
RouteGroup interface      ←  handlers implement this
App struct                ←  owns all store construction, builds []RouteGroup
NewRouter([]RouteGroup)   ←  iterates and registers, never changes again
```

---

## 1. `RouteGroup` interface
**New file: `internal/api/handler/handler.go`**

```go
package handler

import "github.com/go-chi/chi/v5"

type RouteGroup interface {
    Register(r chi.Router)
}
```

---

## 2. Add `Register()` to each handler (no other changes)

```go
// handler/jobs.go
func (h *JobHandler) Register(r chi.Router) {
    r.Route("/jobs", func(r chi.Router) {
        r.Get("/", h.List)
        r.Get("/{id}", h.Get)
        r.Delete("/{id}", h.Dismiss)
        r.Patch("/{id}/save", h.SetSaved)
        r.Patch("/{id}/status", h.SetStatus)
        r.Patch("/{id}/notes", h.SetNotes)
    })
}

// handler/applications.go
func (h *ApplicationHandler) Register(r chi.Router) {
    r.Route("/jobs/{id}/applications", func(r chi.Router) {
        r.Get("/", h.List)
        r.Post("/", h.Create)
        r.Patch("/{app_id}/stage", h.UpdateStage)
        r.Patch("/{app_id}/note", h.UpdateNote)
    })
    r.Get("/applications", h.ListAll)
}

// handler/companies.go
func (h *CompanyHandler) Register(r chi.Router) {
    r.Route("/companies", func(r chi.Router) {
        r.Get("/", h.List)
        r.Post("/", h.Create)
        r.Patch("/active", h.SetAllActive)
        r.Route("/queue", func(r chi.Router) {
            r.Get("/", h.ListQueue)
            r.Delete("/{entry_id}", h.RemoveFromQueue)
            r.Post("/{entry_id}/move-up", h.MoveQueueItemUp)
            r.Post("/{entry_id}/move-down", h.MoveQueueItemDown)
        })
        r.Route("/{id}", func(r chi.Router) {
            r.Patch("/", h.Update)
            r.Delete("/", h.Delete)
            r.Get("/scan-stats", h.ScanStats)
            r.Post("/scan", h.TriggerScan)
        })
    })
}

// handler/dashboard.go
func (h *DashboardHandler) Register(r chi.Router) {
    r.Route("/dashboard", func(r chi.Router) {
        r.Get("/stats", h.Stats)
        r.Get("/activity", h.Activity)
    })
}

// handler/insights.go
func (h *InsightsHandler) Register(r chi.Router) {
    r.Route("/insights", func(r chi.Router) {
        r.Get("/tech-stack", h.TechStack)
        r.Get("/company-activity", h.CompanyActivity)
        r.Get("/funnel", h.Funnel)
        r.Get("/location", h.Location)
        r.Get("/sponsorship", h.Sponsorship)
        r.Get("/velocity", h.Velocity)
        r.Get("/role-categories", h.RoleCategories)
    })
}
```

---

## 3. `App` struct
**New file: `internal/api/app.go`**

```go
package api

import (
    "github.com/trushilpatel/jobradar/config"
    "github.com/trushilpatel/jobradar/internal/api/handler"
    "github.com/trushilpatel/jobradar/internal/store"
    "gorm.io/gorm"
)

type App struct {
    Jobs      store.JobStore
    Ledger    store.SeenLedgerStore
    Discard   store.DiscardStore
    Snapshot  store.SnapshotWriter
    Apps      store.ApplicationStore
    Companies store.CompanyStore
    ScanLog   store.CompanyScanLogStore
    Insights  store.InsightsReader
    ScanQueue store.ScanQueueStore

    scanIntervalMinutes int
}

func NewApp(db *gorm.DB, cfg config.Config) *App {
    return &App{
        Jobs:                store.NewJobStore(db),
        Ledger:              store.NewSeenLedgerStore(db),
        Discard:             store.NewDiscardStore(db),
        Snapshot:            store.NewSnapshotStore(db),
        Apps:                store.NewApplicationStore(db),
        Companies:           store.NewCompanyStore(db),
        ScanLog:             store.NewCompanyScanLogStore(db),
        Insights:            store.NewInsightsStore(db),
        ScanQueue:           store.NewScanQueueStore(db),
        scanIntervalMinutes: cfg.ScanIntervalMinutes,
    }
}

// Handlers returns all route groups. Add one line here per new resource.
func (a *App) Handlers() []handler.RouteGroup {
    return []handler.RouteGroup{
        handler.NewJobHandler(a.Jobs, a.Ledger, a.Discard, a.Snapshot),
        handler.NewApplicationHandler(a.Apps, a.Jobs),
        handler.NewCompanyHandler(a.Companies, a.ScanLog, a.ScanQueue, a.scanIntervalMinutes),
        handler.NewDashboardHandler(a.Jobs, a.ScanLog, a.Insights),
        handler.NewInsightsHandler(a.Insights),
    }
}
```

---

## 4. Updated `router.go`

```go
func NewRouter(log logger.Logger, database *gorm.DB, groups []handler.RouteGroup) *chi.Mux {
    r := chi.NewRouter()
    r.Use(middleware.Logger(log))
    r.Use(middleware.Metrics())
    r.Use(chimiddleware.Recoverer)

    r.Handle("/metrics", promhttp.Handler())
    r.Get("/health", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
    r.Get("/ready", func(w http.ResponseWriter, _ *http.Request) {
        if err := db.Ping(database); err != nil {
            http.Error(w, err.Error(), http.StatusServiceUnavailable)
            return
        }
        w.WriteHeader(http.StatusOK)
    })

    for _, g := range groups {
        g.Register(r)
    }
    return r
}
```

---

## 5. Updated `cmd/api/main.go`

```go
func main() {
    cfg := config.Load()
    log := logger.Init("api")

    database, err := db.Open(cfg)
    if err != nil {
        log.Error("db open failed", err)
        os.Exit(1)
    }

    app := api.NewApp(database, cfg)
    router := api.NewRouter(log, database, app.Handlers())

    // ... http.Server + signal handling unchanged
}
```

---

## Adding a new resource

1. Create `internal/api/handler/foo.go` — implement `Register(r chi.Router)`
2. Add store field to `App` + one line in `NewApp()`
3. Append `handler.NewFooHandler(...)` to `App.Handlers()`

`router.go` and `main.go` never change.
