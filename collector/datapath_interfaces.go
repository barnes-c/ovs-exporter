package collector

import (
	"context"
	"log/slog"
	"strconv"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

func init() {
	registerCollector("datapath-interfaces", DefaultDisabled, newOVSDatapathInterfacesCollector)
}

// ovsDatapathInterfacesCollector exposes the per-port topology embedded
// in `ovs-appctl dpif/show`: which OF port lives on which bridge on
// which datapath, plus its kernel datapath port number, OF port number, and type.
type ovsDatapathInterfacesCollector struct {
	log *slog.Logger
	src DataSource

	info metric.Int64ObservableGauge

	registration metric.Registration
}

func newOVSDatapathInterfacesCollector(log *slog.Logger) (Collector, error) {
	return &ovsDatapathInterfacesCollector{log: log}, nil
}

func (c *ovsDatapathInterfacesCollector) Name() string { return "datapath-interfaces" }

func (c *ovsDatapathInterfacesCollector) Register(meter metric.Meter, src DataSource) error {
	c.src = src

	var err error
	c.info, err = meter.Int64ObservableGauge(
		"ovs.datapath.interface.info",
		metric.WithDescription("Datapath port topology gauge (always 1); carries datapath, bridge, port name, kernel port number, OF port number, and port type as attributes."),
	)
	if err != nil {
		return err
	}

	c.registration, err = meter.RegisterCallback(c.observe, c.info)
	return err
}

func (c *ovsDatapathInterfacesCollector) observe(_ context.Context, o metric.Observer) error {
	snap := c.src.UnixCtlOVS()
	if snap == nil || snap.DPIF == nil {
		return nil
	}
	for dpName, dp := range snap.DPIF.Datapaths {
		for brName, br := range dp.Bridges {
			for _, port := range br.Ports {
				o.ObserveInt64(c.info, 1,
					metric.WithAttributes(
						attribute.String("datapath", dpName),
						attribute.String("bridge", brName),
						attribute.String("port", port.Name),
						attribute.String("port_no", strconv.FormatInt(port.PortNo, 10)),
						attribute.String("of_port", port.OFPortNo),
						attribute.String("type", port.Type),
					))
			}
		}
	}
	return nil
}

func (c *ovsDatapathInterfacesCollector) Close() error {
	if c.registration == nil {
		return nil
	}
	return c.registration.Unregister()
}
