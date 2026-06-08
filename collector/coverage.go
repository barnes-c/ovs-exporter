package collector

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

func init() {
	registerCollector("ovs-coverage", DefaultEnabled, newOVSCoverageCollector)
}

// ovsCoverageCollector exposes cumulative event counts from
// `ovs-appctl coverage/show`
type ovsCoverageCollector struct {
	log *slog.Logger
	src DataSource

	events       metric.Int64ObservableCounter
	registration metric.Registration
}

func newOVSCoverageCollector(log *slog.Logger) (Collector, error) {
	return &ovsCoverageCollector{log: log}, nil
}

func (c *ovsCoverageCollector) Name() string { return "ovs-coverage" }

func (c *ovsCoverageCollector) Register(meter metric.Meter, src DataSource) error {
	c.src = src

	var err error
	c.events, err = meter.Int64ObservableCounter(
		"ovs.coverage.events",
		metric.WithDescription("Cumulative count of OVS coverage events, partitioned by event name."),
		metric.WithUnit("{event}"),
	)
	if err != nil {
		return err
	}

	c.registration, err = meter.RegisterCallback(c.observe, c.events)
	return err
}

func (c *ovsCoverageCollector) observe(_ context.Context, o metric.Observer) error {
	snap := c.src.UnixCtlOVS()
	if snap == nil || snap.Coverage == nil {
		return nil
	}
	for name, total := range snap.Coverage.Events {
		o.ObserveInt64(c.events, total,
			metric.WithAttributes(attribute.String("event", name)))
	}
	return nil
}

func (c *ovsCoverageCollector) Close() error {
	if c.registration == nil {
		return nil
	}
	return c.registration.Unregister()
}
