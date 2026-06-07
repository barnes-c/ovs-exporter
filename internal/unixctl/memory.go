package unixctl

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Memory is the parsed output of `ovs-appctl memory/show`. The command
// emits a single line of `key:N` pairs reported by the ovs-vswitchd
// memory_report handler — most keys are single tokens (handlers,
// ofconns, ports, rules, revalidators) but a few are space-separated
// (notably `udpif keys`).
type Memory struct {
	Usage map[string]int64
}

// ParseMemory decodes the JSON-RPC response from memory/show.
//
// The format treats whitespace as a token separator, but a *key* can span
// multiple tokens — ovs-vswitchd emits e.g. "udpif keys:117" where the
// real key is "udpif keys". We therefore walk tokens left to right,
// accumulating non-value-bearing tokens until we hit a `name:int` whose
// integer suffix closes the current key. Tokens whose suffix doesn't
// parse as int are treated as another name fragment so the next
// `name:int` joins them. Future OVS releases adding new keys with extra
// embedded spaces will surface here without code changes.
func ParseMemory(raw json.RawMessage) (*Memory, error) {
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return nil, fmt.Errorf("unixctl: memory: unmarshal: %w", err)
	}
	out := make(map[string]int64)
	var parts []string
	for _, tok := range strings.Fields(text) {
		idx := strings.LastIndex(tok, ":")
		if idx < 0 {
			parts = append(parts, tok)
			continue
		}
		n, err := strconv.ParseInt(tok[idx+1:], 10, 64)
		if err != nil {
			parts = append(parts, tok)
			continue
		}
		if pre := tok[:idx]; pre != "" {
			parts = append(parts, pre)
		}
		name := strings.Join(parts, " ")
		parts = nil
		if name == "" {
			continue
		}
		out[name] = n
	}
	return &Memory{Usage: out}, nil
}
