package tests

import (
	"context"
	"errors"
	"testing"

	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/platform/objectstorage"
	"github.com/trushilpatel/jobradar/service/resume/dto"
	resumesvc "github.com/trushilpatel/jobradar/service/resume/services"
)

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
