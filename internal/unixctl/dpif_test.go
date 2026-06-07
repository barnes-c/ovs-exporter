package unixctl

import (
	"encoding/json"
	"os"
	"path/filepath"
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

func TestParseDPIF_InvalidJSON(t *testing.T) {
	if _, err := ParseDPIF([]byte("not-json")); err == nil {
		t.Error("expected error for invalid JSON")
	}
}
