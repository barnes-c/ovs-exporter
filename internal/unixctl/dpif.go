package unixctl

import (
	"encoding/json"
	"fmt"
	"regexp"
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

// DPIFDatapath holds the per-datapath fields exposed by dpif/show. The
// Bridges map is populated only when the indented topology lines are
// parsed; default-on collectors that need only lookup stats can ignore
// it. Bridges/Ports are exposed via the opt-in ovs-datapath-interfaces
// collector because of cardinality.
type DPIFDatapath struct {
	Name    string
	Lookups DPIFLookups
	Bridges map[string]*DPIFBridge
}

// DPIFBridge is a bridge entry in a datapath's topology.
type DPIFBridge struct {
	Name  string
	Ports []DPIFPort
}

// DPIFPort describes one OF port under a bridge. OFPortNo is a string
// because `none` is a valid value for patch ports (no underlying
// datapath port number).
type DPIFPort struct {
	Name     string
	PortNo   int64
	OFPortNo string
	Type     string
}

// DPIFLookups is the per-datapath lookup outcome breakdown.
// `lost` is rarely populated by current OVS but kept for forward-compat.
type DPIFLookups struct {
	Hit    int64
	Missed int64
	Lost   int64
}

// portLineRE matches "<name> <portno>/<ofportno>: (<type>...)".
// Examples:
//
//	eth0 1/3: (system)
//	br-int 65534/2: (internal)
//	patch-foo-to-br-int 29/none: (patch: peer=patch-br-int-to-foo)
//	ovn-aaaaaa-0 208/4: (geneve: csum=true, key=flow, remote_ip=192.0.2.21)
var portLineRE = regexp.MustCompile(`^(\S+)\s+(\d+)/(\S+):\s*\(([^:)]+)`)

// ParseDPIF decodes the JSON-RPC response from dpif/show. Column-0
// lines describe datapaths and their inline lookup stats; lines
// indented two spaces are bridge headers (`<bridge_name>:`); lines
// indented four spaces are port entries belonging to the most recent
// bridge.
func ParseDPIF(raw json.RawMessage) (*DPIF, error) {
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return nil, fmt.Errorf("unixctl: dpif: unmarshal: %w", err)
	}
	out := &DPIF{Datapaths: make(map[string]*DPIFDatapath)}

	var curDP *DPIFDatapath
	var curBridge *DPIFBridge
	for _, line := range strings.Split(text, "\n") {
		if line == "" {
			continue
		}
		switch {
		case strings.HasPrefix(line, "    "):
			if curBridge == nil {
				continue
			}
			port, ok := parseDPIFPortLine(strings.TrimLeft(line, " \t"))
			if !ok {
				continue
			}
			curBridge.Ports = append(curBridge.Ports, port)

		case strings.HasPrefix(line, "  "):
			if curDP == nil {
				continue
			}
			trimmed := strings.TrimSpace(line)
			if !strings.HasSuffix(trimmed, ":") {
				continue
			}
			name := strings.TrimSuffix(trimmed, ":")
			curBridge = &DPIFBridge{Name: name}
			if curDP.Bridges == nil {
				curDP.Bridges = make(map[string]*DPIFBridge)
			}
			curDP.Bridges[name] = curBridge

		default:
			dp, ok := parseDPIFDatapathLine(line)
			if !ok {
				continue
			}
			out.Datapaths[dp.Name] = dp
			curDP = dp
			curBridge = nil
		}
	}
	return out, nil
}

func parseDPIFDatapathLine(line string) (*DPIFDatapath, bool) {
	// "name@type: hit:H missed:M [lost:L]"
	name, rest, ok := strings.Cut(line, ":")
	if !ok || name == "" {
		return nil, false
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
	if !seen {
		return nil, false
	}
	return dp, true
}

func parseDPIFPortLine(line string) (DPIFPort, bool) {
	m := portLineRE.FindStringSubmatch(line)
	if m == nil {
		return DPIFPort{}, false
	}
	portNo, err := strconv.ParseInt(m[2], 10, 64)
	if err != nil {
		return DPIFPort{}, false
	}
	return DPIFPort{
		Name:     m[1],
		PortNo:   portNo,
		OFPortNo: m[3],
		Type:     strings.TrimSpace(m[4]),
	}, true
}
