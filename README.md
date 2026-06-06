### M1 — OVS only (ships a complete OVS exporter)

 1. go.mod + module path github.com/barnes-c/ovs-exporter; baseline deps (kingpin, slog, promslog, exporter-toolkit, OTel SDK + Prom
 exporter, libovsdb)
 2. internal/otel/ — MeterProvider + Prom Reader + optional OTLP. /metrics returns valid empty response.
 3. collector/collector.go — registry + Collector interface + filter flags
 4. internal/ovsdb/client.go — libovsdb wrapper for Open_vSwitch DB, MonitorAll + reconnect
 5. collector/ovs_bridges.go — simplest end-to-end. Validates the whole pipeline.
 6. collector/ovs_interfaces.go — per-row labels + counters
 7. internal/unixctl/ client + coverage parser + collector/ovs_coverage.go — validates unixctl path
 8. internal/probes/ — healthz + readyz
 9. Span wiring on libovsdb + unixctl calls
 10. Remaining OVS collectors: ports, datapath, memory, upcall, scrape meta; opt-in interface-info / interface-status / dp-if behind
 flags

### M2 — OVN on top of proven foundation

 1. internal/ovsdb/ — add NB + SB clients with TLS (--ovn.tls.* flags)
 2. internal/unixctl/ — add ovn-northd unixctl socket discovery
 3. collector/ovn_northd.go, ovn_nb.go, ovn_sb.go
 4. Opt-in ovn-logical-switch-info, ovn-sb-logical-flows, ovn-network-port

### M3 — Deployment artifacts

 1. Dockerfile (multi-stage distroless static:nonroot)
 2. examples/systemd/, examples/k8s/{daemonset,deployment-ovn-central,servicemonitor}.yaml
 3. examples/grafana/ — sample dashboards in the new OTel-spec schema
 4. README.md cardinality math + migration notes from greenpau/CERN forks
 5. CHANGELOG.md v1.0.0 entry
