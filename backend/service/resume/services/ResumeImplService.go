package services

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"runtime/debug"

	"github.com/google/uuid"
	domainerrs "github.com/trushilpatel/jobradar/common/errors"
	"github.com/trushilpatel/jobradar/common/logger"
	"github.com/trushilpatel/jobradar/common/tracing"
	"github.com/trushilpatel/jobradar/platform/objectstorage"
	techtag "github.com/trushilpatel/jobradar/service/job/techtag"
	llmdto "github.com/trushilpatel/jobradar/service/llm/dto"
	resumecommon "github.com/trushilpatel/jobradar/service/resume/common"
	"github.com/trushilpatel/jobradar/service/resume/dto"
	resumemodel "github.com/trushilpatel/jobradar/service/resume/models"
	"github.com/trushilpatel/jobradar/service/resume/textextract"
)

type implService struct {
	BaseService
	storage      objectstorage.Storage
	extractor    KeywordExtractor
	keywordStore KeywordStore
	log          logger.Logger
}

func NewImplService(next Service, storage objectstorage.Storage, extractor KeywordExtractor, keywordStore KeywordStore, log logger.Logger) Service {
	return &implService{
		BaseService:  NewBaseService(next),
		storage:      storage,
		extractor:    extractor,
		keywordStore: keywordStore,
		log:          log,
	}
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
	s.scheduleKeywordExtraction(resume)
	return resume, nil
}

// scheduleKeywordExtraction fires best-effort keyword extraction in a
// detached goroutine after a successful upload — it must never make the
// upload response slower or flakier. Uses context.Background() (not the
// request ctx, which is cancelled the moment the HTTP response is written)
// bounded by resumecommon.KeywordExtractionTimeout so a hung LLM call can
// never leak the goroutine. The deferred recover() ensures a panic anywhere
// in the extraction path (a third-party PDF/DOCX parser defect, for
// instance) can never crash the process — on any failure the resume simply
// keeps keywords=[] until the explicit POST /resumes/{id}/keywords retry
// succeeds.
func (s *implService) scheduleKeywordExtraction(resume *resumemodel.Resume) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				s.log.Error("resume impl: recovered panic during background keyword extraction",
					fmt.Errorf("panic: %v", r),
					logger.Str(logger.FieldResumeID, resume.ID),
					logger.Str(logger.FieldStack, string(debug.Stack())))
			}
		}()

		ctx, cancel := context.WithTimeout(context.Background(), resumecommon.KeywordExtractionTimeout)
		defer cancel()

		keywords, err := s.runKeywordExtraction(ctx, resume)
		if err != nil {
			s.log.Warn("resume impl: best-effort keyword extraction failed, resume keeps keywords=[]",
				logger.Str(logger.FieldResumeID, resume.ID), logger.Err(err))
			return
		}
		if err := s.keywordStore.UpdateKeywords(ctx, resume.ID, keywords); err != nil {
			s.log.Warn("resume impl: failed to persist background-extracted keywords",
				logger.Str(logger.FieldResumeID, resume.ID), logger.Err(err))
		}
	}()
}

// ExtractKeywords re-runs extraction synchronously and persists the result —
// unlike the post-upload background path, the caller explicitly asked for
// this and expects to see either the updated resume or the failure.
func (s *implService) ExtractKeywords(ctx context.Context, userID, id string) (_ *resumemodel.Resume, err error) {
	ctx, span := tracing.Start(ctx, "resume.impl.ExtractKeywords")
	defer tracing.End(span, &err)

	resume, err := s.next.Get(ctx, userID, id)
	if err != nil {
		return nil, err
	}

	extractCtx, cancel := context.WithTimeout(ctx, resumecommon.KeywordExtractionTimeout)
	defer cancel()

	keywords, extractErr := s.runKeywordExtraction(extractCtx, resume)
	if extractErr != nil {
		return nil, domainerrs.ErrInternal.WithMessage("resume impl: keyword extraction: " + extractErr.Error())
	}
	if updErr := s.keywordStore.UpdateKeywords(ctx, resume.ID, keywords); updErr != nil {
		return nil, updErr
	}
	return s.next.Get(ctx, userID, id)
}

// runKeywordExtraction is the shared storage-read -> text-extract -> LLM
// pipeline used by both the post-upload background path and the explicit
// retry endpoint. Keywords are run through techtag.Clean for the same
// canonical normalization the job-enrichment pipeline applies to
// LLM-extracted tech stacks — one taxonomy, reused rather than duplicated.
func (s *implService) runKeywordExtraction(ctx context.Context, resume *resumemodel.Resume) (_ []string, err error) {
	ctx, span := tracing.Start(ctx, "resume.impl.runKeywordExtraction")
	defer tracing.End(span, &err)

	reader, _, getErr := s.storage.Get(ctx, resume.StorageKey)
	if getErr != nil {
		return nil, fmt.Errorf("storage get: %w", getErr)
	}
	defer reader.Close()

	text, extractErr := textextract.ExtractText(resume.ContentType, reader)
	if extractErr != nil {
		return nil, fmt.Errorf("extract text: %w", extractErr)
	}

	result, llmErr := s.extractor.ExtractResumeKeywords(ctx, llmdto.ResumeKeywordInput{Text: text})
	if llmErr != nil {
		return nil, fmt.Errorf("llm extract keywords: %w", llmErr)
	}

	return techtag.Clean(result.Keywords), nil
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

func (s *implService) Download(ctx context.Context, userID, id string) (_ io.ReadCloser, _ *resumemodel.Resume, err error) {
	ctx, span := tracing.Start(ctx, "resume.impl.Download")
	defer tracing.End(span, &err)

	resume, err := s.next.Get(ctx, userID, id)
	if err != nil {
		return nil, nil, err
	}
	reader, _, getErr := s.storage.Get(ctx, resume.StorageKey)
	if getErr != nil {
		if errors.Is(getErr, objectstorage.ErrNotFound) {
			return nil, nil, domainerrs.ErrNotFound.WithMessage("resume impl: object missing from storage: " + getErr.Error())
		}
		return nil, nil, domainerrs.ErrInternal.WithMessage("resume impl: storage get: " + getErr.Error())
	}
	return reader, resume, nil
}

var _ Service = (*implService)(nil)
