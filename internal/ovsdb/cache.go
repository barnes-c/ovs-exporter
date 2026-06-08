package ovsdb

import (
	"github.com/ovn-kubernetes/libovsdb/cache"

	"github.com/barnes-c/ovs-exporter/internal/ovsdb/ovsmodel"
)

// OVSView is a thin read-locked accessor over the libovsdb cache for the
// Open_vSwitch DB. Iterator methods invoke fn for each row of the named
// table; iteration order is unspecified. The cache handles read locking
// internally, so callbacks may iterate concurrently with monitor updates.
type OVSView struct {
	cache *cache.TableCache
}

func (v *OVSView) Bridges(fn func(*ovsmodel.Bridge)) {
	v.eachRow("Bridge", func(m any) {
		if b, ok := m.(*ovsmodel.Bridge); ok {
			fn(b)
		}
	})
}

func (v *OVSView) Ports(fn func(*ovsmodel.Port)) {
	v.eachRow("Port", func(m any) {
		if p, ok := m.(*ovsmodel.Port); ok {
			fn(p)
		}
	})
}

func (v *OVSView) Interfaces(fn func(*ovsmodel.Interface)) {
	v.eachRow("Interface", func(m any) {
		if i, ok := m.(*ovsmodel.Interface); ok {
			fn(i)
		}
	})
}

// OpenvSwitch returns the root row of the Open_vSwitch table (which has
// maxRows: 1). Returns nil if the cache does not yet contain it.
func (v *OVSView) OpenvSwitch() *ovsmodel.OpenvSwitch {
	var out *ovsmodel.OpenvSwitch
	v.eachRow("Open_vSwitch", func(m any) {
		if s, ok := m.(*ovsmodel.OpenvSwitch); ok && out == nil {
			out = s
		}
	})
	return out
}

func (v *OVSView) eachRow(table string, fn func(any)) {
	if v == nil || v.cache == nil {
		return
	}
	t := v.cache.Table(table)
	if t == nil {
		return
	}
	for _, row := range t.Rows() {
		if row == nil {
			continue
		}
		fn(row)
	}
}
