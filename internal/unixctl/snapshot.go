package unixctl

// OVSSnapshot holds the parsed appctl results scraped from ovs-vswitchd.
// Fields are populated on a best-effort basis; an absent or failed parse
// leaves the corresponding pointer nil so collectors can degrade
// gracefully. The struct is consumed via atomic.Pointer swap from the
// scrape package, so callers must treat snapshot fields as immutable.
type OVSSnapshot struct {
	Coverage *Coverage
	Memory   *Memory
	DPIF     *DPIF
	Upcall   *Upcall
}
