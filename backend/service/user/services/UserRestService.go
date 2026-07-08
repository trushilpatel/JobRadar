package services

import (
	"context"

	"github.com/trushilpatel/jobradar/apps/api/apihuma"
	"github.com/trushilpatel/jobradar/common/identity"
	userdto "github.com/trushilpatel/jobradar/service/user/dto"
)

// RestService is the HTTP adapter for the user domain. It owns request
// decoding and response shaping for the five /api/me endpoints; routing is
// the responsibility of UserRestRouter.
//
// The RestService never implements the Service interface — it consumes one.
// Per the slice convention, the next field is the only dependency it holds.
type RestService struct {
	next Service
}

// NewRestService constructs the HTTP adapter.
func NewRestService(next Service) *RestService {
	if next == nil {
		panic("user.NewRestService: next must not be nil")
	}
	return &RestService{next: next}
}

// meResponse is the JSON shape returned by GET /api/me. Mirrors the Identity
// struct closely but is a distinct type so middleware-internal additions to
// Identity never leak through the public wire format by accident.
type meResponse struct {
	UserID      string   `json:"user_id"`
	Email       string   `json:"email"`
	Roles       []string `json:"roles"`
	Status      string   `json:"status"`
	DisplayName string   `json:"display_name"`
}

// getMe returns the caller's full identity bundle. The RequireIdentity
// middleware is responsible for attaching the bundle; calling MustFromContext
// here is safe because routes that never run behind that middleware should
// not be wired to this handler.
func (h *RestService) getMe(ctx context.Context, _ *struct{}) (*apihuma.SingleOutput[meResponse], error) {
	id := identity.MustFromContext(ctx)
	roles := append([]string(nil), id.Roles...)
	if roles == nil {
		roles = []string{}
	}
	out := &apihuma.SingleOutput[meResponse]{}
	out.Body.Data = meResponse{
		UserID:      id.UserID,
		Email:       id.Email,
		Roles:       roles,
		Status:      id.Status,
		DisplayName: id.DisplayName,
	}
	return out, nil
}

// updateMeInput is the Huma input for PATCH /api/me. Only display_name is
// editable in v1; future fields land here.
type updateMeInput struct {
	Body struct {
		// required:"false" matches the pre-Huma handler's plain
		// json.Decode, which leaves an absent key at its Go zero value.
		DisplayName string `json:"display_name" required:"false"`
	}
}

// updateMe applies a profile change to the caller's own row. The caller's
// user id always comes from the ctx-bound Identity — never the request body
// — so an attacker cannot pivot a PATCH at someone else's row by tampering
// with JSON.
func (h *RestService) updateMe(ctx context.Context, in *updateMeInput) (*apihuma.SingleOutput[map[string]string], error) {
	id := identity.MustFromContext(ctx)
	if err := h.next.UpdateProfile(ctx, id.UserID, in.Body.DisplayName); err != nil {
		return nil, apihuma.MapError(err)
	}
	out := &apihuma.SingleOutput[map[string]string]{}
	out.Body.Data = map[string]string{
		"user_id":      id.UserID,
		"display_name": in.Body.DisplayName,
	}
	return out, nil
}

// deleteMeOutput has no Body field, so Huma defaults its status to 204.
type deleteMeOutput struct{}

// deleteMe hard-deletes the caller's account. The impl service handles the
// Clerk admin call before the DB delete; we never invoke either directly.
func (h *RestService) deleteMe(ctx context.Context, _ *struct{}) (*deleteMeOutput, error) {
	id := identity.MustFromContext(ctx)
	if err := h.next.Delete(ctx, id.UserID); err != nil {
		return nil, apihuma.MapError(err)
	}
	return &deleteMeOutput{}, nil
}

// getMyConfig returns the caller's UserConfig. New users without a
// user_preferences row receive the zero-value defaults (all booleans false).
func (h *RestService) getMyConfig(ctx context.Context, _ *struct{}) (*apihuma.SingleOutput[*userdto.UserConfig], error) {
	id := identity.MustFromContext(ctx)
	cfg, err := h.next.GetConfig(ctx, id.UserID)
	if err != nil {
		return nil, apihuma.MapError(err)
	}
	out := &apihuma.SingleOutput[*userdto.UserConfig]{}
	out.Body.Data = cfg
	return out, nil
}

// updateMyConfigInput is the Huma input for PATCH /api/me/config.
type updateMyConfigInput struct {
	Body userdto.UserConfig
}

// updateMyConfig merges a partial UserConfig into the caller's stored config.
// The user_id always comes from the ctx-bound Identity — never the request
// body — so an attacker cannot target another user's preferences.
func (h *RestService) updateMyConfig(ctx context.Context, in *updateMyConfigInput) (*apihuma.SingleOutput[*userdto.UserConfig], error) {
	id := identity.MustFromContext(ctx)
	cfg, err := h.next.PatchConfig(ctx, id.UserID, in.Body)
	if err != nil {
		return nil, apihuma.MapError(err)
	}
	out := &apihuma.SingleOutput[*userdto.UserConfig]{}
	out.Body.Data = cfg
	return out, nil
}
