package collector

import (
	"context"
	"log/slog"
	"strconv"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

func init() {
	registerCollector("ovs-upcall", DefaultEnabled, newOVSUpcallCollector)
}

// ovsUpcallCollector exposes the per-datapath upcall / revalidator stats
// reported by `ovs-appctl upcall/show`: current/max/limit flow counts,
// per-tick dump duration, and per-handler key counts.
//
// Dump duration is exposed as int64 milliseconds with unit "ms" rather
// than the OTel-canonical seconds float — keeps the integer-only metric
// path in this exporter consistent and matches what operators read from
// ovs-appctl directly. The "avg" column is dropped (Prometheus rate over
// the current series gives a better moving average than the OVS-internal
// EWMA).
type ovsUpcallCollector struct {
	log *slog.Logger
	src DataSource

	flowsCurrent metric.Int64ObservableGauge
	flowsMax     metric.Int64ObservableGauge
	flowsLimit   metric.Int64ObservableGauge
	dumpDuration metric.Int64ObservableGauge
	handlerKeys  metric.Int64ObservableGauge

	registration metric.Registration
}

func newOVSUpcallCollector(log *slog.Logger) (Collector, error) {
	return &ovsUpcallCollector{log: log}, nil
}

func (c *ovsUpcallCollector) Name() string { return "ovs-upcall" }

func (c *ovsUpcallCollector) Register(meter metric.Meter, src DataSource) error {
	c.src = src

	var err error
	if c.flowsCurrent, err = meter.Int64ObservableGauge(
		"ovs.upcall.flows.current",
		metric.WithDescription("Current number of datapath flows tracked by the upcall handler."),
		metric.WithUnit("{flow}"),
	); err != nil {
		return err
	}
	if c.flowsMax, err = meter.Int64ObservableGauge(
		"ovs.upcall.flows.max",
		metric.WithDescription("Maximum datapath flow count observed by the upcall handler since startup."),
		metric.WithUnit("{flow}"),
	); err != nil {
		return err
	}
	if c.flowsLimit, err = meter.Int64ObservableGauge(
		"ovs.upcall.flows.limit",
		metric.WithDescription("Datapath flow installation limit enforced by the upcall handler."),
		metric.WithUnit("{flow}"),
	); err != nil {
		return err
	}
	if c.dumpDuration, err = meter.Int64ObservableGauge(
		"ovs.upcall.dump.duration",
		metric.WithDescription("Most recent revalidator dump duration."),
		metric.WithUnit("ms"),
	); err != nil {
		return err
	}
	if c.handlerKeys, err = meter.Int64ObservableGauge(
		"ovs.upcall.handler.keys",
		metric.WithDescription("Datapath keys currently held by an upcall handler thread."),
		metric.WithUnit("{key}"),
	); err != nil {
		return err
	}

	c.registration, err = meter.RegisterCallback(c.observe,
		c.flowsCurrent, c.flowsMax, c.flowsLimit, c.dumpDuration, c.handlerKeys)
	return err
}

func (c *ovsUpcallCollector) observe(_ context.Context, o metric.Observer) error {
	snap := c.src.UnixCtlOVS()
	if snap == nil || snap.Upcall == nil {
		return nil
	}
	for name, d := range snap.Upcall.Datapaths {
		dpAttr := metric.WithAttributes(attribute.String("datapath", name))
		o.ObserveInt64(c.flowsCurrent, d.FlowsCurrent, dpAttr)
		o.ObserveInt64(c.flowsMax, d.FlowsMax, dpAttr)
		o.ObserveInt64(c.flowsLimit, d.FlowsLimit, dpAttr)
		o.ObserveInt64(c.dumpDuration, d.DumpDurationMs, dpAttr)
		for id, keys := range d.HandlerKeys {
			o.ObserveInt64(c.handlerKeys, keys,
				metric.WithAttributes(
					attribute.String("datapath", name),
					attribute.String("handler", strconv.Itoa(id)),
				))
		}
	}
	return nil
}

func (c *ovsUpcallCollector) Close() error {
	if c.registration == nil {
		return nil
	}
	return c.registration.Unregister()
}
