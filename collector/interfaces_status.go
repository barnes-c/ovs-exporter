package collector

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/barnes-c/ovs-exporter/internal/ovsdb/ovsmodel"
)

func init() {
	registerCollector("ovs-interface-status", DefaultDisabled, newOVSInterfaceStatusCollector)
}

// ovsInterfaceStatusCollector exposes the contents of the OVS Interface
// table's (status, options, external_ids) as info gauges (value=1)
// carrying the key and value in label attributes.
//
// This is the most cardinality-heavy default-off collector by far. The
// value goes in a label, so every distinct value creates a new series:
//
//   - Interface.status holds driver-reported runtime fields. On a tunnel
//     port `tunnel_egress_iface` changes whenever the underlay route flips.
//   - Interface.options holds tunnel parameters including `remote_ip`,
//     which on an OVN cluster is per-chassis.
//   - Interface.external_ids holds orchestrator-applied UUIDs and tags;
//     k8s and OpenStack both stamp ports with their own object IDs.
type ovsInterfaceStatusCollector struct {
	log *slog.Logger
	src DataSource

	status      metric.Int64ObservableGauge
	options     metric.Int64ObservableGauge
	externalIDs metric.Int64ObservableGauge

	registration metric.Registration
}

func newOVSInterfaceStatusCollector(log *slog.Logger) (Collector, error) {
	return &ovsInterfaceStatusCollector{log: log}, nil
}

func (c *ovsInterfaceStatusCollector) Name() string { return "ovs-interface-status" }

func (c *ovsInterfaceStatusCollector) Register(meter metric.Meter, src DataSource) error {
	c.src = src

	var err error
	if c.status, err = meter.Int64ObservableGauge(
		"ovs.interface.status",
		metric.WithDescription("k/v pairs from Interface.status (driver-reported runtime fields). HIGH CARDINALITY: value is a label."),
	); err != nil {
		return err
	}
	if c.options, err = meter.Int64ObservableGauge(
		"ovs.interface.options",
		metric.WithDescription("k/v pairs from Interface.options (tunnel parameters, port-specific config). HIGH CARDINALITY: value is a label."),
	); err != nil {
		return err
	}
	if c.externalIDs, err = meter.Int64ObservableGauge(
		"ovs.interface.external_ids",
		metric.WithDescription("k/v pairs from Interface.external_ids (orchestrator-applied annotations). HIGH CARDINALITY: value is a label."),
	); err != nil {
		return err
	}

	c.registration, err = meter.RegisterCallback(c.observe, c.status, c.options, c.externalIDs)
	return err
}

func (c *ovsInterfaceStatusCollector) observe(_ context.Context, o metric.Observer) error {
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
				c.emitMap(o, c.status, b.Name, iface.Name, iface.Status)
				c.emitMap(o, c.options, b.Name, iface.Name, iface.Options)
				c.emitMap(o, c.externalIDs, b.Name, iface.Name, iface.ExternalIDs)
			}
		}
	})
	return nil
}

func (c *ovsInterfaceStatusCollector) emitMap(
	o metric.Observer,
	inst metric.Int64ObservableGauge,
	bridge, iface string,
	kv map[string]string,
) {
	for k, v := range kv {
		o.ObserveInt64(inst, 1,
			metric.WithAttributes(
				attribute.String("bridge", bridge),
				attribute.String("interface", iface),
				attribute.String("key", k),
				attribute.String("value", v),
			))
	}
}

func (c *ovsInterfaceStatusCollector) Close() error {
	if c.registration == nil {
		return nil
	}
	return c.registration.Unregister()
}
