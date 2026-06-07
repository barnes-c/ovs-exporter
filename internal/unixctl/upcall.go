package unixctl

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Upcall is the parsed output of `ovs-appctl upcall/show`. Each datapath
// block reports flow counts, dump latency, and per-handler key counts.
type Upcall struct {
	Datapaths map[string]*UpcallDatapath
}

// UpcallDatapath holds the subset of upcall/show fields we expose as
// metrics. DumpDurationMs is the integer millisecond value as printed by
// ovs-vswitchd; the unit suffix ("ms") is stripped during parsing.
type UpcallDatapath struct {
	Name           string
	FlowsCurrent   int64
	FlowsAvg       int64
	FlowsMax       int64
	FlowsLimit     int64
	DumpDurationMs int64
	HandlerKeys    map[int]int64
}

// parenPairRE matches `(name value)` groups inside the flows line:
//
//	flows : (current 0) (avg 0) (max 0) (limit 200000)
var parenPairRE = regexp.MustCompile(`\(([a-z]+)\s+(\d+)\)`)

// handlerLineRE matches per-handler key counts:
//
//	0: (keys 5)
var handlerLineRE = regexp.MustCompile(`^(\d+):\s*\(keys\s+(\d+)\)`)

// ParseUpcall decodes the JSON-RPC response from upcall/show. Unknown
// keys inside parenthesised groups (future OVS versions may add some)
// are silently ignored rather than failing the parse.
func ParseUpcall(raw json.RawMessage) (*Upcall, error) {
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return nil, fmt.Errorf("unixctl: upcall: unmarshal: %w", err)
	}
	out := &Upcall{Datapaths: make(map[string]*UpcallDatapath)}

	var cur *UpcallDatapath
	for _, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		// Datapath header at column 0 ending in ':'.
		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") &&
			strings.HasSuffix(strings.TrimSpace(line), ":") {
			name := strings.TrimSuffix(strings.TrimSpace(line), ":")
			cur = &UpcallDatapath{Name: name, HandlerKeys: make(map[int]int64)}
			out.Datapaths[name] = cur
			continue
		}
		if cur == nil {
			continue
		}
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "flows"):
			for _, m := range parenPairRE.FindAllStringSubmatch(trimmed, -1) {
				n, err := strconv.ParseInt(m[2], 10, 64)
				if err != nil {
					continue
				}
				switch m[1] {
				case "current":
					cur.FlowsCurrent = n
				case "avg":
					cur.FlowsAvg = n
				case "max":
					cur.FlowsMax = n
				case "limit":
					cur.FlowsLimit = n
				}
			}
		case strings.HasPrefix(trimmed, "dump duration"):
			// "dump duration : 12ms"
			_, rest, ok := strings.Cut(trimmed, ":")
			if !ok {
				continue
			}
			v := strings.TrimSuffix(strings.TrimSpace(rest), "ms")
			if n, err := strconv.ParseInt(v, 10, 64); err == nil {
				cur.DumpDurationMs = n
			}
		default:
			if m := handlerLineRE.FindStringSubmatch(trimmed); m != nil {
				id, err1 := strconv.Atoi(m[1])
				keys, err2 := strconv.ParseInt(m[2], 10, 64)
				if err1 == nil && err2 == nil {
					cur.HandlerKeys[id] = keys
				}
			}
		}
	}
	return out, nil
}
