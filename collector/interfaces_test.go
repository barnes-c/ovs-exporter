package collector

import (
	"context"
	"sort"
	"strings"
	"testing"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/barnes-c/ovs-exporter/internal/ovsdb/ovsmodel"
)

func TestOVSInterfaces_Observes(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	c, err := newOVSInterfacesCollector(discardLogger())
	if err != nil {
		t.Fatalf("newOVSInterfacesCollector: %v", err)
	}

	src := &fakeDataSource{
		ovs: &fakeOVSView{
			bridges: []*ovsmodel.Bridge{
				{Name: "br-int", Ports: []string{"port-a", "port-b"}},
			},
			ports: []*ovsmodel.Port{
				{UUID: "port-a", Name: "p-a", Interfaces: []string{"if-a"}},
				{UUID: "port-b", Name: "p-b", Interfaces: []string{"if-b"}},
			},
			interfaces: []*ovsmodel.Interface{
				{UUID: "if-a", Name: "eth0", Statistics: map[string]int{
					"rx_bytes":   1000,
					"rx_packets": 10,
					"tx_bytes":   2000,
					"tx_packets": 20,
					"rx_errors":  1,
					"tx_errors":  2,
					"rx_dropped": 3,
					"tx_dropped": 4,
					"collisions": 5,
				}},
				{UUID: "if-b", Name: "eth1", Statistics: map[string]int{
					"rx_bytes":   500,
					"rx_packets": 5,
				}},
			},
		},
	}
	if err := c.Register(mp.Meter("test"), src); err != nil {
		t.Fatalf("Register: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	got := collectInt64Counters(t, reader)
	want := map[string]int64{
		"ovs.interface.rx.bytes{bridge=br-int,interface=eth0}":            1000,
		"ovs.interface.rx.packets{bridge=br-int,interface=eth0}":          10,
		"ovs.interface.tx.bytes{bridge=br-int,interface=eth0}":            2000,
		"ovs.interface.tx.packets{bridge=br-int,interface=eth0}":          20,
		"ovs.interface.collisions{bridge=br-int,interface=eth0}":          5,
		"ovs.interface.errors{bridge=br-int,direction=rx,interface=eth0}": 1,
		"ovs.interface.errors{bridge=br-int,direction=tx,interface=eth0}": 2,
		"ovs.interface.drops{bridge=br-int,direction=rx,interface=eth0}":  3,
		"ovs.interface.drops{bridge=br-int,direction=tx,interface=eth0}":  4,
		"ovs.interface.rx.bytes{bridge=br-int,interface=eth1}":            500,
		"ovs.interface.rx.packets{bridge=br-int,interface=eth1}":          5,
		"ovs.interface.tx.bytes{bridge=br-int,interface=eth1}":            0,
		"ovs.interface.tx.packets{bridge=br-int,interface=eth1}":          0,
		"ovs.interface.collisions{bridge=br-int,interface=eth1}":          0,
		"ovs.interface.errors{bridge=br-int,direction=rx,interface=eth1}": 0,
		"ovs.interface.errors{bridge=br-int,direction=tx,interface=eth1}": 0,
		"ovs.interface.drops{bridge=br-int,direction=rx,interface=eth1}":  0,
		"ovs.interface.drops{bridge=br-int,direction=tx,interface=eth1}":  0,
	}
	for k, v := range want {
		if g, ok := got[k]; !ok {
			t.Errorf("missing metric %q (got: %v)", k, got)
		} else if g != v {
			t.Errorf("metric %q = %d, want %d", k, g, v)
		}
	}
}

func TestOVSInterfaces_NilView(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	c, err := newOVSInterfacesCollector(discardLogger())
	if err != nil {
		t.Fatalf("newOVSInterfacesCollector: %v", err)
	}

	src := &fakeDataSource{ovs: nil}
	if err := c.Register(mp.Meter("test"), src); err != nil {
		t.Fatalf("Register: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	got := collectInt64Counters(t, reader)
	if len(got) != 0 {
		t.Errorf("expected no data points when OVS view is nil, got %v", got)
	}
}

// TestOVSInterfaces_DanglingRefs verifies the collector tolerates a Bridge
// pointing at a port UUID not present in the Port table (and similarly for
// Port→Interface). The libovsdb cache can briefly be in this state during
// monitor updates.
func TestOVSInterfaces_DanglingRefs(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	c, err := newOVSInterfacesCollector(discardLogger())
	if err != nil {
		t.Fatalf("newOVSInterfacesCollector: %v", err)
	}

	src := &fakeDataSource{
		ovs: &fakeOVSView{
			bridges: []*ovsmodel.Bridge{
				{Name: "br-int", Ports: []string{"missing-port"}},
			},
		},
	}
	if err := c.Register(mp.Meter("test"), src); err != nil {
		t.Fatalf("Register: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	if got := collectInt64Counters(t, reader); len(got) != 0 {
		t.Errorf("expected no data points for dangling refs, got %v", got)
	}
}

// collectInt64Counters triggers Collect on the reader and flattens int64 Sum
// metric points into a map of "metric{attr=value,...}" → int64. Attributes
// are sorted by key so the keys are deterministic regardless of
// AttributeSet iteration order.
func collectInt64Counters(t *testing.T, r *sdkmetric.ManualReader) map[string]int64 {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := r.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	out := make(map[string]int64)
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			s, ok := m.Data.(metricdata.Sum[int64])
			if !ok {
				continue
			}
			for _, dp := range s.DataPoints {
				key := m.Name
				if dp.Attributes.Len() > 0 {
					parts := make([]string, 0, dp.Attributes.Len())
					for _, kv := range dp.Attributes.ToSlice() {
						parts = append(parts, string(kv.Key)+"="+kv.Value.AsString())
					}
					sort.Strings(parts)
					key = m.Name + "{" + strings.Join(parts, ",") + "}"
				}
				out[key] = dp.Value
			}
		}
	}
	return out
}
