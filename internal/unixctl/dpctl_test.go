package unixctl

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestParseDPCTL_Golden_OVS_3_3(t *testing.T) {
	text, err := os.ReadFile(filepath.Join("testdata", "dpctl_show_ovs_3.3.txt"))
	if err != nil {
		t.Fatalf("read testdata: %v", err)
	}
	raw, err := json.Marshal(string(text))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	dp, err := ParseDPCTL(raw)
	if err != nil {
		t.Fatalf("ParseDPCTL: %v", err)
	}
	if got := len(dp.Datapaths); got != 1 {
		t.Fatalf("datapath count = %d, want 1", got)
	}
	d := dp.Datapaths["system@ovs-system"]
	if d == nil {
		t.Fatal("missing system@ovs-system")
	}
	if d.Lookups.Hit != 21901868 || d.Lookups.Missed != 2331661 || d.Lookups.Lost != 0 {
		t.Errorf("Lookups = %+v, want {21901868 2331661 0}", d.Lookups)
	}
	if d.Flows != 122 {
		t.Errorf("Flows = %d, want 122", d.Flows)
	}
	if d.MasksHit != 52499122 {
		t.Errorf("MasksHit = %d, want 52499122", d.MasksHit)
	}
	if d.MasksTotal != 8 {
		t.Errorf("MasksTotal = %d, want 8", d.MasksTotal)
	}
	if d.CacheHit != 18277721 {
		t.Errorf("CacheHit = %d, want 18277721", d.CacheHit)
	}
}

// TestParseDPCTL_PopulatedDatapath covers a fixture with non-zero
// counters across multiple datapaths. Synthetic until a richer
// production capture lands.
func TestParseDPCTL_PopulatedDatapath(t *testing.T) {
	body := "system@ovs-system:\n" +
		"  lookups: hit:14778031 missed:1583622 lost:0\n" +
		"  flows: 89\n" +
		"  masks: hit:14778031 total:5 hit/pkt:0.94\n" +
		"  caches:\n" +
		"    masks-cache: size:256\n" +
		"  port 0: ovs-system (internal)\n" +
		"netdev@ovs-netdev:\n" +
		"  lookups: hit:200 missed:20 lost:2\n" +
		"  flows: 30\n" +
		"  masks: hit:180 total:4 hit/pkt:0.82\n"
	raw, _ := json.Marshal(body)
	dp, err := ParseDPCTL(raw)
	if err != nil {
		t.Fatalf("ParseDPCTL: %v", err)
	}
	if len(dp.Datapaths) != 2 {
		t.Fatalf("datapath count = %d, want 2", len(dp.Datapaths))
	}
	if d := dp.Datapaths["system@ovs-system"]; d == nil || d.Flows != 89 || d.MasksHit != 14778031 {
		t.Errorf("system = %+v, want Flows=89 MasksHit=14778031", d)
	}
	if d := dp.Datapaths["netdev@ovs-netdev"]; d == nil || d.Flows != 30 || d.Lookups.Lost != 2 {
		t.Errorf("netdev = %+v, want Flows=30 Lookups.Lost=2", d)
	}
}

func TestParseDPCTL_UnknownLinesSkipped(t *testing.T) {
	body := "system@ovs-system:\n" +
		"  lookups: hit:1 missed:0 lost:0\n" +
		"  flows: 1\n" +
		"  masks: hit:1 total:1 hit/pkt:1.00\n" +
		"  some-future-field: hit:100\n" + // hypothetical future field
		"  caches:\n" +
		"    masks-cache: size:256\n" +
		"  port 0: ovs-system (internal)\n"
	raw, _ := json.Marshal(body)
	dp, err := ParseDPCTL(raw)
	if err != nil {
		t.Fatalf("ParseDPCTL: %v", err)
	}
	d := dp.Datapaths["system@ovs-system"]
	if d == nil {
		t.Fatal("missing datapath")
	}
	if d.Flows != 1 || d.MasksHit != 1 {
		t.Errorf("dp = %+v, want Flows=1 MasksHit=1", d)
	}
}

func TestParseDPCTL_InvalidJSON(t *testing.T) {
	if _, err := ParseDPCTL([]byte("not-json")); err == nil {
		t.Error("expected error for invalid JSON")
	}
}
