//go:build integration

// Package integration is an end-to-end black-box test of the exporter
// against a real ovs-vswitchd, running in the smoke docker-compose
// stack. The tests do not start or tear down the stack — `make
// test-integration` brings it up first and leaves it up after, so
// iterating on a failing test doesn't pay rebuild cost every run.
//
// Gated by the `integration` build tag so `go test ./...` keeps running
// only unit + race tests.
package integration

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
)

// Built once at startup with the legacy validation scheme — our metrics
// use only ASCII alphanumeric + underscore names. v0.68 of
// prometheus/common requires the TextParser to be constructed with an
// explicit ValidationScheme; the zero value panics. The package-level
// model.NameValidationScheme has no effect on TextParser instances.
var textParser = expfmt.NewTextParser(model.LegacyValidation)

const (
	exporterURL = "http://localhost:10054"
	// Matches --cache.ttl in examples/smoke/docker-compose.yml. Unixctl
	// metrics (datapath, upcall, memory updates beyond idl-cells) need
	// at least one TTL cycle after a mutation.
	cacheTTL = 5 * time.Second

	// collectorPromURL exposes the OTel Collector's prometheus exporter,
	// which re-publishes the metrics it received over OTLP from the
	// exporter. Used to assert the metrics pipeline delivered specific
	// OVS metric names end-to-end.
	collectorPromURL = "http://localhost:8889"
	// collectorTelemetryURL is the Collector's own self-telemetry. We
	// read its `otelcol_receiver_accepted_{spans,log_records}` counters
	// from here — the prometheus exporter only handles metric points,
	// so for traces and logs the receiver counter is what we have.
	collectorTelemetryURL = "http://localhost:8888"
)

func TestIntegration_ProbesReachable(t *testing.T) {
	for _, path := range []string{"/healthz", "/readyz"} {
		t.Run(path, func(t *testing.T) {
			resp, err := http.Get(exporterURL + path)
			if err != nil {
				t.Fatalf("GET %s: %v", path, err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				t.Errorf("GET %s = %d, want 200. Body:\n%s", path, resp.StatusCode, body)
			}
		})
	}
}

func TestIntegration_DefaultCollectorsRegistered(t *testing.T) {
	// Mix of always-emit (bridges/ports — libovsdb monitor pushes
	// immediately) and scrape-dependent (memory/coverage — wait for the
	// first unixctl tick + a couple of ovs-vswitchd housekeeping events
	// like poll_create_node / seq_change to register).
	wantPresent := []string{
		"ovs_bridges_count",
		"ovs_ports_count",
		"ovs_coverage_events_total",
		"ovs_memory_usage",
	}
	eventuallyHasMetrics(t, wantPresent, cacheTTL+5*time.Second)
}

func TestIntegration_AddBridgeIncrementsCount(t *testing.T) {
	const brName = "br-integ-count"
	t.Cleanup(func() { _ = ovsVsctl("del-br", brName) })

	before := gaugeValue(t, scrapeAndParse(t), "ovs_bridges_count")

	if err := ovsVsctl("add-br", brName); err != nil {
		t.Fatalf("ovs-vsctl add-br: %v", err)
	}

	// ovs_bridges_count is driven by libovsdb's monitor cache (push
	// updates from ovsdb-server) — refreshes almost immediately.
	// Still allow a short beat for the JSON-RPC notification to settle.
	time.Sleep(200 * time.Millisecond)

	after := gaugeValue(t, scrapeAndParse(t), "ovs_bridges_count")
	if want := before + 1; after != want {
		t.Errorf("ovs_bridges_count = %v after add-br, want %v", after, want)
	}
}

func TestIntegration_AddBridgeLightsUpDatapathAndUpcall(t *testing.T) {
	const brName = "br-integ-dp"
	t.Cleanup(func() { _ = ovsVsctl("del-br", brName) })

	if err := ovsVsctl("add-br", brName); err != nil {
		t.Fatalf("ovs-vsctl add-br: %v", err)
	}

	// Datapath + upcall metrics come from unixctl scrapes. Poll until
	// the next refresh has picked up the new datapath rather than
	// hard-sleeping a fixed TTL — keeps the test fast on a healthy box
	// and forgiving on a slow one.
	eventuallyHasMetrics(t, []string{
		"ovs_datapath_lookups_total",
		"ovs_upcall_flows_current",
		"ovs_upcall_flows_limit",
	}, cacheTTL+5*time.Second)
}

// TestIntegration_OTel_MetricsPipelineDelivers asserts the metrics OTLP
// push pipeline: ovs-exporter → otel-collector → collector's prometheus
// exporter. Names are the same `ovs_*` set asserted directly off
// /metrics in TestIntegration_DefaultCollectorsRegistered — proving they
// also survive a round-trip through OTLP and the collector's
// prometheusremotewrite-style re-export.
func TestIntegration_OTel_MetricsPipelineDelivers(t *testing.T) {
	wantPresent := []string{
		"ovs_bridges_count",
		"ovs_ports_count",
	}
	// Collector push interval is 1s (see otel-exporter-config.yaml), but
	// the prometheus exporter's first scrape can lag a beat. cacheTTL+5s
	// matches what the direct /metrics check uses and is plenty.
	eventuallyHasMetricsAt(t, collectorPromURL+"/metrics", wantPresent, cacheTTL+5*time.Second)
}

// TestIntegration_OTel_TracesPipelineDelivers asserts spans reach the
// collector. The exporter wraps its HTTP server with otelhttp, so every
// /metrics scrape produces a span. We force a few scrapes, then read the
// collector's self-telemetry to confirm the spans counter advanced.
func TestIntegration_OTel_TracesPipelineDelivers(t *testing.T) {
	for i := 0; i < 5; i++ {
		resp, err := http.Get(exporterURL + "/metrics")
		if err != nil {
			t.Fatalf("forcing scrape: %v", err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
	eventuallyCounterGT(t, collectorTelemetryURL+"/metrics",
		"otelcol_receiver_accepted_spans", 0, 10*time.Second)
}

// TestIntegration_OTel_LogsPipelineDelivers asserts log records reach
// the collector. The exporter's slog handler is tee'd through the OTel
// log pipeline when LogsExporter != "none" (set by the YAML); ovsdb /
// scrape components log on startup and on each scrape, so by the time
// the rest of the suite has run there should already be records in
// flight. Verified via the collector's receiver self-telemetry.
func TestIntegration_OTel_LogsPipelineDelivers(t *testing.T) {
	eventuallyCounterGT(t, collectorTelemetryURL+"/metrics",
		"otelcol_receiver_accepted_log_records", 0, 10*time.Second)
}

// eventuallyHasMetrics polls /metrics every 500ms until every name in
// wants is present, up to timeout. Reports the last seen metric set on
// failure so it's clear what *did* show up.
//
// Why polling instead of a fixed sleep: most metrics appear quickly,
// some take a TTL (unixctl-backed) or a couple of ovs-vswitchd
// housekeeping events (ovs_coverage_events_total). Polling returns as
// soon as the test's preconditions are met, which keeps the suite fast
// on a healthy box without making it brittle on a slow one.
func eventuallyHasMetrics(t *testing.T, wants []string, timeout time.Duration) {
	t.Helper()
	eventuallyHasMetricsAt(t, exporterURL+"/metrics", wants, timeout)
}

// eventuallyHasMetricsAt is the URL-parametric form of eventuallyHasMetrics,
// used to assert metric names on both the exporter's /metrics and the
// OTel Collector's re-export endpoint.
func eventuallyHasMetricsAt(t *testing.T, url string, wants []string, timeout time.Duration) {
	t.Helper()
	const interval = 500 * time.Millisecond
	deadline := time.Now().Add(timeout)
	var (
		missing     []string
		lastMetrics map[string]*dto.MetricFamily
	)
	for {
		lastMetrics = scrapeAndParseAt(t, url)
		missing = missing[:0]
		for _, name := range wants {
			if _, ok := lastMetrics[name]; !ok {
				missing = append(missing, name)
			}
		}
		if len(missing) == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Errorf("after %v, still missing %v from %s; got: %v",
				timeout, missing, url, sortedNames(lastMetrics))
			return
		}
		time.Sleep(interval)
	}
}

// scrapeAndParse GETs /metrics on the exporter and parses with expfmt.
func scrapeAndParse(t *testing.T) map[string]*dto.MetricFamily {
	t.Helper()
	return scrapeAndParseAt(t, exporterURL+"/metrics")
}

// scrapeAndParseAt GETs an arbitrary prometheus text-format endpoint.
func scrapeAndParseAt(t *testing.T, url string) map[string]*dto.MetricFamily {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("scrape %s: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("%s returned %d", url, resp.StatusCode)
	}

	families, err := textParser.TextToMetricFamilies(resp.Body)
	if err != nil {
		t.Fatalf("parse %s: %v", url, err)
	}
	return families
}

// counterSumByPrefix sums every data point of every Counter family whose
// name has the given prefix. The OTel Collector's self-telemetry
// publishes per-receiver / per-transport counters
// (e.g. `otelcol_receiver_accepted_spans_total{receiver=...,transport=...}`),
// and the suffix has historically drifted (`_total` was added when the
// collector migrated to OTel-SDK-based internal metrics). Matching by
// prefix and summing is robust to both.
func counterSumByPrefix(families map[string]*dto.MetricFamily, prefix string) float64 {
	var sum float64
	for name, fam := range families {
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		if fam.GetType() != dto.MetricType_COUNTER {
			continue
		}
		for _, m := range fam.Metric {
			if c := m.GetCounter(); c != nil {
				sum += c.GetValue()
			}
		}
	}
	return sum
}

// eventuallyCounterGT polls a prometheus endpoint until the summed value
// across all metric families whose name has the given prefix exceeds
// threshold, or the timeout expires. Used for OTel Collector self-telemetry
// counters where the exact metric name may vary by collector version.
func eventuallyCounterGT(t *testing.T, url, prefix string, threshold float64, timeout time.Duration) {
	t.Helper()
	const interval = 500 * time.Millisecond
	deadline := time.Now().Add(timeout)
	var last float64
	for {
		last = counterSumByPrefix(scrapeAndParseAt(t, url), prefix)
		if last > threshold {
			return
		}
		if time.Now().After(deadline) {
			t.Errorf("after %v, %s* sum = %v, want > %v", timeout, prefix, last, threshold)
			return
		}
		time.Sleep(interval)
	}
}

// gaugeValue extracts the first data point's value from a Gauge family.
// All our aggregate counts (bridges_count, ports_count) are single-point
// gauges with only the otel_scope attributes.
func gaugeValue(t *testing.T, families map[string]*dto.MetricFamily, name string) float64 {
	t.Helper()
	fam, ok := families[name]
	if !ok {
		t.Fatalf("%s missing from /metrics", name)
	}
	if len(fam.Metric) == 0 {
		t.Fatalf("%s has no data points", name)
	}
	g := fam.Metric[0].GetGauge()
	if g == nil {
		t.Fatalf("%s is not a gauge", name)
	}
	return g.GetValue()
}

func ovsVsctl(args ...string) error {
	runtime := os.Getenv("CONTAINER_RUNTIME")
	if runtime == "" {
		runtime = "docker"
	}
	full := append([]string{"exec", "ovs", "ovs-vsctl"}, args...)
	out, err := exec.Command(runtime, full...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s exec ovs ovs-vsctl %v: %w: %s",
			runtime, args, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func sortedNames(families map[string]*dto.MetricFamily) []string {
	out := make([]string, 0, len(families))
	for name := range families {
		out = append(out, name)
	}
	// Cheap sort: not benchmarking, just want deterministic test output.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1] > out[j]; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out
}
