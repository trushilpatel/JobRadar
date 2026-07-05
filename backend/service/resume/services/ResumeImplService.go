package services

import (
	"bytes"
	"context"
	"fmt"

	"github.com/google/uuid"
	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/common/tracing"
	"github.com/trushilpatel/jobradar/platform/objectstorage"
	resumecommon "github.com/trushilpatel/jobradar/service/resume/common"
	"github.com/trushilpatel/jobradar/service/resume/dto"
	resumemodel "github.com/trushilpatel/jobradar/service/resume/models"
)

type implService struct {
	BaseService
	storage objectstorage.Storage
	log     logger.Logger
}

func NewImplService(next Service, storage objectstorage.Storage, log logger.Logger) Service {
	return &implService{BaseService: NewBaseService(next), storage: storage, log: log}
}

// Upload writes the file to object storage before creating the DB row — a
// crash between the two steps leaves an unreferenced (harmless, cheap)
// object in storage rather than a DB row pointing at nothing. If the DB
// insert itself fails (e.g. the per-user resume cap), the just-written
// object is deleted immediately since the failure is known synchronously.
func (s *implService) Upload(ctx context.Context, userID string, input dto.UploadInput) (_ *resumemodel.Resume, err error) {
	ctx, span := tracing.Start(ctx, "resume.impl.Upload")
	defer tracing.End(span, &err)

	input.ID = uuid.NewString()
	input.StorageKey = fmt.Sprintf("resumes/%s/%s.%s", userID, input.ID, input.Extension)

	if putErr := s.storage.Put(ctx, input.StorageKey, bytes.NewReader(input.RawBytes), input.DeclaredSize, input.ContentType); putErr != nil {
		return nil, domainerrs.ErrInternal.WithMessage("resume impl: storage put: " + putErr.Error())
	}

	resume, err := s.next.Upload(ctx, userID, input)
	if err != nil {
		if delErr := s.storage.Delete(ctx, input.StorageKey); delErr != nil {
			s.log.Warn("resume impl: cleanup after failed create left an orphaned object",
				logger.Str(logger.FieldStorageKey, input.StorageKey), logger.Err(delErr))
		}
		return nil, err
	}
	return resume, nil
}

// Delete removes the DB row first, then best-effort deletes the object from
// storage. A storage-delete failure is logged but never blocks the
// user-visible delete — an orphaned object here is accepted Phase-0 tech
// debt (no reaper yet); a DB row kept alive by a failed storage delete would
// be the worse, user-visible failure mode.
func (s *implService) Delete(ctx context.Context, userID, id string) (err error) {
	ctx, span := tracing.Start(ctx, "resume.impl.Delete")
	defer tracing.End(span, &err)

	resume, err := s.next.Get(ctx, userID, id)
	if err != nil {
		return err
	}
	if delErr := s.next.Delete(ctx, userID, id); delErr != nil {
		return delErr
	}
	if storageErr := s.storage.Delete(ctx, resume.StorageKey); storageErr != nil {
		s.log.Warn("resume impl: storage object delete failed after DB delete succeeded — orphaned object",
			logger.Str(logger.FieldStorageKey, resume.StorageKey), logger.Err(storageErr))
	}
	return nil
}

func (s *implService) PresignDownload(ctx context.Context, userID, id string) (_ string, err error) {
	ctx, span := tracing.Start(ctx, "resume.impl.PresignDownload")
	defer tracing.End(span, &err)

	resume, err := s.next.Get(ctx, userID, id)
	if err != nil {
		return "", err
	}
	url, presignErr := s.storage.PresignGet(ctx, resume.StorageKey, resumecommon.PresignTTL)
	if presignErr != nil {
		return "", domainerrs.ErrInternal.WithMessage("resume impl: presign: " + presignErr.Error())
	}
	return url, nil
}

var _ Service = (*implService)(nil)
