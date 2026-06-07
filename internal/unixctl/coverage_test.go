package unixctl

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestParseCoverage_Golden asserts the parser against a real
// `ovs-appctl coverage/show` capture. We don't pin the exact count to a
// number because new OVS versions add coverage events freely — but we
// do pin a representative set of event names and their captured totals,
// which is enough to detect format-level regressions.
func TestParseCoverage_Golden(t *testing.T) {
	text, err := os.ReadFile(filepath.Join("testdata", "coverage_show_ovs_3.3.txt"))
	if err != nil {
		t.Fatalf("read testdata: %v", err)
	}
	raw, err := json.Marshal(string(text))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	cov, err := ParseCoverage(raw)
	if err != nil {
		t.Fatalf("ParseCoverage: %v", err)
	}

	// At least a couple of dozen events on a busy hypervisor; if the
	// parser breaks the format, this drops to 0.
	if len(cov.Events) < 10 {
		t.Fatalf("event count = %d, want >= 10 (events: %v)", len(cov.Events), cov.Events)
	}

	want := map[string]int64{
		"netlink_sent":          9431740,
		"netlink_received":      9102350,
		"netlink_recv_jumbo":    2369671,
		"ofproto_recv_openflow": 13032709,
		"ofproto_packet_out":    1448,
		"bridge_reconfigure":    7533,
		"flow_extract":          1586779,
		"xlate_actions":         17472045,
		"dpif_flow_put":         1733742,
		"dpif_execute":          1565671,
		"mac_learning_learned":  42107,
		"util_xalloc":           890602098,
	}
	for name, n := range want {
		got, ok := cov.Events[name]
		if !ok {
			t.Errorf("missing event %q", name)
			continue
		}
		if got != n {
			t.Errorf("event %q = %d, want %d", name, got, n)
		}
	}
}

func TestParseCoverage_SkipsHeaderAndTrailer(t *testing.T) {
	body := "Event coverage, avg rate over last: 5 seconds, last minute, last hour,  hash=ea4ab92a\n121 events never hit\n"
	raw, _ := json.Marshal(body)
	cov, err := ParseCoverage(raw)
	if err != nil {
		t.Fatalf("ParseCoverage: %v", err)
	}
	if len(cov.Events) != 0 {
		t.Errorf("events = %v, want empty", cov.Events)
	}
}

func TestParseCoverage_TolerantOfMalformedRows(t *testing.T) {
	body := "good_event    0.0/sec    0.0/sec    0.0/sec   total: 7\n" +
		"missing_total                                                  \n" +
		"bad_count    0.0/sec    0.0/sec    0.0/sec   total: not-a-number\n" +
		"another_good    0.0/sec    0.0/sec    0.0/sec   total: 42\n"
	raw, _ := json.Marshal(body)
	cov, err := ParseCoverage(raw)
	if err != nil {
		t.Fatalf("ParseCoverage: %v", err)
	}
	want := map[string]int64{"good_event": 7, "another_good": 42}
	if len(cov.Events) != len(want) {
		t.Errorf("events = %v, want %v", cov.Events, want)
	}
	for k, v := range want {
		if cov.Events[k] != v {
			t.Errorf("event %q = %d, want %d", k, cov.Events[k], v)
		}
	}
}

func TestParseCoverage_InvalidJSON(t *testing.T) {
	if _, err := ParseCoverage([]byte("not-json")); err == nil {
		t.Error("expected error for invalid JSON")
	}
}
