# Bulk Dismiss API Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `POST /jobs/dismiss/bulk` that dismisses N jobs in one HTTP request via batch DB writes, replacing the N parallel requests the frontend currently fires.

**Architecture:** `BulkDismiss` flows through the full decorator chain: RestService → ValidationService → ImplService → RepositoryService. Four existing single-row store operations each gain a batch sibling. The frontend swaps `Promise.allSettled(N)` for one `apiClient.post` call.

**Tech Stack:** Go 1.22, chi router, GORM + PostgreSQL, TypeScript + TanStack Query

---

## File Map

| File | Change |
|------|--------|
| `backend/service/job/dto/BulkDismissInput.go` | New — `BulkDismissInput` struct |
| `backend/service/job/services/JobFeedRepository.go` | Add `GetJobPostIDs` + `UpdateJobStatusBatch` to `JobReader`/`JobWriter` interfaces and `jobStore`; add `BulkDismiss` + `GetJobPostIDs` to `repositoryService` |
| `backend/service/job/services/JobDiscardRepository.go` | Add `InsertDiscardBatch` to `DiscardStore` interface and `discardStore` |
| `backend/service/job/services/JobSnapshotRepository.go` | Add `UpdateSnapshotActionBatch` to `SnapshotWriter` interface and `snapshotStore` |
| `backend/service/job/services/JobService.go` | Add `BulkDismiss` + `GetJobPostIDs` to `Service` interface and `BaseService` |
| `backend/service/job/services/JobImplService.go` | Add `BulkDismiss` orchestration |
| `backend/service/job/services/JobValidationService.go` | Add `BulkDismiss` validation |
| `backend/service/job/services/JobRestService.go` | Add `BulkDismiss` HTTP handler |
| `backend/service/job/services/JobRestRouter.go` | Register `POST /jobs/dismiss/bulk` |
| `backend/service/job/tests/JobImplService_test.go` | Extend fakes; add `TestBulkDismiss_*` |
| `backend/service/job/tests/JobValidationService_test.go` | Extend `noopService`; add `TestValidateBulkDismiss` |
| `backend/service/job/tests/JobRestService_test.go` | Extend `fakeJobSvc`; add handler tests |
| `ui/console/src/api/jobs.ts` | Add `bulkDismissJobs` |
| `ui/console/src/features/jobs/hooks/useJobActions.ts` | Replace `Promise.allSettled` with single bulk call |

---

## Task 1: BulkDismissInput DTO

**Files:**
- Create: `backend/service/job/dto/BulkDismissInput.go`

- [ ] **Step 1: Create the DTO**

```go
package dto

// BulkDismissInput is the request payload for dismissing multiple jobs in one call.
type BulkDismissInput struct {
	IDs    []string `json:"ids"`
	Reason string   `json:"reason"`
}
```

- [ ] **Step 2: Verify it compiles**

```bash
cd backend && go build ./service/job/dto/...
```

Expected: no output (clean build).

- [ ] **Step 3: Commit**

```bash
git add backend/service/job/dto/BulkDismissInput.go
git commit -m "feat(job): add BulkDismissInput DTO"
```

---

## Task 2: Batch store methods

**Files:**
- Modify: `backend/service/job/services/JobFeedRepository.go`
- Modify: `backend/service/job/services/JobDiscardRepository.go`
- Modify: `backend/service/job/services/JobSnapshotRepository.go`

- [ ] **Step 1: Add `GetJobPostIDs` to `JobReader` interface and implement on `jobStore`**

In `JobFeedRepository.go`, add to the `JobReader` interface (after `GetJobCounts`):

```go
GetJobPostIDs(ctx context.Context, ids []string) ([]string, error)
```

Then add the implementation after `GetJobByID`:

```go
func (s *jobStore) GetJobPostIDs(ctx context.Context, ids []string) (jobPostIDs []string, err error) {
	ctx, span := tracing.Start(ctx, "job.store.GetJobPostIDs")
	defer tracing.End(span, &err)
	if len(ids) == 0 {
		return nil, nil
	}
	err = s.db.WithContext(ctx).Model(&jobmodel.Job{}).
		Where("id IN ?", ids).
		Pluck("job_post_id", &jobPostIDs).Error
	return jobPostIDs, err
}
```

- [ ] **Step 2: Add `UpdateJobStatusBatch` to `JobWriter` interface and implement on `jobStore`**

In `JobFeedRepository.go`, add to the `JobWriter` interface (after `UpdateJobStatus`):

```go
UpdateJobStatusBatch(ctx context.Context, ids []string, status string) (int64, error)
```

Then add the implementation after `UpdateJobStatus`:

```go
func (s *jobStore) UpdateJobStatusBatch(ctx context.Context, ids []string, status string) (_ int64, err error) {
	ctx, span := tracing.Start(ctx, "job.store.UpdateJobStatusBatch")
	defer tracing.End(span, &err)
	if len(ids) == 0 {
		return 0, nil
	}
	result := s.db.WithContext(ctx).Model(&jobmodel.Job{}).
		Where("id IN ?", ids).
		Update("status", status)
	return result.RowsAffected, result.Error
}
```

- [ ] **Step 3: Add `InsertDiscardBatch` to `DiscardStore` interface and implement on `discardStore`**

In `JobDiscardRepository.go`, add to the `DiscardStore` interface:

```go
InsertDiscardBatch(ctx context.Context, ids []string, reason string) error
```

Then add the implementation after `InsertDiscard`:

```go
func (s *discardStore) InsertDiscardBatch(ctx context.Context, ids []string, reason string) (err error) {
	ctx, span := tracing.Start(ctx, "job.store.InsertDiscardBatch")
	defer tracing.End(span, &err)
	if len(ids) == 0 {
		return nil
	}
	rows := make([]jobmodel.Discarded, len(ids))
	for i, id := range ids {
		rows[i] = jobmodel.Discarded{JobResultID: id, Reason: reason}
	}
	return s.db.WithContext(ctx).CreateInBatches(rows, 500).Error
}
```

- [ ] **Step 4: Add `UpdateSnapshotActionBatch` to `SnapshotWriter` interface and implement on `snapshotStore`**

In `JobSnapshotRepository.go`, add to the `SnapshotWriter` interface:

```go
UpdateSnapshotActionBatch(ctx context.Context, ids []string, action string) error
```

Then add the implementation after `UpdateSnapshotAction`:

```go
func (s *snapshotStore) UpdateSnapshotActionBatch(ctx context.Context, ids []string, action string) (err error) {
	ctx, span := tracing.Start(ctx, "job.store.UpdateSnapshotActionBatch")
	defer tracing.End(span, &err)
	if len(ids) == 0 {
		return nil
	}
	now := time.Now()
	return s.db.WithContext(ctx).Model(&jobmodel.JobSnapshot{}).
		Where("id IN ?", ids).
		Updates(map[string]any{
			"user_action": action,
			"actioned_at": now,
		}).Error
}
```

- [ ] **Step 5: Verify it compiles**

```bash
cd backend && go build ./service/job/services/...
```

Expected: no output (clean build).

- [ ] **Step 6: Commit**

```bash
git add backend/service/job/services/JobFeedRepository.go \
        backend/service/job/services/JobDiscardRepository.go \
        backend/service/job/services/JobSnapshotRepository.go
git commit -m "feat(job): add batch store methods for bulk dismiss"
```

---

## Task 3: Extend Service interface, BaseService, and all existing implementers

**Files:**
- Modify: `backend/service/job/services/JobService.go`
- Modify: `backend/service/job/services/JobFeedRepository.go`
- Modify: `backend/service/job/tests/JobImplService_test.go`
- Modify: `backend/service/job/tests/JobValidationService_test.go`
- Modify: `backend/service/job/tests/JobRestService_test.go`

- [ ] **Step 1: Add `BulkDismiss` and `GetJobPostIDs` to the `Service` interface in `JobService.go`**

Add these two lines to the `Service` interface after `MarkApplied`:

```go
BulkDismiss(ctx context.Context, input dto.BulkDismissInput) (int, error)
GetJobPostIDs(ctx context.Context, ids []string) ([]string, error)
```

- [ ] **Step 2: Add `BaseService` auto-delegations in `JobService.go`**

Add after `MarkApplied` delegation:

```go
func (b *BaseService) BulkDismiss(ctx context.Context, input dto.BulkDismissInput) (int, error) {
	return b.next.BulkDismiss(ctx, input)
}

func (b *BaseService) GetJobPostIDs(ctx context.Context, ids []string) ([]string, error) {
	return b.next.GetJobPostIDs(ctx, ids)
}
```

- [ ] **Step 3: Add `BulkDismiss` and `GetJobPostIDs` to `repositoryService` in `JobFeedRepository.go`**

Add after `MarkApplied`:

```go
func (r *repositoryService) BulkDismiss(ctx context.Context, input dto.BulkDismissInput) (_ int, err error) {
	ctx, span := tracing.Start(ctx, "job.repository.BulkDismiss")
	defer tracing.End(span, &err)
	n, err := r.store.UpdateJobStatusBatch(ctx, input.IDs, jobcommon.JobStatusDismissed)
	if err != nil {
		return 0, errors.ErrInternal.WithMessage("failed to bulk update job status")
	}
	return int(n), nil
}

func (r *repositoryService) GetJobPostIDs(ctx context.Context, ids []string) (_ []string, err error) {
	ctx, span := tracing.Start(ctx, "job.repository.GetJobPostIDs")
	defer tracing.End(span, &err)
	jobPostIDs, err := r.store.GetJobPostIDs(ctx, ids)
	if err != nil {
		return nil, errors.ErrInternal.WithMessage("failed to fetch job post IDs")
	}
	return jobPostIDs, nil
}
```

- [ ] **Step 4: Extend `fakeJobStore` in `JobImplService_test.go`**

Add a `jobPostIDs` field to `fakeJobStore`:

```go
type fakeJobStore struct {
	job        *jobmodel.Job
	jobs       []jobmodel.Job
	total      int64
	err        error
	jobPostIDs map[string]string // job_feed id → job_post_id

	updateStatusCalled bool
	updateSavedCalled  bool
	updateNotesCalled  bool
}
```

Add new method implementations to `fakeJobStore`:

```go
func (f *fakeJobStore) GetJobPostIDs(_ context.Context, ids []string) ([]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	var result []string
	for _, id := range ids {
		if jp, ok := f.jobPostIDs[id]; ok {
			result = append(result, jp)
		}
	}
	return result, nil
}

func (f *fakeJobStore) UpdateJobStatusBatch(_ context.Context, ids []string, _ string) (int64, error) {
	f.updateStatusCalled = true
	return int64(len(ids)), f.err
}
```

Add new method implementation to `fakeDiscardStore`:

```go
func (f *fakeDiscardStore) InsertDiscardBatch(_ context.Context, _ []string, _ string) error {
	f.insertCalled = true
	return f.err
}
```

Add new method implementation to `fakeSnapshotWriter`:

```go
func (f *fakeSnapshotWriter) UpdateSnapshotActionBatch(_ context.Context, _ []string, _ string) error {
	f.updateCalled = true
	return f.err
}
```

- [ ] **Step 5: Extend `noopService` in `JobValidationService_test.go`**

Add after `MarkApplied`:

```go
func (n *noopService) BulkDismiss(_ context.Context, _ jobdto.BulkDismissInput) (int, error) {
	return 0, nil
}
func (n *noopService) GetJobPostIDs(_ context.Context, _ []string) ([]string, error) {
	return nil, nil
}
```

- [ ] **Step 6: Extend `fakeJobSvc` in `JobRestService_test.go`**

Add after `MarkApplied`:

```go
func (f *fakeJobSvc) BulkDismiss(_ context.Context, _ jobdto.BulkDismissInput) (int, error) {
	return 0, f.err
}
func (f *fakeJobSvc) GetJobPostIDs(_ context.Context, _ []string) ([]string, error) {
	return nil, f.err
}
```

- [ ] **Step 7: Verify it compiles and existing tests still pass**

```bash
cd backend && go build ./service/job/... && go test ./service/job/tests/...
```

Expected: all existing tests pass, no new failures.

- [ ] **Step 8: Commit**

```bash
git add backend/service/job/services/JobService.go \
        backend/service/job/services/JobFeedRepository.go \
        backend/service/job/tests/JobImplService_test.go \
        backend/service/job/tests/JobValidationService_test.go \
        backend/service/job/tests/JobRestService_test.go
git commit -m "feat(job): extend Service interface with BulkDismiss and GetJobPostIDs"
```

---

## Task 4: ImplService.BulkDismiss (TDD)

**Files:**
- Modify: `backend/service/job/tests/JobImplService_test.go`
- Modify: `backend/service/job/services/JobImplService.go`

- [ ] **Step 1: Write the failing tests**

Append to `backend/service/job/tests/JobImplService_test.go`:

```go
func TestBulkDismiss_DismissesAll(t *testing.T) {
	js := &fakeJobStore{
		jobPostIDs: map[string]string{"j1": "jp1", "j2": "jp2"},
	}
	discard := &fakeDiscardStore{}
	snapshot := &fakeSnapshotWriter{}
	ledger := &fakeLedgerWriter{}

	svc := newImplService(js, ledger, discard, snapshot, &fakeApplicationWriter{})
	n, err := svc.BulkDismiss(context.Background(), jobdto.BulkDismissInput{
		IDs:    []string{"j1", "j2"},
		Reason: jobcommon.DiscardReasonNotRelevant,
	})
	if err != nil {
		t.Fatalf("BulkDismiss() error = %v", err)
	}
	if n != 2 {
		t.Errorf("expected 2 dismissed, got %d", n)
	}
	if !discard.insertCalled {
		t.Error("expected InsertDiscardBatch to be called")
	}
	if !js.updateStatusCalled {
		t.Error("expected UpdateJobStatusBatch to be called")
	}
	if !snapshot.updateCalled {
		t.Error("expected UpdateSnapshotActionBatch to be called")
	}
}

func TestBulkDismiss_SnapshotFailureIsNonFatal(t *testing.T) {
	js := &fakeJobStore{jobPostIDs: map[string]string{"j1": "jp1"}}
	snapshot := &fakeSnapshotWriter{err: errors.New("snapshot failure")}

	svc := newImplService(js, &fakeLedgerWriter{}, &fakeDiscardStore{}, snapshot, &fakeApplicationWriter{})
	_, err := svc.BulkDismiss(context.Background(), jobdto.BulkDismissInput{
		IDs:    []string{"j1"},
		Reason: jobcommon.DiscardReasonNotRelevant,
	})
	if err != nil {
		t.Fatalf("BulkDismiss() must not return error when snapshot fails, got: %v", err)
	}
}

func TestBulkDismiss_DiscardFailureIsFatal(t *testing.T) {
	js := &fakeJobStore{jobPostIDs: map[string]string{"j1": "jp1"}}
	discard := &fakeDiscardStore{err: errors.New("db error")}

	svc := newImplService(js, &fakeLedgerWriter{}, discard, &fakeSnapshotWriter{}, &fakeApplicationWriter{})
	_, err := svc.BulkDismiss(context.Background(), jobdto.BulkDismissInput{
		IDs:    []string{"j1"},
		Reason: jobcommon.DiscardReasonNotRelevant,
	})
	if err == nil {
		t.Fatal("expected error when discard fails")
	}
}
```

- [ ] **Step 2: Run the tests to confirm they fail**

```bash
cd backend && go test ./service/job/tests/... -run TestBulkDismiss -v
```

Expected: FAIL — `BulkDismiss` not implemented on implService yet (uses BaseService delegation which goes to repositoryService; the stub compiles and `n` will be 0, so `TestBulkDismiss_DismissesAll` fails the count check, and `discard.insertCalled` is false).

- [ ] **Step 3: Implement `implService.BulkDismiss` in `JobImplService.go`**

Add after the `Dismiss` method:

```go
// BulkDismiss dismisses multiple jobs in one call: records discards, blocks re-entry,
// hides all from feed, and non-fatally backfills analytics snapshots.
func (s *implService) BulkDismiss(ctx context.Context, input dto.BulkDismissInput) (n int, err error) {
	ctx, span := tracing.Start(ctx, "job.service.BulkDismiss")
	defer tracing.End(span, &err)

	if err := s.discard.InsertDiscardBatch(ctx, input.IDs, input.Reason); err != nil {
		return 0, errors.ErrInternal.WithMessage("failed to record dismissals")
	}

	jobPostIDs, err := s.next.GetJobPostIDs(ctx, input.IDs)
	if err != nil {
		return 0, err
	}

	if err := s.ledger.UpdateSeenReasonBatch(ctx, jobPostIDs, jobcommon.SeenReasonUserDismissed); err != nil {
		return 0, errors.ErrInternal.WithMessage("failed to update seen ledger")
	}

	n, err = s.next.BulkDismiss(ctx, input)
	if err != nil {
		return 0, err
	}

	_ = s.snapshot.UpdateSnapshotActionBatch(ctx, input.IDs, jobcommon.JobStatusDismissed)

	return n, nil
}
```

- [ ] **Step 4: Run the tests to confirm they pass**

```bash
cd backend && go test ./service/job/tests/... -run TestBulkDismiss -v
```

Expected: all three `TestBulkDismiss_*` tests PASS.

- [ ] **Step 5: Run all job tests to confirm no regressions**

```bash
cd backend && go test ./service/job/tests/...
```

Expected: all tests pass.

- [ ] **Step 6: Commit**

```bash
git add backend/service/job/services/JobImplService.go \
        backend/service/job/tests/JobImplService_test.go
git commit -m "feat(job): implement BulkDismiss orchestration in ImplService"
```

---

## Task 5: ValidationService.BulkDismiss (TDD)

**Files:**
- Modify: `backend/service/job/tests/JobValidationService_test.go`
- Modify: `backend/service/job/services/JobValidationService.go`

- [ ] **Step 1: Write the failing test**

Append to `backend/service/job/tests/JobValidationService_test.go`:

```go
func TestValidateBulkDismiss(t *testing.T) {
	v := newValidator()

	ids101 := make([]string, 101)
	for i := range ids101 {
		ids101[i] = "x"
	}

	tests := []struct {
		name    string
		ids     []string
		wantErr bool
	}{
		{name: "valid single", ids: []string{"j1"}},
		{name: "valid 100", ids: make([]string, 100)},
		{name: "nil ids", ids: nil, wantErr: true},
		{name: "empty ids", ids: []string{}, wantErr: true},
		{name: "101 ids", ids: ids101, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := v.BulkDismiss(context.Background(), jobdto.BulkDismissInput{
				IDs:    tc.ids,
				Reason: jobcommon.DiscardReasonNotRelevant,
			})
			if (err != nil) != tc.wantErr {
				t.Errorf("BulkDismiss() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}
```

- [ ] **Step 2: Run the test to confirm it fails**

```bash
cd backend && go test ./service/job/tests/... -run TestValidateBulkDismiss -v
```

Expected: FAIL — with nil/empty ids the noopService returns `nil`, but the test expects an error.

- [ ] **Step 3: Implement `validationService.BulkDismiss` in `JobValidationService.go`**

Add after the `SetNotes` method:

```go
// BulkDismiss validates that ids is non-empty and within the 100-item limit.
func (v *validationService) BulkDismiss(ctx context.Context, input dto.BulkDismissInput) (int, error) {
	var errs errors.ValidationErrors
	if len(input.IDs) == 0 {
		errs = append(errs, errors.ValidationError{
			Field:   "ids",
			Code:    "REQUIRED",
			Message: "ids must not be empty",
		})
	}
	if len(input.IDs) > 100 {
		errs = append(errs, errors.ValidationError{
			Field:   "ids",
			Code:    "INVALID_VALUE",
			Message: "ids must contain 100 or fewer entries",
		})
	}
	if len(errs) > 0 {
		return 0, errs
	}
	return v.next.BulkDismiss(ctx, input)
}
```

- [ ] **Step 4: Run the test to confirm it passes**

```bash
cd backend && go test ./service/job/tests/... -run TestValidateBulkDismiss -v
```

Expected: all 5 table-driven cases PASS.

- [ ] **Step 5: Run all job tests**

```bash
cd backend && go test ./service/job/tests/...
```

Expected: all tests pass.

- [ ] **Step 6: Commit**

```bash
git add backend/service/job/services/JobValidationService.go \
        backend/service/job/tests/JobValidationService_test.go
git commit -m "feat(job): add BulkDismiss validation — ids non-empty, max 100"
```

---

## Task 6: HTTP handler and route (TDD)

**Files:**
- Modify: `backend/service/job/tests/JobRestService_test.go`
- Modify: `backend/service/job/services/JobRestService.go`
- Modify: `backend/service/job/services/JobRestRouter.go`

- [ ] **Step 1: Write the failing handler tests**

Append to `backend/service/job/tests/JobRestService_test.go`:

```go
func TestBulkDismissHandler_OK(t *testing.T) {
	r := newJobTestRouter(&fakeJobSvc{})

	body, _ := json.Marshal(map[string][]string{"ids": {"j1", "j2"}})
	req := httptest.NewRequest(http.MethodPost, "/jobs/dismiss/bulk", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("got status %d, want 200: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"dismissed"`) {
		t.Errorf("body missing dismissed field: %s", w.Body.String())
	}
}

func TestBulkDismissHandler_BadBody(t *testing.T) {
	r := newJobTestRouter(&fakeJobSvc{})

	req := httptest.NewRequest(http.MethodPost, "/jobs/dismiss/bulk", strings.NewReader("not-json"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want 400", w.Code)
	}
}

func TestBulkDismissHandler_ServiceError(t *testing.T) {
	r := newJobTestRouter(&fakeJobSvc{err: errors.ErrInternal.WithMessage("db error")})

	body, _ := json.Marshal(map[string][]string{"ids": {"j1"}})
	req := httptest.NewRequest(http.MethodPost, "/jobs/dismiss/bulk", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("got status %d, want 500", w.Code)
	}
}
```

- [ ] **Step 2: Run the tests to confirm they fail**

```bash
cd backend && go test ./service/job/tests/... -run TestBulkDismissHandler -v
```

Expected: FAIL — route not yet registered, all requests return 404.

- [ ] **Step 3: Add the `BulkDismiss` handler to `JobRestService.go`**

Add after the `Dismiss` method:

```go
// POST /jobs/dismiss/bulk
func (h *RestService) BulkDismiss(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IDs []string `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		handler.RespondError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}

	input := dto.BulkDismissInput{
		IDs:    body.IDs,
		Reason: jobcommon.DiscardReasonNotRelevant,
	}

	n, err := h.next.BulkDismiss(r.Context(), input)
	if err != nil {
		handler.HandleError(w, err)
		return
	}

	handler.Respond(w, http.StatusOK, map[string]int{"dismissed": n})
}
```

- [ ] **Step 4: Register the route in `JobRestRouter.go`**

Add as the first route in `Register`, before `/jobs`:

```go
router.Post("/jobs/dismiss/bulk", r.handler.BulkDismiss)
```

The full `Register` body becomes:

```go
func (r *RestRouter) Register(router chi.Router) {
	router.Post("/jobs/dismiss/bulk", r.handler.BulkDismiss)
	router.Get("/jobs", r.handler.List)
	router.Get("/jobs/counts", r.handler.GetCounts)
	router.Get("/jobs/{id}", r.handler.Get)
	router.Delete("/jobs/{id}", r.handler.Dismiss)
	router.Patch("/jobs/{id}/save", r.handler.SetSaved)
	router.Patch("/jobs/{id}/status", r.handler.SetStatus)
	router.Patch("/jobs/{id}/notes", r.handler.SetNotes)
}
```

- [ ] **Step 5: Run the handler tests to confirm they pass**

```bash
cd backend && go test ./service/job/tests/... -run TestBulkDismissHandler -v
```

Expected: all three `TestBulkDismissHandler_*` tests PASS.

- [ ] **Step 6: Run all job tests**

```bash
cd backend && go test ./service/job/tests/...
```

Expected: all tests pass.

- [ ] **Step 7: Run backend lint**

```bash
cd backend && golangci-lint run --config ../.github/.golangci.yml --timeout 5m
```

Expected: no new lint errors.

- [ ] **Step 8: Commit**

```bash
git add backend/service/job/services/JobRestService.go \
        backend/service/job/services/JobRestRouter.go \
        backend/service/job/tests/JobRestService_test.go
git commit -m "feat(job): add POST /jobs/dismiss/bulk handler and route"
```

---

## Task 7: Frontend

**Files:**
- Modify: `ui/console/src/api/jobs.ts`
- Modify: `ui/console/src/features/jobs/hooks/useJobActions.ts`

- [ ] **Step 1: Add `bulkDismissJobs` to `ui/console/src/api/jobs.ts`**

Append after the `dismissJob` function:

```typescript
// POST /jobs/dismiss/bulk
export async function bulkDismissJobs(ids: string[]): Promise<{ dismissed: number }> {
  return apiClient.post<{ dismissed: number }>('/jobs/dismiss/bulk', { ids })
}
```

- [ ] **Step 2: Update the import in `useJobActions.ts`**

Change:

```typescript
import { setJobStatus, dismissJob } from '@/api/jobs'
```

To:

```typescript
import { setJobStatus, dismissJob, bulkDismissJobs } from '@/api/jobs'
```

- [ ] **Step 3: Replace `bulkDismiss` mutation in `useJobActions.ts`**

Replace the existing `bulkDismiss` mutation:

```typescript
const bulkDismiss = useMutation({
  mutationFn: (ids: string[]) =>
    Promise.allSettled(ids.map((id) => dismissJob(id, 'user_not_relevant'))),
  onSuccess: (results) => {
    const dismissed = results.filter((r) => r.status === 'fulfilled').length
    toast.success(`${dismissed} job${dismissed !== 1 ? 's' : ''} dismissed`)
    invalidateJobs()
  },
  onError: () => toast.error('Failed to dismiss jobs'),
})
```

With:

```typescript
const bulkDismiss = useMutation({
  mutationFn: (ids: string[]) => bulkDismissJobs(ids),
  onSuccess: (result) => {
    toast.success(`${result.dismissed} job${result.dismissed !== 1 ? 's' : ''} dismissed`)
    invalidateJobs()
  },
  onError: () => toast.error('Failed to dismiss jobs'),
})
```

- [ ] **Step 4: Run frontend lint**

```bash
cd ui/console && npm run lint
```

Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add ui/console/src/api/jobs.ts \
        ui/console/src/features/jobs/hooks/useJobActions.ts
git commit -m "feat(jobs): replace N parallel dismiss requests with single bulk dismiss call"
```
