package main

import "net/http"

// healthzHandler is the orchestrator liveness endpoint. It reports only that the
// process is alive and serving HTTP — it performs no dependency checks (no DB,
// NATS, or downstream calls) by design.
//
// Liveness must stay cheap: on a resource-constrained node a transient DB or
// NATS slowdown must never make the kubelet kill an otherwise-healthy
// orchestrator. Dependency health is a readiness concern, served separately by
// the domain routes.
func healthzHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}
