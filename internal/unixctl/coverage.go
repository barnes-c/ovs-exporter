package unixctl

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Coverage is the parsed output of `ovs-appctl coverage/show`. Only the
// cumulative event totals are kept; the per-event short-window averages
// (the "0.0/sec  0.083/sec  0.0014/sec" columns) are dropped because
// Prometheus `rate()` derives them better from the cumulative counters.
type Coverage struct {
	Events map[string]int64
}

// ParseCoverage decodes the JSON-RPC response from coverage/show. The
// appctl protocol wraps the text payload as a JSON string, so the raw
// message is first unmarshalled into a Go string and then split line by
// line. Header and trailer lines are skipped silently — only lines
// containing `total: <N>` are treated as event rows.
//
// Format (OVS 3.1):
//
//	Event coverage, avg rate over last: 5 seconds, last minute, ...
//	ofproto_recv_openflow       0.0/sec     0.083/sec     0.0014/sec   total: 5
//	...
//	121 events never hit
func ParseCoverage(raw json.RawMessage) (*Coverage, error) {
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return nil, fmt.Errorf("unixctl: coverage: unmarshal: %w", err)
	}
	events := make(map[string]int64)
	for _, line := range strings.Split(text, "\n") {
		idx := strings.LastIndex(line, "total:")
		if idx < 0 {
			continue
		}
		head := strings.Fields(line[:idx])
		if len(head) == 0 {
			continue
		}
		tail := strings.TrimSpace(line[idx+len("total:"):])
		n, err := strconv.ParseInt(tail, 10, 64)
		if err != nil {
			continue
		}
		events[head[0]] = n
	}
	return &Coverage{Events: events}, nil
}
