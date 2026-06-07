package unixctl

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestParseMemory_Golden_OVS_3_1(t *testing.T) {
	text, err := os.ReadFile(filepath.Join("testdata", "memory_show_ovs_3.1.txt"))
	if err != nil {
		t.Fatalf("read testdata: %v", err)
	}
	raw, err := json.Marshal(string(text))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	mem, err := ParseMemory(raw)
	if err != nil {
		t.Fatalf("ParseMemory: %v", err)
	}
	want := map[string]int64{
		"handlers":     1,
		"ofconns":      1,
		"ports":        5,
		"revalidators": 1,
		"rules":        12,
	}
	if len(mem.Usage) != len(want) {
		t.Errorf("usage size = %d, want %d (usage: %v)", len(mem.Usage), len(want), mem.Usage)
	}
	for k, v := range want {
		if got, ok := mem.Usage[k]; !ok {
			t.Errorf("missing key %q", k)
		} else if got != v {
			t.Errorf("usage[%q] = %d, want %d", k, got, v)
		}
	}
}

func TestParseMemory_SkipsMalformedTokens(t *testing.T) {
	body := "handlers:5 ofconns:not-a-number ports:7 standalone_token revalidators:1\n"
	raw, _ := json.Marshal(body)
	mem, err := ParseMemory(raw)
	if err != nil {
		t.Fatalf("ParseMemory: %v", err)
	}
	want := map[string]int64{"handlers": 5, "ports": 7, "revalidators": 1}
	if len(mem.Usage) != len(want) {
		t.Errorf("usage = %v, want %v", mem.Usage, want)
	}
}

func TestParseMemory_InvalidJSON(t *testing.T) {
	if _, err := ParseMemory([]byte("not-json")); err == nil {
		t.Error("expected error for invalid JSON")
	}
}
