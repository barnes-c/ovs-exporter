// Package datasource adapts the libovsdb client and unixctl scrape
// orchestrator to the collector.DataSource interface. It is the
// production data-path wiring; tests in the collector package inject
// their own fakes against the same interface.
//
// Either field may be nil — the wiring degrades gracefully when a
// transport is not configured (e.g. running on a host with no
// ovs-vswitchd) and the affected collector callbacks simply return no
// data.
package datasource

import (
	"github.com/barnes-c/ovs-exporter/collector"
	"github.com/barnes-c/ovs-exporter/internal/ovsdb"
	"github.com/barnes-c/ovs-exporter/internal/scrape"
	"github.com/barnes-c/ovs-exporter/internal/unixctl"
)

// Compile-time check that DataSource implements collector.DataSource.
var _ collector.DataSource = (*DataSource)(nil)

// DataSource composes the OVSDB and unixctl data sources into a single
// collector.DataSource. OVN-side fields are stubbed for M2.
type DataSource struct {
	ovs       *ovsdb.Client
	ovsScrape *scrape.Scraper[unixctl.OVSSnapshot]
}

// New constructs a DataSource. Either argument may be nil; the
// corresponding collector callbacks will see a nil view / snapshot and
// return no data points.
func New(ovs *ovsdb.Client, ovsScrape *scrape.Scraper[unixctl.OVSSnapshot]) *DataSource {
	return &DataSource{ovs: ovs, ovsScrape: ovsScrape}
}

// OVS returns a read-locked view over the OVSDB cache, or nil if no
// libovsdb client is wired.
func (d *DataSource) OVS() collector.OVSView {
	if d.ovs == nil {
		return nil
	}
	return d.ovs.View()
}

// UnixCtlOVS returns the most recently scraped ovs-vswitchd appctl
// snapshot, or nil before the first successful scrape (or when no
// scraper is wired).
func (d *DataSource) UnixCtlOVS() *unixctl.OVSSnapshot {
	if d.ovsScrape == nil {
		return nil
	}
	return d.ovsScrape.Snapshot()
}

// OVNNB, OVNSB, UnixCtlNorthd are M2 stubs.
func (d *DataSource) OVNNB() collector.OVNNBView                     { return nil }
func (d *DataSource) OVNSB() collector.OVNSBView                     { return nil }
func (d *DataSource) UnixCtlNorthd() collector.UnixCtlNorthdSnapshot { return nil }
