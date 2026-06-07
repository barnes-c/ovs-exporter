package probes

import (
	"context"
	"fmt"
	"net/http"
	"sort"
)

// Checker reports whether a single readiness dependency is currently
// usable. A nil error means ready; any non-nil error fails the overall
// readyz response and its Error() text is rendered in the response body
// for debugging.
type Checker interface {
	Check(ctx context.Context) error
}

// CheckerFunc adapts an ordinary function to the Checker interface so
// callers can pass closures without declaring a type.
type CheckerFunc func(ctx context.Context) error

// Check implements Checker.
func (f CheckerFunc) Check(ctx context.Context) error { return f(ctx) }

// Ready returns the readiness probe handler. It runs every supplied
// Checker on each request, using the request context so cancellation
// (and HTTP timeouts) propagate. The response is 200 only when every
// check passes; otherwise 503. Checks are emitted in alphabetical order
// so the response body is stable across requests.
//
// With no checks the handler always returns 200 — useful when the
// exporter is configured to scrape nothing.
func Ready(checks map[string]Checker) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		names := make([]string, 0, len(checks))
		for name := range checks {
			names = append(names, name)
		}
		sort.Strings(names)

		results := make(map[string]string, len(checks))
		allOK := true
		for _, name := range names {
			if err := checks[name].Check(r.Context()); err != nil {
				results[name] = err.Error()
				allOK = false
				continue
			}
			results[name] = "ok"
		}

		status := http.StatusOK
		if !allOK {
			status = http.StatusServiceUnavailable
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(status)
		for _, name := range names {
			_, _ = fmt.Fprintf(w, "%s: %s\n", name, results[name])
		}
	})
}
