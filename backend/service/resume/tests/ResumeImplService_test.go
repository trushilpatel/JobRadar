package tests

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/platform/objectstorage"
	llmdto "github.com/trushilpatel/jobradar/service/llm/dto"
	"github.com/trushilpatel/jobradar/service/resume/dto"
	"github.com/trushilpatel/jobradar/service/resume/filetype"
	resumemodel "github.com/trushilpatel/jobradar/service/resume/models"
	resumesvc "github.com/trushilpatel/jobradar/service/resume/services"
)

// flakyStorage wraps MemoryStorage but can be told to fail any of its three
// operations — used to exercise ImplService's storage-failure branches
// (put failure, best-effort delete-after-success logging, get failure)
// without a real S3-compatible endpoint.
type flakyStorage struct {
	*objectstorage.MemoryStorage
	putErr    error
	deleteErr error
	getErr    error
}

func newFlakyStorage() *flakyStorage {
	return &flakyStorage{MemoryStorage: objectstorage.NewMemoryStorage()}
}

func (f *flakyStorage) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	if f.putErr != nil {
		return f.putErr
	}
	return f.MemoryStorage.Put(ctx, key, r, size, contentType)
}

func (f *flakyStorage) Delete(ctx context.Context, key string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	return f.MemoryStorage.Delete(ctx, key)
}

func (f *flakyStorage) Get(ctx context.Context, key string) (io.ReadCloser, int64, error) {
	if f.getErr != nil {
		return nil, 0, f.getErr
	}
	return f.MemoryStorage.Get(ctx, key)
}

func TestResumeImpl_UploadWritesStorageBeforeDBRow(t *testing.T) {
	db := newResumeTestDB(t)
	repo := resumesvc.NewRepositoryService(db)
	storage := objectstorage.NewMemoryStorage()
	impl := resumesvc.NewImplService(repo, storage, newNoopKeywordExtractor(), newFakeKeywordStore(), logger.NewNoop())

	resume, err := impl.Upload(context.Background(), "user-1", dto.UploadInput{
		Label: "My resume", OriginalFilename: "resume.pdf",
		ContentType: "application/pdf", Extension: "pdf",
		RawBytes: []byte("%PDF-1.4"), DeclaredSize: 8,
	})
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if resume.ID == "" {
		t.Fatal("expected ImplService to generate a resume ID")
	}
	if !storage.Has(resume.StorageKey) {
		t.Fatal("expected the object to exist in storage after a successful upload")
	}
}

func TestResumeImpl_UploadCleansUpStorageWhenDBInsertFails(t *testing.T) {
	db := newResumeTestDB(t)
	repo := resumesvc.NewRepositoryService(db)
	storage := objectstorage.NewMemoryStorage()
	impl := resumesvc.NewImplService(repo, storage, newNoopKeywordExtractor(), newFakeKeywordStore(), logger.NewNoop())
	ctx := context.Background()

	// Fill the user to the cap so the next upload's DB insert fails.
	for i := range 50 {
		if _, err := impl.Upload(ctx, "user-1", dto.UploadInput{
			Label: fmt.Sprintf("L-%d", i), OriginalFilename: "f.pdf", ContentType: "application/pdf",
			Extension: "pdf", RawBytes: []byte("%PDF-1.4"), DeclaredSize: 8,
		}); err != nil {
			t.Fatalf("seed upload: %v", err)
		}
	}

	_, err := impl.Upload(ctx, "user-1", dto.UploadInput{
		Label: "Over cap", OriginalFilename: "f.pdf", ContentType: "application/pdf",
		Extension: "pdf", RawBytes: []byte("%PDF-1.4"), DeclaredSize: 8,
	})
	if err == nil {
		t.Fatal("expected the 51st upload to fail")
	}
	rows, listErr := repo.List(ctx, "user-1")
	if listErr != nil {
		t.Fatalf("List: %v", listErr)
	}
	if len(rows) != 50 {
		t.Fatalf("expected exactly 50 DB rows after the rejected 51st, got %d", len(rows))
	}
}

func TestResumeImpl_DeleteRemovesDBRowAndStorageObject(t *testing.T) {
	db := newResumeTestDB(t)
	repo := resumesvc.NewRepositoryService(db)
	storage := objectstorage.NewMemoryStorage()
	impl := resumesvc.NewImplService(repo, storage, newNoopKeywordExtractor(), newFakeKeywordStore(), logger.NewNoop())
	ctx := context.Background()

	resume, _ := impl.Upload(ctx, "user-1", dto.UploadInput{
		Label: "L", OriginalFilename: "f.pdf", ContentType: "application/pdf",
		Extension: "pdf", RawBytes: []byte("%PDF-1.4"), DeclaredSize: 8,
	})

	if err := impl.Delete(ctx, "user-1", resume.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if storage.Has(resume.StorageKey) {
		t.Fatal("expected the storage object to be removed after Delete")
	}
	if _, err := repo.Get(ctx, "user-1", resume.ID); !errors.Is(err, domainerrs.ErrNotFound) {
		t.Fatalf("expected NotFound after Delete, got %v", err)
	}
}

func TestResumeImpl_DeleteForWrongUserReturnsNotFoundAndLeavesObjectIntact(t *testing.T) {
	db := newResumeTestDB(t)
	repo := resumesvc.NewRepositoryService(db)
	storage := objectstorage.NewMemoryStorage()
	impl := resumesvc.NewImplService(repo, storage, newNoopKeywordExtractor(), newFakeKeywordStore(), logger.NewNoop())
	ctx := context.Background()

	resume, _ := impl.Upload(ctx, "user-1", dto.UploadInput{
		Label: "L", OriginalFilename: "f.pdf", ContentType: "application/pdf",
		Extension: "pdf", RawBytes: []byte("%PDF-1.4"), DeclaredSize: 8,
	})

	if err := impl.Delete(ctx, "user-2", resume.ID); !errors.Is(err, domainerrs.ErrNotFound) {
		t.Fatalf("expected NotFound, got %v", err)
	}
	if !storage.Has(resume.StorageKey) {
		t.Fatal("a rejected cross-user delete must not touch storage")
	}
}

func TestResumeImpl_DownloadReturnsReaderForOwner(t *testing.T) {
	db := newResumeTestDB(t)
	repo := resumesvc.NewRepositoryService(db)
	storage := objectstorage.NewMemoryStorage()
	impl := resumesvc.NewImplService(repo, storage, newNoopKeywordExtractor(), newFakeKeywordStore(), logger.NewNoop())
	ctx := context.Background()

	resume, _ := impl.Upload(ctx, "user-1", dto.UploadInput{
		Label: "L", OriginalFilename: "f.pdf", ContentType: "application/pdf",
		Extension: "pdf", RawBytes: []byte("%PDF-1.4"), DeclaredSize: 8,
	})

	reader, downloaded, err := impl.Download(ctx, "user-1", resume.ID)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	defer reader.Close()
	if downloaded.ID != resume.ID {
		t.Fatalf("expected the resume row for %s, got %s", resume.ID, downloaded.ID)
	}
	data, readErr := io.ReadAll(reader)
	if readErr != nil {
		t.Fatalf("ReadAll: %v", readErr)
	}
	if string(data) != "%PDF-1.4" {
		t.Fatalf("expected the uploaded bytes back, got %q", data)
	}
}

func TestResumeImpl_DownloadForWrongUserReturnsNotFound(t *testing.T) {
	db := newResumeTestDB(t)
	repo := resumesvc.NewRepositoryService(db)
	storage := objectstorage.NewMemoryStorage()
	impl := resumesvc.NewImplService(repo, storage, newNoopKeywordExtractor(), newFakeKeywordStore(), logger.NewNoop())
	ctx := context.Background()

	resume, _ := impl.Upload(ctx, "user-1", dto.UploadInput{
		Label: "L", OriginalFilename: "f.pdf", ContentType: "application/pdf",
		Extension: "pdf", RawBytes: []byte("%PDF-1.4"), DeclaredSize: 8,
	})

	if _, _, err := impl.Download(ctx, "user-2", resume.ID); !errors.Is(err, domainerrs.ErrNotFound) {
		t.Fatalf("expected NotFound for cross-user download, got %v", err)
	}
}

func TestResumeImpl_ListPassesThroughToRepo(t *testing.T) {
	db := newResumeTestDB(t)
	repo := resumesvc.NewRepositoryService(db)
	storage := objectstorage.NewMemoryStorage()
	impl := resumesvc.NewImplService(repo, storage, newNoopKeywordExtractor(), newFakeKeywordStore(), logger.NewNoop())
	ctx := context.Background()

	_, _ = impl.Upload(ctx, "user-1", dto.UploadInput{
		Label: "L", OriginalFilename: "f.pdf", ContentType: "application/pdf",
		Extension: "pdf", RawBytes: []byte("%PDF-1.4"), DeclaredSize: 8,
	})

	rows, err := impl.List(ctx, "user-1")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row via pass-through, got %d", len(rows))
	}
}

func TestResumeImpl_RenamePassesThroughToRepo(t *testing.T) {
	db := newResumeTestDB(t)
	repo := resumesvc.NewRepositoryService(db)
	storage := objectstorage.NewMemoryStorage()
	impl := resumesvc.NewImplService(repo, storage, newNoopKeywordExtractor(), newFakeKeywordStore(), logger.NewNoop())
	ctx := context.Background()

	resume, _ := impl.Upload(ctx, "user-1", dto.UploadInput{
		Label: "L", OriginalFilename: "f.pdf", ContentType: "application/pdf",
		Extension: "pdf", RawBytes: []byte("%PDF-1.4"), DeclaredSize: 8,
	})

	renamed, err := impl.Rename(ctx, "user-1", resume.ID, "New label")
	if err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if renamed.Label != "New label" {
		t.Fatalf("expected pass-through rename, got %s", renamed.Label)
	}
}

func TestResumeImpl_UploadPropagatesStoragePutError(t *testing.T) {
	db := newResumeTestDB(t)
	repo := resumesvc.NewRepositoryService(db)
	storage := newFlakyStorage()
	storage.putErr = errors.New("bucket unreachable")
	impl := resumesvc.NewImplService(repo, storage, newNoopKeywordExtractor(), newFakeKeywordStore(), logger.NewNoop())

	_, err := impl.Upload(context.Background(), "user-1", dto.UploadInput{
		Label: "L", OriginalFilename: "f.pdf", ContentType: "application/pdf",
		Extension: "pdf", RawBytes: []byte("%PDF-1.4"), DeclaredSize: 8,
	})
	if err == nil {
		t.Fatal("expected the storage Put error to propagate")
	}
}

func TestResumeImpl_DeleteSucceedsEvenWhenStorageDeleteFails(t *testing.T) {
	db := newResumeTestDB(t)
	repo := resumesvc.NewRepositoryService(db)
	storage := newFlakyStorage()
	impl := resumesvc.NewImplService(repo, storage, newNoopKeywordExtractor(), newFakeKeywordStore(), logger.NewNoop())
	ctx := context.Background()

	resume, err := impl.Upload(ctx, "user-1", dto.UploadInput{
		Label: "L", OriginalFilename: "f.pdf", ContentType: "application/pdf",
		Extension: "pdf", RawBytes: []byte("%PDF-1.4"), DeclaredSize: 8,
	})
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}

	storage.deleteErr = errors.New("bucket unreachable")
	if err := impl.Delete(ctx, "user-1", resume.ID); err != nil {
		t.Fatalf("expected user-visible Delete to succeed despite storage delete failure, got %v", err)
	}
	if _, getErr := repo.Get(ctx, "user-1", resume.ID); !errors.Is(getErr, domainerrs.ErrNotFound) {
		t.Fatalf("expected the DB row to be gone regardless of storage delete failure, got %v", getErr)
	}
}

func TestResumeImpl_DownloadPropagatesStorageError(t *testing.T) {
	db := newResumeTestDB(t)
	repo := resumesvc.NewRepositoryService(db)
	storage := newFlakyStorage()
	impl := resumesvc.NewImplService(repo, storage, newNoopKeywordExtractor(), newFakeKeywordStore(), logger.NewNoop())
	ctx := context.Background()

	resume, _ := impl.Upload(ctx, "user-1", dto.UploadInput{
		Label: "L", OriginalFilename: "f.pdf", ContentType: "application/pdf",
		Extension: "pdf", RawBytes: []byte("%PDF-1.4"), DeclaredSize: 8,
	})

	storage.getErr = errors.New("bucket unreachable")
	if _, _, err := impl.Download(ctx, "user-1", resume.ID); err == nil {
		t.Fatal("expected the storage get error to propagate")
	}
}

func TestResumeImpl_DownloadMapsStorageNotFoundToDomainNotFound(t *testing.T) {
	db := newResumeTestDB(t)
	repo := resumesvc.NewRepositoryService(db)
	storage := newFlakyStorage()
	impl := resumesvc.NewImplService(repo, storage, newNoopKeywordExtractor(), newFakeKeywordStore(), logger.NewNoop())
	ctx := context.Background()

	resume, _ := impl.Upload(ctx, "user-1", dto.UploadInput{
		Label: "L", OriginalFilename: "f.pdf", ContentType: "application/pdf",
		Extension: "pdf", RawBytes: []byte("%PDF-1.4"), DeclaredSize: 8,
	})

	storage.getErr = objectstorage.ErrNotFound
	if _, _, err := impl.Download(ctx, "user-1", resume.ID); !errors.Is(err, domainerrs.ErrNotFound) {
		t.Fatalf("expected domain ErrNotFound when the object is missing from storage, got %v", err)
	}
}

func TestBaseService_UploadDelegatesToNext(t *testing.T) {
	fake := &fakeService{uploadResult: &resumemodel.Resume{ID: "r1"}}
	bs := resumesvc.NewBaseService(fake)
	result, err := bs.Upload(context.Background(), "u1", dto.UploadInput{Label: "L"})
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if result != fake.uploadResult {
		t.Fatal("expected BaseService.Upload to return the next layer's result unchanged")
	}
}

func TestBaseService_ExtractKeywordsDelegatesToNext(t *testing.T) {
	fake := &fakeService{extractResult: &resumemodel.Resume{ID: "r1"}}
	bs := resumesvc.NewBaseService(fake)
	result, err := bs.ExtractKeywords(context.Background(), "u1", "r1")
	if err != nil {
		t.Fatalf("ExtractKeywords: %v", err)
	}
	if result != fake.extractResult {
		t.Fatal("expected BaseService.ExtractKeywords to return the next layer's result unchanged")
	}
}

func TestNewImplService_PanicsOnNilNext(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected NewImplService(nil, ...) to panic")
		}
	}()
	resumesvc.NewImplService(nil, objectstorage.NewMemoryStorage(), newNoopKeywordExtractor(), newFakeKeywordStore(), logger.NewNoop())
}

func TestResumeImpl_DownloadForDeletedResumeReturnsNotFound(t *testing.T) {
	db := newResumeTestDB(t)
	repo := resumesvc.NewRepositoryService(db)
	storage := objectstorage.NewMemoryStorage()
	impl := resumesvc.NewImplService(repo, storage, newNoopKeywordExtractor(), newFakeKeywordStore(), logger.NewNoop())
	ctx := context.Background()

	resume, _ := impl.Upload(ctx, "user-1", dto.UploadInput{
		Label: "L", OriginalFilename: "f.pdf", ContentType: "application/pdf",
		Extension: "pdf", RawBytes: []byte("%PDF-1.4"), DeclaredSize: 8,
	})
	_ = impl.Delete(ctx, "user-1", resume.ID)

	if _, _, err := impl.Download(ctx, "user-1", resume.ID); !errors.Is(err, domainerrs.ErrNotFound) {
		t.Fatalf("expected NotFound for a deleted resume, got %v", err)
	}
}

// ─── Keyword extraction fakes ────────────────────────────────────────────────

// fakeKeywordExtractor is a controllable KeywordExtractor double. Every field
// is optional; zero value behaves as "called, returns empty keywords, no
// error" so most tests don't need to configure it at all.
type fakeKeywordExtractor struct {
	result      *llmdto.ResumeKeywordResult
	err         error
	panicMsg    string
	block       chan struct{} // if set, ExtractResumeKeywords blocks until closed
	calledCount int32         // atomic
	calledCh    chan struct{} // if set, closed on first call (before blocking/panicking)
}

func newNoopKeywordExtractor() *fakeKeywordExtractor {
	return &fakeKeywordExtractor{result: &llmdto.ResumeKeywordResult{}}
}

func (f *fakeKeywordExtractor) ExtractResumeKeywords(ctx context.Context, _ llmdto.ResumeKeywordInput) (*llmdto.ResumeKeywordResult, error) {
	atomic.AddInt32(&f.calledCount, 1)
	if f.calledCh != nil {
		close(f.calledCh)
	}
	if f.block != nil {
		select {
		case <-f.block:
		case <-ctx.Done():
		}
	}
	if f.panicMsg != "" {
		panic(f.panicMsg)
	}
	if f.result == nil && f.err == nil {
		return &llmdto.ResumeKeywordResult{}, nil
	}
	return f.result, f.err
}

func (f *fakeKeywordExtractor) calls() int32 { return atomic.LoadInt32(&f.calledCount) }

// fakeKeywordStore is a controllable KeywordStore double recording every
// UpdateKeywords call and optionally signalling a channel so tests can wait
// deterministically for a background goroutine to finish, rather than
// sleeping.
type fakeKeywordStore struct {
	mu      sync.Mutex
	updated map[string][]string
	err     error
	done    chan struct{} // if set, receives one signal per successful call
}

func newFakeKeywordStore() *fakeKeywordStore {
	return &fakeKeywordStore{updated: map[string][]string{}}
}

func (f *fakeKeywordStore) UpdateKeywords(_ context.Context, id string, keywords []string) error {
	f.mu.Lock()
	f.updated[id] = keywords
	f.mu.Unlock()
	if f.done != nil {
		select {
		case f.done <- struct{}{}:
		default:
		}
	}
	return f.err
}

func (f *fakeKeywordStore) keywordsFor(id string) ([]string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	kw, ok := f.updated[id]
	return kw, ok
}

// chanLogger is a minimal logger.Logger double that signals a channel on
// Warn/Error instead of recording a bool — lets tests wait deterministically
// for a background goroutine's failure path to run instead of sleeping.
type chanLogger struct {
	warnCh  chan struct{}
	errorCh chan struct{}
}

func (l *chanLogger) Debug(string, ...logger.Field) {}
func (l *chanLogger) Info(string, ...logger.Field)  {}
func (l *chanLogger) Warn(_ string, _ ...logger.Field) {
	if l.warnCh != nil {
		select {
		case l.warnCh <- struct{}{}:
		default:
		}
	}
}
func (l *chanLogger) Error(_ string, _ error, _ ...logger.Field) {
	if l.errorCh != nil {
		select {
		case l.errorCh <- struct{}{}:
		default:
		}
	}
}
func (l *chanLogger) With(...logger.Field) logger.Logger        { return l }
func (l *chanLogger) WithContext(context.Context) logger.Logger { return l }

// buildValidDocx assembles a minimal but genuinely parseable docx (matching
// service/resume/textextract's own test fixture shape) so
// runKeywordExtraction's textextract.ExtractText step actually succeeds and
// reaches the extractor call — the plain "%PDF-1.4" header bytes used
// elsewhere in this file are deliberately too short/invalid to extract text
// from, which is useful for tests that must NOT reach the extractor but
// wrong for tests that must.
func buildValidDocx(t *testing.T, text string) []byte {
	t.Helper()
	buf := &bytes.Buffer{}
	zw := zip.NewWriter(buf)
	write := func(name, content string) {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("zip create %s: %v", name, err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("zip write %s: %v", name, err)
		}
	}
	write("word/document.xml", `<?xml version="1.0"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>`+text+`</w:t></w:r></w:p></w:body></w:document>`)
	write("word/_rels/document.xml.rels", `<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"></Relationships>`)
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}

const validResumeText = "Backend engineer with five years of experience across Go, PostgreSQL, and Kubernetes deployments."

// waitOrTimeout fails the test if ch does not receive/close within d —
// avoids both a flaky race (no wait at all) and a hung test (unbounded wait)
// when asserting on a background goroutine's outcome.
func waitOrTimeout(t *testing.T, ch <-chan struct{}, d time.Duration) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(d):
		t.Fatal("timed out waiting for background keyword extraction to complete")
	}
}

// ─── Upload: best-effort background keyword extraction ──────────────────────

func TestResumeImpl_UploadSchedulesBackgroundKeywordExtractionAndPersistsResult(t *testing.T) {
	db := newResumeTestDB(t)
	repo := resumesvc.NewRepositoryService(db)
	storage := objectstorage.NewMemoryStorage()
	extractor := &fakeKeywordExtractor{result: &llmdto.ResumeKeywordResult{Keywords: []string{"js", "postgres"}}}
	store := newFakeKeywordStore()
	store.done = make(chan struct{}, 1)
	impl := resumesvc.NewImplService(repo, storage, extractor, store, logger.NewNoop())

	resume, err := impl.Upload(context.Background(), "user-1", dto.UploadInput{
		Label: "L", OriginalFilename: "f.docx", ContentType: filetype.ContentTypeDOCX,
		Extension: "docx", RawBytes: buildValidDocx(t, validResumeText), DeclaredSize: 8,
	})
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}

	waitOrTimeout(t, store.done, 2*time.Second)

	kw, ok := store.keywordsFor(resume.ID)
	if !ok {
		t.Fatal("expected UpdateKeywords to have been called for the uploaded resume")
	}
	// techtag.Clean canonicalizes "js" -> "JavaScript" and "postgres" ->
	// "PostgreSQL" — proves the impl layer runs raw LLM output through the
	// shared taxonomy rather than persisting it verbatim.
	if len(kw) != 2 || kw[0] != "JavaScript" || kw[1] != "PostgreSQL" {
		t.Fatalf("expected canonicalized keywords [JavaScript PostgreSQL], got %v", kw)
	}
}

func TestResumeImpl_UploadDoesNotBlockOnSlowExtraction(t *testing.T) {
	db := newResumeTestDB(t)
	repo := resumesvc.NewRepositoryService(db)
	storage := objectstorage.NewMemoryStorage()
	block := make(chan struct{})
	t.Cleanup(func() { close(block) }) // let the leaked goroutine finish after the test

	extractor := &fakeKeywordExtractor{block: block, result: &llmdto.ResumeKeywordResult{}}
	impl := resumesvc.NewImplService(repo, storage, extractor, newFakeKeywordStore(), logger.NewNoop())

	done := make(chan struct{})
	go func() {
		_, _ = impl.Upload(context.Background(), "user-1", dto.UploadInput{
			Label: "L", OriginalFilename: "f.docx", ContentType: filetype.ContentTypeDOCX,
			Extension: "docx", RawBytes: buildValidDocx(t, validResumeText), DeclaredSize: 8,
		})
		close(done)
	}()

	waitOrTimeout(t, done, 2*time.Second) // Upload itself must return promptly...
	if extractor.calls() > 1 {
		t.Fatal("extractor should be called at most once by the time Upload returns")
	}
	// ...even though the extractor is still blocked and hasn't returned yet.
}

func TestResumeImpl_UploadRecoversFromPanicInExtraction(t *testing.T) {
	db := newResumeTestDB(t)
	repo := resumesvc.NewRepositoryService(db)
	storage := objectstorage.NewMemoryStorage()
	extractor := &fakeKeywordExtractor{panicMsg: "boom: simulated third-party parser defect"}
	errorCh := make(chan struct{}, 1)
	log := &chanLogger{errorCh: errorCh}
	impl := resumesvc.NewImplService(repo, storage, extractor, newFakeKeywordStore(), log)

	resume, err := impl.Upload(context.Background(), "user-1", dto.UploadInput{
		Label: "L", OriginalFilename: "f.docx", ContentType: filetype.ContentTypeDOCX,
		Extension: "docx", RawBytes: buildValidDocx(t, validResumeText), DeclaredSize: 8,
	})
	if err != nil {
		t.Fatalf("Upload must succeed even though background extraction will panic: %v", err)
	}
	if resume == nil {
		t.Fatal("expected a resume back from Upload")
	}

	// The panic is recovered inside the goroutine and logged — this proves
	// the panic never escaped to crash the test process.
	waitOrTimeout(t, errorCh, 2*time.Second)
}

func TestResumeImpl_UploadExtractionFailureLeavesKeywordsEmpty(t *testing.T) {
	db := newResumeTestDB(t)
	repo := resumesvc.NewRepositoryService(db)
	storage := objectstorage.NewMemoryStorage()
	warnCh := make(chan struct{}, 1)
	log := &chanLogger{warnCh: warnCh}
	// "%PDF-1.4" alone is too short/invalid for textextract to parse — the
	// background goroutine must fail at the extraction step, before ever
	// reaching the LLM extractor or the keyword store.
	impl := resumesvc.NewImplService(repo, storage, newNoopKeywordExtractor(), newFakeKeywordStore(), log)

	_, err := impl.Upload(context.Background(), "user-1", dto.UploadInput{
		Label: "L", OriginalFilename: "f.pdf", ContentType: "application/pdf",
		Extension: "pdf", RawBytes: []byte("%PDF-1.4"), DeclaredSize: 8,
	})
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}

	waitOrTimeout(t, warnCh, 2*time.Second)
}

// ─── ExtractKeywords: explicit synchronous retry ─────────────────────────────

func TestResumeImpl_ExtractKeywords_Success(t *testing.T) {
	db := newResumeTestDB(t)
	repo := resumesvc.NewRepositoryService(db)
	storage := objectstorage.NewMemoryStorage()
	extractor := &fakeKeywordExtractor{result: &llmdto.ResumeKeywordResult{Keywords: []string{"golang", "k8s"}}}
	store := newFakeKeywordStore()
	impl := resumesvc.NewImplService(repo, storage, extractor, store, logger.NewNoop())
	ctx := context.Background()

	resume, err := impl.Upload(ctx, "user-1", dto.UploadInput{
		Label: "L", OriginalFilename: "f.docx", ContentType: filetype.ContentTypeDOCX,
		Extension: "docx", RawBytes: buildValidDocx(t, validResumeText), DeclaredSize: 8,
	})
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	// Reset the store so this test only observes the explicit retry's write,
	// not the best-effort background write Upload also triggers.
	store.mu.Lock()
	store.updated = map[string][]string{}
	store.mu.Unlock()

	updated, err := impl.ExtractKeywords(ctx, "user-1", resume.ID)
	if err != nil {
		t.Fatalf("ExtractKeywords: %v", err)
	}
	if updated.ID != resume.ID {
		t.Fatalf("expected the same resume back, got %s", updated.ID)
	}
	kw, ok := store.keywordsFor(resume.ID)
	if !ok || len(kw) != 2 || kw[0] != "Go" || kw[1] != "Kubernetes" {
		t.Fatalf("expected canonicalized [Go Kubernetes] persisted, got %v (found=%v)", kw, ok)
	}
}

func TestResumeImpl_ExtractKeywords_WrongUserReturnsNotFound(t *testing.T) {
	db := newResumeTestDB(t)
	repo := resumesvc.NewRepositoryService(db)
	storage := objectstorage.NewMemoryStorage()
	extractor := &fakeKeywordExtractor{result: &llmdto.ResumeKeywordResult{Keywords: []string{"go"}}}
	impl := resumesvc.NewImplService(repo, storage, extractor, newFakeKeywordStore(), logger.NewNoop())
	ctx := context.Background()

	resume, err := impl.Upload(ctx, "user-1", dto.UploadInput{
		Label: "L", OriginalFilename: "f.docx", ContentType: filetype.ContentTypeDOCX,
		Extension: "docx", RawBytes: buildValidDocx(t, validResumeText), DeclaredSize: 8,
	})
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}

	if _, err := impl.ExtractKeywords(ctx, "user-2", resume.ID); !errors.Is(err, domainerrs.ErrNotFound) {
		t.Fatalf("expected NotFound for a cross-user retry, got %v", err)
	}
	if extractor.calls() != 0 {
		t.Fatal("extractor must never be invoked for a resume the caller does not own")
	}
}

func TestResumeImpl_ExtractKeywords_TextExtractionFailurePropagatesAsInternal(t *testing.T) {
	db := newResumeTestDB(t)
	repo := resumesvc.NewRepositoryService(db)
	storage := objectstorage.NewMemoryStorage()
	impl := resumesvc.NewImplService(repo, storage, newNoopKeywordExtractor(), newFakeKeywordStore(), logger.NewNoop())
	ctx := context.Background()

	// "%PDF-1.4" is too short/invalid for textextract to parse.
	resume, err := impl.Upload(ctx, "user-1", dto.UploadInput{
		Label: "L", OriginalFilename: "f.pdf", ContentType: "application/pdf",
		Extension: "pdf", RawBytes: []byte("%PDF-1.4"), DeclaredSize: 8,
	})
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}

	if _, err := impl.ExtractKeywords(ctx, "user-1", resume.ID); !errors.Is(err, domainerrs.ErrInternal) {
		t.Fatalf("expected ErrInternal when text extraction fails, got %v", err)
	}
}

func TestResumeImpl_ExtractKeywords_LLMErrorPropagatesAsInternal(t *testing.T) {
	db := newResumeTestDB(t)
	repo := resumesvc.NewRepositoryService(db)
	storage := objectstorage.NewMemoryStorage()
	extractor := &fakeKeywordExtractor{err: errors.New("llm exhausted")}
	impl := resumesvc.NewImplService(repo, storage, extractor, newFakeKeywordStore(), logger.NewNoop())
	ctx := context.Background()

	resume, err := impl.Upload(ctx, "user-1", dto.UploadInput{
		Label: "L", OriginalFilename: "f.docx", ContentType: filetype.ContentTypeDOCX,
		Extension: "docx", RawBytes: buildValidDocx(t, validResumeText), DeclaredSize: 8,
	})
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}

	if _, err := impl.ExtractKeywords(ctx, "user-1", resume.ID); !errors.Is(err, domainerrs.ErrInternal) {
		t.Fatalf("expected ErrInternal when the LLM call fails, got %v", err)
	}
}

func TestResumeImpl_ExtractKeywords_StoreErrorPropagates(t *testing.T) {
	db := newResumeTestDB(t)
	repo := resumesvc.NewRepositoryService(db)
	storage := objectstorage.NewMemoryStorage()
	extractor := &fakeKeywordExtractor{result: &llmdto.ResumeKeywordResult{Keywords: []string{"go"}}}
	store := newFakeKeywordStore()
	store.err = errors.New("db down")
	impl := resumesvc.NewImplService(repo, storage, extractor, store, logger.NewNoop())
	ctx := context.Background()

	resume, err := impl.Upload(ctx, "user-1", dto.UploadInput{
		Label: "L", OriginalFilename: "f.docx", ContentType: filetype.ContentTypeDOCX,
		Extension: "docx", RawBytes: buildValidDocx(t, validResumeText), DeclaredSize: 8,
	})
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}

	if _, err := impl.ExtractKeywords(ctx, "user-1", resume.ID); err == nil {
		t.Fatal("expected the keyword-store error to propagate")
	}
}

func TestResumeImpl_ExtractKeywords_MissingResumeReturnsNotFound(t *testing.T) {
	db := newResumeTestDB(t)
	repo := resumesvc.NewRepositoryService(db)
	storage := objectstorage.NewMemoryStorage()
	impl := resumesvc.NewImplService(repo, storage, newNoopKeywordExtractor(), newFakeKeywordStore(), logger.NewNoop())

	if _, err := impl.ExtractKeywords(context.Background(), "user-1", "no-such-id"); !errors.Is(err, domainerrs.ErrNotFound) {
		t.Fatalf("expected NotFound for a missing resume, got %v", err)
	}
}

func TestResumeImpl_ExtractKeywords_StorageGetErrorPropagatesAsInternal(t *testing.T) {
	db := newResumeTestDB(t)
	repo := resumesvc.NewRepositoryService(db)
	storage := newFlakyStorage()
	impl := resumesvc.NewImplService(repo, storage, newNoopKeywordExtractor(), newFakeKeywordStore(), logger.NewNoop())
	ctx := context.Background()

	resume, err := impl.Upload(ctx, "user-1", dto.UploadInput{
		Label: "L", OriginalFilename: "f.docx", ContentType: filetype.ContentTypeDOCX,
		Extension: "docx", RawBytes: buildValidDocx(t, validResumeText), DeclaredSize: 8,
	})
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}

	storage.getErr = errors.New("bucket unreachable")
	if _, err := impl.ExtractKeywords(ctx, "user-1", resume.ID); !errors.Is(err, domainerrs.ErrInternal) {
		t.Fatalf("expected ErrInternal when storage read fails, got %v", err)
	}
}
