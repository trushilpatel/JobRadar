// Package services holds the decorator chain for the resume domain.
package services

import (
	"context"
	"io"

	llmdto "github.com/trushilpatel/jobradar/service/llm/dto"
	"github.com/trushilpatel/jobradar/service/resume/dto"
	resumemodel "github.com/trushilpatel/jobradar/service/resume/models"
)

type Service interface {
	List(ctx context.Context, userID string) ([]resumemodel.Resume, error)
	Get(ctx context.Context, userID, id string) (*resumemodel.Resume, error)
	Upload(ctx context.Context, userID string, input dto.UploadInput) (*resumemodel.Resume, error)
	Rename(ctx context.Context, userID, id, label string) (*resumemodel.Resume, error)
	Delete(ctx context.Context, userID, id string) error
	// Download returns a reader over the resume's object-storage bytes plus
	// its metadata row (for content-type/filename/size) — the REST layer
	// streams the reader directly to the client. Callers must Close() it.
	Download(ctx context.Context, userID, id string) (io.ReadCloser, *resumemodel.Resume, error)
	// ExtractKeywords re-runs text extraction + LLM keyword extraction for an
	// existing resume and persists the result, returning the updated row.
	// Unlike the best-effort background extraction kicked off by Upload,
	// this is synchronous — the caller explicitly asked for it (first-time
	// extraction for a pre-feature resume, or a retry after a failed
	// best-effort attempt) and expects to see the outcome.
	ExtractKeywords(ctx context.Context, userID, id string) (*resumemodel.Resume, error)
}

type BaseService struct {
	next Service
}

func NewBaseService(next Service) BaseService {
	if next == nil {
		panic("resume.BaseService: next must not be nil — check wire setup")
	}
	return BaseService{next: next}
}

func (b *BaseService) List(ctx context.Context, userID string) ([]resumemodel.Resume, error) {
	return b.next.List(ctx, userID)
}

func (b *BaseService) Get(ctx context.Context, userID, id string) (*resumemodel.Resume, error) {
	return b.next.Get(ctx, userID, id)
}

func (b *BaseService) Upload(ctx context.Context, userID string, input dto.UploadInput) (*resumemodel.Resume, error) {
	return b.next.Upload(ctx, userID, input)
}

func (b *BaseService) Rename(ctx context.Context, userID, id, label string) (*resumemodel.Resume, error) {
	return b.next.Rename(ctx, userID, id, label)
}

func (b *BaseService) Delete(ctx context.Context, userID, id string) error {
	return b.next.Delete(ctx, userID, id)
}

func (b *BaseService) Download(ctx context.Context, userID, id string) (io.ReadCloser, *resumemodel.Resume, error) {
	return b.next.Download(ctx, userID, id)
}

func (b *BaseService) ExtractKeywords(ctx context.Context, userID, id string) (*resumemodel.Resume, error) {
	return b.next.ExtractKeywords(ctx, userID, id)
}

// KeywordExtractor is the narrow dependency ImplService needs from the LLM
// domain — satisfied structurally by service/llm/services.Scorer (which
// carries this method plus ScoreBatch/EnrichBatch the resume domain never
// calls). Declaring the narrow interface here, rather than depending on the
// full Scorer contract, mirrors service/job's ApplicationWriter precedent
// for a cross-domain dependency.
type KeywordExtractor interface {
	ExtractResumeKeywords(ctx context.Context, input llmdto.ResumeKeywordInput) (*llmdto.ResumeKeywordResult, error)
}

// KeywordStore persists an extraction result. Separate from Service because
// UpdateKeywords is an internal write invoked only by ImplService's own
// orchestration (both the post-upload background path and the explicit
// retry endpoint) — never exposed 1:1 through the public decorator chain the
// way List/Upload/Rename/Delete/Download are. Implemented by the same
// repositoryService struct via NewKeywordStore, mirroring
// service/applyqueue's EngagementChecker/ResumeOwnershipChecker precedent.
type KeywordStore interface {
	UpdateKeywords(ctx context.Context, id string, keywords []string) error
}
