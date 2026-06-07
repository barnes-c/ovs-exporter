// Package probes provides HTTP handlers for Kubernetes-style /healthz
// (liveness) and /readyz (readiness) probes.
//
// Liveness is a process-level signal: an unhealthy liveness probe
// triggers a restart by the orchestrator, so it must not depend on
// external services that could legitimately be unavailable (a stale
// ovsdb cache should not restart the exporter container).
//
// Readiness is a serving signal: an unhealthy readiness probe takes the
// instance out of rotation but does not restart it. It aggregates a
// caller-supplied set of checks (libovsdb connection, unixctl scrape
// freshness, etc.) and returns 200 only when every check passes.
package probes

import "net/http"

// Health returns the liveness probe handler. It writes 200 OK
// unconditionally — see the package doc for why.
func Health() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok\n"))
	})
}
