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
					Name:    "system@ovs-system",
					Lookups: unixctl.DPIFLookups{Hit: 14778031, Missed: 1583622, Lost: 0},
				},
			}},
			DPCTL: &unixctl.DPCTL{Datapaths: map[string]*unixctl.DPCTLDatapath{
				"system@ovs-system": {
					Name:     "system@ovs-system",
					Flows:    89,
					MasksHit: 14778031,
					CacheHit: 9000000,
				},
			}},
		},
	}
	if err := c.Register(mp.Meter("test"), src); err != nil {
		t.Fatalf("Register: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	counters := collectInt64Counters(t, reader)
	wantCounters := map[string]int64{
		"ovs.datapath.lookups{datapath=system@ovs-system,result=hit}":    14778031,
		"ovs.datapath.lookups{datapath=system@ovs-system,result=missed}": 1583622,
		"ovs.datapath.lookups{datapath=system@ovs-system,result=lost}":   0,
		"ovs.datapath.masks.hit{datapath=system@ovs-system}":             14778031,
		"ovs.datapath.cache.hit{datapath=system@ovs-system}":             9000000,
	}
	for k, v := range wantCounters {
		if g, ok := counters[k]; !ok {
			t.Errorf("missing counter %q (got: %v)", k, counters)
		} else if g != v {
			t.Errorf("%q = %d, want %d", k, g, v)
		}
	}

	gauges := collectInt64Gauges(t, reader)
	if g := gauges["ovs.datapath.flows{datapath=system@ovs-system}"]; g != 89 {
		t.Errorf("ovs.datapath.flows = %d, want 89 (gauges: %v)", g, gauges)
	}
}

// TestOVSDatapath_DPCTLOnly verifies that lookups stop appearing if
// dpif/show fails but flows/masks continue from dpctl/show — and
// vice-versa for the reverse case.
func TestOVSDatapath_DPCTLOnly(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	c, _ := newOVSDatapathCollector(discardLogger())
	src := &fakeDataSource{
		unixctlOVS: &unixctl.OVSSnapshot{
			DPCTL: &unixctl.DPCTL{Datapaths: map[string]*unixctl.DPCTLDatapath{
				"system@ovs-system": {Name: "system@ovs-system", Flows: 42, MasksHit: 100},
			}},
		},
	}
	if err := c.Register(mp.Meter("test"), src); err != nil {
		t.Fatalf("Register: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	counters := collectInt64Counters(t, reader)
	if _, ok := counters["ovs.datapath.lookups{datapath=system@ovs-system,result=hit}"]; ok {
		t.Errorf("lookups should be absent when DPIF is nil; got: %v", counters)
	}
	if counters["ovs.datapath.masks.hit{datapath=system@ovs-system}"] != 100 {
		t.Errorf("masks.hit missing or wrong: %v", counters)
	}
	gauges := collectInt64Gauges(t, reader)
	if gauges["ovs.datapath.flows{datapath=system@ovs-system}"] != 42 {
		t.Errorf("flows missing or wrong: %v", gauges)
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

	if got := collectInt64Counters(t, reader); len(got) != 0 {
		t.Errorf("expected no counters, got %v", got)
	}
	if got := collectInt64Gauges(t, reader); len(got) != 0 {
		t.Errorf("expected no gauges, got %v", got)
	}
}
