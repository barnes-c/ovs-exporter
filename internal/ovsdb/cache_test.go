package ovsdb

import (
	"testing"

	"github.com/barnes-c/ovs-exporter/internal/ovsdb/ovsmodel"
)

// A nil OVSView and a OVSView with no underlying cache must be safe to call
// iterator methods on. Collectors rely on this when the libovsdb client
// hasn't connected yet.
func TestOVSView_NilSafe(t *testing.T) {
	cases := map[string]*OVSView{
		"nil view":       nil,
		"view nil cache": {cache: nil},
	}
	for name, v := range cases {
		t.Run(name, func(t *testing.T) {
			calls := 0
			v.Bridges(func(*ovsmodel.Bridge) { calls++ })
			v.Ports(func(*ovsmodel.Port) { calls++ })
			v.Interfaces(func(*ovsmodel.Interface) { calls++ })
			if v.OpenvSwitch() != nil {
				t.Error("OpenvSwitch() should return nil for empty view")
			}
			if calls != 0 {
				t.Errorf("got %d iterator hits on empty view, want 0", calls)
			}
		})
	}
}
