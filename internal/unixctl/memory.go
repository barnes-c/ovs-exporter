package unixctl

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Memory is the parsed output of `ovs-appctl memory/show`. The command
// emits a single line of space-separated `key:N` tokens reported by the
// ovs-vswitchd memory_report handler (handlers, ofconns, ports, rules,
// revalidators, etc.).
type Memory struct {
	Usage map[string]int64
}

// ParseMemory decodes the JSON-RPC response from memory/show. Tokens
// without a colon or with a non-integer value are skipped rather than
// failing the whole parse; future OVS versions may add new keys that
// don't yet match the simple `name:int` shape.
func ParseMemory(raw json.RawMessage) (*Memory, error) {
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return nil, fmt.Errorf("unixctl: memory: unmarshal: %w", err)
	}
	out := make(map[string]int64)
	for _, tok := range strings.Fields(text) {
		name, val, ok := strings.Cut(tok, ":")
		if !ok {
			continue
		}
		n, err := strconv.ParseInt(val, 10, 64)
		if err != nil {
			continue
		}
		out[name] = n
	}
	return &Memory{Usage: out}, nil
}
