# Collector

Package `collector` is the registry and Collector interface for the
ovs-exporter sub-collectors. Each sub-collector lives in its own file
(ovs_bridges.go, ovs_interfaces.go, …) and registers itself from init()
via registerCollector.

The interface follows OTel's callback model. A Collector goes through two
phases:

- Register is called once at startup. The collector creates its OTel
    instruments on the supplied metric.Meter and wires their callbacks.
    The callbacks read from the injected DataSource — never from a
    transport (libovsdb, unixctl) directly — so scrape latency stays
    O(view iteration) regardless of network conditions.
- Close is called at process shutdown.
