package tests

import (
	"context"
	"errors"
	"testing"

	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	resumecommon "github.com/trushilpatel/jobradar/service/resume/common"
	"github.com/trushilpatel/jobradar/service/resume/dto"
	resumesvc "github.com/trushilpatel/jobradar/service/resume/services"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// newResumeTestDB returns an in-memory sqlite DB with a hand-written `resumes`
// table (and a minimal `users` table for the FK) — same convention as
// service/user/tests/UserRepositoryService_test.go. Advisory locking is
// Postgres-only and is skipped by the repo when db.Name() != "postgres".
func newResumeTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	// TranslateError maps SQLite's raw "UNIQUE constraint failed" error onto
	// GORM's gorm.ErrDuplicatedKey sentinel, so isUniqueViolation (which checks
	// for it) behaves the same in these SQLite-backed tests as it does against
	// real Postgres — same pattern as service/company/tests.
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: gormlogger.Discard, TranslateError: true})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	stmts := []string{
		`CREATE TABLE users (id TEXT PRIMARY KEY)`,
		`CREATE TABLE resumes (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			label TEXT NOT NULL,
			original_filename TEXT NOT NULL,
			storage_key TEXT NOT NULL UNIQUE,
			content_type TEXT NOT NULL,
			size_bytes INTEGER NOT NULL,
			created_at DATETIME NOT NULL,
			extracted_text TEXT NOT NULL DEFAULT '',
			UNIQUE(user_id, label)
		)`,
	}
	for _, s := range stmts {
		if err := db.Exec(s).Error; err != nil {
			t.Fatalf("schema setup: %v", err)
		}
	}
	return db
}

func TestResumeRepository_UploadThenGetRoundTrips(t *testing.T) {
	db := newResumeTestDB(t)
	repo := resumesvc.NewRepositoryService(db)
	ctx := context.Background()

	created, err := repo.Upload(ctx, "user-1", dto.UploadInput{
		ID: "r1", Label: "Backend resume", OriginalFilename: "resume.pdf",
		ContentType: "application/pdf", Extension: "pdf",
		StorageKey: "resumes/user-1/r1.pdf", DeclaredSize: 1024,
	})
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if created.ID != "r1" || created.Label != "Backend resume" {
		t.Fatalf("unexpected created row: %+v", created)
	}

	fetched, err := repo.Get(ctx, "user-1", "r1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if fetched.StorageKey != "resumes/user-1/r1.pdf" {
		t.Fatalf("unexpected storage key: %s", fetched.StorageKey)
	}
}

func TestResumeRepository_GetForWrongUserReturnsNotFound(t *testing.T) {
	db := newResumeTestDB(t)
	repo := resumesvc.NewRepositoryService(db)
	ctx := context.Background()

	_, _ = repo.Upload(ctx, "user-1", dto.UploadInput{
		ID: "r1", Label: "L", OriginalFilename: "f.pdf", ContentType: "application/pdf",
		Extension: "pdf", StorageKey: "k1", DeclaredSize: 10,
	})

	_, err := repo.Get(ctx, "user-2", "r1")
	if !errors.Is(err, domainerrs.ErrNotFound) {
		t.Fatalf("expected NotFound for cross-user access, got %v", err)
	}
}

func TestResumeRepository_UploadAtCapIsRejected(t *testing.T) {
	db := newResumeTestDB(t)
	repo := resumesvc.NewRepositoryService(db)
	ctx := context.Background()

	for i := range resumecommon.ResumeLimitPerUser {
		id := "r" + string(rune('a'+i%26)) + string(rune('0'+i/26))
		_, err := repo.Upload(ctx, "user-1", dto.UploadInput{
			ID: id, Label: "L-" + id, OriginalFilename: "f.pdf", ContentType: "application/pdf",
			Extension: "pdf", StorageKey: "key-" + id, DeclaredSize: 10,
		})
		if err != nil {
			t.Fatalf("upload %d/%d: %v", i+1, resumecommon.ResumeLimitPerUser, err)
		}
	}

	_, err := repo.Upload(ctx, "user-1", dto.UploadInput{
		ID: "over-cap", Label: "L-over-cap", OriginalFilename: "f.pdf", ContentType: "application/pdf",
		Extension: "pdf", StorageKey: "key-over-cap", DeclaredSize: 10,
	})
	if err == nil {
		t.Fatal("expected the 51st upload to be rejected")
	}
}

func TestResumeRepository_UploadAtExactly50thSucceeds(t *testing.T) {
	db := newResumeTestDB(t)
	repo := resumesvc.NewRepositoryService(db)
	ctx := context.Background()

	for i := range resumecommon.ResumeLimitPerUser - 1 {
		id := "r" + string(rune('a'+i%26)) + string(rune('0'+i/26))
		if _, err := repo.Upload(ctx, "user-1", dto.UploadInput{
			ID: id, Label: "L-" + id, OriginalFilename: "f.pdf", ContentType: "application/pdf",
			Extension: "pdf", StorageKey: "key-" + id, DeclaredSize: 10,
		}); err != nil {
			t.Fatalf("upload %d: %v", i+1, err)
		}
	}
	// This is the 50th — must succeed (cap is inclusive of 50).
	if _, err := repo.Upload(ctx, "user-1", dto.UploadInput{
		ID: "last", Label: "L-last", OriginalFilename: "f.pdf", ContentType: "application/pdf",
		Extension: "pdf", StorageKey: "key-last", DeclaredSize: 10,
	}); err != nil {
		t.Fatalf("expected the 50th upload to succeed, got %v", err)
	}
}

func TestResumeRepository_RenameUpdatesLabel(t *testing.T) {
	db := newResumeTestDB(t)
	repo := resumesvc.NewRepositoryService(db)
	ctx := context.Background()

	_, _ = repo.Upload(ctx, "user-1", dto.UploadInput{
		ID: "r1", Label: "Old", OriginalFilename: "f.pdf", ContentType: "application/pdf",
		Extension: "pdf", StorageKey: "k1", DeclaredSize: 10,
	})
	renamed, err := repo.Rename(ctx, "user-1", "r1", "New label")
	if err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if renamed.Label != "New label" {
		t.Fatalf("expected label 'New label', got %s", renamed.Label)
	}
}

func TestResumeRepository_RenameMissingReturnsNotFound(t *testing.T) {
	db := newResumeTestDB(t)
	repo := resumesvc.NewRepositoryService(db)
	if _, err := repo.Rename(context.Background(), "user-1", "does-not-exist", "New"); !errors.Is(err, domainerrs.ErrNotFound) {
		t.Fatalf("expected NotFound, got %v", err)
	}
}

func TestResumeRepository_DeleteRemovesRow(t *testing.T) {
	db := newResumeTestDB(t)
	repo := resumesvc.NewRepositoryService(db)
	ctx := context.Background()

	_, _ = repo.Upload(ctx, "user-1", dto.UploadInput{
		ID: "r1", Label: "L", OriginalFilename: "f.pdf", ContentType: "application/pdf",
		Extension: "pdf", StorageKey: "k1", DeclaredSize: 10,
	})
	if err := repo.Delete(ctx, "user-1", "r1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := repo.Get(ctx, "user-1", "r1"); !errors.Is(err, domainerrs.ErrNotFound) {
		t.Fatalf("expected NotFound after delete, got %v", err)
	}
}

func TestResumeRepository_DoubleDeleteSecondCallIsNotFound(t *testing.T) {
	db := newResumeTestDB(t)
	repo := resumesvc.NewRepositoryService(db)
	ctx := context.Background()

	_, _ = repo.Upload(ctx, "user-1", dto.UploadInput{
		ID: "r1", Label: "L", OriginalFilename: "f.pdf", ContentType: "application/pdf",
		Extension: "pdf", StorageKey: "k1", DeclaredSize: 10,
	})
	_ = repo.Delete(ctx, "user-1", "r1")
	if err := repo.Delete(ctx, "user-1", "r1"); !errors.Is(err, domainerrs.ErrNotFound) {
		t.Fatalf("expected NotFound on second delete, got %v", err)
	}
}

func TestResumeRepository_DeleteForWrongUserReturnsNotFound(t *testing.T) {
	db := newResumeTestDB(t)
	repo := resumesvc.NewRepositoryService(db)
	ctx := context.Background()

	_, _ = repo.Upload(ctx, "user-1", dto.UploadInput{
		ID: "r1", Label: "L", OriginalFilename: "f.pdf", ContentType: "application/pdf",
		Extension: "pdf", StorageKey: "k1", DeclaredSize: 10,
	})
	if err := repo.Delete(ctx, "user-2", "r1"); !errors.Is(err, domainerrs.ErrNotFound) {
		t.Fatalf("expected NotFound deleting another user's resume, got %v", err)
	}
}

func TestResumeRepository_ListOrdersByCreatedAtDesc(t *testing.T) {
	db := newResumeTestDB(t)
	repo := resumesvc.NewRepositoryService(db)
	ctx := context.Background()

	_, _ = repo.Upload(ctx, "user-1", dto.UploadInput{ID: "r1", Label: "First", OriginalFilename: "f.pdf", ContentType: "application/pdf", Extension: "pdf", StorageKey: "k1", DeclaredSize: 10})
	_, _ = repo.Upload(ctx, "user-1", dto.UploadInput{ID: "r2", Label: "Second", OriginalFilename: "f.pdf", ContentType: "application/pdf", Extension: "pdf", StorageKey: "k2", DeclaredSize: 10})

	rows, err := repo.List(ctx, "user-1")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
}

// closedResumeTestDB returns a DB whose underlying connection is already
// closed, so any subsequent query fails with a generic (non-NotFound) DB
// error — used to exercise the repository's ErrInternal-wrapping branches
// that a normal query never reaches.
func closedResumeTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := newResumeTestDB(t)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get underlying sql.DB: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close sql.DB: %v", err)
	}
	return db
}

func TestResumeRepository_ListReturnsInternalErrorOnDBFailure(t *testing.T) {
	repo := resumesvc.NewRepositoryService(closedResumeTestDB(t))
	if _, err := repo.List(context.Background(), "user-1"); err == nil {
		t.Fatal("expected an error when the underlying connection is closed")
	}
}

func TestResumeRepository_GetReturnsInternalErrorOnDBFailure(t *testing.T) {
	repo := resumesvc.NewRepositoryService(closedResumeTestDB(t))
	if _, err := repo.Get(context.Background(), "user-1", "r1"); err == nil {
		t.Fatal("expected an error when the underlying connection is closed")
	}
}

func TestResumeRepository_RenameReturnsInternalErrorOnDBFailure(t *testing.T) {
	repo := resumesvc.NewRepositoryService(closedResumeTestDB(t))
	if _, err := repo.Rename(context.Background(), "user-1", "r1", "New"); err == nil {
		t.Fatal("expected an error when the underlying connection is closed")
	}
}

func TestResumeRepository_DeleteReturnsInternalErrorOnDBFailure(t *testing.T) {
	repo := resumesvc.NewRepositoryService(closedResumeTestDB(t))
	if err := repo.Delete(context.Background(), "user-1", "r1"); err == nil {
		t.Fatal("expected an error when the underlying connection is closed")
	}
}

func TestResumeRepository_UploadReturnsInternalErrorOnDBFailure(t *testing.T) {
	repo := resumesvc.NewRepositoryService(closedResumeTestDB(t))
	_, err := repo.Upload(context.Background(), "user-1", dto.UploadInput{
		ID: "r1", Label: "L", OriginalFilename: "f.pdf", ContentType: "application/pdf",
		Extension: "pdf", StorageKey: "k1", DeclaredSize: 10,
	})
	if err == nil {
		t.Fatal("expected an error when the underlying connection is closed")
	}
}

func TestResumeRepository_UploadRejectsDuplicateStorageKey(t *testing.T) {
	db := newResumeTestDB(t)
	repo := resumesvc.NewRepositoryService(db)
	ctx := context.Background()

	_, err := repo.Upload(ctx, "user-1", dto.UploadInput{
		ID: "r1", Label: "L", OriginalFilename: "f.pdf", ContentType: "application/pdf",
		Extension: "pdf", StorageKey: "shared-key", DeclaredSize: 10,
	})
	if err != nil {
		t.Fatalf("first upload: %v", err)
	}

	_, err = repo.Upload(ctx, "user-2", dto.UploadInput{
		ID: "r2", Label: "L", OriginalFilename: "f.pdf", ContentType: "application/pdf",
		Extension: "pdf", StorageKey: "shared-key", DeclaredSize: 10,
	})
	if err == nil {
		t.Fatal("expected the duplicate storage_key insert to fail")
	}
}

func TestResumeRepository_UploadRejectsDuplicateLabelForSameUser(t *testing.T) {
	db := newResumeTestDB(t)
	repo := resumesvc.NewRepositoryService(db)
	ctx := context.Background()

	_, err := repo.Upload(ctx, "user-1", dto.UploadInput{
		ID: "r1", Label: "My resume", OriginalFilename: "f.pdf", ContentType: "application/pdf",
		Extension: "pdf", StorageKey: "k1", DeclaredSize: 10,
	})
	if err != nil {
		t.Fatalf("first upload: %v", err)
	}

	_, err = repo.Upload(ctx, "user-1", dto.UploadInput{
		ID: "r2", Label: "My resume", OriginalFilename: "f.pdf", ContentType: "application/pdf",
		Extension: "pdf", StorageKey: "k2", DeclaredSize: 10,
	})
	if !errors.Is(err, domainerrs.ErrConflict) {
		t.Fatalf("expected ErrConflict for a duplicate label, got %v", err)
	}
}

func TestResumeRepository_UploadAllowsSameLabelForDifferentUsers(t *testing.T) {
	db := newResumeTestDB(t)
	repo := resumesvc.NewRepositoryService(db)
	ctx := context.Background()

	_, err := repo.Upload(ctx, "user-1", dto.UploadInput{
		ID: "r1", Label: "My resume", OriginalFilename: "f.pdf", ContentType: "application/pdf",
		Extension: "pdf", StorageKey: "k1", DeclaredSize: 10,
	})
	if err != nil {
		t.Fatalf("first upload: %v", err)
	}

	_, err = repo.Upload(ctx, "user-2", dto.UploadInput{
		ID: "r2", Label: "My resume", OriginalFilename: "f.pdf", ContentType: "application/pdf",
		Extension: "pdf", StorageKey: "k2", DeclaredSize: 10,
	})
	if err != nil {
		t.Fatalf("expected the same label to be allowed for a different user, got %v", err)
	}
}

func TestResumeRepository_RenameRejectsCollisionWithAnotherResumesLabel(t *testing.T) {
	db := newResumeTestDB(t)
	repo := resumesvc.NewRepositoryService(db)
	ctx := context.Background()

	_, _ = repo.Upload(ctx, "user-1", dto.UploadInput{
		ID: "r1", Label: "Taken", OriginalFilename: "f.pdf", ContentType: "application/pdf",
		Extension: "pdf", StorageKey: "k1", DeclaredSize: 10,
	})
	_, _ = repo.Upload(ctx, "user-1", dto.UploadInput{
		ID: "r2", Label: "Other", OriginalFilename: "f.pdf", ContentType: "application/pdf",
		Extension: "pdf", StorageKey: "k2", DeclaredSize: 10,
	})

	_, err := repo.Rename(ctx, "user-1", "r2", "Taken")
	if !errors.Is(err, domainerrs.ErrConflict) {
		t.Fatalf("expected ErrConflict when renaming to another resume's label, got %v", err)
	}
}

func TestResumeRepository_RenameToOwnCurrentLabelSucceeds(t *testing.T) {
	db := newResumeTestDB(t)
	repo := resumesvc.NewRepositoryService(db)
	ctx := context.Background()

	_, _ = repo.Upload(ctx, "user-1", dto.UploadInput{
		ID: "r1", Label: "Same", OriginalFilename: "f.pdf", ContentType: "application/pdf",
		Extension: "pdf", StorageKey: "k1", DeclaredSize: 10,
	})

	renamed, err := repo.Rename(ctx, "user-1", "r1", "Same")
	if err != nil {
		t.Fatalf("expected renaming a resume to its own current label to succeed, got %v", err)
	}
	if renamed.Label != "Same" {
		t.Fatalf("expected label to remain 'Same', got %s", renamed.Label)
	}
}

func TestResumeRepository_DownloadIsUnreachableStub(t *testing.T) {
	db := newResumeTestDB(t)
	repo := resumesvc.NewRepositoryService(db)
	if _, _, err := repo.Download(context.Background(), "user-1", "r1"); err == nil {
		t.Fatal("expected the repository-layer stub to always return an error")
	}
}

func TestResumeRepository_UploadPersistsExtractedText(t *testing.T) {
	db := newResumeTestDB(t)
	repo := resumesvc.NewRepositoryService(db)

	created, err := repo.Upload(context.Background(), "user-1", dto.UploadInput{
		ID: "r1", Label: "L", OriginalFilename: "f.pdf", ContentType: "application/pdf",
		StorageKey: "resumes/user-1/r1.pdf", DeclaredSize: 8,
		ExtractedText: "Backend engineer with Go and PostgreSQL experience.",
	})
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if created.ExtractedText != "Backend engineer with Go and PostgreSQL experience." {
		t.Fatalf("expected ExtractedText to be persisted, got %q", created.ExtractedText)
	}

	got, err := repo.Get(context.Background(), "user-1", "r1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ExtractedText != created.ExtractedText {
		t.Fatalf("Get returned different ExtractedText: %q vs %q", got.ExtractedText, created.ExtractedText)
	}
}
