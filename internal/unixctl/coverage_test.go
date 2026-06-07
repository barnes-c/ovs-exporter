package unixctl

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestParseCoverage_Golden(t *testing.T) {
	cases := []struct {
		name     string
		fixture  string
		want     map[string]int64
		wantSize int
	}{
		{
			name:    "OVS 3.1",
			fixture: "coverage_show_ovs_3.1.txt",
			want: map[string]int64{
				"ofproto_recv_openflow": 5,
				"ofproto_flush":         0,
				"bridge_reconfigure":    2,
				"ofproto_update_port":   1,
				"xlate_actions":         100,
				"flow_extract":          200,
				"miniflow_malloc":       5,
				"mac_learning_learned":  12,
			},
			wantSize: 8,
		},
		{
			name:    "OVS 3.3",
			fixture: "coverage_show_ovs_3.3.txt",
			want: map[string]int64{
				"ofproto_recv_openflow":    30,
				"xlate_actions":            300,
				"flow_extract":             450,
				"mac_learning_learned":     25,
				"mac_learning_evicted":     0,
				"dpif_flow_put":            18,
				"dpif_execute":             75,
				"handler_duplicate_upcall": 1,
				"rev_flow_table":           1,
			},
			wantSize: 12,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			text, err := os.ReadFile(filepath.Join("testdata", tc.fixture))
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
			if len(cov.Events) != tc.wantSize {
				t.Errorf("event count = %d, want %d (events: %v)", len(cov.Events), tc.wantSize, cov.Events)
			}
			for name, n := range tc.want {
				got, ok := cov.Events[name]
				if !ok {
					t.Errorf("missing event %q", name)
					continue
				}
				if got != n {
					t.Errorf("event %q = %d, want %d", name, got, n)
				}
			}
		})
	}
}

func TestParseCoverage_SkipsHeaderAndTrailer(t *testing.T) {
	// Only the header and trailer — no event rows.
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
