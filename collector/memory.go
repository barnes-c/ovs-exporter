package collector

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/barnes-c/ovs-exporter/internal/unixctl"
)

func init() {
	registerCollector("memory", DefaultEnabled, newOVSMemoryCollector,
		UnixctlHas(func(s *unixctl.OVSSnapshot) bool { return s.Memory != nil }))
}

// ovsMemoryCollector exposes the per-resource counts reported by
// `ovs-appctl memory/show` (handlers, ofconns, ports, rules, revalidators, ...).
type ovsMemoryCollector struct {
	registrar
	log *slog.Logger
	src DataSource

	usage metric.Int64ObservableGauge
}

func newOVSMemoryCollector(log *slog.Logger) (Collector, error) {
	return &ovsMemoryCollector{log: log}, nil
}

func (c *ovsMemoryCollector) Name() string { return "memory" }

func (c *ovsMemoryCollector) Register(meter metric.Meter, src DataSource) error {
	c.src = src

	var err error
	c.usage, err = meter.Int64ObservableGauge(
		"ovs.memory.usage",
		metric.WithDescription("OVS in-memory resource usage, partitioned by resource name (handlers, ofconns, ports, rules, revalidators, ...)."),
		metric.WithUnit("{resource}"),
	)
	if err != nil {
		return err
	}

	c.registration, err = meter.RegisterCallback(c.observe, c.usage)
	return err
}

func (c *ovsMemoryCollector) observe(_ context.Context, o metric.Observer) error {
	snap := c.src.UnixCtlOVS()
	if snap == nil || snap.Memory == nil {
		return nil
	}
	for name, n := range snap.Memory.Usage {
		o.ObserveInt64(c.usage, n,
			metric.WithAttributes(attribute.String("resource", name)))
	}
	return nil
}
