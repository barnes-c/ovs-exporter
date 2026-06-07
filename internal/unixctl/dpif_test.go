package unixctl

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseDPIF_Golden(t *testing.T) {
	text, err := os.ReadFile(filepath.Join("testdata", "dpif_show_ovs_3.3.txt"))
	if err != nil {
		t.Fatalf("read testdata: %v", err)
	}
	raw, err := json.Marshal(string(text))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	dp, err := ParseDPIF(raw)
	if err != nil {
		t.Fatalf("ParseDPIF: %v", err)
	}
	if got := len(dp.Datapaths); got != 1 {
		t.Fatalf("datapath count = %d, want 1 (datapaths: %v)", got, dp.Datapaths)
	}
	d := dp.Datapaths["system@ovs-system"]
	if d == nil {
		t.Fatal("missing datapath system@ovs-system")
	}
	if d.Lookups.Hit != 14778031 {
		t.Errorf("Lookups.Hit = %d, want 14778031", d.Lookups.Hit)
	}
	if d.Lookups.Missed != 1583622 {
		t.Errorf("Lookups.Missed = %d, want 1583622", d.Lookups.Missed)
	}
	// `lost` not emitted by current OVS dpif/show; should default to 0.
	if d.Lookups.Lost != 0 {
		t.Errorf("Lookups.Lost = %d, want 0 (absent in real output)", d.Lookups.Lost)
	}
}

func TestParseDPIF_CapturesLostWhenPresent(t *testing.T) {
	// Forward-compat: confirm that if a future OVS reintroduces lost: on
	// the inline summary, the parser picks it up.
	body := "system@ovs-system: hit:100 missed:5 lost:2\n"
	raw, _ := json.Marshal(body)
	dp, err := ParseDPIF(raw)
	if err != nil {
		t.Fatalf("ParseDPIF: %v", err)
	}
	d := dp.Datapaths["system@ovs-system"]
	if d == nil {
		t.Fatal("missing datapath")
	}
	if d.Lookups.Lost != 2 {
		t.Errorf("Lookups.Lost = %d, want 2", d.Lookups.Lost)
	}
}

func TestParseDPIF_MultipleDatapaths(t *testing.T) {
	body := "system@ovs-system: hit:10 missed:1\n" +
		"  br-int:\n" +
		"    eth0 1/3: (system)\n" +
		"netdev@ovs-netdev: hit:200 missed:20\n" +
		"  br-ex:\n" +
		"    eno1 2/4: (system)\n"
	raw, _ := json.Marshal(body)
	dp, err := ParseDPIF(raw)
	if err != nil {
		t.Fatalf("ParseDPIF: %v", err)
	}
	if len(dp.Datapaths) != 2 {
		t.Fatalf("datapath count = %d, want 2", len(dp.Datapaths))
	}
	if d := dp.Datapaths["system@ovs-system"]; d == nil || d.Lookups.Hit != 10 {
		t.Errorf("system datapath = %+v, want Hit=10", d)
	}
	if d := dp.Datapaths["netdev@ovs-netdev"]; d == nil || d.Lookups.Hit != 200 {
		t.Errorf("netdev datapath = %+v, want Hit=200", d)
	}
}

func TestParseDPIF_IgnoresIndentedTopologyLines(t *testing.T) {
	// Indented bridge / port lines must not be parsed as datapaths even
	// when they contain `:` and digit-looking suffixes.
	body := "system@ovs-system: hit:1 missed:0\n" +
		"  br-int:\n" +
		"    eth0 1/3: (system)\n" +
		"    vxlan_sys_4789 5/none: (vxlan: csum=true, key=flow)\n"
	raw, _ := json.Marshal(body)
	dp, err := ParseDPIF(raw)
	if err != nil {
		t.Fatalf("ParseDPIF: %v", err)
	}
	if len(dp.Datapaths) != 1 {
		t.Errorf("datapath count = %d, want 1 (datapaths: %v)", len(dp.Datapaths), dp.Datapaths)
	}
}

// TestParseDPIF_Topology_Golden asserts the parser extracts the indented
// bridge / port hierarchy from the same real capture used elsewhere.
func TestParseDPIF_Topology_Golden(t *testing.T) {
	text, err := os.ReadFile(filepath.Join("testdata", "dpif_show_ovs_3.3.txt"))
	if err != nil {
		t.Fatalf("read testdata: %v", err)
	}
	raw, err := json.Marshal(string(text))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	dp, err := ParseDPIF(raw)
	if err != nil {
		t.Fatalf("ParseDPIF: %v", err)
	}
	d := dp.Datapaths["system@ovs-system"]
	if d == nil {
		t.Fatal("missing datapath")
	}
	if len(d.Bridges) != 2 {
		t.Errorf("bridges = %d, want 2 (bridges: %v)", len(d.Bridges), d.Bridges)
	}
	brEth0 := d.Bridges["br-eth0"]
	if brEth0 == nil {
		t.Fatal("missing br-eth0")
	}
	if len(brEth0.Ports) != 3 {
		t.Errorf("br-eth0 ports = %d, want 3", len(brEth0.Ports))
	}
	// Confirm a representative port carries the expected fields.
	var eth0 *DPIFPort
	for i := range brEth0.Ports {
		if brEth0.Ports[i].Name == "eth0" {
			eth0 = &brEth0.Ports[i]
			break
		}
	}
	if eth0 == nil {
		t.Fatal("missing eth0 port")
	}
	if eth0.PortNo != 1 || eth0.OFPortNo != "3" || eth0.Type != "system" {
		t.Errorf("eth0 = %+v, want {PortNo:1 OFPortNo:3 Type:system}", *eth0)
	}

	// Patch port: OFPortNo is "none", not a number.
	brInt := d.Bridges["br-int"]
	if brInt == nil {
		t.Fatal("missing br-int")
	}
	var patch *DPIFPort
	for i := range brInt.Ports {
		if strings.HasPrefix(brInt.Ports[i].Name, "patch-") {
			patch = &brInt.Ports[i]
			break
		}
	}
	if patch == nil {
		t.Fatal("missing patch port in br-int")
	}
	if patch.OFPortNo != "none" || patch.Type != "patch" {
		t.Errorf("patch = %+v, want {OFPortNo:none Type:patch}", *patch)
	}
}

func TestParseDPIF_InvalidJSON(t *testing.T) {
	if _, err := ParseDPIF([]byte("not-json")); err == nil {
		t.Error("expected error for invalid JSON")
	}
}
