package collector

import (
	"context"
	"testing"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"

	"github.com/barnes-c/ovs-exporter/internal/ovsdb/ovsmodel"
)

func TestOVSInterfaceStatus_Observes(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	c, err := newOVSInterfaceStatusCollector(discardLogger())
	if err != nil {
		t.Fatalf("newOVSInterfaceStatusCollector: %v", err)
	}

	src := &fakeDataSource{
		ovs: &fakeOVSView{
			bridges: []*ovsmodel.Bridge{
				{Name: "br-int", Ports: []string{"port-a"}},
			},
			ports: []*ovsmodel.Port{
				{UUID: "port-a", Interfaces: []string{"if-a"}},
			},
			interfaces: []*ovsmodel.Interface{
				{
					UUID:        "if-a",
					Name:        "geneve_sys_6081",
					Type:        "geneve",
					Status:      map[string]string{"tunnel_egress_iface": "eth0"},
					Options:     map[string]string{"remote_ip": "192.0.2.21", "key": "flow"},
					ExternalIDs: map[string]string{"iface-id": "00000000-0000-0000-0000-000000000042"},
				},
			},
		},
	}
	if err := c.Register(mp.Meter("test"), src); err != nil {
		t.Fatalf("Register: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	got := collectInt64Gauges(t, reader)
	want := map[string]int64{
		"ovs.interface.status{bridge=br-int,interface=geneve_sys_6081,key=tunnel_egress_iface,value=eth0}":                            1,
		"ovs.interface.options{bridge=br-int,interface=geneve_sys_6081,key=remote_ip,value=192.0.2.21}":                               1,
		"ovs.interface.options{bridge=br-int,interface=geneve_sys_6081,key=key,value=flow}":                                           1,
		"ovs.interface.external_ids{bridge=br-int,interface=geneve_sys_6081,key=iface-id,value=00000000-0000-0000-0000-000000000042}": 1,
	}
	for k, v := range want {
		if g, ok := got[k]; !ok {
			t.Errorf("missing %q (got: %v)", k, got)
		} else if g != v {
			t.Errorf("%q = %d, want %d", k, g, v)
		}
	}
}

func TestOVSInterfaceStatus_EmptyMapsEmitNothing(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	c, _ := newOVSInterfaceStatusCollector(discardLogger())
	src := &fakeDataSource{
		ovs: &fakeOVSView{
			bridges: []*ovsmodel.Bridge{
				{Name: "br-int", Ports: []string{"port-a"}},
			},
			ports: []*ovsmodel.Port{
				{UUID: "port-a", Interfaces: []string{"if-a"}},
			},
			interfaces: []*ovsmodel.Interface{
				{UUID: "if-a", Name: "eth0"},
			},
		},
	}
	if err := c.Register(mp.Meter("test"), src); err != nil {
		t.Fatalf("Register: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	if got := collectInt64Gauges(t, reader); len(got) != 0 {
		t.Errorf("expected no data points for empty maps, got %v", got)
	}
}
