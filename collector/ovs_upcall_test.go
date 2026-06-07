package collector

import (
	"context"
	"testing"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"

	"github.com/barnes-c/ovs-exporter/internal/unixctl"
)

func TestOVSUpcall_Observes(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	c, err := newOVSUpcallCollector(discardLogger())
	if err != nil {
		t.Fatalf("newOVSUpcallCollector: %v", err)
	}

	src := &fakeDataSource{
		unixctlOVS: &unixctl.OVSSnapshot{
			Upcall: &unixctl.Upcall{Datapaths: map[string]*unixctl.UpcallDatapath{
				"system@ovs-system": {
					Name:           "system@ovs-system",
					FlowsCurrent:   42,
					FlowsMax:       120,
					FlowsLimit:     200000,
					DumpDurationMs: 7,
					HandlerKeys:    map[int]int64{0: 12, 1: 9, 2: 21},
				},
			}},
		},
	}
	if err := c.Register(mp.Meter("test"), src); err != nil {
		t.Fatalf("Register: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	got := collectInt64Gauges(t, reader)
	want := map[string]int64{
		"ovs.upcall.flows.current{datapath=system@ovs-system}":          42,
		"ovs.upcall.flows.max{datapath=system@ovs-system}":              120,
		"ovs.upcall.flows.limit{datapath=system@ovs-system}":            200000,
		"ovs.upcall.dump.duration{datapath=system@ovs-system}":          7,
		"ovs.upcall.handler.keys{datapath=system@ovs-system,handler=0}": 12,
		"ovs.upcall.handler.keys{datapath=system@ovs-system,handler=1}": 9,
		"ovs.upcall.handler.keys{datapath=system@ovs-system,handler=2}": 21,
	}
	for k, v := range want {
		if g, ok := got[k]; !ok {
			t.Errorf("missing %q (got: %v)", k, got)
		} else if g != v {
			t.Errorf("%q = %d, want %d", k, g, v)
		}
	}
}

func TestOVSUpcall_NilSnapshot(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	c, _ := newOVSUpcallCollector(discardLogger())
	src := &fakeDataSource{unixctlOVS: nil}
	if err := c.Register(mp.Meter("test"), src); err != nil {
		t.Fatalf("Register: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	if got := collectInt64Gauges(t, reader); len(got) != 0 {
		t.Errorf("expected no gauges, got %v", got)
	}
}
