package services

import "github.com/go-chi/chi/v5"

type RestRouter struct{ handler *RestService }

func NewRestRouter(h *RestService) *RestRouter { return &RestRouter{handler: h} }

func (rr *RestRouter) Register(r chi.Router) {
	r.Get("/resumes", rr.handler.list)
	r.Post("/resumes", rr.handler.upload)
	r.Get("/resumes/{id}/download", rr.handler.download)
	r.Patch("/resumes/{id}", rr.handler.rename)
	r.Delete("/resumes/{id}", rr.handler.remove)
}
