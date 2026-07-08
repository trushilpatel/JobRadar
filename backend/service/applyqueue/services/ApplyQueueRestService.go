package services

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/trushilpatel/jobradar/apps/api/apihuma"
	"github.com/trushilpatel/jobradar/apps/api/request"
	"github.com/trushilpatel/jobradar/common/identity"
	applyqueuecommon "github.com/trushilpatel/jobradar/service/applyqueue/common"
	"github.com/trushilpatel/jobradar/service/applyqueue/dto"
)

// RestService is the HTTP adapter for the apply-queue Service chain.
type RestService struct {
	next Service
}

// NewRestService constructs a RestService.
func NewRestService(next Service) *RestService {
	if next == nil {
		panic("applyqueue.NewRestService: next must not be nil")
	}
	return &RestService{next: next}
}

// listInput is the Huma input for GET /api/apply-queue. Limit uses the
// shared apps/api/request.ParseLimit — this slice's own inline limit
// parsing (no upper clamp) was a second, independent implementation of the
// same default that request.ParsePagination already provides; migrating it
// onto the shared helper collapses that duplication (a deliberate,
// pre-approved behavior tightening: an out-of-range limit is now clamped to
// 100 instead of passed through unbounded — no existing test relied on the
// unbounded behavior).
type listInput struct {
	Filter string `query:"filter" doc:"all or saved; defaults to all"`
	Cursor string `query:"cursor" doc:"Opaque keyset pagination cursor"`
	Limit  string `query:"limit" doc:"Page size 1-100; invalid or missing values default to 20" example:"20"`

	filter string
	limit  int
}

func (in *listInput) Resolve(_ huma.Context) []error {
	in.filter = in.Filter
	if in.filter == "" {
		in.filter = applyqueuecommon.FilterAll
	}
	in.limit = request.ParseLimit(in.Limit, request.DefaultPagination().Limit)
	return nil
}

// list serves GET /api/apply-queue?filter=all|saved&cursor=&limit=.
func (h *RestService) list(ctx context.Context, in *listInput) (*apihuma.SingleOutput[dto.QueueListResult], error) {
	userID := identity.MustFromContext(ctx).UserID
	result, err := h.next.List(ctx, userID, in.filter, in.Cursor, in.limit)
	if err != nil {
		return nil, apihuma.MapError(err)
	}
	out := &apihuma.SingleOutput[dto.QueueListResult]{}
	out.Body.Data = result
	return out, nil
}

// setResumeInput is the Huma input for PUT /api/apply-queue/{jobId}/resume.
type setResumeInput struct {
	JobID string `path:"jobId"`
	Body  dto.SetResumeInput
}

// setResumeOutput uses a Body func(huma.Context) — Huma's escape hatch for
// fully manual response writing — because this single operation must
// sometimes respond 200 with a JSON body (resume attached) and sometimes
// 204 with NO body at all (attachment cleared). Huma's typed Body mechanism
// always writes something (even "null" for a nil pointer field), which
// would not byte-match the legacy handler's true empty 204
// (handler.RespondNoContent). Writing the response by hand here reproduces
// the legacy handler's exact byte output in both branches.
type setResumeOutput struct {
	Body func(huma.Context)
}

// setResume serves PUT /api/apply-queue/{jobId}/resume. A null resume_id
// clears the attachment (204 No Content); a non-null resume_id attaches it
// (200 with the resulting AttachmentView).
func (h *RestService) setResume(ctx context.Context, in *setResumeInput) (*setResumeOutput, error) {
	userID := identity.MustFromContext(ctx).UserID
	view, err := h.next.SetResume(ctx, userID, in.JobID, in.Body.ResumeID)
	if err != nil {
		return nil, apihuma.MapError(err)
	}
	return &setResumeOutput{Body: func(hctx huma.Context) {
		if view == nil {
			hctx.SetStatus(http.StatusNoContent)
			return
		}
		hctx.SetHeader("Content-Type", "application/json")
		hctx.SetStatus(http.StatusOK)
		body := apihuma.SingleOutput[*dto.AttachmentView]{}
		body.Body.Data = view
		_ = json.NewEncoder(hctx.BodyWriter()).Encode(body.Body)
	}}, nil
}
