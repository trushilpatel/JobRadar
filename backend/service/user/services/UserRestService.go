package services

import (
	"encoding/json"
	"net/http"

	"github.com/trushilpatel/jobradar/apps/api/handler"
	"github.com/trushilpatel/jobradar/common/identity"
	userdto "github.com/trushilpatel/jobradar/service/user/dto"
)

// RestService is the HTTP adapter for the user domain. It owns request
// decoding and response shaping for the three /api/me endpoints; routing is
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

// updateMeRequest is the JSON body for PATCH /api/me. Only display_name is
// editable in v1; future fields land here.
type updateMeRequest struct {
	DisplayName string `json:"display_name"`
}

// getMe returns the caller's full identity bundle. The RequireIdentity
// middleware is responsible for attaching the bundle; calling MustFromContext
// here is safe because routes that never run behind that middleware should
// not be wired to this handler.
func (h *RestService) getMe(w http.ResponseWriter, r *http.Request) {
	id := identity.MustFromContext(r.Context())
	roles := append([]string(nil), id.Roles...)
	if roles == nil {
		roles = []string{}
	}
	handler.Respond(w, http.StatusOK, meResponse{
		UserID:      id.UserID,
		Email:       id.Email,
		Roles:       roles,
		Status:      id.Status,
		DisplayName: id.DisplayName,
	})
}

// updateMe applies a profile change to the caller's own row. The caller's
// user id always comes from the ctx-bound Identity — never the request body
// — so an attacker cannot pivot a PATCH at someone else's row by tampering
// with JSON.
func (h *RestService) updateMe(w http.ResponseWriter, r *http.Request) {
	id := identity.MustFromContext(r.Context())

	var req updateMeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		handler.RespondError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}
	if err := h.next.UpdateProfile(r.Context(), id.UserID, req.DisplayName); err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, map[string]string{
		"user_id":      id.UserID,
		"display_name": req.DisplayName,
	})
}

// deleteMe hard-deletes the caller's account. The impl service handles the
// Supabase admin call before the DB delete; we never invoke either directly.
func (h *RestService) deleteMe(w http.ResponseWriter, r *http.Request) {
	id := identity.MustFromContext(r.Context())
	if err := h.next.Delete(r.Context(), id.UserID); err != nil {
		handler.HandleError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// getMyConfig returns the caller's UserConfig. New users without a
// user_preferences row receive the zero-value defaults (all booleans false).
func (h *RestService) getMyConfig(w http.ResponseWriter, r *http.Request) {
	id := identity.MustFromContext(r.Context())
	cfg, err := h.next.GetConfig(r.Context(), id.UserID)
	if err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, cfg)
}

// updateMyConfig merges a partial UserConfig into the caller's stored config.
// The user_id always comes from the ctx-bound Identity — never the request
// body — so an attacker cannot target another user's preferences.
func (h *RestService) updateMyConfig(w http.ResponseWriter, r *http.Request) {
	id := identity.MustFromContext(r.Context())
	var partial userdto.UserConfig
	if err := json.NewDecoder(r.Body).Decode(&partial); err != nil {
		handler.RespondError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid request body")
		return
	}
	cfg, err := h.next.PatchConfig(r.Context(), id.UserID, partial)
	if err != nil {
		handler.HandleError(w, err)
		return
	}
	handler.Respond(w, http.StatusOK, cfg)
}
