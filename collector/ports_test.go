package collector

import (
	"context"
	"testing"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"

	"github.com/barnes-c/ovs-exporter/internal/ovsdb/ovsmodel"
)

func TestOVSPorts_Observes(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	c, err := newOVSPortsCollector(discardLogger())
	if err != nil {
		t.Fatalf("newOVSPortsCollector: %v", err)
	}

	src := &fakeDataSource{
		ovs: &fakeOVSView{
			ports: []*ovsmodel.Port{
				{UUID: "p1", Name: "p-a"},
				{UUID: "p2", Name: "p-b"},
				{UUID: "p3", Name: "p-c"},
				{UUID: "p4", Name: "p-d"},
			},
		},
	}
	if err := c.Register(mp.Meter("test"), src); err != nil {
		t.Fatalf("Register: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	got := collectInt64Gauges(t, reader)
	if g, ok := got["ovs.ports.count"]; !ok {
		t.Errorf("missing ovs.ports.count (got: %v)", got)
	} else if g != 4 {
		t.Errorf("ovs.ports.count = %d, want 4", g)
	}
}

func TestOVSPorts_NilView(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	c, err := newOVSPortsCollector(discardLogger())
	if err != nil {
		t.Fatalf("newOVSPortsCollector: %v", err)
	}
	src := &fakeDataSource{ovs: nil}
	if err := c.Register(mp.Meter("test"), src); err != nil {
		t.Fatalf("Register: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	if got := collectInt64Gauges(t, reader); len(got) != 0 {
		t.Errorf("expected no data points when OVS view is nil, got %v", got)
	}
}
