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
		},
	}
	if err := c.Register(mp.Meter("test"), src); err != nil {
		t.Fatalf("Register: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	counters := collectInt64Counters(t, reader)
	want := map[string]int64{
		"ovs.datapath.lookups{datapath=system@ovs-system,result=hit}":    14778031,
		"ovs.datapath.lookups{datapath=system@ovs-system,result=missed}": 1583622,
		"ovs.datapath.lookups{datapath=system@ovs-system,result=lost}":   0,
	}
	for k, v := range want {
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

	if got := collectInt64Counters(t, reader); len(got) != 0 {
		t.Errorf("expected no counters, got %v", got)
	}
}
