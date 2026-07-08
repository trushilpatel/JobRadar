package services

import (
	"context"

	"github.com/danielgtaylor/huma/v2"
	"github.com/trushilpatel/jobradar/apps/api/apihuma"
	"github.com/trushilpatel/jobradar/apps/api/request"
	"github.com/trushilpatel/jobradar/apps/api/response"
	companydto "github.com/trushilpatel/jobradar/service/company/dto"
)

// AdminRestService is the HTTP adapter for the admin company chain. It
// depends only on CompanyAdminService; validation and cache invalidation
// are handled by the decorator chain.
type AdminRestService struct {
	next CompanyAdminService
}

// NewAdminRestService constructs the admin RestService.
func NewAdminRestService(next CompanyAdminService) *AdminRestService {
	return &AdminRestService{next: next}
}

// adminListInput is the Huma input for GET /admin/companies. Active,
// HasSettings, and Limit are read as raw strings (not native types) and
// resolved in Resolve to reproduce the pre-Huma handler's exact parsing
// quirks: a bool query param sets the pointer to true only when the raw
// value is literally "true" (any other non-empty value, including "false",
// sets false); an absent param leaves the pointer nil. AdapterTypes prefers
// the repeated adapter_types param and falls back to the singular
// adapter_type only when adapter_types is entirely absent.
type adminListInput struct {
	AdapterTypes []string `query:"adapter_types,explode" doc:"Repeatable ATS adapter type filter"`
	AdapterType  string   `query:"adapter_type" doc:"Singular adapter type filter, used when adapter_types is absent"`
	Active       string   `query:"active" doc:"true or false"`
	HasSettings  string   `query:"has_settings" doc:"true or false"`
	Search       string   `query:"search" doc:"ILIKE match on company name"`
	SortBy       string   `query:"sort_by"`
	SortDir      string   `query:"sort_dir" doc:"asc or desc"`
	ScanStatus   string   `query:"scan_status" doc:"ok, error, queued, or running"`
	Cursor       string   `query:"cursor" doc:"Opaque keyset pagination cursor"`
	Limit        string   `query:"limit" doc:"Page size 1-100; invalid or missing values default to 20" example:"20"`

	filter companydto.CompanyFilter
	limit  int
}

// Resolve builds the CompanyFilter and clamped limit after Huma parses the
// raw query strings.
func (in *adminListInput) Resolve(_ huma.Context) []error {
	in.filter = companydto.CompanyFilter{
		Search:     in.Search,
		SortBy:     in.SortBy,
		SortDir:    in.SortDir,
		ScanStatus: in.ScanStatus,
		Cursor:     in.Cursor,
	}
	if len(in.AdapterTypes) > 0 {
		in.filter.AdapterTypes = in.AdapterTypes
	} else if in.AdapterType != "" {
		in.filter.AdapterTypes = []string{in.AdapterType}
	}
	if in.Active != "" {
		active := in.Active == "true"
		in.filter.Active = &active
	}
	if in.HasSettings != "" {
		hasSettings := in.HasSettings == "true"
		in.filter.HasSettings = &hasSettings
	}
	in.limit = request.ParseLimit(in.Limit, request.DefaultPagination().Limit)
	return nil
}

// list handles GET /admin/companies.
func (h *AdminRestService) list(ctx context.Context, in *adminListInput) (*apihuma.AdminCursorPageOutput[companydto.CompanyAdminListView], error) {
	result, err := h.next.List(ctx, in.filter, in.limit)
	if err != nil {
		return nil, apihuma.MapError(err)
	}

	out := &apihuma.AdminCursorPageOutput[companydto.CompanyAdminListView]{}
	out.Body = response.Response[apihuma.AdminCursorPage[companydto.CompanyAdminListView]]{
		Data: apihuma.AdminCursorPage[companydto.CompanyAdminListView]{
			Data:        result.Items,
			Total:       result.Total,
			TotalActive: result.TotalActive,
			Limit:       in.limit,
			NextCursor:  result.NextCursor,
		},
	}
	return out, nil
}

// adminGetInput is the Huma input for GET/PATCH/DELETE /admin/companies/{id}
// and PATCH /admin/companies/{id}/scan-settings.
type adminGetInput struct {
	ID string `path:"id"`
}

// get handles GET /admin/companies/{id}.
func (h *AdminRestService) get(ctx context.Context, in *adminGetInput) (*apihuma.SingleOutput[*companydto.CompanyAdminView], error) {
	view, err := h.next.Get(ctx, in.ID)
	if err != nil {
		return nil, apihuma.MapError(err)
	}

	out := &apihuma.SingleOutput[*companydto.CompanyAdminView]{}
	out.Body = response.Response[*companydto.CompanyAdminView]{Data: view}
	return out, nil
}

// adminCreateRequest is the POST /admin/companies body envelope. Public
// fields and scan_settings travel together so admins can create + onboard
// in one round trip. required:"false" throughout: Create's required-field
// enforcement (id/name/slug non-empty) happens downstream in
// AdminValidationService, exactly like the pre-Huma handler's plain
// json.Decode — an absent key must reach validation as a zero value, never
// fail at the framework layer.
type adminCreateRequest struct {
	companydto.CompanyPublicInput
	ScanSettings *companydto.CompanyScanSettingsInput `json:"scan_settings" required:"false"`
}

// createInput is the Huma input for POST /admin/companies.
type createInput struct {
	Body adminCreateRequest
}

// create handles POST /admin/companies.
func (h *AdminRestService) create(ctx context.Context, in *createInput) (*apihuma.SingleOutput[*companydto.CompanyAdminView], error) {
	settings := companydto.CompanyScanSettingsInput{}
	if in.Body.ScanSettings != nil {
		settings = *in.Body.ScanSettings
	}

	view, err := h.next.Create(ctx, in.Body.CompanyPublicInput, settings)
	if err != nil {
		return nil, apihuma.MapError(err)
	}

	out := &apihuma.SingleOutput[*companydto.CompanyAdminView]{}
	out.Body = response.Response[*companydto.CompanyAdminView]{Data: view}
	return out, nil
}

// updatePublicInput is the Huma input for PATCH /admin/companies/{id}.
type updatePublicInput struct {
	ID   string `path:"id"`
	Body companydto.CompanyPublicInput
}

// companyIDResponse is the response body for a partial-patch endpoint that only
// echoes back the ID of the row it touched.
type companyIDResponse struct {
	ID string `json:"id"`
}

// updatePublic handles PATCH /admin/companies/{id}.
func (h *AdminRestService) updatePublic(ctx context.Context, in *updatePublicInput) (*apihuma.SingleOutput[companyIDResponse], error) {
	if err := h.next.UpdatePublic(ctx, in.ID, in.Body); err != nil {
		return nil, apihuma.MapError(err)
	}

	out := &apihuma.SingleOutput[companyIDResponse]{}
	out.Body = response.Response[companyIDResponse]{Data: companyIDResponse{ID: in.ID}}
	return out, nil
}

// updateScanSettingsInput is the Huma input for PATCH /admin/companies/{id}/scan-settings.
type updateScanSettingsInput struct {
	ID   string `path:"id"`
	Body companydto.CompanyScanSettingsInput
}

// updateScanSettings handles PATCH /admin/companies/{id}/scan-settings.
func (h *AdminRestService) updateScanSettings(ctx context.Context, in *updateScanSettingsInput) (*apihuma.SingleOutput[companyIDResponse], error) {
	if err := h.next.UpdateScanSettings(ctx, in.ID, in.Body); err != nil {
		return nil, apihuma.MapError(err)
	}

	out := &apihuma.SingleOutput[companyIDResponse]{}
	out.Body = response.Response[companyIDResponse]{Data: companyIDResponse{ID: in.ID}}
	return out, nil
}

// deleteOutput has no Body field, so Huma defaults its status to 204 (see
// huma's own op.DefaultStatus inference: no output body -> StatusNoContent).
type deleteOutput struct{}

// delete handles DELETE /admin/companies/{id}.
func (h *AdminRestService) delete(ctx context.Context, in *adminGetInput) (*deleteOutput, error) {
	if err := h.next.Delete(ctx, in.ID); err != nil {
		return nil, apihuma.MapError(err)
	}
	return &deleteOutput{}, nil
}

// setAllActiveInput is the Huma input for PATCH /admin/companies/active.
// Active is NOT marked required:"false" — unlike every other body field in
// this migration, a missing active value must still 400. The pre-Huma
// handler special-cased this with a custom {"error":{"code":"REQUIRED",...}}
// response; no existing test asserts that exact body (only the 400 status),
// so leaving Active Huma-required and letting the framework's own
// field-list validation reject an absent value is an accepted shape
// trade-off — same precedent as Slice 5's multipart upload validation.
type setAllActiveInput struct {
	Body struct {
		Active bool `json:"active"`
	}
}

// activeResponse is the response body for PATCH /admin/companies/active.
type activeResponse struct {
	Active bool `json:"active"`
}

// setAllActive handles PATCH /admin/companies/active.
func (h *AdminRestService) setAllActive(ctx context.Context, in *setAllActiveInput) (*apihuma.SingleOutput[activeResponse], error) {
	if err := h.next.SetAllActive(ctx, in.Body.Active); err != nil {
		return nil, apihuma.MapError(err)
	}

	out := &apihuma.SingleOutput[activeResponse]{}
	out.Body = response.Response[activeResponse]{Data: activeResponse{Active: in.Body.Active}}
	return out, nil
}
