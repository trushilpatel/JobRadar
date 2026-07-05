// Package services holds the decorator chain for the apply-queue domain.
package services

import (
	"context"

	"github.com/trushilpatel/jobradar/service/applyqueue/dto"
)

// Service is the single user-facing contract for the apply-queue domain.
// Every method takes userID explicitly, pulled from ctx by RestService via
// identity.MustFromContext — this mirrors service/resume's convention
// (userID as an explicit parameter threaded through every layer) rather than
// job's ctx-implicit pattern, per this slice's mandate to mirror resume/'s
// file layout and calling convention exactly.
type Service interface {
	// List returns a page of the caller's apply queue. filter is
	// applyqueuecommon.FilterAll or applyqueuecommon.FilterSaved.
	List(ctx context.Context, userID, filter, cursor string, limit int) (dto.QueueListResult, error)
	// SetResume attaches (resumeID != nil) or clears (resumeID == nil) the
	// resume associated with jobID for userID. Returns nil on a successful
	// clear; returns the resulting attachment view on a successful attach.
	SetResume(ctx context.Context, userID, jobID string, resumeID *string) (*dto.AttachmentView, error)
}

// BaseService auto-delegates every Service method to the next layer. Embed
// this in ImplService and ValidationService; override only the methods that
// layer is responsible for.
type BaseService struct {
	next Service
}

// NewBaseService constructs a BaseService wrapping next. Panics if next is
// nil — a nil next is always a wiring error, not a runtime condition.
func NewBaseService(next Service) BaseService {
	if next == nil {
		panic("applyqueue.BaseService: next must not be nil — check wire setup")
	}
	return BaseService{next: next}
}

func (b *BaseService) List(ctx context.Context, userID, filter, cursor string, limit int) (dto.QueueListResult, error) {
	return b.next.List(ctx, userID, filter, cursor, limit)
}

func (b *BaseService) SetResume(ctx context.Context, userID, jobID string, resumeID *string) (*dto.AttachmentView, error) {
	return b.next.SetResume(ctx, userID, jobID, resumeID)
}

// -----------------------------------------------------------------------------
// Narrow read interfaces used by ValidationService to perform existence /
// ownership checks before allowing a resume attachment. Each is implemented
// by a dedicated constructor in ApplyQueueRepositoryService.go
// (NewEngagementChecker / NewResumeOwnershipChecker) backed by the same
// stateless repositoryService struct as NewRepositoryService — three
// separate constructors, each returning its own narrow interface, avoids any
// ambiguity in the Wire provider graph (a single constructor returning a
// combined interface would create two candidate providers for Service once
// ImplService's constructor is added to the set).
// -----------------------------------------------------------------------------

// EngagementChecker confirms a user_job_state row exists for (userID, jobID)
// — a resume cannot be attached to a job the user has never engaged with
// (never saved, applied to, etc).
type EngagementChecker interface {
	HasEngagement(ctx context.Context, userID, jobID string) (bool, error)
}

// ResumeOwnershipChecker confirms a resume belongs to userID before it may
// be attached to a job — prevents attaching another user's resume.
type ResumeOwnershipChecker interface {
	ResumeOwnedByUser(ctx context.Context, userID, resumeID string) (bool, error)
}
