package unixctl

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// DPIF is the parsed output of `ovs-appctl dpif/show`. Each column-0
// line describes a datapath and carries inline lookup counters:
//
//	system@ovs-system: hit:14778031 missed:1583622
//	  br-eth0:
//	    eth0 1/3: (system)
//	    ...
//	  br-int:
//	    ...
//
// Indented bridge / port lines hold topology data (bridge → datapath
// mapping, port type, OF port numbers, tunnel remote_ip, patch peer).
// They are intentionally not parsed here — that's the opt-in
// --collector.ovs-datapath-interfaces collector in T13. The flow count
// and mask-cache stats that the original plan called for are not in
// `dpif/show` output; they live in `dpctl/show`, which we'll add as a
// separate scrape target if/when those metrics become a priority.
type DPIF struct {
	Datapaths map[string]*DPIFDatapath
}

// DPIFDatapath holds the per-datapath fields exposed by dpif/show.
type DPIFDatapath struct {
	Name    string
	Lookups DPIFLookups
}

// DPIFLookups is the per-datapath lookup outcome breakdown.
// `lost` is rarely populated by current OVS but kept for forward-compat.
type DPIFLookups struct {
	Hit    int64
	Missed int64
	Lost   int64
}

// ParseDPIF decodes the JSON-RPC response from dpif/show. Only column-0
// lines that contain at least one `key:int` pair are treated as datapath
// rows; indented lines and unrecognised keys are ignored.
func ParseDPIF(raw json.RawMessage) (*DPIF, error) {
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return nil, fmt.Errorf("unixctl: dpif: unmarshal: %w", err)
	}
	out := &DPIF{Datapaths: make(map[string]*DPIFDatapath)}

	for _, line := range strings.Split(text, "\n") {
		if line == "" || strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			continue
		}
		// "name@type: hit:H missed:M [lost:L]"
		name, rest, ok := strings.Cut(line, ":")
		if !ok || name == "" {
			continue
		}

		dp := &DPIFDatapath{Name: name}
		seen := false
		for _, tok := range strings.Fields(rest) {
			k, v, ok := strings.Cut(tok, ":")
			if !ok {
				continue
			}
			n, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				continue
			}
			switch k {
			case "hit":
				dp.Lookups.Hit = n
				seen = true
			case "missed":
				dp.Lookups.Missed = n
				seen = true
			case "lost":
				dp.Lookups.Lost = n
				seen = true
			}
		}
		if seen {
			out.Datapaths[name] = dp
		}
	}
	return out, nil
}
