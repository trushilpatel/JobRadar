package handler

import "github.com/go-chi/chi/v5"

// RouteGroup is implemented by every handler.
// Register mounts the handler's routes onto r.
// The handler is responsible for its own prefix — router.go never changes.
type RouteGroup interface {
	Register(r chi.Router)
}
