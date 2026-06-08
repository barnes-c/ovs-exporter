package collector

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/barnes-c/ovs-exporter/internal/ovsdb/ovsmodel"
)

func init() {
	registerCollector("interfaces", DefaultEnabled, newOVSInterfacesCollector)
}

// ovsInterfacesCollector exposes per-interface statistics counters from the
// Open_vSwitch DB Interface.statistics column. Bridge attribution is derived
// by walking Bridge.Ports → Port.Interfaces against the libovsdb cache.
//
// Per-interface metadata (admin/link state, MTU, MAC, etc.) lives in
// interfaces_info.go behind an opt-in flag because of cardinality.
type ovsInterfacesCollector struct {
	log *slog.Logger
	src DataSource

	rxBytes    metric.Int64ObservableCounter
	rxPackets  metric.Int64ObservableCounter
	txBytes    metric.Int64ObservableCounter
	txPackets  metric.Int64ObservableCounter
	errors     metric.Int64ObservableCounter
	drops      metric.Int64ObservableCounter
	collisions metric.Int64ObservableCounter

	registration metric.Registration
}

func newOVSInterfacesCollector(log *slog.Logger) (Collector, error) {
	return &ovsInterfacesCollector{log: log}, nil
}

func (c *ovsInterfacesCollector) Name() string { return "interfaces" }

func (c *ovsInterfacesCollector) Register(meter metric.Meter, src DataSource) error {
	c.src = src

	var err error
	if c.rxBytes, err = meter.Int64ObservableCounter(
		"ovs.interface.rx.bytes",
		metric.WithDescription("Bytes received on an OVS interface."),
		metric.WithUnit("By"),
	); err != nil {
		return err
	}
	if c.rxPackets, err = meter.Int64ObservableCounter(
		"ovs.interface.rx.packets",
		metric.WithDescription("Packets received on an OVS interface."),
		metric.WithUnit("{packet}"),
	); err != nil {
		return err
	}
	if c.txBytes, err = meter.Int64ObservableCounter(
		"ovs.interface.tx.bytes",
		metric.WithDescription("Bytes transmitted on an OVS interface."),
		metric.WithUnit("By"),
	); err != nil {
		return err
	}
	if c.txPackets, err = meter.Int64ObservableCounter(
		"ovs.interface.tx.packets",
		metric.WithDescription("Packets transmitted on an OVS interface."),
		metric.WithUnit("{packet}"),
	); err != nil {
		return err
	}
	if c.errors, err = meter.Int64ObservableCounter(
		"ovs.interface.errors",
		metric.WithDescription("Errors observed on an OVS interface, partitioned by direction."),
		metric.WithUnit("{error}"),
	); err != nil {
		return err
	}
	if c.drops, err = meter.Int64ObservableCounter(
		"ovs.interface.drops",
		metric.WithDescription("Packets dropped on an OVS interface, partitioned by direction."),
		metric.WithUnit("{packet}"),
	); err != nil {
		return err
	}
	if c.collisions, err = meter.Int64ObservableCounter(
		"ovs.interface.collisions",
		metric.WithDescription("Collisions observed on an OVS interface."),
		metric.WithUnit("{collision}"),
	); err != nil {
		return err
	}

	c.registration, err = meter.RegisterCallback(
		c.observe,
		c.rxBytes, c.rxPackets, c.txBytes, c.txPackets,
		c.errors, c.drops, c.collisions,
	)
	return err
}

func (c *ovsInterfacesCollector) observe(_ context.Context, o metric.Observer) error {
	view := c.src.OVS()
	if view == nil {
		return nil
	}

	ports := make(map[string]*ovsmodel.Port)
	view.Ports(func(p *ovsmodel.Port) { ports[p.UUID] = p })

	interfaces := make(map[string]*ovsmodel.Interface)
	view.Interfaces(func(i *ovsmodel.Interface) { interfaces[i.UUID] = i })

	view.Bridges(func(b *ovsmodel.Bridge) {
		for _, portUUID := range b.Ports {
			p, ok := ports[portUUID]
			if !ok {
				continue
			}
			for _, ifUUID := range p.Interfaces {
				iface, ok := interfaces[ifUUID]
				if !ok {
					continue
				}
				c.observeInterface(o, b.Name, iface)
			}
		}
	})
	return nil
}

func (c *ovsInterfacesCollector) observeInterface(o metric.Observer, bridge string, iface *ovsmodel.Interface) {
	stats := iface.Statistics
	base := metric.WithAttributes(
		attribute.String("bridge", bridge),
		attribute.String("interface", iface.Name),
	)
	o.ObserveInt64(c.rxBytes, int64(stats["rx_bytes"]), base)
	o.ObserveInt64(c.rxPackets, int64(stats["rx_packets"]), base)
	o.ObserveInt64(c.txBytes, int64(stats["tx_bytes"]), base)
	o.ObserveInt64(c.txPackets, int64(stats["tx_packets"]), base)
	o.ObserveInt64(c.collisions, int64(stats["collisions"]), base)

	o.ObserveInt64(c.errors, int64(stats["rx_errors"]),
		metric.WithAttributes(
			attribute.String("bridge", bridge),
			attribute.String("interface", iface.Name),
			attribute.String("direction", "rx"),
		))
	o.ObserveInt64(c.errors, int64(stats["tx_errors"]),
		metric.WithAttributes(
			attribute.String("bridge", bridge),
			attribute.String("interface", iface.Name),
			attribute.String("direction", "tx"),
		))
	o.ObserveInt64(c.drops, int64(stats["rx_dropped"]),
		metric.WithAttributes(
			attribute.String("bridge", bridge),
			attribute.String("interface", iface.Name),
			attribute.String("direction", "rx"),
		))
	o.ObserveInt64(c.drops, int64(stats["tx_dropped"]),
		metric.WithAttributes(
			attribute.String("bridge", bridge),
			attribute.String("interface", iface.Name),
			attribute.String("direction", "tx"),
		))
}

func (c *ovsInterfacesCollector) Close() error {
	if c.registration == nil {
		return nil
	}
	return c.registration.Unregister()
}
