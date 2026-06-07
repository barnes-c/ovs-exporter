package collector

import (
	"context"
	"testing"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"

	"github.com/barnes-c/ovs-exporter/internal/unixctl"
)

func TestOVSCoverage_Observes(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	c, err := newOVSCoverageCollector(discardLogger())
	if err != nil {
		t.Fatalf("newOVSCoverageCollector: %v", err)
	}

	src := &fakeDataSource{
		unixctlOVS: &unixctl.OVSSnapshot{
			Coverage: &unixctl.Coverage{
				Events: map[string]int64{
					"flow_extract":          200,
					"xlate_actions":         100,
					"bridge_reconfigure":    2,
					"ofproto_recv_openflow": 5,
				},
			},
		},
	}
	if err := c.Register(mp.Meter("test"), src); err != nil {
		t.Fatalf("Register: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	got := collectInt64Counters(t, reader)
	want := map[string]int64{
		"ovs.coverage.events{event=flow_extract}":          200,
		"ovs.coverage.events{event=xlate_actions}":         100,
		"ovs.coverage.events{event=bridge_reconfigure}":    2,
		"ovs.coverage.events{event=ofproto_recv_openflow}": 5,
	}
	for k, v := range want {
		if g, ok := got[k]; !ok {
			t.Errorf("missing metric %q (got: %v)", k, got)
		} else if g != v {
			t.Errorf("metric %q = %d, want %d", k, g, v)
		}
	}
}

func TestOVSCoverage_NilSnapshot(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	c, err := newOVSCoverageCollector(discardLogger())
	if err != nil {
		t.Fatalf("newOVSCoverageCollector: %v", err)
	}

	src := &fakeDataSource{unixctlOVS: nil}
	if err := c.Register(mp.Meter("test"), src); err != nil {
		t.Fatalf("Register: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	if got := collectInt64Counters(t, reader); len(got) != 0 {
		t.Errorf("expected no data points when snapshot is nil, got %v", got)
	}
}

func TestOVSCoverage_NilCoverageField(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	c, err := newOVSCoverageCollector(discardLogger())
	if err != nil {
		t.Fatalf("newOVSCoverageCollector: %v", err)
	}

	// Snapshot present but Coverage parser hasn't populated yet.
	src := &fakeDataSource{unixctlOVS: &unixctl.OVSSnapshot{}}
	if err := c.Register(mp.Meter("test"), src); err != nil {
		t.Fatalf("Register: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	if got := collectInt64Counters(t, reader); len(got) != 0 {
		t.Errorf("expected no data points when Coverage is nil, got %v", got)
	}
}
