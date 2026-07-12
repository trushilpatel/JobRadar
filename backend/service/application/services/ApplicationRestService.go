package services

import (
	"context"

	"github.com/danielgtaylor/huma/v2"
	"github.com/trushilpatel/jobradar/apps/api/apihuma"
	"github.com/trushilpatel/jobradar/apps/api/request"
	"github.com/trushilpatel/jobradar/service/application/dto"
	appmodel "github.com/trushilpatel/jobradar/service/application/models"
)

// RestService handles HTTP requests for the application domain. It owns
// no business logic — it parses the request, calls next, and writes the
// response.
type RestService struct {
	next ApplicationService
}

// NewRestService constructs a RestService.
func NewRestService(next ApplicationService) *RestService {
	return &RestService{next: next}
}

// listByJobInput is the Huma input for GET /api/jobs/{id}/applications.
type listByJobInput struct {
	ID string `path:"id"`
}

// list serves GET /api/jobs/{id}/applications.
func (h *RestService) list(ctx context.Context, in *listByJobInput) (*apihuma.SingleOutput[[]appmodel.JobApplication], error) {
	apps, err := h.next.ListByJob(ctx, in.ID)
	if err != nil {
		return nil, apihuma.MapError(err)
	}
	out := &apihuma.SingleOutput[[]appmodel.JobApplication]{}
	out.Body.Data = apps
	return out, nil
}

// createInput is the Huma input for POST /api/jobs/{id}/applications.
type createInput struct {
	ID   string `path:"id"`
	Body dto.CreateApplicationInput
}

// create serves POST /api/jobs/{id}/applications.
func (h *RestService) create(ctx context.Context, in *createInput) (*apihuma.SingleOutput[*appmodel.JobApplication], error) {
	app, err := h.next.Create(ctx, in.ID, in.Body)
	if err != nil {
		return nil, apihuma.MapError(err)
	}
	out := &apihuma.SingleOutput[*appmodel.JobApplication]{}
	out.Body.Data = app
	return out, nil
}

// listAllInput is the Huma input for GET /api/applications. Page/Limit are
// raw strings resolved via apps/api/request.ParsePage/ParseLimit, matching
// request.ParsePagination's exact clamp/fallback semantics.
type listAllInput struct {
	Page  string `query:"page" doc:"Page number, default 1" example:"1"`
	Limit string `query:"limit" doc:"Page size 1-100, default 20" example:"20"`

	page  int
	limit int
}

func (in *listAllInput) Resolve(_ huma.Context) []error {
	def := request.DefaultPagination()
	in.page = request.ParsePage(in.Page, def.Page)
	in.limit = request.ParseLimit(in.Limit, def.Limit)
	return nil
}

// listAll serves GET /api/applications.
func (h *RestService) listAll(ctx context.Context, in *listAllInput) (*apihuma.PagedOutput[ApplicationWithJob], error) {
	apps, total, err := h.next.ListAll(ctx, in.page, in.limit)
	if err != nil {
		return nil, apihuma.MapError(err)
	}
	out := &apihuma.PagedOutput[ApplicationWithJob]{}
	out.Body.Data = apps
	out.Body.Total = total
	out.Body.Page = in.page
	out.Body.Limit = in.limit
	return out, nil
}

// updateStageInput is the Huma input for PATCH /api/jobs/{id}/applications/{app_id}/stage.
type updateStageInput struct {
	ID    string `path:"id"`
	AppID string `path:"app_id"`
	Body  dto.UpdateStageInput
}

type stageResponse struct {
	AppID string `json:"app_id"`
	Stage string `json:"stage"`
}

// updateStage serves PATCH /api/jobs/{id}/applications/{app_id}/stage.
func (h *RestService) updateStage(ctx context.Context, in *updateStageInput) (*apihuma.SingleOutput[stageResponse], error) {
	if err := h.next.UpdateStage(ctx, in.ID, in.AppID, in.Body); err != nil {
		return nil, apihuma.MapError(err)
	}
	out := &apihuma.SingleOutput[stageResponse]{}
	out.Body.Data = stageResponse{AppID: in.AppID, Stage: in.Body.Stage}
	return out, nil
}

// updateStageByIDInput is the Huma input for PATCH /api/applications/{id}/stage.
type updateStageByIDInput struct {
	ID   string `path:"id"`
	Body dto.UpdateStageInput
}

// updateStageByID serves PATCH /api/applications/{id}/stage — top-level
// shortcut used by the dashboard interview tracker.
func (h *RestService) updateStageByID(ctx context.Context, in *updateStageByIDInput) (*apihuma.SingleOutput[stageResponse], error) {
	if err := h.next.UpdateStageByID(ctx, in.ID, in.Body); err != nil {
		return nil, apihuma.MapError(err)
	}
	out := &apihuma.SingleOutput[stageResponse]{}
	out.Body.Data = stageResponse{AppID: in.ID, Stage: in.Body.Stage}
	return out, nil
}

// updateNoteInput is the Huma input for PATCH /api/jobs/{id}/applications/{app_id}/note.
type updateNoteInput struct {
	ID    string `path:"id"`
	AppID string `path:"app_id"`
	Body  dto.UpdateNoteInput
}

type noteResponse struct {
	AppID string `json:"app_id"`
	Note  string `json:"note"`
}

// updateNote serves PATCH /api/jobs/{id}/applications/{app_id}/note.
func (h *RestService) updateNote(ctx context.Context, in *updateNoteInput) (*apihuma.SingleOutput[noteResponse], error) {
	if err := h.next.UpdateNote(ctx, in.ID, in.AppID, in.Body); err != nil {
		return nil, apihuma.MapError(err)
	}
	out := &apihuma.SingleOutput[noteResponse]{}
	out.Body.Data = noteResponse{AppID: in.AppID, Note: in.Body.Note}
	return out, nil
}
