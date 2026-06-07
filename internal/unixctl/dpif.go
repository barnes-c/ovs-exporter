package unixctl

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// DPIF is the parsed output of `ovs-appctl dpif/show`. ovs-vswitchd may
// manage multiple datapaths (system, netdev), each block starting with a
// "name@type:" header and followed by indented stats lines.
type DPIF struct {
	Datapaths map[string]*DPIFDatapath
}

// DPIFDatapath holds the subset of dpif/show fields we expose as metrics.
// Per-port lines from the same block are not parsed here — that's the
// opt-in `ovs-datapath-interfaces` collector (T13).
type DPIFDatapath struct {
	Name     string
	Lookups  DPIFLookups
	Flows    int64
	MasksHit int64
}

// DPIFLookups is the per-datapath lookup outcome breakdown.
type DPIFLookups struct {
	Hit    int64
	Missed int64
	Lost   int64
}

// ParseDPIF decodes the JSON-RPC response from dpif/show. Lines that
// don't match a known prefix are skipped, so future OVS versions may add
// lines (e.g. a "cache:" line introduced in some 3.x patch) without
// failing the parse.
func ParseDPIF(raw json.RawMessage) (*DPIF, error) {
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return nil, fmt.Errorf("unixctl: dpif: unmarshal: %w", err)
	}
	out := &DPIF{Datapaths: make(map[string]*DPIFDatapath)}

	var cur *DPIFDatapath
	for _, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		// Datapath header: starts at column 0, ends with ':'. Indented
		// lines are stats under the most recent header.
		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") &&
			strings.HasSuffix(strings.TrimSpace(line), ":") {
			name := strings.TrimSuffix(strings.TrimSpace(line), ":")
			cur = &DPIFDatapath{Name: name}
			out.Datapaths[name] = cur
			continue
		}
		if cur == nil {
			continue
		}
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "lookups:"):
			parseDPIFKVMap(trimmed[len("lookups:"):], func(k string, n int64) {
				switch k {
				case "hit":
					cur.Lookups.Hit = n
				case "missed":
					cur.Lookups.Missed = n
				case "lost":
					cur.Lookups.Lost = n
				}
			})
		case strings.HasPrefix(trimmed, "flows:"):
			if n, err := strconv.ParseInt(strings.TrimSpace(trimmed[len("flows:"):]), 10, 64); err == nil {
				cur.Flows = n
			}
		case strings.HasPrefix(trimmed, "masks:"):
			parseDPIFKVMap(trimmed[len("masks:"):], func(k string, n int64) {
				if k == "hit" {
					cur.MasksHit = n
				}
			})
		}
	}
	return out, nil
}

// parseDPIFKVMap parses tokens like "hit:46 missed:0 lost:0" and invokes
// fn for each `key:int64` pair. Non-integer values (e.g. "hit/pkt:0.95")
// are silently skipped.
func parseDPIFKVMap(rest string, fn func(string, int64)) {
	for _, tok := range strings.Fields(rest) {
		k, v, ok := strings.Cut(tok, ":")
		if !ok {
			continue
		}
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			continue
		}
		fn(k, n)
	}
}
