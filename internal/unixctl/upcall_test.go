package unixctl

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestParseUpcall_Golden(t *testing.T) {
	text, err := os.ReadFile(filepath.Join("testdata", "upcall_show_ovs_3.3.txt"))
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
	if d.FlowsCurrent != 259 || d.FlowsAvg != 287 || d.FlowsMax != 2511 || d.FlowsLimit != 200000 {
		t.Errorf("flows = {cur:%d avg:%d max:%d limit:%d}, want {259 287 2511 200000}",
			d.FlowsCurrent, d.FlowsAvg, d.FlowsMax, d.FlowsLimit)
	}
	if d.DumpDurationMs != 1 {
		t.Errorf("DumpDurationMs = %d, want 1", d.DumpDurationMs)
	}
	wantHandlers := map[int]int64{
		75: 24, 76: 29, 77: 34, 78: 26, 79: 31,
		80: 22, 81: 23, 82: 34, 83: 37,
	}
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
