package services

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/tracing"
	resumecommon "github.com/trushilpatel/jobradar/service/resume/common"
	"github.com/trushilpatel/jobradar/service/resume/dto"
	resumemodel "github.com/trushilpatel/jobradar/service/resume/models"
	"gorm.io/gorm"
)

// isUniqueViolation detects PostgreSQL unique-constraint failures across the
// GORM driver landscape (SQLSTATE 23505) — same pattern as
// service/company/services/CompanyScanSettingsRepositoryService.go. pgx wraps
// the code on a *pgconn.PgError; GORM's generic wrapper hides it behind
// ErrDuplicatedKey on recent versions. Checked both for portability across
// local dev (pgx direct) and CI.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}

type repositoryService struct {
	db *gorm.DB
}

func NewRepositoryService(db *gorm.DB) Service {
	return &repositoryService{db: db}
}

func (r *repositoryService) List(ctx context.Context, userID string) (_ []resumemodel.Resume, err error) {
	ctx, span := tracing.Start(ctx, "resume.repo.List")
	defer tracing.End(span, &err)

	var rows []resumemodel.Resume
	if err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Find(&rows).Error; err != nil {
		return nil, domainerrs.ErrInternal.WithMessage("resume repo: list: " + err.Error())
	}
	return rows, nil
}

func (r *repositoryService) Get(ctx context.Context, userID, id string) (_ *resumemodel.Resume, err error) {
	ctx, span := tracing.Start(ctx, "resume.repo.Get")
	defer tracing.End(span, &err)

	var row resumemodel.Resume
	dbErr := r.db.WithContext(ctx).Where("id = ? AND user_id = ?", id, userID).First(&row).Error
	if dbErr != nil {
		if errors.Is(dbErr, gorm.ErrRecordNotFound) {
			return nil, domainerrs.ErrNotFound.WithMessage("resume not found")
		}
		return nil, domainerrs.ErrInternal.WithMessage("resume repo: get: " + dbErr.Error())
	}
	return &row, nil
}

// Upload inserts a new resume row inside a transaction that also enforces the
// per-user resume cap. On Postgres, a session-scoped advisory lock
// (pg_advisory_xact_lock, keyed by a hash of the user id) serializes
// concurrent uploads from the same user for the transaction's lifetime,
// closing the race where two requests both read count=49 before either
// commits. SQLite (used by these unit tests) has no advisory-lock primitive,
// so that step is skipped there — count+insert still runs atomically inside
// one transaction, sufficient for a single-connection test double; the
// concurrent-race guarantee itself is an integration-tier concern to verify
// against real Postgres.
func (r *repositoryService) Upload(ctx context.Context, userID string, input dto.UploadInput) (_ *resumemodel.Resume, err error) {
	ctx, span := tracing.Start(ctx, "resume.repo.Upload")
	defer tracing.End(span, &err)

	row := resumemodel.Resume{
		ID:               input.ID,
		UserID:           userID,
		Label:            input.Label,
		OriginalFilename: input.OriginalFilename,
		StorageKey:       input.StorageKey,
		ContentType:      input.ContentType,
		SizeBytes:        input.DeclaredSize,
		CreatedAt:        time.Now().UTC(),
		ExtractedText:    input.ExtractedText,
	}

	txErr := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if tx.Name() == "postgres" {
			if lockErr := tx.Exec("SELECT pg_advisory_xact_lock(hashtext(?))", userID).Error; lockErr != nil {
				return domainerrs.ErrInternal.WithMessage("resume repo: advisory lock: " + lockErr.Error())
			}
		}
		var count int64
		if countErr := tx.Model(&resumemodel.Resume{}).Where("user_id = ?", userID).Count(&count).Error; countErr != nil {
			return domainerrs.ErrInternal.WithMessage("resume repo: count: " + countErr.Error())
		}
		if count >= resumecommon.ResumeLimitPerUser {
			return domainerrs.ErrConflict.WithMessage("resume repo: user at resume limit")
		}
		if createErr := tx.Create(&row).Error; createErr != nil {
			if isUniqueViolation(createErr) {
				return domainerrs.ErrConflict.
					WithMessage("resume repo: insert: " + createErr.Error()).
					WithExternal(fmt.Sprintf("a resume named %q already exists", input.Label))
			}
			return domainerrs.ErrInternal.WithMessage("resume repo: insert: " + createErr.Error())
		}
		return nil
	})
	if txErr != nil {
		return nil, txErr
	}
	return &row, nil
}

func (r *repositoryService) Rename(ctx context.Context, userID, id, label string) (_ *resumemodel.Resume, err error) {
	ctx, span := tracing.Start(ctx, "resume.repo.Rename")
	defer tracing.End(span, &err)

	res := r.db.WithContext(ctx).Model(&resumemodel.Resume{}).
		Where("id = ? AND user_id = ?", id, userID).
		Update("label", label)
	if res.Error != nil {
		if isUniqueViolation(res.Error) {
			return nil, domainerrs.ErrConflict.
				WithMessage("resume repo: rename: " + res.Error.Error()).
				WithExternal(fmt.Sprintf("a resume named %q already exists", label))
		}
		return nil, domainerrs.ErrInternal.WithMessage("resume repo: rename: " + res.Error.Error())
	}
	if res.RowsAffected == 0 {
		return nil, domainerrs.ErrNotFound.WithMessage("resume not found")
	}
	return r.Get(ctx, userID, id)
}

func (r *repositoryService) Delete(ctx context.Context, userID, id string) (err error) {
	ctx, span := tracing.Start(ctx, "resume.repo.Delete")
	defer tracing.End(span, &err)

	res := r.db.WithContext(ctx).Where("id = ? AND user_id = ?", id, userID).Delete(&resumemodel.Resume{})
	if res.Error != nil {
		return domainerrs.ErrInternal.WithMessage("resume repo: delete: " + res.Error.Error())
	}
	if res.RowsAffected == 0 {
		return domainerrs.ErrNotFound.WithMessage("resume not found")
	}
	return nil
}

// Download is unreachable in the real chain: ImplService overrides it and
// never delegates further down (object storage access does not belong in
// the GORM-only repository layer). Implemented only so repositoryService
// satisfies Service completely.
func (r *repositoryService) Download(_ context.Context, _, _ string) (io.ReadCloser, *resumemodel.Resume, error) {
	return nil, nil, domainerrs.ErrInternal.WithMessage("resume repo: download not supported at repository layer")
}

var _ Service = (*repositoryService)(nil)
