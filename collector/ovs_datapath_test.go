package collector

import (
	"context"
	"testing"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"

	"github.com/barnes-c/ovs-exporter/internal/unixctl"
)

func TestOVSDatapath_Observes(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	c, err := newOVSDatapathCollector(discardLogger())
	if err != nil {
		t.Fatalf("newOVSDatapathCollector: %v", err)
	}

	src := &fakeDataSource{
		unixctlOVS: &unixctl.OVSSnapshot{
			DPIF: &unixctl.DPIF{Datapaths: map[string]*unixctl.DPIFDatapath{
				"system@ovs-system": {
					Name:     "system@ovs-system",
					Lookups:  unixctl.DPIFLookups{Hit: 1234, Missed: 56, Lost: 7},
					Flows:    89,
					MasksHit: 1000,
				},
			}},
		},
	}
	if err := c.Register(mp.Meter("test"), src); err != nil {
		t.Fatalf("Register: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	gauges := collectInt64Gauges(t, reader)
	if g := gauges["ovs.datapath.flows{datapath=system@ovs-system}"]; g != 89 {
		t.Errorf("flows = %d, want 89 (gauges: %v)", g, gauges)
	}

	counters := collectInt64Counters(t, reader)
	wantCounters := map[string]int64{
		"ovs.datapath.lookups{datapath=system@ovs-system,result=hit}":    1234,
		"ovs.datapath.lookups{datapath=system@ovs-system,result=missed}": 56,
		"ovs.datapath.lookups{datapath=system@ovs-system,result=lost}":   7,
		"ovs.datapath.masks.hit{datapath=system@ovs-system}":             1000,
	}
	for k, v := range wantCounters {
		if g, ok := counters[k]; !ok {
			t.Errorf("missing counter %q (got: %v)", k, counters)
		} else if g != v {
			t.Errorf("%q = %d, want %d", k, g, v)
		}
	}
}

func TestOVSDatapath_NilSnapshot(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	c, _ := newOVSDatapathCollector(discardLogger())
	src := &fakeDataSource{unixctlOVS: nil}
	if err := c.Register(mp.Meter("test"), src); err != nil {
		t.Fatalf("Register: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	if got := collectInt64Gauges(t, reader); len(got) != 0 {
		t.Errorf("expected no gauges, got %v", got)
	}
	if got := collectInt64Counters(t, reader); len(got) != 0 {
		t.Errorf("expected no counters, got %v", got)
	}
}
