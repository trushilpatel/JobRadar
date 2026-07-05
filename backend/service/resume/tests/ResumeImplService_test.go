package tests

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"

	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/platform/objectstorage"
	"github.com/trushilpatel/jobradar/service/resume/dto"
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

func TestResumeImpl_DownloadReturnsReaderForOwner(t *testing.T) {
	db := newResumeTestDB(t)
	repo := resumesvc.NewRepositoryService(db)
	storage := objectstorage.NewMemoryStorage()
	impl := resumesvc.NewImplService(repo, storage, logger.NewNoop())
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
	impl := resumesvc.NewImplService(repo, storage, logger.NewNoop())
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

func TestResumeImpl_DownloadPropagatesStorageError(t *testing.T) {
	db := newResumeTestDB(t)
	repo := resumesvc.NewRepositoryService(db)
	storage := newFlakyStorage()
	impl := resumesvc.NewImplService(repo, storage, logger.NewNoop())
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
	impl := resumesvc.NewImplService(repo, storage, logger.NewNoop())
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

func TestNewImplService_PanicsOnNilNext(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected NewImplService(nil, ...) to panic")
		}
	}()
	resumesvc.NewImplService(nil, objectstorage.NewMemoryStorage(), logger.NewNoop())
}

func TestResumeImpl_DownloadForDeletedResumeReturnsNotFound(t *testing.T) {
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

	if _, _, err := impl.Download(ctx, "user-1", resume.ID); !errors.Is(err, domainerrs.ErrNotFound) {
		t.Fatalf("expected NotFound for a deleted resume, got %v", err)
	}
}
