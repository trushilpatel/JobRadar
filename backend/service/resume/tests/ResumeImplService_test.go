package tests

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/platform/objectstorage"
	"github.com/trushilpatel/jobradar/service/resume/dto"
	resumemodel "github.com/trushilpatel/jobradar/service/resume/models"
	resumesvc "github.com/trushilpatel/jobradar/service/resume/services"
)

// flakyStorage wraps MemoryStorage but can be told to fail any of its three
// operations — used to exercise ImplService's storage-failure branches
// (put failure, best-effort delete-after-success logging, presign failure)
// without a real S3-compatible endpoint.
type flakyStorage struct {
	*objectstorage.MemoryStorage
	putErr     error
	deleteErr  error
	presignErr error
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

func (f *flakyStorage) PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error) {
	if f.presignErr != nil {
		return "", f.presignErr
	}
	return f.MemoryStorage.PresignGet(ctx, key, ttl)
}

func TestResumeImpl_UploadWritesStorageBeforeDBRow(t *testing.T) {
	db := newResumeTestDB(t)
	repo := resumesvc.NewRepositoryService(db)
	storage := objectstorage.NewMemoryStorage()
	impl := resumesvc.NewImplService(repo, storage, logger.NewNoop())

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
	impl := resumesvc.NewImplService(repo, storage, logger.NewNoop())
	ctx := context.Background()

	// Fill the user to the cap so the next upload's DB insert fails.
	for range 50 {
		if _, err := impl.Upload(ctx, "user-1", dto.UploadInput{
			Label: "L", OriginalFilename: "f.pdf", ContentType: "application/pdf",
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
	impl := resumesvc.NewImplService(repo, storage, logger.NewNoop())
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
	impl := resumesvc.NewImplService(repo, storage, logger.NewNoop())
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

func TestResumeImpl_PresignDownloadReturnsURLForOwner(t *testing.T) {
	db := newResumeTestDB(t)
	repo := resumesvc.NewRepositoryService(db)
	storage := objectstorage.NewMemoryStorage()
	impl := resumesvc.NewImplService(repo, storage, logger.NewNoop())
	ctx := context.Background()

	resume, _ := impl.Upload(ctx, "user-1", dto.UploadInput{
		Label: "L", OriginalFilename: "f.pdf", ContentType: "application/pdf",
		Extension: "pdf", RawBytes: []byte("%PDF-1.4"), DeclaredSize: 8,
	})

	url, err := impl.PresignDownload(ctx, "user-1", resume.ID)
	if err != nil {
		t.Fatalf("PresignDownload: %v", err)
	}
	if url == "" {
		t.Fatal("expected a non-empty presigned URL")
	}
}

func TestResumeImpl_PresignDownloadForWrongUserReturnsNotFound(t *testing.T) {
	db := newResumeTestDB(t)
	repo := resumesvc.NewRepositoryService(db)
	storage := objectstorage.NewMemoryStorage()
	impl := resumesvc.NewImplService(repo, storage, logger.NewNoop())
	ctx := context.Background()

	resume, _ := impl.Upload(ctx, "user-1", dto.UploadInput{
		Label: "L", OriginalFilename: "f.pdf", ContentType: "application/pdf",
		Extension: "pdf", RawBytes: []byte("%PDF-1.4"), DeclaredSize: 8,
	})

	if _, err := impl.PresignDownload(ctx, "user-2", resume.ID); !errors.Is(err, domainerrs.ErrNotFound) {
		t.Fatalf("expected NotFound for cross-user download, got %v", err)
	}
}

func TestResumeImpl_ListPassesThroughToRepo(t *testing.T) {
	db := newResumeTestDB(t)
	repo := resumesvc.NewRepositoryService(db)
	storage := objectstorage.NewMemoryStorage()
	impl := resumesvc.NewImplService(repo, storage, logger.NewNoop())
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
	impl := resumesvc.NewImplService(repo, storage, logger.NewNoop())
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
	impl := resumesvc.NewImplService(repo, storage, logger.NewNoop())

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
	impl := resumesvc.NewImplService(repo, storage, logger.NewNoop())
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

func TestResumeImpl_PresignDownloadPropagatesStorageError(t *testing.T) {
	db := newResumeTestDB(t)
	repo := resumesvc.NewRepositoryService(db)
	storage := newFlakyStorage()
	impl := resumesvc.NewImplService(repo, storage, logger.NewNoop())
	ctx := context.Background()

	resume, _ := impl.Upload(ctx, "user-1", dto.UploadInput{
		Label: "L", OriginalFilename: "f.pdf", ContentType: "application/pdf",
		Extension: "pdf", RawBytes: []byte("%PDF-1.4"), DeclaredSize: 8,
	})

	storage.presignErr = errors.New("signing failed")
	if _, err := impl.PresignDownload(ctx, "user-1", resume.ID); err == nil {
		t.Fatal("expected the storage presign error to propagate")
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

func TestNewImplService_PanicsOnNilNext(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected NewImplService(nil, ...) to panic")
		}
	}()
	resumesvc.NewImplService(nil, objectstorage.NewMemoryStorage(), logger.NewNoop())
}

func TestResumeImpl_PresignDownloadForDeletedResumeReturnsNotFound(t *testing.T) {
	db := newResumeTestDB(t)
	repo := resumesvc.NewRepositoryService(db)
	storage := objectstorage.NewMemoryStorage()
	impl := resumesvc.NewImplService(repo, storage, logger.NewNoop())
	ctx := context.Background()

	resume, _ := impl.Upload(ctx, "user-1", dto.UploadInput{
		Label: "L", OriginalFilename: "f.pdf", ContentType: "application/pdf",
		Extension: "pdf", RawBytes: []byte("%PDF-1.4"), DeclaredSize: 8,
	})
	_ = impl.Delete(ctx, "user-1", resume.ID)

	if _, err := impl.PresignDownload(ctx, "user-1", resume.ID); !errors.Is(err, domainerrs.ErrNotFound) {
		t.Fatalf("expected NotFound for a deleted resume, got %v", err)
	}
}
