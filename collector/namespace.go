package collector

// Metric namespaces used by sub-collectors. Names are dot-separated OTel
// instrument names; the OTel Prometheus exporter converts the dots to
// underscores and appends `_total` to counters per the OTel Prom compat
// spec.
const (
	OVSNamespace = "ovs"
	OVNNamespace = "ovn"
)
