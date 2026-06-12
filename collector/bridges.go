package collector

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/barnes-c/ovs-exporter/internal/ovsdb/ovsmodel"
)

func init() {
	registerCollector("bridges", DefaultEnabled, newOVSBridgesCollector, OVSViewAvailable)
}

type ovsBridgesCollector struct {
	registrar
	log *slog.Logger
	src DataSource

	bridgesCount metric.Int64ObservableGauge
	portsCount   metric.Int64ObservableGauge
}

func newOVSBridgesCollector(log *slog.Logger) (Collector, error) {
	return &ovsBridgesCollector{log: log}, nil
}

func (c *ovsBridgesCollector) Name() string { return "bridges" }

func (c *ovsBridgesCollector) Register(meter metric.Meter, src DataSource) error {
	c.src = src

	var err error
	c.bridgesCount, err = meter.Int64ObservableGauge(
		"ovs.bridges.count",
		metric.WithDescription("Number of OVS bridges configured on this host."),
		metric.WithUnit("{bridge}"),
	)
	if err != nil {
		return err
	}

	c.portsCount, err = meter.Int64ObservableGauge(
		"ovs.bridge.ports.count",
		metric.WithDescription("Number of ports attached to an OVS bridge."),
		metric.WithUnit("{port}"),
	)
	if err != nil {
		return err
	}

	c.registration, err = meter.RegisterCallback(c.observe, c.bridgesCount, c.portsCount)
	return err
}

func (c *ovsBridgesCollector) observe(_ context.Context, o metric.Observer) error {
	view := c.src.OVS()
	if view == nil {
		return nil
	}
	var total int64
	view.Bridges(func(b *ovsmodel.Bridge) {
		total++
		o.ObserveInt64(c.portsCount, int64(len(b.Ports)),
			metric.WithAttributes(attribute.String("bridge", b.Name)))
	})
	o.ObserveInt64(c.bridgesCount, total)
	return nil
}
