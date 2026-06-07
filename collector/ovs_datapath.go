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

// ovsDatapathCollector exposes per-datapath flow and lookup stats from
// `ovs-appctl dpif/show`. Per-port lines from the same output are
// intentionally not exposed here — the per-datapath-interface info
// gauges live behind the opt-in --collector.ovs-datapath-interfaces flag
// in T13 due to cardinality.
type ovsDatapathCollector struct {
	log *slog.Logger
	src DataSource

	flows    metric.Int64ObservableGauge
	lookups  metric.Int64ObservableCounter
	masksHit metric.Int64ObservableCounter

	registration metric.Registration
}

func newOVSDatapathCollector(log *slog.Logger) (Collector, error) {
	return &ovsDatapathCollector{log: log}, nil
}

func (c *ovsDatapathCollector) Name() string { return "ovs-datapath" }

func (c *ovsDatapathCollector) Register(meter metric.Meter, src DataSource) error {
	c.src = src

	var err error
	if c.flows, err = meter.Int64ObservableGauge(
		"ovs.datapath.flows",
		metric.WithDescription("Current number of flows installed in an OVS datapath."),
		metric.WithUnit("{flow}"),
	); err != nil {
		return err
	}
	if c.lookups, err = meter.Int64ObservableCounter(
		"ovs.datapath.lookups",
		metric.WithDescription("Cumulative datapath flow lookups, partitioned by outcome (hit, missed, lost)."),
		metric.WithUnit("{lookup}"),
	); err != nil {
		return err
	}
	if c.masksHit, err = meter.Int64ObservableCounter(
		"ovs.datapath.masks.hit",
		metric.WithDescription("Cumulative mask-cache hits on an OVS datapath."),
		metric.WithUnit("{hit}"),
	); err != nil {
		return err
	}

	c.registration, err = meter.RegisterCallback(c.observe, c.flows, c.lookups, c.masksHit)
	return err
}

func (c *ovsDatapathCollector) observe(_ context.Context, o metric.Observer) error {
	snap := c.src.UnixCtlOVS()
	if snap == nil || snap.DPIF == nil {
		return nil
	}
	for name, d := range snap.DPIF.Datapaths {
		dpAttr := metric.WithAttributes(attribute.String("datapath", name))
		o.ObserveInt64(c.flows, d.Flows, dpAttr)
		o.ObserveInt64(c.masksHit, d.MasksHit, dpAttr)
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
