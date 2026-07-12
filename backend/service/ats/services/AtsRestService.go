package services

import (
	"context"

	"github.com/trushilpatel/jobradar/apps/api/apihuma"
	"github.com/trushilpatel/jobradar/apps/api/response"
	atsdto "github.com/trushilpatel/jobradar/service/ats/dto"
)

// AtsRestService is the HTTP adapter for the ats domain. It owns request
// decoding and response shaping for the three /ats endpoints; routing is
// the responsibility of AtsRestRouter.
//
// AtsRestService does NOT implement AtsService — it consumes one. Per the
// slice convention, the next field is the only dependency it holds.
type AtsRestService struct {
	next AtsService
}

// NewRestService constructs the HTTP adapter.
func NewRestService(next AtsService) *AtsRestService {
	if next == nil {
		panic("ats.NewRestService: next must not be nil")
	}
	return &AtsRestService{next: next}
}

// getInput is the Huma input for GET/PATCH /ats/{name}.
type getInput struct {
	Name string `path:"name"`
}

// updateInput is the Huma input for PATCH /ats/{name}.
type updateInput struct {
	Name string `path:"name"`
	Body atsdto.UpdateATSInput
}

// list handles GET /ats — returns all ATS rows.
func (h *AtsRestService) list(ctx context.Context, _ *struct{}) (*apihuma.SingleOutput[[]atsdto.ATSDTO], error) {
	items, err := h.next.ListATS(ctx)
	if err != nil {
		return nil, apihuma.MapError(err)
	}

	out := &apihuma.SingleOutput[[]atsdto.ATSDTO]{}
	out.Body = response.Response[[]atsdto.ATSDTO]{Data: items}
	return out, nil
}

// get handles GET /ats/{name} — returns a single ATS row by name.
func (h *AtsRestService) get(ctx context.Context, in *getInput) (*apihuma.SingleOutput[atsdto.ATSDTO], error) {
	item, err := h.next.GetATS(ctx, in.Name)
	if err != nil {
		return nil, apihuma.MapError(err)
	}

	out := &apihuma.SingleOutput[atsdto.ATSDTO]{}
	out.Body = response.Response[atsdto.ATSDTO]{Data: item}
	return out, nil
}

// update handles PATCH /ats/{name} — applies a partial update to an ATS row.
func (h *AtsRestService) update(ctx context.Context, in *updateInput) (*apihuma.SingleOutput[atsdto.ATSDTO], error) {
	item, err := h.next.UpdateATS(ctx, in.Name, in.Body)
	if err != nil {
		return nil, apihuma.MapError(err)
	}

	out := &apihuma.SingleOutput[atsdto.ATSDTO]{}
	out.Body = response.Response[atsdto.ATSDTO]{Data: item}
	return out, nil
}
