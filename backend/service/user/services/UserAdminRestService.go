package services

import (
	"context"

	"github.com/danielgtaylor/huma/v2"
	"github.com/trushilpatel/jobradar/apps/api/apihuma"
	"github.com/trushilpatel/jobradar/apps/api/request"
	"github.com/trushilpatel/jobradar/apps/api/response"
	"github.com/trushilpatel/jobradar/common/identity"
)

// AdminRestService is the HTTP adapter for the admin user chain. It owns
// request decoding and response shaping for the /admin/users endpoints;
// routing is the responsibility of UserAdminRestRouter.
type AdminRestService struct {
	next AdminService
}

// NewAdminRestService constructs the admin HTTP adapter.
func NewAdminRestService(next AdminService) *AdminRestService {
	if next == nil {
		panic("user.NewAdminRestService: next must not be nil")
	}
	return &AdminRestService{next: next}
}

// adminListInput is the Huma input for GET /admin/users. Page/Limit are raw
// strings resolved via request.ParsePage/ParseLimit, matching
// request.ParsePagination's exact clamp/fallback semantics — this endpoint
// uses classic page+offset pagination (the documented admin-only exception
// in service/CLAUDE.md), not keyset cursors.
type adminListInput struct {
	Status string `query:"status" doc:"Filter by user status"`
	Email  string `query:"email" doc:"ILIKE match on email"`
	Page   string `query:"page" doc:"Page number, default 1" example:"1"`
	Limit  string `query:"limit" doc:"Page size 1-100, default 20" example:"20"`

	filter AdminUserFilter
	page   int
	limit  int
}

func (in *adminListInput) Resolve(_ huma.Context) []error {
	if in.Status != "" {
		in.filter.Status = &in.Status
	}
	in.filter.Email = in.Email

	def := request.DefaultPagination()
	in.page = request.ParsePage(in.Page, def.Page)
	in.limit = request.ParseLimit(in.Limit, def.Limit)
	return nil
}

// list handles GET /admin/users.
func (h *AdminRestService) list(ctx context.Context, in *adminListInput) (*apihuma.PagedOutput[UserWithRoles], error) {
	result, err := h.next.List(ctx, in.filter, in.page, in.limit)
	if err != nil {
		return nil, apihuma.MapError(err)
	}

	items := result.Items
	if items == nil {
		items = []UserWithRoles{}
	}

	out := &apihuma.PagedOutput[UserWithRoles]{}
	out.Body = response.PagedResponse[UserWithRoles]{
		Data:  items,
		Total: int(result.Total),
		Page:  in.page,
		Limit: in.limit,
	}
	return out, nil
}

// adminGetInput is the Huma input for GET/PATCH/DELETE /admin/users/{id}
// operations that only take the path ID.
type adminGetInput struct {
	ID string `path:"id"`
}

// get handles GET /admin/users/{id}.
func (h *AdminRestService) get(ctx context.Context, in *adminGetInput) (*apihuma.SingleOutput[*UserWithRoles], error) {
	view, err := h.next.GetWithRoles(ctx, in.ID)
	if err != nil {
		return nil, apihuma.MapError(err)
	}

	out := &apihuma.SingleOutput[*UserWithRoles]{}
	out.Body = response.Response[*UserWithRoles]{Data: view}
	return out, nil
}

// userIDResponse is the response body for a partial-patch endpoint that only
// echoes back the ID of the row it touched.
type userIDResponse struct {
	ID string `json:"id"`
}

// setRolesInput is the Huma input for PATCH /admin/users/{id}/roles.
// RoleIDs is required:"false" — an absent role_ids key is the documented
// "clear all roles" path, not a validation failure.
type setRolesInput struct {
	ID   string `path:"id"`
	Body struct {
		RoleIDs []string `json:"role_ids" required:"false"`
	}
}

// setRoles handles PATCH /admin/users/{id}/roles.
func (h *AdminRestService) setRoles(ctx context.Context, in *setRolesInput) (*apihuma.SingleOutput[userIDResponse], error) {
	grantedBy := identity.UserIDFrom(ctx)
	roleIDs := in.Body.RoleIDs
	if roleIDs == nil {
		roleIDs = []string{}
	}

	if err := h.next.SetRoles(ctx, in.ID, roleIDs, grantedBy); err != nil {
		return nil, apihuma.MapError(err)
	}

	out := &apihuma.SingleOutput[userIDResponse]{}
	out.Body = response.Response[userIDResponse]{Data: userIDResponse{ID: in.ID}}
	return out, nil
}

// statusResponse is the response body for PATCH /admin/users/{id}/status.
type statusResponse struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

// setStatusInput is the Huma input for PATCH /admin/users/{id}/status.
// Status is NOT marked required:"false" — unlike setRoles, a missing status
// must still 400 before the repo runs. The pre-Huma handler special-cased
// this with a custom {"error":{"code":"REQUIRED",...}} response; no
// existing test asserts that exact body (only the 400 status and that the
// repo is never invoked), so leaving Status Huma-required and letting the
// framework's own field-list validation reject an absent value — which
// happens before this handler function is ever called — is an accepted
// shape trade-off, same precedent as Slice 10's setAllActive.
type setStatusInput struct {
	ID   string `path:"id"`
	Body struct {
		Status string `json:"status"`
	}
}

// setStatus handles PATCH /admin/users/{id}/status.
func (h *AdminRestService) setStatus(ctx context.Context, in *setStatusInput) (*apihuma.SingleOutput[statusResponse], error) {
	if err := h.next.SetStatus(ctx, in.ID, in.Body.Status); err != nil {
		return nil, apihuma.MapError(err)
	}

	out := &apihuma.SingleOutput[statusResponse]{}
	out.Body = response.Response[statusResponse]{Data: statusResponse{ID: in.ID, Status: in.Body.Status}}
	return out, nil
}

// adminDeleteOutput has no Body field, so Huma defaults its status to 204
// (see huma's own op.DefaultStatus inference: no output body -> StatusNoContent).
type adminDeleteOutput struct{}

// delete handles DELETE /admin/users/{id}.
func (h *AdminRestService) delete(ctx context.Context, in *adminGetInput) (*adminDeleteOutput, error) {
	if err := h.next.AdminDelete(ctx, in.ID); err != nil {
		return nil, apihuma.MapError(err)
	}
	return &adminDeleteOutput{}, nil
}
