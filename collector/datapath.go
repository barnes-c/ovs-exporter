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

// ovsDatapathCollector exposes per-datapath lookup, flow, and mask
// counters. Lookup totals come from `ovs-appctl dpif/show` (terse
// inline summary at column 0); the current flow count and mask-cache
// hits come from `ovs-appctl dpctl/show`. The two sources are scraped
// together by the orchestrator, so if dpctl/show is unavailable the
// flows/masks gauges simply drop out and lookups continue.
//
// Per-port topology lines from the same dpif/show response are routed
// to the opt-in `--collector.ovs-datapath-interfaces` collector because
// their cardinality scales with port count.
type ovsDatapathCollector struct {
	log *slog.Logger
	src DataSource

	lookups  metric.Int64ObservableCounter
	flows    metric.Int64ObservableGauge
	masksHit metric.Int64ObservableCounter
	cacheHit metric.Int64ObservableCounter

	registration metric.Registration
}

func newOVSDatapathCollector(log *slog.Logger) (Collector, error) {
	return &ovsDatapathCollector{log: log}, nil
}

func (c *ovsDatapathCollector) Name() string { return "ovs-datapath" }

func (c *ovsDatapathCollector) Register(meter metric.Meter, src DataSource) error {
	c.src = src

	var err error
	if c.lookups, err = meter.Int64ObservableCounter(
		"ovs.datapath.lookups",
		metric.WithDescription("Cumulative datapath flow lookups, partitioned by outcome (hit, missed, lost)."),
		metric.WithUnit("{lookup}"),
	); err != nil {
		return err
	}
	if c.flows, err = meter.Int64ObservableGauge(
		"ovs.datapath.flows",
		metric.WithDescription("Current number of flows installed in the datapath."),
		metric.WithUnit("{flow}"),
	); err != nil {
		return err
	}
	if c.masksHit, err = meter.Int64ObservableCounter(
		"ovs.datapath.masks.hit",
		metric.WithDescription("Cumulative datapath mask-cache hits."),
		metric.WithUnit("{hit}"),
	); err != nil {
		return err
	}
	if c.cacheHit, err = meter.Int64ObservableCounter(
		"ovs.datapath.cache.hit",
		metric.WithDescription("Cumulative datapath megaflow-cache hits (the cache that sits in front of the exact-match cache)."),
		metric.WithUnit("{hit}"),
	); err != nil {
		return err
	}

	c.registration, err = meter.RegisterCallback(c.observe, c.lookups, c.flows, c.masksHit, c.cacheHit)
	return err
}

func (c *ovsDatapathCollector) observe(_ context.Context, o metric.Observer) error {
	snap := c.src.UnixCtlOVS()
	if snap == nil {
		return nil
	}
	if snap.DPIF != nil {
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
	}
	if snap.DPCTL != nil {
		for name, d := range snap.DPCTL.Datapaths {
			dpAttr := metric.WithAttributes(attribute.String("datapath", name))
			o.ObserveInt64(c.flows, d.Flows, dpAttr)
			o.ObserveInt64(c.masksHit, d.MasksHit, dpAttr)
			o.ObserveInt64(c.cacheHit, d.CacheHit, dpAttr)
		}
	}
	return nil
}

func (c *ovsDatapathCollector) Close() error {
	if c.registration == nil {
		return nil
	}
	return c.registration.Unregister()
}
