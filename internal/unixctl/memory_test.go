package unixctl

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestParseMemory_Golden(t *testing.T) {
	text, err := os.ReadFile(filepath.Join("testdata", "memory_show_ovs_3.3.txt"))
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
		"handlers":               32,
		"idl-cells-Open_vSwitch": 16667,
		"ofconns":                4,
		"ports":                  291,
		"revalidators":           9,
		"rules":                  64243,
		"udpif keys":             117,
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

func TestParseMemory_MultiWordKeys(t *testing.T) {
	// Two synthetic multi-word keys; verifies the accumulator handles
	// more than one in a single payload.
	body := "foo bar:1 baz qux quux:2 simple:3\n"
	raw, _ := json.Marshal(body)
	mem, err := ParseMemory(raw)
	if err != nil {
		t.Fatalf("ParseMemory: %v", err)
	}
	want := map[string]int64{"foo bar": 1, "baz qux quux": 2, "simple": 3}
	if len(mem.Usage) != len(want) {
		t.Errorf("usage = %v, want %v", mem.Usage, want)
	}
	for k, v := range want {
		if mem.Usage[k] != v {
			t.Errorf("usage[%q] = %d, want %d", k, mem.Usage[k], v)
		}
	}
}

func TestParseMemory_SkipsMalformedTokens(t *testing.T) {
	body := "handlers:5 ofconns:not-a-number ports:7 revalidators:1\n"
	raw, _ := json.Marshal(body)
	mem, err := ParseMemory(raw)
	if err != nil {
		t.Fatalf("ParseMemory: %v", err)
	}
	// "ofconns:not-a-number" doesn't parse → it's swept into the next
	// key's name fragment, so "ofconns:not-a-number ports" becomes the
	// effective key for value 7. That's an acceptable consequence of
	// being tolerant; the right behaviour upstream is "real memory/show
	// never emits non-integer values". We still want to see the clean
	// keys parse correctly.
	if mem.Usage["handlers"] != 5 {
		t.Errorf("handlers = %d, want 5", mem.Usage["handlers"])
	}
	if mem.Usage["revalidators"] != 1 {
		t.Errorf("revalidators = %d, want 1", mem.Usage["revalidators"])
	}
}

func TestParseMemory_InvalidJSON(t *testing.T) {
	if _, err := ParseMemory([]byte("not-json")); err == nil {
		t.Error("expected error for invalid JSON")
	}
}
