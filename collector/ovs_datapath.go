package collector

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

func init() {
	registerCollector("ovs-datapath", DefaultEnabled, newOVSDatapathCollector)
}

// ovsDatapathCollector exposes the per-datapath lookup counters from
// `ovs-appctl dpif/show`.
//
// The original plan also called for `ovs.datapath.flows` (current flow
// count) and `ovs.datapath.masks.hit`, but those fields aren't in
// dpif/show — they belong to `dpctl/show` output. They'll land as a
// separate collector backed by a second appctl call once the
// scrape-orchestrator wiring supports multi-method snapshots.
//
// Per-port topology lines from the same dpif/show response are routed
// to the opt-in `--collector.ovs-datapath-interfaces` collector in T13
// because their cardinality scales with port count.
type ovsDatapathCollector struct {
	log *slog.Logger
	src DataSource

	lookups metric.Int64ObservableCounter

	registration metric.Registration
}

func newOVSDatapathCollector(log *slog.Logger) (Collector, error) {
	return &ovsDatapathCollector{log: log}, nil
}

func (c *ovsDatapathCollector) Name() string { return "ovs-datapath" }

func (c *ovsDatapathCollector) Register(meter metric.Meter, src DataSource) error {
	c.src = src

	var err error
	c.lookups, err = meter.Int64ObservableCounter(
		"ovs.datapath.lookups",
		metric.WithDescription("Cumulative datapath flow lookups, partitioned by outcome (hit, missed, lost)."),
		metric.WithUnit("{lookup}"),
	)
	if err != nil {
		return err
	}

	c.registration, err = meter.RegisterCallback(c.observe, c.lookups)
	return err
}

func (c *ovsDatapathCollector) observe(_ context.Context, o metric.Observer) error {
	snap := c.src.UnixCtlOVS()
	if snap == nil || snap.DPIF == nil {
		return nil
	}
	for name, d := range snap.DPIF.Datapaths {
		o.ObserveInt64(c.lookups, d.Lookups.Hit,
			metric.WithAttributes(
				attribute.String("datapath", name),
				attribute.String("result", "hit"),
			))
		o.ObserveInt64(c.lookups, d.Lookups.Missed,
			metric.WithAttributes(
				attribute.String("datapath", name),
				attribute.String("result", "missed"),
			))
		o.ObserveInt64(c.lookups, d.Lookups.Lost,
			metric.WithAttributes(
				attribute.String("datapath", name),
				attribute.String("result", "lost"),
			))
	}
	return nil
}

func (c *ovsDatapathCollector) Close() error {
	if c.registration == nil {
		return nil
	}
	return c.registration.Unregister()
}
