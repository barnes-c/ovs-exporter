package unixctl

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestParseUpcall_Golden_OVS_3_1(t *testing.T) {
	text, err := os.ReadFile(filepath.Join("testdata", "upcall_show_ovs_3.1.txt"))
	if err != nil {
		t.Fatalf("read testdata: %v", err)
	}
	raw, err := json.Marshal(string(text))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	up, err := ParseUpcall(raw)
	if err != nil {
		t.Fatalf("ParseUpcall: %v", err)
	}
	if len(up.Datapaths) != 1 {
		t.Fatalf("datapath count = %d, want 1", len(up.Datapaths))
	}
	d := up.Datapaths["system@ovs-system"]
	if d == nil {
		t.Fatal("missing datapath")
	}
	if d.FlowsCurrent != 42 || d.FlowsAvg != 35 || d.FlowsMax != 120 || d.FlowsLimit != 200000 {
		t.Errorf("flows = {cur:%d avg:%d max:%d limit:%d}, want {42 35 120 200000}",
			d.FlowsCurrent, d.FlowsAvg, d.FlowsMax, d.FlowsLimit)
	}
	if d.DumpDurationMs != 7 {
		t.Errorf("DumpDurationMs = %d, want 7", d.DumpDurationMs)
	}
	wantHandlers := map[int]int64{0: 12, 1: 9, 2: 21}
	if !reflect.DeepEqual(d.HandlerKeys, wantHandlers) {
		t.Errorf("HandlerKeys = %v, want %v", d.HandlerKeys, wantHandlers)
	}
}

func TestParseUpcall_MissingFieldsTolerated(t *testing.T) {
	// Datapath block with only a flows line — no dump duration, no handlers.
	body := "system@ovs-system:\n" +
		"  flows         : (current 5) (limit 1000)\n"
	raw, _ := json.Marshal(body)
	up, err := ParseUpcall(raw)
	if err != nil {
		t.Fatalf("ParseUpcall: %v", err)
	}
	d := up.Datapaths["system@ovs-system"]
	if d == nil {
		t.Fatal("missing datapath")
	}
	if d.FlowsCurrent != 5 || d.FlowsLimit != 1000 {
		t.Errorf("flows = {cur:%d limit:%d}, want {5 1000}", d.FlowsCurrent, d.FlowsLimit)
	}
	if d.DumpDurationMs != 0 {
		t.Errorf("DumpDurationMs = %d, want 0 (absent)", d.DumpDurationMs)
	}
	if len(d.HandlerKeys) != 0 {
		t.Errorf("HandlerKeys = %v, want empty", d.HandlerKeys)
	}
}

func TestParseUpcall_InvalidJSON(t *testing.T) {
	if _, err := ParseUpcall([]byte("not-json")); err == nil {
		t.Error("expected error for invalid JSON")
	}
}
