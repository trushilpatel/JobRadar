# SOA Pipeline Refactor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix all 8 SOA violations so pipeline workers go through proper service interfaces, the service layer has no HTTP imports, and each domain owns its own table writes.

**Architecture:** Create a `domain` package for shared error types; introduce `job.PipelineService` and `scan.CrawlerService` as pipeline-facing service interfaces; move `buildJob` logic out of `pipeline/analyzer` into the job service; define narrow pipeline interfaces in each pipeline package so workers depend on service abstractions, never on stores directly.

**Tech Stack:** Go, GORM, Kafka (franz-go), Google Wire (API only), standard `testing` package

---

## File Map

### New files

| File                                           | Purpose                                                                                                                                    |
| ---------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------ |
| `backend/domain/errors.go`                     | `ServiceError` type + sentinel values (`ErrNotFound`, `ErrInternal`, etc.) — replaces `api/response.ServiceError` in service bodies        |
| `backend/service/job/pipeline_service.go`      | `PipelineService` interface + `pipelineService` implementation; contains `buildJobRow` and `nullableStr` (moved from `analyzer/worker.go`) |
| `backend/service/job/pipeline_service_test.go` | Tests for `buildJobRow` (moved from `analyzer/worker_test.go`) + `PipelineService` unit tests                                              |
| `backend/service/scan/crawler_service.go`      | `CrawlerService` interface + `crawlerService` implementation; wraps `ScanLogWriter + companyScanUpdater`                                   |
| `backend/pipeline/crawler/interfaces.go`       | `CompanyReader`, `PipelineLedger`, `ScanRecorder` — local interfaces the engine depends on                                                 |
| `backend/pipeline/filter/interfaces.go`        | `FilterLedger` — local interface the filter worker depends on                                                                              |

### Modified files

| File                                               | What changes                                                                                                                       |
| -------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------- |
| `backend/service/job/service.go`                   | Remove `api/response` import; use `domain.Err*`; `List` takes `page, limit int`                                                    |
| `backend/service/job/service_test.go`              | Use `domain.ServiceError`; `List` call uses bare ints                                                                              |
| `backend/service/company/service.go`               | Same `api/response` + `api/request` cleanup                                                                                        |
| `backend/service/company/service_test.go`          | Use `domain.ServiceError`; `List` call uses bare ints; add `UpdateScanStatus` to `fakeStore`                                       |
| `backend/service/company/handler.go`               | `h.companies.List(filter, p)` → `h.companies.List(filter, p.Page, p.Limit)`                                                        |
| `backend/service/job/handler.go`                   | `h.jobs.List(filter, p)` → `h.jobs.List(filter, p.Page, p.Limit)`                                                                  |
| `backend/service/dashboard/service.go`             | `response.ErrInternal` → `domain.ErrInternal`                                                                                      |
| `backend/service/insights/service.go`              | Same                                                                                                                               |
| `backend/service/company/store.go`                 | Add `UpdateScanStatus(id, status string) error` to `CompanyWriter`; add GORM impl                                                  |
| `backend/service/scan/store.go`                    | Remove `UpdateCompanyScanStatus` from `ScanLogWriter`; delete GORM impl                                                            |
| `backend/api/handler/respond.go`                   | `*response.ServiceError` check → `*domain.ServiceError`                                                                            |
| `backend/pipeline/crawler/engine.go`               | Field types from store interfaces → local interfaces; ledger method names updated; `recordResult` → single `RecordScanResult` call |
| `backend/pipeline/filter/worker.go`                | `ledger jobsvc.SeenLedgerWriter` → `ledger FilterLedger`; call semantic methods                                                    |
| `backend/pipeline/filter/worker_test.go`           | `fakeLedger` satisfies `FilterLedger` (2 methods, not 4); assertions updated                                                       |
| `backend/pipeline/analyzer/worker.go`              | 3 store fields → `pipeline job.PipelineService`; remove `buildJob`, `nullableString`; add `toEnrichedInput`                        |
| `backend/pipeline/analyzer/worker_test.go`         | Replace 3 fakes with `fakePipelineService`; delete `buildJob` tests (moved); adapt `enrich` tests                                  |
| `backend/cmd/job_listing_crawler/main.go`          | Wire `PipelineService` + `CrawlerService`; pass to engine                                                                          |
| `backend/cmd/job_listing_relevance_filter/main.go` | Pass `pipelineSvc` instead of `ledgerStore`                                                                                        |
| `backend/cmd/job_post_analyzer/main.go`            | Pass `pipelineSvc` instead of 3 stores                                                                                             |
| `backend/cmd/company_scan_scheduler/main.go`       | `enqueuedue` param: `companysvc.CompanyReader` → `companysvc.SchedulerService`                                                     |

---

## Task 1: Create `backend/domain/errors.go`

**Files:**

- Create: `backend/domain/errors.go`

- [ ] **Step 1: Create the file**

```go
// backend/domain/errors.go
package domain

// ServiceError is a typed error returned by service methods.
// Status carries the HTTP status code; the handler layer maps it to a response.
// Pipeline workers can inspect Code without any HTTP concern.
type ServiceError struct {
	Code    string
	Message string
	Status  int
}

func (e *ServiceError) Error() string { return e.Message }

// WithMessage returns a new ServiceError with the given message, preserving Code and Status.
func (e *ServiceError) WithMessage(msg string) *ServiceError {
	return &ServiceError{Code: e.Code, Message: msg, Status: e.Status}
}

// Sentinel service errors. Use WithMessage to attach a human-readable description.
var (
	ErrNotFound   = &ServiceError{Code: "NOT_FOUND",   Status: 404}
	ErrConflict   = &ServiceError{Code: "CONFLICT",    Status: 409}
	ErrBadRequest = &ServiceError{Code: "BAD_REQUEST", Status: 400}
	ErrInternal   = &ServiceError{Code: "INTERNAL",    Status: 500}
)
```

- [ ] **Step 2: Verify it compiles**

```bash
cd backend && go build ./domain/...
```

Expected: no output, exit 0.

---

## Task 2: Replace `api/response.ServiceError` with `domain.ServiceError` across service files

**Files:**

- Modify: `backend/service/job/service.go`
- Modify: `backend/service/company/service.go`
- Modify: `backend/service/dashboard/service.go`
- Modify: `backend/service/insights/service.go`
- Modify: `backend/api/handler/respond.go`
- Modify: `backend/service/job/service_test.go`
- Modify: `backend/service/company/service_test.go`

- [ ] **Step 1: Update `service/job/service.go`**

Replace the import block:

```go
import (
	"github.com/trushilpatel/jobradar/api/request"
	"github.com/trushilpatel/jobradar/api/response"
	"github.com/trushilpatel/jobradar/model"
	"github.com/trushilpatel/jobradar/store"
)
```

With:

```go
import (
	"github.com/trushilpatel/jobradar/api/request"
	"github.com/trushilpatel/jobradar/domain"
	"github.com/trushilpatel/jobradar/model"
	"github.com/trushilpatel/jobradar/store"
)
```

Then replace every `response.ErrInternal` → `domain.ErrInternal` and `response.ErrNotFound` → `domain.ErrNotFound` throughout the file. Full updated method bodies:

```go
func (s *service) List(filter JobFilter, p request.Pagination) ([]model.Job, int, error) {
	jobs, total, err := s.jobs.GetJobs(filter, p.Page, p.Limit)
	if err != nil {
		return nil, 0, domain.ErrInternal.WithMessage("failed to fetch jobs")
	}
	return jobs, int(total), nil
}

func (s *service) GetCounts() (*JobCounts, error) {
	counts, err := s.jobs.GetJobCounts()
	if err != nil {
		return nil, domain.ErrInternal.WithMessage("failed to fetch job counts")
	}
	return counts, nil
}

func (s *service) Get(id string) (*model.Job, error) {
	job, err := s.jobs.GetJobByID(id)
	if err != nil {
		if store.IsNotFound(err) {
			return nil, domain.ErrNotFound.WithMessage("job not found")
		}
		return nil, domain.ErrInternal.WithMessage("failed to fetch job")
	}
	return job, nil
}

func (s *service) Dismiss(id string, input DismissInput) error {
	job, err := s.Get(id)
	if err != nil {
		return err
	}
	if err := s.discard.InsertDiscard(id, input.Reason, input.Note); err != nil {
		return domain.ErrInternal.WithMessage("failed to discard job")
	}
	if err := s.ledger.UpdateSeenReason(job.JobPostID, model.SeenReasonUserDismissed); err != nil {
		return domain.ErrInternal.WithMessage("failed to update seen ledger")
	}
	if err := s.jobs.UpdateJobStatus(id, model.JobStatusDismissed); err != nil {
		return domain.ErrInternal.WithMessage("failed to update job status")
	}
	_ = s.snapshot.UpdateSnapshotAction(id, model.JobStatusDismissed)
	return nil
}

func (s *service) SetSaved(id string, saved bool) error {
	if err := s.jobs.UpdateJobSaved(id, saved); err != nil {
		return domain.ErrInternal.WithMessage("failed to update job")
	}
	if saved {
		_ = s.snapshot.UpdateSnapshotAction(id, model.JobStatusSaved)
	}
	return nil
}

func (s *service) SetStatus(id, status string) error {
	if err := s.jobs.UpdateJobStatus(id, status); err != nil {
		return domain.ErrInternal.WithMessage("failed to update status")
	}
	if status == model.JobStatusApplied {
		_ = s.snapshot.UpdateSnapshotAction(id, model.JobStatusApplied)
	}
	return nil
}

func (s *service) SetNotes(id, notes string) error {
	if err := s.jobs.UpdateJobNotes(id, notes); err != nil {
		return domain.ErrInternal.WithMessage("failed to update notes")
	}
	return nil
}
```

- [ ] **Step 2: Update `service/company/service.go`**

Replace import block — remove `api/response`, add `domain`:

```go
import (
	"time"

	"github.com/trushilpatel/jobradar/api/request"
	"github.com/trushilpatel/jobradar/domain"
	"github.com/trushilpatel/jobradar/model"
	"github.com/trushilpatel/jobradar/store"
)
```

Replace all `response.ErrInternal` → `domain.ErrInternal`, `response.ErrNotFound` → `domain.ErrNotFound`. Full updated methods:

```go
func (s *service) List(filter CompanyFilter, p request.Pagination) (CompanyListResult, error) {
	companies, total, err := s.store.GetCompanies(filter, p.Page, p.Limit)
	if err != nil {
		return CompanyListResult{}, domain.ErrInternal.WithMessage("failed to fetch companies")
	}
	_, totalActive, err := s.store.GetCompanyCounts()
	if err != nil {
		return CompanyListResult{}, domain.ErrInternal.WithMessage("failed to fetch company counts")
	}
	views := make([]CompanyView, 0, len(companies))
	for _, c := range companies {
		v := CompanyView{Company: c}
		if c.LastScanAt != nil {
			t := c.LastScanAt.Add(time.Duration(s.config.ScanIntervalMinutes) * time.Minute)
			v.NextScanAt = &t
		}
		views = append(views, v)
	}
	return CompanyListResult{Items: views, Total: total, TotalActive: totalActive}, nil
}

func (s *service) GetAll() ([]CompanyView, error) {
	companies, err := s.store.GetAllCompanies()
	if err != nil {
		return nil, domain.ErrInternal.WithMessage("failed to fetch companies")
	}
	views := make([]CompanyView, 0, len(companies))
	for _, c := range companies {
		v := CompanyView{Company: c}
		if c.LastScanAt != nil {
			t := c.LastScanAt.Add(time.Duration(s.config.ScanIntervalMinutes) * time.Minute)
			v.NextScanAt = &t
		}
		views = append(views, v)
	}
	return views, nil
}

func (s *service) Get(id string) (*model.Company, error) {
	c, err := s.store.GetCompanyByID(id)
	if err != nil {
		if store.IsNotFound(err) {
			return nil, domain.ErrNotFound.WithMessage("company not found")
		}
		return nil, domain.ErrInternal.WithMessage("failed to fetch company")
	}
	return &c, nil
}

func (s *service) Create(input CreateCompanyInput) (*model.Company, error) {
	active := true
	if input.Active != nil {
		active = *input.Active
	}
	now := time.Now()
	company := model.Company{
		ID:          input.ID,
		Name:        input.Name,
		URL:         input.URL,
		AdapterType: input.AdapterType,
		Active:      active,
		LastScanAt:  &now,
	}
	if err := s.store.CreateCompany(company); err != nil {
		return nil, domain.ErrInternal.WithMessage("failed to create company")
	}
	return &company, nil
}

func (s *service) Update(id string, input UpdateCompanyInput) error {
	fields := make(map[string]any)
	if input.Name != nil {
		fields["name"] = *input.Name
	}
	if input.URL != nil {
		fields["url"] = *input.URL
	}
	if input.AdapterType != nil {
		fields["adapter_type"] = *input.AdapterType
	}
	if input.Active != nil {
		fields["active"] = *input.Active
	}
	if err := s.store.UpdateCompany(id, fields); err != nil {
		if store.IsNotFound(err) {
			return domain.ErrNotFound.WithMessage("company not found")
		}
		return domain.ErrInternal.WithMessage("failed to update company")
	}
	return nil
}

func (s *service) Delete(id string) error {
	if _, err := s.store.GetCompanyByID(id); err != nil {
		if store.IsNotFound(err) {
			return domain.ErrNotFound.WithMessage("company not found")
		}
		return domain.ErrInternal.WithMessage("failed to fetch company")
	}
	if err := s.store.DeleteCompany(id); err != nil {
		return domain.ErrInternal.WithMessage("failed to delete company")
	}
	return nil
}

func (s *service) SetAllActive(active bool) error {
	if err := s.store.SetAllCompaniesActive(active); err != nil {
		return domain.ErrInternal.WithMessage("failed to update companies")
	}
	return nil
}
```

- [ ] **Step 3: Update `service/dashboard/service.go`**

Replace import: remove `api/response`, add `domain`:

```go
import (
	"github.com/trushilpatel/jobradar/domain"
	"github.com/trushilpatel/jobradar/service/insights"
	"github.com/trushilpatel/jobradar/service/scan"
)
```

Replace all `response.ErrInternal` → `domain.ErrInternal` in the file body.

- [ ] **Step 4: Update `service/insights/service.go`**

Replace import: remove `api/response`, add `domain`:

```go
import "github.com/trushilpatel/jobradar/domain"
```

Replace all `response.ErrInternal` → `domain.ErrInternal` in the file body.

- [ ] **Step 5: Update `api/handler/respond.go`**

Add `domain` import and change `HandleError` to use `*domain.ServiceError`:

```go
import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/trushilpatel/jobradar/api/response"
	"github.com/trushilpatel/jobradar/domain"
)

// HandleError inspects err and writes the appropriate error response.
// ValidationError → 400, domain.ServiceError → its Status, anything else → 500.
func HandleError(w http.ResponseWriter, err error) {
	var vErr *response.ValidationError
	if errors.As(err, &vErr) {
		RespondError(w, http.StatusBadRequest, vErr.Code, vErr.Message)
		return
	}
	var sErr *domain.ServiceError
	if errors.As(err, &sErr) {
		RespondError(w, sErr.Status, sErr.Code, sErr.Message)
		return
	}
	RespondError(w, http.StatusInternalServerError, "INTERNAL", "unexpected error")
}
```

- [ ] **Step 6: Update `service/job/service_test.go`**

Replace `api/response` import with `domain`:

```go
import (
	"errors"
	"testing"

	"github.com/trushilpatel/jobradar/api/request"
	"github.com/trushilpatel/jobradar/domain"
	"github.com/trushilpatel/jobradar/model"
	"gorm.io/gorm"
)
```

Update `TestGet_NotFound` to use `*domain.ServiceError`:

```go
func TestGet_NotFound(t *testing.T) {
	svc := NewService(&fakeJobStore{}, &fakeLedgerWriter{}, &fakeDiscardStore{}, &fakeSnapshotWriter{})

	_, err := svc.Get("missing")
	if err == nil {
		t.Fatal("expected error for missing job")
	}
	var sErr *domain.ServiceError
	if !errors.As(err, &sErr) || sErr.Code != "NOT_FOUND" {
		t.Errorf("expected NOT_FOUND error, got %v", err)
	}
}
```

- [ ] **Step 7: Update `service/company/service_test.go`**

Replace `api/response` import with `domain`:

```go
import (
	"errors"
	"testing"
	"time"

	"github.com/trushilpatel/jobradar/api/request"
	"github.com/trushilpatel/jobradar/domain"
	"github.com/trushilpatel/jobradar/model"
	"gorm.io/gorm"
)
```

Update `TestDelete_ReturnsNotFoundWhenMissing`:

```go
func TestDelete_ReturnsNotFoundWhenMissing(t *testing.T) {
	fs := &fakeStore{companies: []model.Company{}}
	svc := NewService(fs, Config{})

	err := svc.Delete("nonexistent")
	if err == nil {
		t.Fatal("expected error for missing company")
	}
	var sErr *domain.ServiceError
	if !errors.As(err, &sErr) {
		t.Fatalf("expected ServiceError, got %T: %v", err, err)
	}
	if sErr.Code != "NOT_FOUND" {
		t.Errorf("expected NOT_FOUND, got %s", sErr.Code)
	}
}
```

- [ ] **Step 8: Verify tests still pass**

```bash
cd backend && go test ./service/job/... ./service/company/... ./service/dashboard/... ./service/insights/... ./api/...
```

Expected: all PASS.

Also confirm the scan service (which imports `api/response` in `service.go`) still compiles — we are NOT changing scan service in this task:

```bash
cd backend && go build ./service/scan/...
```

---

## Task 3: Strip `api/request.Pagination` from service method signatures

**Files:**

- Modify: `backend/service/job/service.go`
- Modify: `backend/service/company/service.go`
- Modify: `backend/service/job/handler.go`
- Modify: `backend/service/company/handler.go`
- Modify: `backend/service/job/service_test.go`
- Modify: `backend/service/company/service_test.go`

- [ ] **Step 1: Update `job.Service` interface and `List` implementation**

In `service/job/service.go`, change the `Service` interface:

```go
type Service interface {
	List(filter JobFilter, page, limit int) ([]model.Job, int, error)
	Get(id string) (*model.Job, error)
	GetCounts() (*JobCounts, error)
	Dismiss(id string, input DismissInput) error
	SetSaved(id string, saved bool) error
	SetStatus(id, status string) error
	SetNotes(id, notes string) error
}
```

Update the `List` implementation:

```go
func (s *service) List(filter JobFilter, page, limit int) ([]model.Job, int, error) {
	jobs, total, err := s.jobs.GetJobs(filter, page, limit)
	if err != nil {
		return nil, 0, domain.ErrInternal.WithMessage("failed to fetch jobs")
	}
	return jobs, int(total), nil
}
```

Remove `api/request` from the import block (no longer needed in service.go):

```go
import (
	"github.com/trushilpatel/jobradar/domain"
	"github.com/trushilpatel/jobradar/model"
	"github.com/trushilpatel/jobradar/store"
)
```

- [ ] **Step 2: Update `company.Service` interface and `List` implementation**

In `service/company/service.go`, change the `Service` interface:

```go
type Service interface {
	List(filter CompanyFilter, page, limit int) (CompanyListResult, error)
	GetAll() ([]CompanyView, error)
	Get(id string) (*model.Company, error)
	Create(input CreateCompanyInput) (*model.Company, error)
	Update(id string, input UpdateCompanyInput) error
	Delete(id string) error
	SetAllActive(active bool) error
}
```

Update the `List` implementation:

```go
func (s *service) List(filter CompanyFilter, page, limit int) (CompanyListResult, error) {
	companies, total, err := s.store.GetCompanies(filter, page, limit)
	if err != nil {
		return CompanyListResult{}, domain.ErrInternal.WithMessage("failed to fetch companies")
	}
	_, totalActive, err := s.store.GetCompanyCounts()
	if err != nil {
		return CompanyListResult{}, domain.ErrInternal.WithMessage("failed to fetch company counts")
	}
	views := make([]CompanyView, 0, len(companies))
	for _, c := range companies {
		v := CompanyView{Company: c}
		if c.LastScanAt != nil {
			t := c.LastScanAt.Add(time.Duration(s.config.ScanIntervalMinutes) * time.Minute)
			v.NextScanAt = &t
		}
		views = append(views, v)
	}
	return CompanyListResult{Items: views, Total: total, TotalActive: totalActive}, nil
}
```

Remove `api/request` from the import block:

```go
import (
	"time"

	"github.com/trushilpatel/jobradar/domain"
	"github.com/trushilpatel/jobradar/model"
	"github.com/trushilpatel/jobradar/store"
)
```

- [ ] **Step 3: Update `service/job/handler.go`**

Change the `List` handler to extract page/limit from pagination before calling the service:

```go
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	p := request.ParsePagination(r)
	q := r.URL.Query()

	filter := JobFilter{
		Status:           q.Get("status"),
		Companies:        q["companies"],
		EmployTypes:      q["employ_types"],
		ExperienceRanges: q["experience_ranges"],
	}

	if filter.Status == "" {
		filter.Status = model.JobStatusNew
	}

	if v := q.Get("remote"); v == "true" {
		t := true
		filter.Remote = &t
	}
	if v := q.Get("sponsorship"); v != "" {
		b := v == "true"
		filter.Sponsorship = &b
	}
	if q.Get("no_requirements") == "true" {
		filter.NoRequirements = true
	}
	if v := q.Get("requires_citizen"); v != "" {
		b := v == "true"
		filter.RequiresCitizen = &b
	}
	if q.Get("on_site") == "true" {
		filter.OnSite = true
	}
	if v := q.Get("days_added"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			filter.DaysAdded = &n
		}
	}

	if err := h.validator.ValidateList(filter, p); err != nil {
		handler.HandleError(w, err)
		return
	}

	jobs, total, err := h.jobs.List(filter, p.Page, p.Limit)
	if err != nil {
		handler.HandleError(w, err)
		return
	}

	handler.RespondPaged(w, http.StatusOK, jobs, total, p.Page, p.Limit)
}
```

- [ ] **Step 4: Update `service/company/handler.go`**

Change the `List` handler call:

```go
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	p := request.ParsePagination(r)
	q := r.URL.Query()

	var filter CompanyFilter
	if v := q.Get("adapter_type"); v != "" {
		filter.AdapterType = v
	}
	if v := q.Get("active"); v != "" {
		active := v == "true"
		filter.Active = &active
	}
	if v := q.Get("search"); v != "" {
		filter.Search = v
	}

	if err := h.validator.ValidateList(filter, p); err != nil {
		handler.HandleError(w, err)
		return
	}

	result, err := h.companies.List(filter, p.Page, p.Limit)
	if err != nil {
		handler.HandleError(w, err)
		return
	}

	handler.Respond(w, http.StatusOK, companyListResponse{
		Data:        result.Items,
		Total:       result.Total,
		TotalActive: result.TotalActive,
		Page:        p.Page,
		Limit:       p.Limit,
	})
}
```

- [ ] **Step 5: Update `service/job/service_test.go`**

Remove `api/request` from imports. Update `TestList_ReturnsPaged`:

```go
func TestList_ReturnsPaged(t *testing.T) {
	jobs := []model.Job{{ID: "j1"}, {ID: "j2"}}
	svc := NewService(&fakeJobStore{jobs: jobs, total: 2}, &fakeLedgerWriter{}, &fakeDiscardStore{}, &fakeSnapshotWriter{})

	result, total, err := svc.List(JobFilter{Status: "new"}, 1, 20)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 jobs, got %d", len(result))
	}
	if total != 2 {
		t.Errorf("expected total 2, got %d", total)
	}
}
```

Final imports for the file:

```go
import (
	"errors"
	"testing"

	"github.com/trushilpatel/jobradar/domain"
	"github.com/trushilpatel/jobradar/model"
	"gorm.io/gorm"
)
```

- [ ] **Step 6: Update `service/company/service_test.go`**

Remove `api/request` from imports. Update both `TestList_*` tests:

```go
func TestList_ComputesNextScanAt(t *testing.T) {
	lastScan := time.Now().Add(-1 * time.Hour)
	companies := []model.Company{
		{ID: "c1", Name: "C1", LastScanAt: &lastScan},
		{ID: "c2", Name: "C2", LastScanAt: nil},
	}
	svc := NewService(&fakeStore{companies: companies}, Config{ScanIntervalMinutes: 60})

	result, err := svc.List(CompanyFilter{}, 1, 20)
	// ... rest of test unchanged
}

func TestList_FiltersActiveCompanies(t *testing.T) {
	companies := []model.Company{{ID: "c1", Active: true}}
	svc := NewService(&fakeStore{companies: companies}, Config{})

	active := true
	result, err := svc.List(CompanyFilter{Active: &active}, 1, 20)
	// ... rest of test unchanged
}
```

Final imports for the file:

```go
import (
	"errors"
	"testing"
	"time"

	"github.com/trushilpatel/jobradar/domain"
	"github.com/trushilpatel/jobradar/model"
	"gorm.io/gorm"
)
```

- [ ] **Step 7: Verify compilation and tests**

```bash
cd backend && go test ./service/job/... ./service/company/... ./api/...
```

Expected: all PASS. The `scan` service still uses `response` in its `service.go` but the scan service's `api/response` import will be fixed in a later task (scan service doesn't need it for pipeline workers).

---

## Task 4: Fix cross-domain table write — add `UpdateScanStatus` to company domain

**Files:**

- Modify: `backend/service/company/store.go`
- Modify: `backend/service/scan/store.go`
- Modify: `backend/service/company/service_test.go`

- [ ] **Step 1: Add `UpdateScanStatus` to `CompanyWriter` in `service/company/store.go`**

Add the method to the interface:

```go
// CompanyWriter covers mutations from the API.
type CompanyWriter interface {
	CreateCompany(c model.Company) error
	UpdateCompany(id string, fields map[string]any) error
	DeleteCompany(id string) error
	SetAllCompaniesActive(active bool) error
	// UpdateScanStatus sets last_scan_at to NOW() and last_scan_status.
	// Called by the crawler engine after each company scan completes.
	UpdateScanStatus(id, status string) error
}
```

Add the GORM implementation on `companyStore` at the end of the store implementations section:

```go
func (s *companyStore) UpdateScanStatus(id, status string) error {
	return s.db.Model(&model.Company{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"last_scan_at":     gorm.Expr("NOW()"),
			"last_scan_status": status,
		}).Error
}
```

- [ ] **Step 2: Remove `UpdateCompanyScanStatus` from `service/scan/store.go`**

In `ScanLogWriter` interface, remove the method:

```go
// ScanLogWriter records scan summaries.
type ScanLogWriter interface {
	InsertCompanyScanLog(companyID string, jobsFetched, jobsNew, durationMs int, errMsg *string) error
}
```

Delete the GORM implementation `func (s *scanLogStore) UpdateCompanyScanStatus(...)` entirely (lines 264–271).

Also update the comment on `ScanLogWriter` to remove the reference to company status:

```go
// ScanLogWriter records scan run summaries to the company_scan_log table.
type ScanLogWriter interface {
	InsertCompanyScanLog(companyID string, jobsFetched, jobsNew, durationMs int, errMsg *string) error
}
```

- [ ] **Step 3: Add `UpdateScanStatus` to `fakeStore` in `service/company/service_test.go`**

Add the method to the `fakeStore` test double:

```go
func (f *fakeStore) UpdateScanStatus(_, _ string) error { return f.err }
```

- [ ] **Step 4: Verify compilation and tests**

```bash
cd backend && go test ./service/company/... ./service/scan/...
```

Expected: all PASS.

Note: `pipeline/crawler/engine.go` will now fail to compile because it calls `e.scanLog.UpdateCompanyScanStatus` which no longer exists on `ScanLogWriter`. That is expected — it gets fixed in Task 7.

---

## Task 5: Create `job.PipelineService` — move `buildJob` logic from analyzer

**Files:**

- Create: `backend/service/job/pipeline_service.go`
- Create: `backend/service/job/pipeline_service_test.go`

- [ ] **Step 1: Write the failing tests for `buildJobRow`**

Create `backend/service/job/pipeline_service_test.go`:

```go
package job

import (
	"testing"

	"github.com/trushilpatel/jobradar/model"
)

// ── buildJobRow tests (moved from pipeline/analyzer/worker_test.go) ──────────

func TestBuildJobRow_FieldMapping(t *testing.T) {
	exp := 3
	min, max := 80000, 120000
	input := EnrichedJobInput{
		JobPostID:         "greenhouse-999",
		CompanyID:         "company-abc",
		Title:             "Backend Engineer",
		Location:          "San Francisco, CA",
		URL:               "https://example.com/jobs/999",
		EmployType:        model.EmployFullTime,
		ExperienceYears:   &exp,
		TechStack:         []string{"Go", "Postgres"},
		SalaryMin:         &min,
		SalaryMax:         &max,
		SalaryCurrency:    "USD",
		LLMSummary:        "A great role.",
		Remote:            true,
		Country:           "USA",
		State:             "CA",
		Sponsorship:       true,
		RequiresCitizen:   false,
		SecurityClearance: false,
		RoleBrief:         "Build APIs.",
		Responsibilities:  []string{"Design systems"},
		RoleCategory:      "backend",
	}

	job, err := buildJobRow(input)
	if err != nil {
		t.Fatalf("buildJobRow error: %v", err)
	}

	if job.JobPostID != input.JobPostID {
		t.Errorf("JobPostID: want %q got %q", input.JobPostID, job.JobPostID)
	}
	if job.CompanyID != input.CompanyID {
		t.Errorf("CompanyID: want %q got %q", input.CompanyID, job.CompanyID)
	}
	if job.Title != input.Title {
		t.Errorf("Title: want %q got %q", input.Title, job.Title)
	}
	if !job.Remote {
		t.Error("Remote: want true got false")
	}
	if job.Country == nil || *job.Country != "USA" {
		t.Errorf("Country: want 'USA' got %v", job.Country)
	}
	if job.State == nil || *job.State != "CA" {
		t.Errorf("State: want 'CA' got %v", job.State)
	}
	if !job.Sponsorship {
		t.Error("Sponsorship: want true")
	}
	if job.Status != model.JobStatusNew {
		t.Errorf("Status: want %q got %q", model.JobStatusNew, job.Status)
	}
	if job.ID == "" {
		t.Error("ID should be a non-empty UUID")
	}
}

func TestBuildJobRow_NullableStrings(t *testing.T) {
	input := EnrichedJobInput{
		JobPostID: "lever-1",
		CompanyID: "c1",
		Title:     "SWE",
		URL:       "https://x.com",
		TechStack: []string{},
	}

	job, err := buildJobRow(input)
	if err != nil {
		t.Fatalf("buildJobRow error: %v", err)
	}
	if job.Location != nil {
		t.Error("empty location should be nil")
	}
	if job.Country != nil {
		t.Error("empty country should be nil")
	}
	if job.Remote {
		t.Error("Remote should be false when input.Remote is false")
	}
}

// ── PipelineService unit tests ─────────────────────────────────────────────────

type fakePipelineJobWriter struct {
	inserted []*model.Job
	err      error
}

func (f *fakePipelineJobWriter) InsertJob(job *model.Job) error {
	if f.err != nil {
		return f.err
	}
	f.inserted = append(f.inserted, job)
	return nil
}
func (f *fakePipelineJobWriter) DeleteJob(_ string) error          { return nil }
func (f *fakePipelineJobWriter) UpdateJobStatus(_, _ string) error { return nil }
func (f *fakePipelineJobWriter) UpdateJobSaved(_ string, _ bool) error { return nil }
func (f *fakePipelineJobWriter) UpdateJobNotes(_, _ string) error  { return nil }

type fakePipelineLedger struct {
	seenIDs    map[string]bool
	reasons    map[string]string
	batchReasons []string
}

func (f *fakePipelineLedger) HasSeen(jobID string) (bool, error) {
	return f.seenIDs[jobID], nil
}
func (f *fakePipelineLedger) MarkSeen(jobID, _, _ string) error {
	if f.seenIDs == nil {
		f.seenIDs = make(map[string]bool)
	}
	f.seenIDs[jobID] = true
	return nil
}
func (f *fakePipelineLedger) UpdateSeenReason(jobID, reason string) error {
	if f.reasons == nil {
		f.reasons = make(map[string]string)
	}
	f.reasons[jobID] = reason
	return nil
}
func (f *fakePipelineLedger) UpdateSeenReasonBatch(ids []string, reason string) error {
	f.batchReasons = append(f.batchReasons, reason)
	return nil
}
func (f *fakePipelineLedger) MarkRemovedBatch(_ []string) error                    { return nil }
func (f *fakePipelineLedger) GetActiveJobIDsByCompany(_ string) ([]string, error)  { return nil, nil }

type fakePipelineSnapshot struct {
	called bool
	err    error
}

func (f *fakePipelineSnapshot) InsertSnapshot(_ SnapshotInput) error {
	f.called = true
	return f.err
}
func (f *fakePipelineSnapshot) UpdateSnapshotAction(_, _ string) error { return nil }

var _ SeenLedgerStore = (*fakePipelineLedger)(nil)

func TestPipelineService_InsertEnrichedJob_HappyPath(t *testing.T) {
	jobs := &fakePipelineJobWriter{}
	ledger := &fakePipelineLedger{}
	snap := &fakePipelineSnapshot{}
	svc := NewPipelineService(jobs, ledger, snap)

	err := svc.InsertEnrichedJob(EnrichedJobInput{
		JobPostID:   "greenhouse-1",
		CompanyID:   "stripe",
		Title:       "Backend Engineer",
		URL:         "https://example.com",
		TechStack:   []string{"Go"},
		Sponsorship: true,
	})
	if err != nil {
		t.Fatalf("InsertEnrichedJob error: %v", err)
	}
	if len(jobs.inserted) != 1 {
		t.Errorf("expected 1 inserted job, got %d", len(jobs.inserted))
	}
	if !snap.called {
		t.Error("expected snapshot to be called")
	}
}

func TestPipelineService_InsertEnrichedJob_SnapshotFailureIsNonFatal(t *testing.T) {
	jobs := &fakePipelineJobWriter{}
	ledger := &fakePipelineLedger{}
	snap := &fakePipelineSnapshot{err: errSimulated}
	svc := NewPipelineService(jobs, ledger, snap)

	err := svc.InsertEnrichedJob(EnrichedJobInput{
		JobPostID: "lever-2",
		CompanyID: "stripe",
		Title:     "SWE",
		URL:       "https://example.com",
		TechStack: []string{},
	})
	if err != nil {
		t.Fatalf("InsertEnrichedJob should succeed even when snapshot fails, got: %v", err)
	}
	if len(jobs.inserted) != 1 {
		t.Error("job should be inserted even when snapshot fails")
	}
}

func TestPipelineService_MarkLedgerIrrelevantBatch(t *testing.T) {
	ledger := &fakePipelineLedger{}
	svc := NewPipelineService(&fakePipelineJobWriter{}, ledger, &fakePipelineSnapshot{})

	if err := svc.MarkLedgerIrrelevantBatch([]string{"j1", "j2"}); err != nil {
		t.Fatalf("MarkLedgerIrrelevantBatch error: %v", err)
	}
	if len(ledger.batchReasons) != 1 || ledger.batchReasons[0] != model.SeenReasonLLMIrrelevant {
		t.Errorf("expected reason %q, got %v", model.SeenReasonLLMIrrelevant, ledger.batchReasons)
	}
}

func TestPipelineService_MarkLedgerErrorBatch(t *testing.T) {
	ledger := &fakePipelineLedger{}
	svc := NewPipelineService(&fakePipelineJobWriter{}, ledger, &fakePipelineSnapshot{})

	if err := svc.MarkLedgerErrorBatch([]string{"j1"}); err != nil {
		t.Fatalf("MarkLedgerErrorBatch error: %v", err)
	}
	if len(ledger.batchReasons) != 1 || ledger.batchReasons[0] != model.SeenReasonError {
		t.Errorf("expected reason %q, got %v", model.SeenReasonError, ledger.batchReasons)
	}
}

// errSimulated is a sentinel error used in tests.
var errSimulated = fmt.Errorf("simulated error")
```

Add `"fmt"` to the imports.

- [ ] **Step 2: Run test to verify it fails**

```bash
cd backend && go test ./service/job/... -run TestBuildJobRow -v
```

Expected: FAIL — `buildJobRow` not defined, `PipelineService` not defined, `EnrichedJobInput` not defined.

- [ ] **Step 3: Create `backend/service/job/pipeline_service.go`**

```go
package job

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/trushilpatel/jobradar/model"
	"gorm.io/datatypes"
)

// PipelineService is the interface pipeline workers use for all job domain operations.
// It carries no api/* imports and returns plain Go errors.
type PipelineService interface {
	// InsertEnrichedJob writes a completed enriched job to job_feed and records its
	// analytics snapshot. The snapshot write is non-fatal — job row is saved even if
	// snapshot fails.
	InsertEnrichedJob(input EnrichedJobInput) error

	// HasSeenJob checks if a job already exists in job_pipeline_ledger.
	HasSeenJob(jobID string) (bool, error)
	// MarkLedgerSeen records a new job in job_pipeline_ledger.
	MarkLedgerSeen(jobID, companyID, reason string) error
	// MarkLedgerError updates a job's reason to 'error' in job_pipeline_ledger.
	MarkLedgerError(jobID string) error
	// MarkLedgerErrorBatch updates multiple jobs' reason to 'error'.
	MarkLedgerErrorBatch(jobIDs []string) error
	// MarkLedgerIrrelevantBatch updates multiple jobs to 'llm_irrelevant'.
	// Encapsulates the reason constant so callers don't depend on it.
	MarkLedgerIrrelevantBatch(jobIDs []string) error
	// GetActiveJobIDsByCompany returns ledger-active job IDs for removal detection.
	GetActiveJobIDsByCompany(companyID string) ([]string, error)
	// MarkRemovedBatch timestamps removed_at for jobs no longer on a careers page.
	MarkRemovedBatch(jobIDs []string) error
}

// EnrichedJobInput carries all data needed to write a completed job row + snapshot.
// Produced by the analyzer after LLM enrichment succeeds.
type EnrichedJobInput struct {
	JobPostID         string
	CompanyID         string
	Title             string
	Location          string
	URL               string
	Remote            bool
	Country           string
	State             string
	EmployType        string
	ExperienceYears   *int
	TechStack         []string
	Sponsorship       bool
	RequiresCitizen   bool
	SecurityClearance bool
	RoleBrief         string
	Responsibilities  []string
	LLMSummary        string
	SalaryMin         *int
	SalaryMax         *int
	SalaryCurrency    string
	RoleCategory      string
}

type pipelineService struct {
	jobs     JobWriter
	ledger   SeenLedgerStore
	snapshot SnapshotWriter
}

// NewPipelineService constructs a PipelineService implementation.
func NewPipelineService(jobs JobWriter, ledger SeenLedgerStore, snapshot SnapshotWriter) PipelineService {
	return &pipelineService{jobs: jobs, ledger: ledger, snapshot: snapshot}
}

func (s *pipelineService) InsertEnrichedJob(input EnrichedJobInput) error {
	row, err := buildJobRow(input)
	if err != nil {
		return fmt.Errorf("build job row: %w", err)
	}
	if err := s.jobs.InsertJob(row); err != nil {
		return fmt.Errorf("insert job: %w", err)
	}
	techBytes, _ := json.Marshal(input.TechStack)
	// Snapshot write is non-fatal — job is already saved.
	_ = s.snapshot.InsertSnapshot(SnapshotInput{
		ID:                row.ID,
		CompanyID:         input.CompanyID,
		Country:           input.Country,
		State:             input.State,
		Remote:            input.Remote,
		Sponsorship:       input.Sponsorship,
		RequiresCitizen:   input.RequiresCitizen,
		SecurityClearance: input.SecurityClearance,
		EmployType:        input.EmployType,
		ExperienceYears:   input.ExperienceYears,
		TechStack:         techBytes,
		RoleCategory:      input.RoleCategory,
	})
	return nil
}

func (s *pipelineService) HasSeenJob(jobID string) (bool, error) {
	return s.ledger.HasSeen(jobID)
}

func (s *pipelineService) MarkLedgerSeen(jobID, companyID, reason string) error {
	return s.ledger.MarkSeen(jobID, companyID, reason)
}

func (s *pipelineService) MarkLedgerError(jobID string) error {
	return s.ledger.UpdateSeenReason(jobID, model.SeenReasonError)
}

func (s *pipelineService) MarkLedgerErrorBatch(jobIDs []string) error {
	return s.ledger.UpdateSeenReasonBatch(jobIDs, model.SeenReasonError)
}

func (s *pipelineService) MarkLedgerIrrelevantBatch(jobIDs []string) error {
	return s.ledger.UpdateSeenReasonBatch(jobIDs, model.SeenReasonLLMIrrelevant)
}

func (s *pipelineService) GetActiveJobIDsByCompany(companyID string) ([]string, error) {
	return s.ledger.GetActiveJobIDsByCompany(companyID)
}

func (s *pipelineService) MarkRemovedBatch(jobIDs []string) error {
	return s.ledger.MarkRemovedBatch(jobIDs)
}

// buildJobRow constructs a model.Job ready for DB insert from an EnrichedJobInput.
// Private — callers use InsertEnrichedJob.
func buildJobRow(input EnrichedJobInput) (*model.Job, error) {
	techStack, err := json.Marshal(input.TechStack)
	if err != nil {
		return nil, fmt.Errorf("marshal tech_stack: %w", err)
	}
	responsibilities, err := json.Marshal(input.Responsibilities)
	if err != nil {
		return nil, fmt.Errorf("marshal responsibilities: %w", err)
	}
	salaryRange, err := json.Marshal(map[string]any{
		"min":      input.SalaryMin,
		"max":      input.SalaryMax,
		"currency": input.SalaryCurrency,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal salary_range: %w", err)
	}
	return &model.Job{
		ID:                uuid.New().String(),
		JobPostID:         input.JobPostID,
		CompanyID:         input.CompanyID,
		Title:             input.Title,
		Location:          nullableStr(input.Location),
		Remote:            input.Remote,
		Country:           nullableStr(input.Country),
		State:             nullableStr(input.State),
		URL:               input.URL,
		EmployType:        nullableStr(input.EmployType),
		ExperienceYears:   input.ExperienceYears,
		TechStack:         datatypes.JSON(techStack),
		SalaryRange:       datatypes.JSON(salaryRange),
		LLMSummary:        nullableStr(input.LLMSummary),
		Sponsorship:       input.Sponsorship,
		RequiresCitizen:   input.RequiresCitizen,
		SecurityClearance: input.SecurityClearance,
		RoleBrief:         nullableStr(input.RoleBrief),
		Responsibilities:  datatypes.JSON(responsibilities),
		Status:            model.JobStatusNew,
	}, nil
}

// nullableStr converts an empty string to nil, used for nullable DB columns.
func nullableStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// Ensure pipelineService satisfies the interface at compile time.
var _ PipelineService = (*pipelineService)(nil)

// contextKey is needed to satisfy the context import if unused elsewhere — remove if unused.
var _ = context.Background // keep context import for interface doc; remove if linter complains
```

Note: remove the `context` import if it causes an "imported and not used" error. The `context` package is only needed if any method uses it — none do in this implementation.

Final imports:

```go
import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/trushilpatel/jobradar/model"
	"gorm.io/datatypes"
)
```

- [ ] **Step 4: Run the tests to verify they pass**

```bash
cd backend && go test ./service/job/... -run "TestBuildJobRow|TestPipelineService" -v
```

Expected: all PASS.

---

## Task 6: Create `scan.CrawlerService`

**Files:**

- Create: `backend/service/scan/crawler_service.go`

- [ ] **Step 1: Create the file**

```go
// backend/service/scan/crawler_service.go
package scan

import "github.com/trushilpatel/jobradar/model"

// CrawlerService is the interface the crawler engine uses to record scan results.
// It coordinates the scan log write and company status update together.
type CrawlerService interface {
	// RecordScanResult writes a scan log entry and updates company.last_scan_at +
	// last_scan_status. Both writes always happen together — never one without the other.
	RecordScanResult(companyID string, fetched, newCount, durationMs int, errMsg *string) error
}

// companyScanUpdater is the narrow interface CrawlerService needs to update
// company scan status. Satisfied by company.CompanyWriter.
type companyScanUpdater interface {
	UpdateScanStatus(id, status string) error
}

type crawlerService struct {
	log     ScanLogWriter
	company companyScanUpdater
}

// NewCrawlerService constructs a CrawlerService.
// log writes to company_scan_log; company updates companies.last_scan_at.
func NewCrawlerService(log ScanLogWriter, company companyScanUpdater) CrawlerService {
	return &crawlerService{log: log, company: company}
}

func (s *crawlerService) RecordScanResult(companyID string, fetched, newCount, durationMs int, errMsg *string) error {
	if err := s.log.InsertCompanyScanLog(companyID, fetched, newCount, durationMs, errMsg); err != nil {
		return err
	}
	status := model.ScanStatusOK
	if errMsg != nil {
		status = model.ScanStatusError
	}
	return s.company.UpdateScanStatus(companyID, status)
}

// Ensure crawlerService satisfies the interface at compile time.
var _ CrawlerService = (*crawlerService)(nil)
```

- [ ] **Step 2: Verify compilation**

```bash
cd backend && go build ./service/scan/...
```

Expected: no output, exit 0.

---

## Task 7: Create pipeline interface files + update crawler engine

**Files:**

- Create: `backend/pipeline/crawler/interfaces.go`
- Create: `backend/pipeline/filter/interfaces.go`
- Modify: `backend/pipeline/crawler/engine.go`

- [ ] **Step 1: Create `backend/pipeline/crawler/interfaces.go`**

```go
// backend/pipeline/crawler/interfaces.go
package crawler

import "github.com/trushilpatel/jobradar/model"

// CompanyReader is the interface the crawler engine uses to look up company data.
// Satisfied by service/company.Store.
type CompanyReader interface {
	GetActiveCompanies() ([]model.Company, error)
	GetCompanyByID(id string) (model.Company, error)
}

// PipelineLedger is the interface the crawler engine uses for seen-ledger operations.
// Satisfied by service/job.PipelineService.
type PipelineLedger interface {
	HasSeenJob(jobID string) (bool, error)
	MarkLedgerSeen(jobID, companyID, reason string) error
	GetActiveJobIDsByCompany(companyID string) ([]string, error)
	MarkRemovedBatch(jobIDs []string) error
	MarkLedgerError(jobID string) error
}

// ScanRecorder is the interface the crawler engine uses to record scan results.
// Satisfied by service/scan.CrawlerService.
type ScanRecorder interface {
	RecordScanResult(companyID string, fetched, newCount, durationMs int, errMsg *string) error
}
```

- [ ] **Step 2: Create `backend/pipeline/filter/interfaces.go`**

```go
// backend/pipeline/filter/interfaces.go
package filter

// FilterLedger is the interface the filter worker uses for pipeline ledger operations.
// Satisfied by service/job.PipelineService.
// Using named methods (not UpdateSeenReasonBatch) so the worker expresses intent,
// not implementation detail.
type FilterLedger interface {
	MarkLedgerErrorBatch(jobIDs []string) error
	MarkLedgerIrrelevantBatch(jobIDs []string) error
}
```

- [ ] **Step 3: Update `backend/pipeline/crawler/engine.go`**

Replace the entire file with the updated version. Key changes:

1. Remove `companysvc` and `scansvc` imports — use local interfaces
2. Remove the local `jobLedger` interface (replaced by `PipelineLedger` from interfaces.go)
3. Change struct field types
4. Update method names for ledger calls (`HasSeen` → `HasSeenJob`, `MarkSeen` → `MarkLedgerSeen`)
5. Simplify `recordResult` to a single `RecordScanResult` call

```go
package crawler

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/trushilpatel/jobradar/pipeline/ats"
	"github.com/trushilpatel/jobradar/pipeline/ats/google"
	"github.com/trushilpatel/jobradar/infra/kafka"
	"github.com/trushilpatel/jobradar/infra/logger"
	"github.com/trushilpatel/jobradar/infra/metrics"
	"github.com/trushilpatel/jobradar/model"
)

// Engine crawls company careers pages, deduplicates jobs, and publishes new ones to Kafka.
type Engine struct {
	companies CompanyReader   // looks up company data
	ledger    PipelineLedger  // seen-ledger dedup
	scanLog   ScanRecorder    // records scan results
	publisher kafka.Publisher
	googleCfg google.Config
	log       logger.Logger
}

func NewEngine(
	companies CompanyReader,
	ledger PipelineLedger,
	scanLog ScanRecorder,
	publisher kafka.Publisher,
	googleCfg google.Config,
	log logger.Logger,
) *Engine {
	return &Engine{
		companies: companies,
		ledger:    ledger,
		scanLog:   scanLog,
		publisher: publisher,
		googleCfg: googleCfg,
		log:       log,
	}
}

// RunOnce runs one full scan cycle across all active companies.
func (e *Engine) RunOnce(ctx context.Context) {
	companies, err := e.companies.GetActiveCompanies()
	if err != nil {
		e.log.Error("failed to load companies", err)
		return
	}
	e.log.Info("starting scan cycle", logger.Int("active_companies", len(companies)))
	for _, company := range companies {
		e.scanCompany(ctx, company)
	}
	e.log.Info("scan cycle complete")
}

// RunForCompany runs a scan for a single company by ID.
func (e *Engine) RunForCompany(ctx context.Context, companyID string) {
	company, err := e.companies.GetCompanyByID(companyID)
	if err != nil {
		e.log.Error("company not found", err, logger.Str(logger.FieldCompany, companyID))
		return
	}
	if !company.Active {
		e.log.Warn("skipping inactive company",
			logger.Str(logger.FieldCompany, company.ID),
			logger.Str("name", company.Name),
		)
		return
	}
	e.log.Debug("triggered scan",
		logger.Str(logger.FieldCompany, company.ID),
		logger.Str("name", company.Name),
	)
	e.scanCompany(ctx, company)
}

func (e *Engine) scanCompany(ctx context.Context, company model.Company) {
	start := time.Now()
	companyLog := e.log.With(
		logger.Str(logger.FieldCompany, company.ID),
		logger.Str("name", company.Name),
	)

	adp, err := ats.RegistryWithGoogleConfig(company.AdapterType, e.googleCfg)
	if err != nil {
		errMsg := fmt.Sprintf("no adapter for type %q: %v", company.AdapterType, err)
		companyLog.Error("no adapter found", err)
		e.recordResult(ctx, company.ID, 0, 0, time.Since(start), &errMsg)
		metrics.CrawlerScansTotal.WithLabelValues("error").Inc()
		return
	}

	result, err := adp.FetchJobs(ctx, company.ID)
	if err != nil {
		errMsg := fmt.Sprintf("fetch failed: %v", err)
		companyLog.Error("fetch jobs failed", err)
		e.recordResult(ctx, company.ID, 0, 0, time.Since(start), &errMsg)
		metrics.CrawlerScansTotal.WithLabelValues("error").Inc()
		return
	}

	fetched := len(result.Jobs)
	newCount := 0

	for _, item := range result.Jobs {
		seen, err := e.ledger.HasSeenJob(item.ID)
		if err != nil {
			companyLog.Error("seen_ledger check failed", err, logger.Str(logger.FieldJobID, item.ID))
			continue
		}
		if seen {
			continue
		}

		if err := e.ledger.MarkLedgerSeen(item.ID, company.ID, model.SeenReasonFetch); err != nil {
			companyLog.Error("failed to mark seen", err, logger.Str(logger.FieldJobID, item.ID))
			continue
		}

		rawJob := toRawJob(item, company.ID, company.AdapterType)
		if err := e.publish(ctx, rawJob); err != nil {
			companyLog.Error("publish failed", err, logger.Str(logger.FieldJobID, item.ID))
			continue
		}

		newCount++
	}

	e.detectRemovedJobs(company.ID, result.Jobs, companyLog)

	elapsed := time.Since(start)
	metrics.CrawlerScansTotal.WithLabelValues("success").Inc()
	metrics.CrawlerJobsFetchedTotal.Add(float64(fetched))
	metrics.CrawlerScanDuration.Observe(elapsed.Seconds())

	companyLog.Info("scan complete",
		logger.Int(logger.FieldFetched, fetched),
		logger.Int(logger.FieldNew, newCount),
		logger.Dur(logger.FieldDuration, elapsed.Round(time.Millisecond)),
	)
	e.recordResult(ctx, company.ID, fetched, newCount, elapsed, nil)
}

func toRawJob(item ats.FetchJobsItem, companyID, adapterType string) ats.RawJob {
	return ats.RawJob{
		ID:          item.ID,
		CompanyID:   companyID,
		Adapter:     adapterType,
		Title:       item.Title,
		Location:    item.Location,
		URL:         item.URL,
		EmployType:  item.EmploymentType,
		FullContent: item.FullContent,
	}
}

func (e *Engine) publish(ctx context.Context, job ats.RawJob) error {
	payload, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	return e.publisher.Publish(ctx, kafka.TopicJobListingsRaw, payload)
}

func (e *Engine) detectRemovedJobs(companyID string, currentJobs []ats.FetchJobsItem, log logger.Logger) {
	previousIDs, err := e.ledger.GetActiveJobIDsByCompany(companyID)
	if err != nil {
		log.Error("failed to load previous job IDs for removal detection", err)
		return
	}
	if len(previousIDs) == 0 {
		return
	}

	currentSet := make(map[string]struct{}, len(currentJobs))
	for _, job := range currentJobs {
		currentSet[job.ID] = struct{}{}
	}

	var removedIDs []string
	for _, id := range previousIDs {
		if _, exists := currentSet[id]; !exists {
			removedIDs = append(removedIDs, id)
		}
	}

	if len(removedIDs) == 0 {
		return
	}

	if err := e.ledger.MarkRemovedBatch(removedIDs); err != nil {
		log.Error("failed to mark removed jobs", err, logger.Int("count", len(removedIDs)))
		return
	}

	log.Info("marked removed jobs",
		logger.Int("removed", len(removedIDs)),
		logger.Int("still_active", len(previousIDs)-len(removedIDs)),
	)
}

func (e *Engine) recordResult(ctx context.Context, companyID string, fetched, newCount int, duration time.Duration, errMsg *string) {
	durationMs := int(duration.Milliseconds())
	if err := e.scanLog.RecordScanResult(companyID, fetched, newCount, durationMs, errMsg); err != nil {
		e.log.Error("failed to record scan result", err, logger.Str(logger.FieldCompany, companyID))
	}
}
```

- [ ] **Step 4: Verify compilation**

```bash
cd backend && go build ./pipeline/crawler/...
```

Expected: no output, exit 0.

---

## Task 8: Update filter worker to use `FilterLedger`

**Files:**

- Modify: `backend/pipeline/filter/worker.go`
- Modify: `backend/pipeline/filter/worker_test.go`

- [ ] **Step 1: Update `backend/pipeline/filter/worker.go`**

Change the `ledger` field type and remove the `jobsvc` import. Find and update:

```go
import (
	"context"
	"encoding/json"
	"time"

	"github.com/trushilpatel/jobradar/pipeline/ats"
	"github.com/trushilpatel/jobradar/infra/kafka"
	"github.com/trushilpatel/jobradar/pipeline/llm"
	"github.com/trushilpatel/jobradar/infra/logger"
	"github.com/trushilpatel/jobradar/infra/metrics"
)
```

Update the `Worker` struct:

```go
type Worker struct {
	consumer     *kafka.Consumer
	publisher    kafka.Publisher
	scorer       llm.Scorer
	ledger       FilterLedger      // was: jobsvc.SeenLedgerWriter
	batchSize    int
	flushTimeout time.Duration
	log          logger.Logger
}
```

Update `NewWorker`:

```go
func NewWorker(
	consumer *kafka.Consumer,
	publisher kafka.Publisher,
	scorer llm.Scorer,
	ledger FilterLedger,
	batchSize int,
	flushTimeoutSeconds int,
	log logger.Logger,
) *Worker {
	return &Worker{
		consumer:     consumer,
		publisher:    publisher,
		scorer:       scorer,
		ledger:       ledger,
		batchSize:    batchSize,
		flushTimeout: time.Duration(flushTimeoutSeconds) * time.Second,
		log:          log,
	}
}
```

Update `flushBatch` — replace both `UpdateSeenReasonBatch` calls with semantic methods:

```go
func (w *Worker) flushBatch(ctx context.Context, batch []ats.RawJob) {
	inputs := make([]llm.ScorerInput, len(batch))
	for i, job := range batch {
		inputs[i] = llm.ScorerInput{ID: job.ID, Title: job.Title}
	}

	start := time.Now()
	relevantIDs, err := w.scorer.ScoreBatch(ctx, inputs)
	metrics.FilterLLMDuration.Observe(time.Since(start).Seconds())

	if err != nil {
		w.log.Error("ScoreBatch failed, marking batch as error", err, logger.Int(logger.FieldBatch, len(batch)))
		ids := make([]string, len(batch))
		for i, job := range batch {
			ids[i] = job.ID
		}
		if merr := w.ledger.MarkLedgerErrorBatch(ids); merr != nil {
			w.log.Error("mark error batch failed", merr)
		}
		metrics.FilterJobsTotal.WithLabelValues("error").Add(float64(len(batch)))
		return
	}

	relevant := make(map[string]bool, len(relevantIDs))
	for _, id := range relevantIDs {
		relevant[id] = true
	}

	var irrelevantIDs []string
	for _, job := range batch {
		if relevant[job.ID] {
			w.publishDetail(ctx, job)
		} else {
			irrelevantIDs = append(irrelevantIDs, job.ID)
		}
	}

	if len(irrelevantIDs) > 0 {
		if err := w.ledger.MarkLedgerIrrelevantBatch(irrelevantIDs); err != nil {
			w.log.Error("mark irrelevant failed", err)
		}
	}

	metrics.FilterJobsTotal.WithLabelValues("relevant").Add(float64(len(relevantIDs)))
	metrics.FilterJobsTotal.WithLabelValues("irrelevant").Add(float64(len(irrelevantIDs)))

	w.log.Info("batch scored",
		logger.Int(logger.FieldBatch, len(batch)),
		logger.Int(logger.FieldRelevant, len(relevantIDs)),
		logger.Int("irrelevant", len(batch)-len(relevantIDs)),
	)
}
```

- [ ] **Step 2: Update `backend/pipeline/filter/worker_test.go`**

The `fakeLedger` currently satisfies `SeenLedgerWriter` (4 methods). After the change it only needs to satisfy `FilterLedger` (2 methods). Update the fake and the test assertions:

```go
package filter

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/trushilpatel/jobradar/pipeline/ats"
	"github.com/trushilpatel/jobradar/pipeline/llm"
	"github.com/trushilpatel/jobradar/infra/logger"
)

// ── fakes ─────────────────────────────────────────────────────────────────────

type fakeScorer struct {
	relevantIDs []string
	err         error
	calls       int
}

func (f *fakeScorer) ScoreBatch(_ context.Context, jobs []llm.ScorerInput) ([]string, error) {
	if len(jobs) == 0 {
		return nil, nil
	}
	f.calls++
	return f.relevantIDs, f.err
}

func (f *fakeScorer) Enrich(_ context.Context, _ llm.EnrichInput) (*llm.EnrichResult, error) {
	return nil, nil
}

type ledgerCall struct {
	ids    []string
	method string // "error" or "irrelevant"
}

type fakeLedger struct {
	calls []ledgerCall
}

func (f *fakeLedger) MarkLedgerErrorBatch(ids []string) error {
	f.calls = append(f.calls, ledgerCall{ids: ids, method: "error"})
	return nil
}

func (f *fakeLedger) MarkLedgerIrrelevantBatch(ids []string) error {
	f.calls = append(f.calls, ledgerCall{ids: ids, method: "irrelevant"})
	return nil
}

// Ensure fakeLedger satisfies FilterLedger.
var _ FilterLedger = (*fakeLedger)(nil)

type fakePublisher struct {
	payloads [][]byte
}

func (f *fakePublisher) Publish(_ context.Context, _ string, value []byte) error {
	f.payloads = append(f.payloads, value)
	return nil
}

// ── helpers ────────────────────────────────────────────────────────────────────

func makeWorker(scorer *fakeScorer, ledger *fakeLedger, pub *fakePublisher) *Worker {
	return &Worker{
		scorer:    scorer,
		ledger:    ledger,
		publisher: pub,
		batchSize: 10,
		log:       logger.NewNoop(),
	}
}

func makeJob(id string) ats.RawJob {
	return ats.RawJob{ID: id, Title: "Software Engineer"}
}

// ── tests ──────────────────────────────────────────────────────────────────────

func TestFlushBatch_AllRelevant(t *testing.T) {
	scorer := &fakeScorer{relevantIDs: []string{"job-1", "job-2", "job-3"}}
	ledger := &fakeLedger{}
	pub := &fakePublisher{}

	w := makeWorker(scorer, ledger, pub)
	w.flushBatch(context.Background(), []ats.RawJob{
		makeJob("job-1"), makeJob("job-2"), makeJob("job-3"),
	})

	if len(pub.payloads) != 3 {
		t.Errorf("expected 3 published jobs, got %d", len(pub.payloads))
	}
	if len(ledger.calls) != 0 {
		t.Errorf("expected no ledger calls, got %d", len(ledger.calls))
	}
}

func TestFlushBatch_PartialRelevant(t *testing.T) {
	scorer := &fakeScorer{relevantIDs: []string{"job-1", "job-3"}}
	ledger := &fakeLedger{}
	pub := &fakePublisher{}

	w := makeWorker(scorer, ledger, pub)
	w.flushBatch(context.Background(), []ats.RawJob{
		makeJob("job-1"), makeJob("job-2"), makeJob("job-3"),
	})

	if len(pub.payloads) != 2 {
		t.Errorf("expected 2 published jobs, got %d", len(pub.payloads))
	}

	for _, payload := range pub.payloads {
		var job ats.RawJob
		if err := json.Unmarshal(payload, &job); err != nil {
			t.Fatalf("unmarshal published payload: %v", err)
		}
		if job.ID != "job-1" && job.ID != "job-3" {
			t.Errorf("unexpected published job ID: %s", job.ID)
		}
	}

	if len(ledger.calls) != 1 {
		t.Fatalf("expected 1 ledger call, got %d", len(ledger.calls))
	}
	call := ledger.calls[0]
	if call.method != "irrelevant" {
		t.Errorf("expected method 'irrelevant', got %q", call.method)
	}
	if len(call.ids) != 1 || call.ids[0] != "job-2" {
		t.Errorf("expected irrelevant IDs [job-2], got %v", call.ids)
	}
}

func TestFlushBatch_ScorerError(t *testing.T) {
	scorer := &fakeScorer{err: context.DeadlineExceeded}
	ledger := &fakeLedger{}
	pub := &fakePublisher{}

	w := makeWorker(scorer, ledger, pub)
	w.flushBatch(context.Background(), []ats.RawJob{
		makeJob("job-1"), makeJob("job-2"),
	})

	if len(pub.payloads) != 0 {
		t.Errorf("expected no published jobs on scorer error, got %d", len(pub.payloads))
	}
	if len(ledger.calls) != 1 {
		t.Fatalf("expected 1 ledger call, got %d", len(ledger.calls))
	}
	call := ledger.calls[0]
	if call.method != "error" {
		t.Errorf("expected method 'error', got %q", call.method)
	}
	if len(call.ids) != 2 {
		t.Errorf("expected 2 error IDs, got %v", call.ids)
	}
}

func TestFlushBatch_EmptyBatch(t *testing.T) {
	scorer := &fakeScorer{}
	ledger := &fakeLedger{}
	pub := &fakePublisher{}

	w := makeWorker(scorer, ledger, pub)
	w.flushBatch(context.Background(), []ats.RawJob{})

	if scorer.calls != 0 {
		t.Errorf("expected scorer not called on empty batch, got %d calls", scorer.calls)
	}
	if len(pub.payloads) != 0 {
		t.Errorf("expected no publishes, got %d", len(pub.payloads))
	}
	if len(ledger.calls) != 0 {
		t.Errorf("expected no ledger calls, got %d", len(ledger.calls))
	}
}
```

- [ ] **Step 3: Run filter tests**

```bash
cd backend && go test ./pipeline/filter/... -v
```

Expected: all PASS.

---

## Task 9: Update analyzer worker to use `job.PipelineService`

**Files:**

- Modify: `backend/pipeline/analyzer/worker.go`
- Modify: `backend/pipeline/analyzer/worker_test.go`

- [ ] **Step 1: Update `backend/pipeline/analyzer/worker.go`**

Replace the full file:

```go
package analyzer

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/trushilpatel/jobradar/pipeline/ats"
	"github.com/trushilpatel/jobradar/infra/kafka"
	"github.com/trushilpatel/jobradar/pipeline/llm"
	"github.com/trushilpatel/jobradar/infra/logger"
	"github.com/trushilpatel/jobradar/infra/metrics"
	jobsvc "github.com/trushilpatel/jobradar/service/job"
)

// enrichConcurrency is the number of jobs processed in parallel.
const enrichConcurrency = 5

// Worker consumes jobs.filtered, fetches the full job page,
// runs LLM enrichment, and writes the final row to the job table via PipelineService.
type Worker struct {
	consumer    *kafka.Consumer
	scorer      llm.Scorer
	pipeline    jobsvc.PipelineService
	client      *http.Client
	log         logger.Logger
	maxExpYears int
}

func NewWorker(
	consumer *kafka.Consumer,
	scorer llm.Scorer,
	pipeline jobsvc.PipelineService,
	log logger.Logger,
	maxExpYears int,
) *Worker {
	return &Worker{
		consumer:    consumer,
		scorer:      scorer,
		pipeline:    pipeline,
		client:      &http.Client{Timeout: 15 * time.Second},
		log:         log,
		maxExpYears: maxExpYears,
	}
}

// Run starts consuming jobs.filtered and blocks until ctx is cancelled.
func (w *Worker) Run(ctx context.Context) error {
	w.log.Info("worker started", logger.Int("concurrency", enrichConcurrency))

	sem := make(chan struct{}, enrichConcurrency)
	var wg sync.WaitGroup

	err := w.consumer.Poll(ctx, func(_ string, value []byte) error {
		var job ats.RawJob
		if err := json.Unmarshal(value, &job); err != nil {
			w.log.Warn("malformed message, skipping", logger.Err(err))
			return nil
		}

		w.log.Debug("job received",
			logger.Str(logger.FieldJobID, job.ID),
			logger.Str("title", job.Title),
		)

		sem <- struct{}{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			w.enrich(ctx, job)
		}()

		return nil
	})

	wg.Wait()
	return err
}

// enrich runs the full enrichment pipeline for a single job.
func (w *Worker) enrich(ctx context.Context, job ats.RawJob) {
	jobLog := w.log.With(
		logger.Str(logger.FieldJobID, job.ID),
		logger.Str("title", job.Title),
	)

	// 1. Get full job content.
	var content ats.FetchJobContentResponse
	if job.FullContent != nil {
		content = fullContentToResponse(job)
	} else {
		var err error
		content, err = w.fetchContent(ctx, job)
		if err != nil {
			jobLog.Error("fetch content failed", err)
			w.markError(job.ID)
			metrics.EnricherJobsTotal.WithLabelValues("error").Inc()
			return
		}
	}

	// 2. Send to LLM for structured extraction.
	rawText, err := json.Marshal(content)
	if err != nil {
		jobLog.Error("marshal content failed", err)
		w.markError(job.ID)
		metrics.EnricherJobsTotal.WithLabelValues("error").Inc()
		return
	}

	llmStart := time.Now()
	result, err := w.scorer.Enrich(ctx, llm.EnrichInput{
		ID:      job.ID,
		RawText: string(rawText),
	})
	metrics.EnricherLLMDuration.Observe(time.Since(llmStart).Seconds())
	if err != nil {
		jobLog.Error("LLM enrich failed", err)
		w.markError(job.ID)
		metrics.EnricherJobsTotal.WithLabelValues("error").Inc()
		return
	}

	// 3. Drop ineligible jobs before writing to DB.
	if reason := w.dropJob(result); reason != "" {
		jobLog.Info("job dropped", logger.Str("reason", reason))
		w.markError(job.ID)
		metrics.EnricherJobsTotal.WithLabelValues("dropped").Inc()
		return
	}

	// 4. Insert via pipeline service (builds row + inserts + snapshot).
	if err := w.pipeline.InsertEnrichedJob(toEnrichedInput(job, result)); err != nil {
		jobLog.Error("insert enriched job failed", err)
		w.markError(job.ID)
		metrics.EnricherJobsTotal.WithLabelValues("error").Inc()
		return
	}

	metrics.EnricherJobsTotal.WithLabelValues("saved").Inc()
	jobLog.Info("enriched and saved")
}

// markError updates the seen_ledger so failed jobs aren't re-processed endlessly.
func (w *Worker) markError(jobID string) {
	if err := w.pipeline.MarkLedgerError(jobID); err != nil {
		w.log.Error("failed to mark error in seen_ledger", err, logger.Str(logger.FieldJobID, jobID))
	}
}

// toEnrichedInput translates ats.RawJob + llm.EnrichResult into the pipeline service input type.
func toEnrichedInput(raw ats.RawJob, enriched *llm.EnrichResult) jobsvc.EnrichedJobInput {
	return jobsvc.EnrichedJobInput{
		JobPostID:         raw.ID,
		CompanyID:         raw.CompanyID,
		Title:             raw.Title,
		Location:          raw.Location,
		URL:               raw.URL,
		Remote:            enriched.Remote,
		Country:           enriched.Country,
		State:             enriched.State,
		EmployType:        raw.EmployType,
		ExperienceYears:   enriched.ExperienceYears,
		TechStack:         enriched.TechStack,
		Sponsorship:       enriched.Sponsorship,
		RequiresCitizen:   enriched.RequiresCitizen,
		SecurityClearance: enriched.SecurityClearance,
		RoleBrief:         enriched.RoleBrief,
		Responsibilities:  enriched.Responsibilities,
		LLMSummary:        enriched.Summary,
		SalaryMin:         enriched.SalaryMin,
		SalaryMax:         enriched.SalaryMax,
		SalaryCurrency:    enriched.SalaryCurrency,
		RoleCategory:      enriched.RoleCategory,
	}
}

// fetchContent fetches the full content of a job posting.
func (w *Worker) fetchContent(ctx context.Context, job ats.RawJob) (ats.FetchJobContentResponse, error) {
	adp, err := ats.Registry(job.Adapter)
	if err == nil {
		if cf, ok := adp.(ats.ContentFetcher); ok {
			return cf.FetchJobContent(ctx, job)
		}
	}
	html, err := w.fetchPage(ctx, job.URL)
	if err != nil {
		return ats.FetchJobContentResponse{}, err
	}
	return ats.FetchJobContentResponse{
		ID:          job.ID,
		Title:       job.Title,
		Location:    job.Location,
		URL:         job.URL,
		Description: ats.StripHTML(html),
	}, nil
}

// fetchPage does a simple HTTP GET and returns the raw response body.
func (w *Worker) fetchPage(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; JobRadar/1.0)")

	resp, err := w.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("get: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 50*1024))
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}
	return string(body), nil
}

// dropJob returns a non-empty reason string if the job should be discarded.
func (w *Worker) dropJob(e *llm.EnrichResult) string {
	if !e.Sponsorship {
		return "no sponsorship"
	}
	if e.RequiresCitizen {
		return "requires citizenship"
	}
	if e.SecurityClearance {
		return "requires security clearance"
	}
	if w.maxExpYears > 0 && e.ExperienceYears != nil && *e.ExperienceYears > w.maxExpYears {
		return fmt.Sprintf("experience required (%d yrs) exceeds max (%d yrs)", *e.ExperienceYears, w.maxExpYears)
	}
	return ""
}

// fullContentToResponse converts a pre-fetched FullJobContent into the pipeline shape.
func fullContentToResponse(job ats.RawJob) ats.FetchJobContentResponse {
	fc := job.FullContent
	return ats.FetchJobContentResponse{
		ID:             job.ID,
		Title:          job.Title,
		Location:       job.Location,
		URL:            job.URL,
		Description:    fc.Description,
		EmploymentType: job.EmployType,
		Department:     fc.Department,
		Compensation:   fc.Compensation,
		WorkplaceType:  fc.WorkplaceType,
		Requirements:   fc.Requirements,
		PublishedDate:  fc.PublishedDate,
	}
}
```

Add `"encoding/json"` to imports (needed for `json.Unmarshal` and `json.Marshal`).

- [ ] **Step 2: Update `backend/pipeline/analyzer/worker_test.go`**

Replace the test file. The `buildJob`/`buildJobRow` tests have moved to `service/job/pipeline_service_test.go`. The `enrich` tests are adapted to use `fakePipelineService`:

```go
package analyzer

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/trushilpatel/jobradar/pipeline/ats"
	atstypes "github.com/trushilpatel/jobradar/pipeline/ats/types"
	"github.com/trushilpatel/jobradar/pipeline/llm"
	"github.com/trushilpatel/jobradar/infra/logger"
	"github.com/trushilpatel/jobradar/model"
	jobsvc "github.com/trushilpatel/jobradar/service/job"
)

// ── fakes ─────────────────────────────────────────────────────────────────────

type fakeScorer struct {
	result *llm.EnrichResult
	err    error
}

func (f *fakeScorer) ScoreBatch(_ context.Context, _ []llm.ScorerInput) ([]string, error) {
	return nil, nil
}
func (f *fakeScorer) Enrich(_ context.Context, _ llm.EnrichInput) (*llm.EnrichResult, error) {
	return f.result, f.err
}

type fakePipelineService struct {
	insertCalled bool
	insertInput  jobsvc.EnrichedJobInput
	insertErr    error
	errorIDs     []string
	markErrorErr error
}

func (f *fakePipelineService) InsertEnrichedJob(input jobsvc.EnrichedJobInput) error {
	f.insertCalled = true
	f.insertInput = input
	return f.insertErr
}
func (f *fakePipelineService) HasSeenJob(_ string) (bool, error)              { return false, nil }
func (f *fakePipelineService) MarkLedgerSeen(_, _, _ string) error            { return nil }
func (f *fakePipelineService) MarkLedgerError(jobID string) error {
	f.errorIDs = append(f.errorIDs, jobID)
	return f.markErrorErr
}
func (f *fakePipelineService) MarkLedgerErrorBatch(_ []string) error          { return nil }
func (f *fakePipelineService) MarkLedgerIrrelevantBatch(_ []string) error     { return nil }
func (f *fakePipelineService) GetActiveJobIDsByCompany(_ string) ([]string, error) { return nil, nil }
func (f *fakePipelineService) MarkRemovedBatch(_ []string) error               { return nil }

var _ jobsvc.PipelineService = (*fakePipelineService)(nil)

// ── helpers ───────────────────────────────────────────────────────────────────

func makeWorkerWithPipeline(scorer *fakeScorer, pipeline *fakePipelineService) *Worker {
	return NewWorker(nil, scorer, pipeline, logger.NewNoop(), 0)
}

func baseRawJob() ats.RawJob {
	return ats.RawJob{
		ID:        "ashby-abc123",
		CompanyID: "company-xyz",
		Title:     "Platform Engineer",
		URL:       "https://jobs.ashbyhq.com/co/abc123",
		Adapter:   model.AdapterAshby,
	}
}

func baseEnrichResult() *llm.EnrichResult {
	return &llm.EnrichResult{
		TechStack:   []string{"Go", "K8s"},
		Remote:      true,
		Country:     "USA",
		RoleCategory: "backend",
		Sponsorship: true, // must be true to pass dropJob
	}
}

// ── enrich tests ──────────────────────────────────────────────────────────────

func TestEnrich_LLMError_MarksError(t *testing.T) {
	scorer := &fakeScorer{err: errors.New("LLM timeout")}
	pipeline := &fakePipelineService{}
	w := makeWorkerWithPipeline(scorer, pipeline)

	// enrich calls fetchContent which may hit the real ATS registry for ashby.
	// Use FullContent to bypass the HTTP fetch.
	job := baseRawJob()
	job.FullContent = &atstypes.FullJobContent{Description: "A great role."}

	w.enrich(context.Background(), job)

	if pipeline.insertCalled {
		t.Error("no job should be inserted on LLM error")
	}
	if len(pipeline.errorIDs) != 1 || pipeline.errorIDs[0] != job.ID {
		t.Errorf("expected error marked for job %q, got %v", job.ID, pipeline.errorIDs)
	}
}

func TestEnrich_InsertError_MarksError(t *testing.T) {
	scorer := &fakeScorer{result: baseEnrichResult()}
	pipeline := &fakePipelineService{insertErr: errors.New("db constraint")}
	w := makeWorkerWithPipeline(scorer, pipeline)

	job := baseRawJob()
	job.FullContent = &atstypes.FullJobContent{Description: "A great role."}

	w.enrich(context.Background(), job)

	if !pipeline.insertCalled {
		t.Error("InsertEnrichedJob should have been attempted")
	}
	if len(pipeline.errorIDs) != 1 || pipeline.errorIDs[0] != job.ID {
		t.Errorf("expected error marked for job %q after insert failure, got %v", job.ID, pipeline.errorIDs)
	}
}

func TestEnrich_HappyPath_InsertsCalled(t *testing.T) {
	scorer := &fakeScorer{result: baseEnrichResult()}
	pipeline := &fakePipelineService{}
	w := makeWorkerWithPipeline(scorer, pipeline)

	job := baseRawJob()
	job.FullContent = &atstypes.FullJobContent{Description: "Build distributed systems."}

	w.enrich(context.Background(), job)

	if !pipeline.insertCalled {
		t.Error("expected InsertEnrichedJob to be called on happy path")
	}
	if len(pipeline.errorIDs) != 0 {
		t.Errorf("expected no error marks on happy path, got %v", pipeline.errorIDs)
	}
	if pipeline.insertInput.CompanyID != job.CompanyID {
		t.Errorf("CompanyID: want %q got %q", job.CompanyID, pipeline.insertInput.CompanyID)
	}
	if !pipeline.insertInput.Remote {
		t.Error("Remote should be true from enrich result")
	}
}

// ── FullContent / skip-fetch tests ────────────────────────────────────────────

type capturingScorer struct {
	result      *llm.EnrichResult
	err         error
	lastInput   llm.EnrichInput
	enrichCalls int
}

func (c *capturingScorer) ScoreBatch(_ context.Context, _ []llm.ScorerInput) ([]string, error) {
	return nil, nil
}
func (c *capturingScorer) Enrich(_ context.Context, input llm.EnrichInput) (*llm.EnrichResult, error) {
	c.lastInput = input
	c.enrichCalls++
	return c.result, c.err
}

type failingTransport struct {
	called bool
}

func (ft *failingTransport) RoundTrip(_ *http.Request) (*http.Response, error) {
	ft.called = true
	return nil, errors.New("HTTP client must not be called when FullContent is present")
}

func TestEnrich_SkipsFetchWhenFullContentPresent(t *testing.T) {
	transport := &failingTransport{}
	scorer := &capturingScorer{result: &llm.EnrichResult{
		TechStack:   []string{"Go"},
		Remote:      true,
		Country:     "USA",
		Sponsorship: true,
	}}
	pipeline := &fakePipelineService{}

	w := NewWorker(nil, scorer, pipeline, logger.NewNoop(), 0)
	w.client = &http.Client{Transport: transport}

	job := ats.RawJob{
		ID:        "google-123",
		CompanyID: "company-google",
		Title:     "Senior Go Engineer",
		Location:  "Remote",
		URL:       "https://careers.google.com/jobs/123",
		Adapter:   "google",
		EmployType: model.EmployFullTime,
		FullContent: &atstypes.FullJobContent{
			Description:   "Go engineer role building large-scale systems.",
			Requirements:  "5 years Go experience required.",
			Compensation:  "$200,000",
			WorkplaceType: "remote",
			Department:    "Engineering",
			PublishedDate: "2026-04-01",
		},
	}

	w.enrich(context.Background(), job)

	if transport.called {
		t.Error("HTTP client should not be called when FullContent is present")
	}
	if scorer.enrichCalls != 1 {
		t.Errorf("Enrich call count: want 1 got %d", scorer.enrichCalls)
	}
	if !strings.Contains(scorer.lastInput.RawText, "Go engineer role") {
		t.Errorf("LLM RawText should contain description text; got: %s", scorer.lastInput.RawText)
	}
	if len(pipeline.errorIDs) != 0 {
		t.Errorf("expected no ledger error on success, got %v", pipeline.errorIDs)
	}
}

func TestFullContentToResponse_FieldMapping(t *testing.T) {
	job := ats.RawJob{
		ID:        "google-456",
		CompanyID: "company-google",
		Title:     "Staff Software Engineer",
		Location:  "Mountain View, CA",
		URL:       "https://careers.google.com/jobs/456",
		Adapter:   "google",
		EmployType: model.EmployFullTime,
		FullContent: &atstypes.FullJobContent{
			Description:   "Design and build distributed systems.",
			Requirements:  "7+ years of backend experience.",
			Compensation:  "$250,000 - $350,000",
			WorkplaceType: "hybrid",
			Department:    "Infrastructure",
			PublishedDate: "2026-03-15",
		},
	}

	resp := fullContentToResponse(job)

	if resp.ID != job.ID {
		t.Errorf("ID: want %q got %q", job.ID, resp.ID)
	}
	if resp.Title != job.Title {
		t.Errorf("Title: want %q got %q", job.Title, resp.Title)
	}
	if resp.Location != job.Location {
		t.Errorf("Location: want %q got %q", job.Location, resp.Location)
	}
	if resp.URL != job.URL {
		t.Errorf("URL: want %q got %q", job.URL, resp.URL)
	}
	if resp.EmploymentType != job.EmployType {
		t.Errorf("EmploymentType: want %q got %q", job.EmployType, resp.EmploymentType)
	}

	fc := job.FullContent
	if resp.Description != fc.Description {
		t.Errorf("Description: want %q got %q", fc.Description, resp.Description)
	}
	if resp.Requirements != fc.Requirements {
		t.Errorf("Requirements: want %q got %q", fc.Requirements, resp.Requirements)
	}
	if resp.Compensation != fc.Compensation {
		t.Errorf("Compensation: want %q got %q", fc.Compensation, resp.Compensation)
	}
	if resp.WorkplaceType != fc.WorkplaceType {
		t.Errorf("WorkplaceType: want %q got %q", fc.WorkplaceType, resp.WorkplaceType)
	}
	if resp.Department != fc.Department {
		t.Errorf("Department: want %q got %q", fc.Department, resp.Department)
	}
	if resp.PublishedDate != fc.PublishedDate {
		t.Errorf("PublishedDate: want %q got %q", fc.PublishedDate, resp.PublishedDate)
	}
}
```

- [ ] **Step 3: Run analyzer tests**

```bash
cd backend && go test ./pipeline/analyzer/... -v
```

Expected: all PASS.

---

## Task 10: Update `cmd/` wiring

**Files:**

- Modify: `backend/cmd/job_listing_crawler/main.go`
- Modify: `backend/cmd/job_listing_relevance_filter/main.go`
- Modify: `backend/cmd/job_post_analyzer/main.go`

- [ ] **Step 1: Update `cmd/job_listing_crawler/main.go`**

Replace the wiring section (after the queue `ResetStuck` call):

```go
import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/trushilpatel/jobradar/config"
	"github.com/trushilpatel/jobradar/pipeline/ats/google"
	"github.com/trushilpatel/jobradar/infra/db"
	"github.com/trushilpatel/jobradar/infra/kafka"
	"github.com/trushilpatel/jobradar/infra/logger"
	"github.com/trushilpatel/jobradar/infra/metrics"
	companysvc "github.com/trushilpatel/jobradar/service/company"
	jobsvc "github.com/trushilpatel/jobradar/service/job"
	scansvc "github.com/trushilpatel/jobradar/service/scan"
	"github.com/trushilpatel/jobradar/pipeline/crawler"
)
```

Replace the store construction and engine creation:

```go
	companyStore  := companysvc.NewStore(database)
	jobStore      := jobsvc.NewJobStore(database)
	ledgerStore   := jobsvc.NewSeenLedgerStore(database)
	snapshotStore := jobsvc.NewSnapshotStore(database)
	scanLogStore  := scansvc.NewLogStore(database)
	queue         := scansvc.NewQueueStore(database)

	pipelineSvc := jobsvc.NewPipelineService(jobStore, ledgerStore, snapshotStore)
	crawlerSvc  := scansvc.NewCrawlerService(scanLogStore, companyStore)

	companyDelay := time.Duration(cfg.ScanCompanyDelaySeconds) * time.Second
	googleCfg := google.Config{
		MinDelayMs:  cfg.GoogleScraperMinDelayMs,
		MaxDelayMs:  cfg.GoogleScraperMaxDelayMs,
		TimeoutMs:   cfg.GoogleScraperTimeoutMs,
		MaxPages:    cfg.GoogleScraperMaxPages,
		EnrichMinMs: cfg.GoogleScraperEnrichMinMs,
		EnrichMaxMs: cfg.GoogleScraperEnrichMaxMs,
	}
	engine := crawler.NewEngine(companyStore, pipelineSvc, crawlerSvc, producer, googleCfg, log)
```

The `queue.ResetStuck`, `pollAndProcess` function, and the ticker loop remain unchanged.

- [ ] **Step 2: Update `cmd/job_listing_relevance_filter/main.go`**

Replace the ledger store construction and worker creation:

```go
	pipelineSvc := jobsvc.NewPipelineService(
		jobsvc.NewJobStore(database),
		jobsvc.NewSeenLedgerStore(database),
		jobsvc.NewSnapshotStore(database),
	)
	worker := filter.NewWorker(consumer, producer, scorer, pipelineSvc,
		cfg.ScorerBatchSize, cfg.ScorerFlushTimeoutSeconds, log)
```

Remove the old line: `ledger := jobsvc.NewSeenLedgerStore(database)`.

Add `jobsvc.NewJobStore` and `jobsvc.NewSnapshotStore` to imports if not already present (they're in the same `jobsvc` alias).

- [ ] **Step 3: Update `cmd/job_post_analyzer/main.go`**

Replace the three store lines and worker construction:

```go
	pipelineSvc := jobsvc.NewPipelineService(
		jobsvc.NewJobStore(database),
		jobsvc.NewSeenLedgerStore(database),
		jobsvc.NewSnapshotStore(database),
	)
	worker := analyzer.NewWorker(consumer, scorer, pipelineSvc, log, cfg.MaxExpYears)
```

Remove the old lines:

```go
	jobs := jobsvc.NewJobStore(database)
	ledger := jobsvc.NewSeenLedgerStore(database)
	snapshot := jobsvc.NewSnapshotStore(database)
```

- [ ] **Step 4: Verify all binaries compile**

```bash
cd backend && go build ./cmd/...
```

Expected: no output, exit 0.

---

## Task 11: Add `company.SchedulerService` and update scheduler

**Files:**

- Modify: `backend/service/company/store.go`
- Modify: `backend/cmd/company_scan_scheduler/main.go`

- [ ] **Step 1: Add `SchedulerService` interface to `service/company/store.go`**

Add after the `CompanyWriter` interface:

```go
// SchedulerService is the narrow interface the scan scheduler needs.
// It declares only what the scheduler uses — not the full CompanyReader.
// Satisfied by the companyStore returned by NewStore.
type SchedulerService interface {
	GetCompaniesDueForScan(intervalMinutes int) ([]model.Company, error)
}
```

- [ ] **Step 2: Update `cmd/company_scan_scheduler/main.go`**

Change the `enqueuedue` function signature to accept `companysvc.SchedulerService` instead of `companysvc.CompanyReader`:

```go
func enqueuedue(ctx context.Context, log logger.Logger, companies companysvc.SchedulerService, queue scansvc.ScanQueueStore, intervalMinutes int) {
	due, err := companies.GetCompaniesDueForScan(intervalMinutes)
	if err != nil {
		log.Error("failed to query due companies", err)
		return
	}
	if len(due) == 0 {
		return
	}

	log.Info("companies due for scan", logger.Int("count", len(due)))

	for _, c := range due {
		if err := queue.Enqueue(c.ID, model.QueueSourceScheduler); err != nil {
			log.Error("enqueue failed", err, logger.Str(logger.FieldCompany, c.ID))
			continue
		}
		metrics.SchedulerCompaniesEnqueuedTotal.Inc()
		log.Debug("queued company", logger.Str(logger.FieldCompany, c.ID), logger.Str("name", c.Name))
	}
}
```

The `main()` wiring stays the same — `companysvc.NewStore(database)` returns `companysvc.Store` which embeds `CompanyReader` which includes `GetCompaniesDueForScan`, so it satisfies `SchedulerService`.

- [ ] **Step 3: Update the scheduler test (`cmd/company_scan_scheduler/main_test.go`)**

The `fakeCompanyReader` in the test currently satisfies `companysvc.CompanyReader`. After the change, `enqueuedue` accepts `companysvc.SchedulerService`. Check whether the test double satisfies the new interface — `SchedulerService` has only 1 method (`GetCompaniesDueForScan`) so most test doubles will already satisfy it. Update the interface assertion if the test file has one:

```go
// If the test file has: var _ companysvc.CompanyReader = (*fakeCompanyReader)(nil)
// Change it to:
var _ companysvc.SchedulerService = (*fakeCompanyReader)(nil)
```

- [ ] **Step 4: Final full build and test run**

```bash
cd backend && go build ./...
```

Expected: no output, exit 0.

```bash
cd backend && go test ./...
```

Expected: all PASS. Any pre-existing skips are fine; no new failures.

---

## Self-Review

**Spec coverage check:**

| Violation                                       | Task that fixes it                                 |
| ----------------------------------------------- | -------------------------------------------------- |
| V7: api/ imports in service layer               | Tasks 2, 3                                         |
| V6: scan store writes companies table           | Task 4                                             |
| V8: no pipeline-facing service interface        | Task 5 (PipelineService) + Task 6 (CrawlerService) |
| V5: pipeline engine depends on store interfaces | Task 7                                             |
| V3: filter uses raw store                       | Task 8                                             |
| V4: analyzer uses 3 raw stores                  | Task 9                                             |
| V2: crawler bypasses service                    | Task 10                                            |
| V1: scheduler type annotation                   | Task 11                                            |

**Placeholder scan:** None found. All steps contain complete code.

**Type consistency check:**

- `PipelineService.InsertEnrichedJob(input EnrichedJobInput)` — defined Task 5, used in Task 9 `toEnrichedInput()` return type ✓
- `FilterLedger` methods `MarkLedgerErrorBatch` / `MarkLedgerIrrelevantBatch` — defined Task 7, implemented in `filter/worker.go` Task 8, faked in `filter/worker_test.go` Task 8 ✓
- `CrawlerService.RecordScanResult` — defined Task 6, called in `engine.recordResult` Task 7 ✓
- `CompanyWriter.UpdateScanStatus` — added Task 4, used in `crawlerService` Task 6 ✓
- `nullableStr` (not `nullableString`) — defined in `pipeline_service.go` Task 5, not referenced in analyzer (worker uses `toEnrichedInput` which maps plain strings) ✓
