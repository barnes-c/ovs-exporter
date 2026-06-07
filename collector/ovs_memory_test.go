package collector

import (
	"context"
	"testing"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"

	"github.com/barnes-c/ovs-exporter/internal/unixctl"
)

func TestOVSMemory_Observes(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	c, err := newOVSMemoryCollector(discardLogger())
	if err != nil {
		t.Fatalf("newOVSMemoryCollector: %v", err)
	}

	src := &fakeDataSource{
		unixctlOVS: &unixctl.OVSSnapshot{
			Memory: &unixctl.Memory{Usage: map[string]int64{
				"handlers":     1,
				"ofconns":      1,
				"ports":        5,
				"revalidators": 1,
				"rules":        12,
			}},
		},
	}
	if err := c.Register(mp.Meter("test"), src); err != nil {
		t.Fatalf("Register: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	got := collectInt64Gauges(t, reader)
	want := map[string]int64{
		"ovs.memory.usage{resource=handlers}":     1,
		"ovs.memory.usage{resource=ofconns}":      1,
		"ovs.memory.usage{resource=ports}":        5,
		"ovs.memory.usage{resource=revalidators}": 1,
		"ovs.memory.usage{resource=rules}":        12,
	}
	for k, v := range want {
		if g, ok := got[k]; !ok {
			t.Errorf("missing %q (got: %v)", k, got)
		} else if g != v {
			t.Errorf("%q = %d, want %d", k, g, v)
		}
	}
}

func TestOVSMemory_NilSnapshot(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	c, _ := newOVSMemoryCollector(discardLogger())
	src := &fakeDataSource{unixctlOVS: nil}
	if err := c.Register(mp.Meter("test"), src); err != nil {
		t.Fatalf("Register: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	if got := collectInt64Gauges(t, reader); len(got) != 0 {
		t.Errorf("expected no data points, got %v", got)
	}
}
