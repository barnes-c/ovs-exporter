package collector

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/barnes-c/ovs-exporter/internal/ovsdb/ovsmodel"
)

func init() {
	registerCollector("ovs-interface-info", DefaultDisabled, newOVSInterfaceInfoCollector)
}

// ovsInterfaceInfoCollector exposes per-interface metadata: admin/link
// state, MTU, link speed, OpenFlow port number, ifindex, and ingress
// policing limits. Plus an `ovs.interface.info` gauge=1 carrying the
// string-valued attributes (type, MAC, duplex).
type ovsInterfaceInfoCollector struct {
	log *slog.Logger
	src DataSource

	adminState        metric.Int64ObservableGauge
	linkState         metric.Int64ObservableGauge
	mtu               metric.Int64ObservableGauge
	linkSpeed         metric.Int64ObservableGauge
	ifIndex           metric.Int64ObservableGauge
	ofPort            metric.Int64ObservableGauge
	policingRate      metric.Int64ObservableGauge
	policingBurst     metric.Int64ObservableGauge
	policingKpktsRate metric.Int64ObservableGauge
	policingKpktsBur  metric.Int64ObservableGauge
	info              metric.Int64ObservableGauge

	registration metric.Registration
}

func newOVSInterfaceInfoCollector(log *slog.Logger) (Collector, error) {
	return &ovsInterfaceInfoCollector{log: log}, nil
}

func (c *ovsInterfaceInfoCollector) Name() string { return "ovs-interface-info" }

func (c *ovsInterfaceInfoCollector) Register(meter metric.Meter, src DataSource) error {
	c.src = src

	var err error
	if c.adminState, err = meter.Int64ObservableGauge(
		"ovs.interface.admin_state",
		metric.WithDescription("OVS interface administrative state (1=up, 0=down)."),
	); err != nil {
		return err
	}
	if c.linkState, err = meter.Int64ObservableGauge(
		"ovs.interface.link_state",
		metric.WithDescription("OVS interface link state (1=up, 0=down)."),
	); err != nil {
		return err
	}
	if c.mtu, err = meter.Int64ObservableGauge(
		"ovs.interface.mtu",
		metric.WithDescription("OVS interface MTU."),
		metric.WithUnit("By"),
	); err != nil {
		return err
	}
	if c.linkSpeed, err = meter.Int64ObservableGauge(
		"ovs.interface.link_speed",
		metric.WithDescription("OVS interface negotiated link speed."),
		metric.WithUnit("bit/s"),
	); err != nil {
		return err
	}
	if c.ifIndex, err = meter.Int64ObservableGauge(
		"ovs.interface.if_index",
		metric.WithDescription("OVS interface kernel ifindex."),
	); err != nil {
		return err
	}
	if c.ofPort, err = meter.Int64ObservableGauge(
		"ovs.interface.of_port",
		metric.WithDescription("OVS interface OpenFlow port number."),
	); err != nil {
		return err
	}
	if c.policingRate, err = meter.Int64ObservableGauge(
		"ovs.interface.ingress_policing.rate",
		metric.WithDescription("OVS interface ingress policing rate."),
		metric.WithUnit("kbit/s"),
	); err != nil {
		return err
	}
	if c.policingBurst, err = meter.Int64ObservableGauge(
		"ovs.interface.ingress_policing.burst",
		metric.WithDescription("OVS interface ingress policing burst size."),
		metric.WithUnit("kbit"),
	); err != nil {
		return err
	}
	if c.policingKpktsRate, err = meter.Int64ObservableGauge(
		"ovs.interface.ingress_policing.kpkts_rate",
		metric.WithDescription("OVS interface ingress policing packet rate."),
		metric.WithUnit("{kpacket}/s"),
	); err != nil {
		return err
	}
	if c.policingKpktsBur, err = meter.Int64ObservableGauge(
		"ovs.interface.ingress_policing.kpkts_burst",
		metric.WithDescription("OVS interface ingress policing packet burst."),
		metric.WithUnit("{kpacket}"),
	); err != nil {
		return err
	}
	if c.info, err = meter.Int64ObservableGauge(
		"ovs.interface.info",
		metric.WithDescription("OVS interface metadata gauge (always 1); carries string attributes type, mac, duplex."),
	); err != nil {
		return err
	}

	c.registration, err = meter.RegisterCallback(c.observe,
		c.adminState, c.linkState, c.mtu, c.linkSpeed,
		c.ifIndex, c.ofPort,
		c.policingRate, c.policingBurst, c.policingKpktsRate, c.policingKpktsBur,
		c.info)
	return err
}

func (c *ovsInterfaceInfoCollector) observe(_ context.Context, o metric.Observer) error {
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

func (c *ovsInterfaceInfoCollector) observeInterface(o metric.Observer, bridge string, iface *ovsmodel.Interface) {
	base := metric.WithAttributes(
		attribute.String("bridge", bridge),
		attribute.String("interface", iface.Name),
	)

	if iface.AdminState != nil {
		v := int64(0)
		if *iface.AdminState == ovsmodel.InterfaceAdminStateUp {
			v = 1
		}
		o.ObserveInt64(c.adminState, v, base)
	}
	if iface.LinkState != nil {
		v := int64(0)
		if *iface.LinkState == ovsmodel.InterfaceLinkStateUp {
			v = 1
		}
		o.ObserveInt64(c.linkState, v, base)
	}
	if iface.MTU != nil {
		o.ObserveInt64(c.mtu, int64(*iface.MTU), base)
	}
	if iface.LinkSpeed != nil {
		o.ObserveInt64(c.linkSpeed, int64(*iface.LinkSpeed), base)
	}
	if iface.Ifindex != nil {
		o.ObserveInt64(c.ifIndex, int64(*iface.Ifindex), base)
	}
	if iface.Ofport != nil {
		o.ObserveInt64(c.ofPort, int64(*iface.Ofport), base)
	}
	o.ObserveInt64(c.policingRate, int64(iface.IngressPolicingRate), base)
	o.ObserveInt64(c.policingBurst, int64(iface.IngressPolicingBurst), base)
	o.ObserveInt64(c.policingKpktsRate, int64(iface.IngressPolicingKpktsRate), base)
	o.ObserveInt64(c.policingKpktsBur, int64(iface.IngressPolicingKpktsBurst), base)

	infoAttrs := []attribute.KeyValue{
		attribute.String("bridge", bridge),
		attribute.String("interface", iface.Name),
		attribute.String("type", iface.Type),
	}
	if iface.MAC != nil {
		infoAttrs = append(infoAttrs, attribute.String("mac", *iface.MAC))
	}
	if iface.Duplex != nil {
		infoAttrs = append(infoAttrs, attribute.String("duplex", *iface.Duplex))
	}
	o.ObserveInt64(c.info, 1, metric.WithAttributes(infoAttrs...))
}

func (c *ovsInterfaceInfoCollector) Close() error {
	if c.registration == nil {
		return nil
	}
	return c.registration.Unregister()
}
