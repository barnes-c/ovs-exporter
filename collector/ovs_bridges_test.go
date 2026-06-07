package collector

import (
	"context"
	"strings"
	"testing"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/barnes-c/ovs-exporter/internal/ovsdb/ovsmodel"
)

// fakeOVSView yields a fixed bridge set; other methods are no-ops.
type fakeOVSView struct {
	bridges []*ovsmodel.Bridge
}

func (f *fakeOVSView) Bridges(fn func(*ovsmodel.Bridge)) {
	for _, b := range f.bridges {
		fn(b)
	}
}
func (f *fakeOVSView) Ports(func(*ovsmodel.Port))           {}
func (f *fakeOVSView) Interfaces(func(*ovsmodel.Interface)) {}
func (f *fakeOVSView) OpenvSwitch() *ovsmodel.OpenvSwitch   { return nil }

// fakeDataSource implements DataSource for collector tests.
type fakeDataSource struct {
	ovs OVSView
}

func (f *fakeDataSource) OVS() OVSView                   { return f.ovs }
func (f *fakeDataSource) OVNNB() OVNNBView               { return nil }
func (f *fakeDataSource) OVNSB() OVNSBView               { return nil }
func (f *fakeDataSource) UnixCtlOVS() UnixCtlSnapshot    { return nil }
func (f *fakeDataSource) UnixCtlNorthd() UnixCtlSnapshot { return nil }

func TestOVSBridges_Observes(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	c, err := newOVSBridgesCollector(discardLogger())
	if err != nil {
		t.Fatalf("newOVSBridgesCollector: %v", err)
	}

	src := &fakeDataSource{
		ovs: &fakeOVSView{
			bridges: []*ovsmodel.Bridge{
				{Name: "br-int", Ports: []string{"p1", "p2", "p3"}},
				{Name: "br-ex", Ports: []string{"q1"}},
			},
		},
	}
	if err := c.Register(mp.Meter("test"), src); err != nil {
		t.Fatalf("Register: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	got := collectInt64Gauges(t, reader)
	want := map[string]int64{
		"ovs.bridges.count":                     2,
		"ovs.bridge.ports.count{bridge=br-int}": 3,
		"ovs.bridge.ports.count{bridge=br-ex}":  1,
	}
	for k, v := range want {
		if g, ok := got[k]; !ok {
			t.Errorf("missing metric %q (got: %v)", k, got)
		} else if g != v {
			t.Errorf("metric %q = %d, want %d", k, g, v)
		}
	}
}

func TestOVSBridges_NilView(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	c, err := newOVSBridgesCollector(discardLogger())
	if err != nil {
		t.Fatalf("newOVSBridgesCollector: %v", err)
	}

	src := &fakeDataSource{ovs: nil}
	if err := c.Register(mp.Meter("test"), src); err != nil {
		t.Fatalf("Register: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	got := collectInt64Gauges(t, reader)
	if len(got) != 0 {
		t.Errorf("expected no data points when OVS view is nil, got %v", got)
	}
}

// collectInt64Gauges triggers Collect on the reader and flattens the result
// into a map of "metric{attr=value,...}" → int64. Only Int64 gauges are
// captured.
func collectInt64Gauges(t *testing.T, r *sdkmetric.ManualReader) map[string]int64 {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := r.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	out := make(map[string]int64)
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			g, ok := m.Data.(metricdata.Gauge[int64])
			if !ok {
				continue
			}
			for _, dp := range g.DataPoints {
				key := m.Name
				if dp.Attributes.Len() > 0 {
					parts := make([]string, 0, dp.Attributes.Len())
					for _, kv := range dp.Attributes.ToSlice() {
						parts = append(parts, string(kv.Key)+"="+kv.Value.AsString())
					}
					key = m.Name + "{" + strings.Join(parts, ",") + "}"
				}
				out[key] = dp.Value
			}
		}
	}
	return out
}
