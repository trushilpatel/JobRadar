package main

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/nats-io/nats.go"
)

// natsConnStatus is the narrow subset of *nats.Conn that readyzHandler needs.
// Keeping it unexported and minimal lets tests inject a lightweight stub
// without spinning up a real NATS server, while *nats.Conn satisfies it in prod.
type natsConnStatus interface {
	Status() nats.Status
}

// healthzHandler is the orchestrator liveness endpoint. Cheap, dependency-free —
// only reports that the process is alive and serving HTTP.
// Liveness must stay cheap: transient DB/NATS slowdowns must never kill a healthy pod.
func healthzHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// readyzHandler checks NATS connectivity. Readiness is gated on NATS (the
// orchestrator publishes crawl results there) but NOT on Supabase. A slow DB
// causes 500s on /node/work which crawler-nodes handle via retry. An unreachable
// NATS means no results can be published — all crawls stall.
//
// nc == nil (nil interface, not a nil *nats.Conn wrapped in an interface) is
// treated as not connected → 503.
func readyzHandler(nc natsConnStatus) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		if nc == nil || nc.Status() != nats.CONNECTED {
			http.Error(w, "nats not connected", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}
}

// registerHealthRoutes wires /healthz and /readyz onto r.
// Extracting registration prevents recurrence of the incident where
// healthzHandler existed but r.Get("/healthz", ...) was absent from main.go,
// causing 96 pod restarts over 4 days.
//
// A nil *nats.Conn is explicitly converted to a nil natsConnStatus so that
// readyzHandler's nc == nil guard fires correctly (a non-nil interface holding
// a nil pointer would bypass the guard and panic on nc.Status()).
func registerHealthRoutes(r chi.Router, nc *nats.Conn) {
	r.Get("/healthz", healthzHandler)
	var ns natsConnStatus
	if nc != nil {
		ns = nc
	}
	r.Get("/readyz", readyzHandler(ns))
}
