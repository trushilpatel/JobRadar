package services

import (
	"context"

	"github.com/danielgtaylor/huma/v2"
	"github.com/trushilpatel/jobradar/apps/api/apihuma"
	"github.com/trushilpatel/jobradar/apps/api/request"
	"github.com/trushilpatel/jobradar/apps/api/response"
	auditmodel "github.com/trushilpatel/jobradar/service/audit/models"
)

// AdminRestService is the HTTP adapter for the admin audit log. It owns
// request decoding and response shaping for GET /admin/audit-log; routing
// is the responsibility of AuditAdminRestRouter.
type AdminRestService struct {
	next Service
}

// NewAdminRestService constructs the admin audit HTTP adapter.
func NewAdminRestService(next Service) *AdminRestService {
	if next == nil {
		panic("audit.NewAdminRestService: next must not be nil")
	}
	return &AdminRestService{next: next}
}

// listInput is the Huma input for GET /admin/audit-log. Page/Limit are raw
// strings resolved via request.ParsePage/ParseLimit, matching
// request.ParsePagination's exact clamp/fallback semantics — this endpoint
// uses classic page+offset pagination (the documented admin-only exception
// in service/CLAUDE.md), not keyset cursors.
type listInput struct {
	ActorUserID string `query:"actor_user_id" doc:"Filter by acting user ID"`
	Action      string `query:"action" doc:"Filter by action name"`
	Page        string `query:"page" doc:"Page number, default 1" example:"1"`
	Limit       string `query:"limit" doc:"Page size 1-100, default 20" example:"20"`

	filter ListFilter
	page   int
	limit  int
}

func (in *listInput) Resolve(_ huma.Context) []error {
	def := request.DefaultPagination()
	in.page = request.ParsePage(in.Page, def.Page)
	in.limit = request.ParseLimit(in.Limit, def.Limit)

	in.filter = ListFilter{Limit: in.limit, Offset: (in.page - 1) * in.limit}
	if in.ActorUserID != "" {
		in.filter.ActorUserID = &in.ActorUserID
	}
	if in.Action != "" {
		in.filter.Action = &in.Action
	}
	return nil
}

// list handles GET /admin/audit-log.
func (h *AdminRestService) list(ctx context.Context, in *listInput) (*apihuma.PagedOutput[auditmodel.Entry], error) {
	entries, total, err := h.next.List(ctx, in.filter)
	if err != nil {
		return nil, apihuma.MapError(err)
	}

	if entries == nil {
		entries = []auditmodel.Entry{}
	}

	out := &apihuma.PagedOutput[auditmodel.Entry]{}
	out.Body = response.PagedResponse[auditmodel.Entry]{
		Data:  entries,
		Total: int(total),
		Page:  in.page,
		Limit: in.limit,
	}
	return out, nil
}
