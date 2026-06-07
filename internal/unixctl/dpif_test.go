package unixctl

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestParseDPIF_Golden_OVS_3_1(t *testing.T) {
	text, err := os.ReadFile(filepath.Join("testdata", "dpif_show_ovs_3.1.txt"))
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
		t.Fatalf("missing datapath system@ovs-system")
	}
	if d.Lookups.Hit != 1234 || d.Lookups.Missed != 56 || d.Lookups.Lost != 7 {
		t.Errorf("Lookups = %+v, want {1234 56 7}", d.Lookups)
	}
	if d.Flows != 89 {
		t.Errorf("Flows = %d, want 89", d.Flows)
	}
	if d.MasksHit != 1000 {
		t.Errorf("MasksHit = %d, want 1000", d.MasksHit)
	}
}

func TestParseDPIF_MultipleDatapaths(t *testing.T) {
	body := "system@ovs-system:\n" +
		"  lookups: hit:10 missed:1 lost:0\n" +
		"  flows: 5\n" +
		"  masks: hit:9 total:2 hit/pkt:1.80\n" +
		"netdev@ovs-netdev:\n" +
		"  lookups: hit:200 missed:20 lost:2\n" +
		"  flows: 30\n" +
		"  masks: hit:180 total:4 hit/pkt:6.00\n"
	raw, _ := json.Marshal(body)
	dp, err := ParseDPIF(raw)
	if err != nil {
		t.Fatalf("ParseDPIF: %v", err)
	}
	if len(dp.Datapaths) != 2 {
		t.Fatalf("datapath count = %d, want 2", len(dp.Datapaths))
	}
	if d := dp.Datapaths["system@ovs-system"]; d == nil || d.Flows != 5 {
		t.Errorf("system datapath = %+v, want Flows=5", d)
	}
	if d := dp.Datapaths["netdev@ovs-netdev"]; d == nil || d.Flows != 30 {
		t.Errorf("netdev datapath = %+v, want Flows=30", d)
	}
}

func TestParseDPIF_IgnoresPortLinesAndUnknownKeys(t *testing.T) {
	body := "system@ovs-system:\n" +
		"  lookups: hit:1 missed:0 lost:0\n" +
		"  flows: 1\n" +
		"  masks: hit:1 total:1 hit/pkt:1.00\n" +
		"  cache: hit:100\n" + // future field; must not break parse
		"  port 0: ovs-system (internal)\n" +
		"  port 1: br-int (internal)\n"
	raw, _ := json.Marshal(body)
	dp, err := ParseDPIF(raw)
	if err != nil {
		t.Fatalf("ParseDPIF: %v", err)
	}
	d := dp.Datapaths["system@ovs-system"]
	if d == nil {
		t.Fatal("missing datapath")
	}
	if d.Flows != 1 || d.MasksHit != 1 {
		t.Errorf("dp = %+v", d)
	}
}

func TestParseDPIF_InvalidJSON(t *testing.T) {
	if _, err := ParseDPIF([]byte("not-json")); err == nil {
		t.Error("expected error for invalid JSON")
	}
}
