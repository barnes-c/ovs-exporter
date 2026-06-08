package collector

import (
	"context"
	"testing"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"

	"github.com/barnes-c/ovs-exporter/internal/unixctl"
)

func TestOVSDatapathInterfaces_Observes(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	c, err := newOVSDatapathInterfacesCollector(discardLogger())
	if err != nil {
		t.Fatalf("newOVSDatapathInterfacesCollector: %v", err)
	}

	src := &fakeDataSource{
		unixctlOVS: &unixctl.OVSSnapshot{
			DPIF: &unixctl.DPIF{Datapaths: map[string]*unixctl.DPIFDatapath{
				"system@ovs-system": {
					Name: "system@ovs-system",
					Bridges: map[string]*unixctl.DPIFBridge{
						"br-int": {
							Name: "br-int",
							Ports: []unixctl.DPIFPort{
								{Name: "br-int", PortNo: 65534, OFPortNo: "2", Type: "internal"},
								{Name: "eth0", PortNo: 1, OFPortNo: "3", Type: "system"},
								{Name: "patch-foo", PortNo: 29, OFPortNo: "none", Type: "patch"},
							},
						},
					},
				},
			}},
		},
	}
	if err := c.Register(mp.Meter("test"), src); err != nil {
		t.Fatalf("Register: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	got := collectInt64Gauges(t, reader)
	want := map[string]int64{
		"ovs.datapath.interface.info{bridge=br-int,datapath=system@ovs-system,of_port=2,port=br-int,port_no=65534,type=internal}": 1,
		"ovs.datapath.interface.info{bridge=br-int,datapath=system@ovs-system,of_port=3,port=eth0,port_no=1,type=system}":         1,
		"ovs.datapath.interface.info{bridge=br-int,datapath=system@ovs-system,of_port=none,port=patch-foo,port_no=29,type=patch}": 1,
	}
	for k, v := range want {
		if g, ok := got[k]; !ok {
			t.Errorf("missing %q (got: %v)", k, got)
		} else if g != v {
			t.Errorf("%q = %d, want %d", k, g, v)
		}
	}
}

func TestOVSDatapathInterfaces_NilSnapshot(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	c, _ := newOVSDatapathInterfacesCollector(discardLogger())
	src := &fakeDataSource{unixctlOVS: nil}
	if err := c.Register(mp.Meter("test"), src); err != nil {
		t.Fatalf("Register: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	if got := collectInt64Gauges(t, reader); len(got) != 0 {
		t.Errorf("expected no data points, got %v", got)
	}
}
