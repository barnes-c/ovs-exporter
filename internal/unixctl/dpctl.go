package unixctl

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// DPCTL is the parsed output of `ovs-appctl dpctl/show`. Unlike
// `dpif/show` (which gives a terse one-line summary per datapath),
// dpctl/show emits the verbose breakdown with separate lookups, flows,
// and masks lines that operators expect for capacity planning.
//
//	system@ovs-system:
//	  lookups: hit:N missed:N lost:N
//	  flows: N
//	  masks: hit:N total:N hit/pkt:F.FF
//	  cache: hit:N hit-rate:F.FF%
//	  caches:
//	    masks-cache: size:N
//	  port N: name (type)
//	  ...
//
// `cache:` is the megaflow cache that sits in front of the exact-match
// cache — a different layer from the masks cache. Its hit-rate is
// derivable from `cache.hit / (lookups.hit + lookups.missed)`, so only
// the counter is exposed.
//
// Per-port lines and the `caches:` nested block (added in OVS 3.x) are
// intentionally skipped — port topology lives in the opt-in
// `ovs-datapath-interfaces` collector sourced from dpif/show, and we
// don't expose cache-size metrics yet. Any future indented line that
// doesn't match a known prefix is silently ignored for forward
// compatibility.
type DPCTL struct {
	Datapaths map[string]*DPCTLDatapath
}

// DPCTLDatapath holds the per-datapath stats. Lookups reuses the DPIF
// shape because the inner `hit/missed/lost` triple is identical across
// both commands.
type DPCTLDatapath struct {
	Name       string
	Lookups    DPIFLookups
	Flows      int64
	MasksHit   int64
	MasksTotal int64
	CacheHit   int64
}

// ParseDPCTL decodes the JSON-RPC response from dpctl/show. Unknown
// keys / extra lines (future OVS versions may add fields like
// `cache:`) are skipped silently rather than failing the parse.
func ParseDPCTL(raw json.RawMessage) (*DPCTL, error) {
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return nil, fmt.Errorf("unixctl: dpctl: unmarshal: %w", err)
	}
	out := &DPCTL{Datapaths: make(map[string]*DPCTLDatapath)}

	var cur *DPCTLDatapath
	for _, line := range strings.Split(text, "\n") {
		if line == "" {
			continue
		}
		// Indented lines belong to the most recent datapath.
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			if cur == nil {
				continue
			}
			parseDPCTLStatsLine(strings.TrimSpace(line), cur)
			continue
		}
		// Column-0 line ending in `:` starts a new datapath.
		trimmed := strings.TrimSpace(line)
		if !strings.HasSuffix(trimmed, ":") {
			continue
		}
		name := strings.TrimSuffix(trimmed, ":")
		cur = &DPCTLDatapath{Name: name}
		out.Datapaths[name] = cur
	}
	return out, nil
}

func parseDPCTLStatsLine(line string, dp *DPCTLDatapath) {
	switch {
	case strings.HasPrefix(line, "lookups:"):
		parseDPCTLKVMap(line[len("lookups:"):], func(k string, n int64) {
			switch k {
			case "hit":
				dp.Lookups.Hit = n
			case "missed":
				dp.Lookups.Missed = n
			case "lost":
				dp.Lookups.Lost = n
			}
		})
	case strings.HasPrefix(line, "flows:"):
		if n, err := strconv.ParseInt(strings.TrimSpace(line[len("flows:"):]), 10, 64); err == nil {
			dp.Flows = n
		}
	case strings.HasPrefix(line, "masks:"):
		parseDPCTLKVMap(line[len("masks:"):], func(k string, n int64) {
			switch k {
			case "hit":
				dp.MasksHit = n
			case "total":
				dp.MasksTotal = n
			}
		})
	case strings.HasPrefix(line, "cache:"):
		// `hit-rate:F.FF%` has a % suffix → parseDPCTLKVMap's ParseInt
		// fails on it → silently skipped. Only `hit:N` is captured.
		parseDPCTLKVMap(line[len("cache:"):], func(k string, n int64) {
			if k == "hit" {
				dp.CacheHit = n
			}
		})
	}
	// `port N: name (type)` and `caches:` (with its nested
	// `masks-cache:` line) are intentionally not parsed here.
}

// parseDPCTLKVMap walks tokens like "hit:46 missed:0 lost:0" and invokes
// fn for each `key:int64` pair. Non-integer values (e.g. "hit/pkt:0.94")
// are silently skipped.
func parseDPCTLKVMap(rest string, fn func(string, int64)) {
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
