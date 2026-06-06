package otel

import (
	"fmt"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	otelprom "go.opentelemetry.io/otel/exporters/prometheus"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

// newPromReader constructs the OTel SDK Prometheus exporter and the matching
// HTTP handler. The exporter is a sdkmetric.Reader attached to the
// MeterProvider; the handler is what /metrics serves.
//
// Target-info and scope-info series are disabled so the output matches
// classic Prometheus exporter conventions and stays portable from the
// greenpau/CERN forks.
func newPromReader(maxRequests int) (sdkmetric.Reader, http.Handler, error) {
	reg := prometheus.NewRegistry()
	exp, err := otelprom.New(
		otelprom.WithRegisterer(reg),
		otelprom.WithoutScopeInfo(),
		otelprom.WithoutTargetInfo(),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("creating Prometheus OTel exporter: %w", err)
	}

	handler := promhttp.HandlerFor(reg, promhttp.HandlerOpts{
		ErrorHandling:       promhttp.ContinueOnError,
		MaxRequestsInFlight: maxRequests,
	})
	return exp, handler, nil
}
