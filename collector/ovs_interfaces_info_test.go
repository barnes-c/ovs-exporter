package collector

import (
	"context"
	"testing"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"

	"github.com/barnes-c/ovs-exporter/internal/ovsdb/ovsmodel"
)

func TestOVSInterfaceInfo_Observes(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	c, err := newOVSInterfaceInfoCollector(discardLogger())
	if err != nil {
		t.Fatalf("newOVSInterfaceInfoCollector: %v", err)
	}

	adminUp := ovsmodel.InterfaceAdminStateUp
	linkUp := ovsmodel.InterfaceLinkStateUp
	mtu := 1500
	linkSpeed := 10_000_000_000
	ifIndex := 7
	ofPort := 3
	mac := "00:11:22:33:44:55"
	duplex := ovsmodel.InterfaceDuplexFull

	src := &fakeDataSource{
		ovs: &fakeOVSView{
			bridges: []*ovsmodel.Bridge{
				{Name: "br-int", Ports: []string{"port-a"}},
			},
			ports: []*ovsmodel.Port{
				{UUID: "port-a", Name: "p-a", Interfaces: []string{"if-a"}},
			},
			interfaces: []*ovsmodel.Interface{
				{
					UUID:                      "if-a",
					Name:                      "eth0",
					Type:                      "system",
					AdminState:                &adminUp,
					LinkState:                 &linkUp,
					MTU:                       &mtu,
					LinkSpeed:                 &linkSpeed,
					Ifindex:                   &ifIndex,
					Ofport:                    &ofPort,
					MAC:                       &mac,
					Duplex:                    &duplex,
					IngressPolicingRate:       1000,
					IngressPolicingBurst:      2000,
					IngressPolicingKpktsRate:  10,
					IngressPolicingKpktsBurst: 20,
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
		"ovs.interface.admin_state{bridge=br-int,interface=eth0}":                                        1,
		"ovs.interface.link_state{bridge=br-int,interface=eth0}":                                         1,
		"ovs.interface.mtu{bridge=br-int,interface=eth0}":                                                1500,
		"ovs.interface.link_speed{bridge=br-int,interface=eth0}":                                         10_000_000_000,
		"ovs.interface.if_index{bridge=br-int,interface=eth0}":                                           7,
		"ovs.interface.of_port{bridge=br-int,interface=eth0}":                                            3,
		"ovs.interface.ingress_policing.rate{bridge=br-int,interface=eth0}":                              1000,
		"ovs.interface.ingress_policing.burst{bridge=br-int,interface=eth0}":                             2000,
		"ovs.interface.ingress_policing.kpkts_rate{bridge=br-int,interface=eth0}":                        10,
		"ovs.interface.ingress_policing.kpkts_burst{bridge=br-int,interface=eth0}":                       20,
		"ovs.interface.info{bridge=br-int,duplex=full,interface=eth0,mac=00:11:22:33:44:55,type=system}": 1,
	}
	for k, v := range want {
		if g, ok := got[k]; !ok {
			t.Errorf("missing metric %q (got: %v)", k, got)
		} else if g != v {
			t.Errorf("%q = %d, want %d", k, g, v)
		}
	}
}

func TestOVSInterfaceInfo_NilFieldsOmitObservation(t *testing.T) {
	// An interface with only a name + type set must not panic and must
	// not emit the optional-pointer gauges.
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	c, _ := newOVSInterfaceInfoCollector(discardLogger())
	src := &fakeDataSource{
		ovs: &fakeOVSView{
			bridges: []*ovsmodel.Bridge{
				{Name: "br-int", Ports: []string{"port-a"}},
			},
			ports: []*ovsmodel.Port{
				{UUID: "port-a", Interfaces: []string{"if-a"}},
			},
			interfaces: []*ovsmodel.Interface{
				{UUID: "if-a", Name: "eth0", Type: "system"},
			},
		},
	}
	if err := c.Register(mp.Meter("test"), src); err != nil {
		t.Fatalf("Register: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	got := collectInt64Gauges(t, reader)
	// Info gauge is always emitted; ingress policing fields are
	// non-pointer ints so always emitted (default 0). The nullable
	// pointer fields (admin_state, link_state, mtu, link_speed,
	// if_index, of_port) should NOT appear.
	mustAbsent := []string{
		"ovs.interface.admin_state{bridge=br-int,interface=eth0}",
		"ovs.interface.link_state{bridge=br-int,interface=eth0}",
		"ovs.interface.mtu{bridge=br-int,interface=eth0}",
		"ovs.interface.link_speed{bridge=br-int,interface=eth0}",
		"ovs.interface.if_index{bridge=br-int,interface=eth0}",
		"ovs.interface.of_port{bridge=br-int,interface=eth0}",
	}
	for _, k := range mustAbsent {
		if _, present := got[k]; present {
			t.Errorf("expected %q absent for nil-pointer field, got %v", k, got[k])
		}
	}
	if got["ovs.interface.info{bridge=br-int,interface=eth0,type=system}"] != 1 {
		t.Errorf("info gauge missing or wrong: %v", got)
	}
}

func TestOVSInterfaceInfo_DefaultDisabled(t *testing.T) {
	// Sanity: this collector ships default-off; the init() registers it
	// with DefaultDisabled. We can't observe the kingpin flag value here
	// (kingpin.Parse hasn't run), but we can confirm the factory exists
	// in the registry.
	if _, ok := factories["ovs-interface-info"]; !ok {
		t.Error("ovs-interface-info not registered in factories")
	}
}
