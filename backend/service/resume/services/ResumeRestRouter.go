package services

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
)

// RestRouter registers the resume routes via Huma.
type RestRouter struct {
	handler *RestService
	zone    huma.Config
}

// NewRestRouter constructs a RestRouter. zone is the shared /api
// middleware-zone Huma config (see apihuma.NewZoneConfig / apispec.APIZoneConfig).
func NewRestRouter(h *RestService, zone huma.Config) *RestRouter {
	return &RestRouter{handler: h, zone: zone}
}

// Register mounts the resume routes onto r.
func (rr *RestRouter) Register(r chi.Router) {
	api := humachi.New(r, rr.zone)

	huma.Register(api, huma.Operation{
		OperationID: "listResumes", Method: http.MethodGet, Path: "/resumes",
		Summary: "List the current user's resumes", Tags: []string{"resumes"},
	}, rr.handler.list)
	huma.Register(api, huma.Operation{
		OperationID: "uploadResume", Method: http.MethodPost, Path: "/resumes",
		Summary: "Upload a resume (PDF or DOCX)", Tags: []string{"resumes"},
		DefaultStatus: http.StatusCreated,
	}, rr.handler.upload)
	huma.Register(api, huma.Operation{
		OperationID: "downloadResume", Method: http.MethodGet, Path: "/resumes/{id}/download",
		Summary: "Download a resume's original file", Tags: []string{"resumes"},
	}, rr.handler.download)
	huma.Register(api, huma.Operation{
		OperationID: "renameResume", Method: http.MethodPatch, Path: "/resumes/{id}",
		Summary: "Rename a resume", Tags: []string{"resumes"},
	}, rr.handler.rename)
	huma.Register(api, huma.Operation{
		OperationID: "deleteResume", Method: http.MethodDelete, Path: "/resumes/{id}",
		Summary: "Delete a resume", Tags: []string{"resumes"},
	}, rr.handler.remove)
}
