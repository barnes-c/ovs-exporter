package collector

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/metric"

	"github.com/barnes-c/ovs-exporter/internal/ovsdb/ovsmodel"
)

func init() {
	registerCollector("ovs-ports", DefaultEnabled, newOVSPortsCollector)
}

// ovsPortsCollector exposes the total number of OVS ports on the host.
// Per-bridge port counts are emitted by ovs_bridges; per-port metadata
// (VLAN tag, bond status, RSTP state) lives behind opt-in cardinality
// flags in M1-T13.
type ovsPortsCollector struct {
	log *slog.Logger
	src DataSource

	count metric.Int64ObservableGauge

	registration metric.Registration
}

func newOVSPortsCollector(log *slog.Logger) (Collector, error) {
	return &ovsPortsCollector{log: log}, nil
}

func (c *ovsPortsCollector) Name() string { return "ovs-ports" }

func (c *ovsPortsCollector) Register(meter metric.Meter, src DataSource) error {
	c.src = src

	var err error
	c.count, err = meter.Int64ObservableGauge(
		"ovs.ports.count",
		metric.WithDescription("Total number of OVS ports across all bridges on this host."),
		metric.WithUnit("{port}"),
	)
	if err != nil {
		return err
	}

	c.registration, err = meter.RegisterCallback(c.observe, c.count)
	return err
}

func (c *ovsPortsCollector) observe(_ context.Context, o metric.Observer) error {
	view := c.src.OVS()
	if view == nil {
		return nil
	}
	var total int64
	view.Ports(func(*ovsmodel.Port) { total++ })
	o.ObserveInt64(c.count, total)
	return nil
}

func (c *ovsPortsCollector) Close() error {
	if c.registration == nil {
		return nil
	}
	return c.registration.Unregister()
}
