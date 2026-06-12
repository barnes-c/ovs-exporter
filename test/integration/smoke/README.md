# test/integration/smoke/

End-to-end test fixture: a real `ovs-vswitchd` + `ovsdb-server` and an OTel Collector, alongside the exporter built from this checkout. Consumed by `../integration_test.go`.

The exporter is wired up two ways at once so the suite can assert both surfaces:

- **Direct Prometheus**: `localhost:10054/metrics` — the SDK's pull reader.
- **OTLP push pipeline**: the exporter's OTel SDK is driven by `otel-exporter-config.yaml` (mounted at `OTEL_CONFIG_FILE`) and pushes metrics, traces, and logs to `otel-collector:4317`. The collector re-exposes received metrics on `localhost:8889` and its own self-telemetry on `localhost:8888`.

## Run the tests

From the repo root:

```bash
make test-integration    # the automated path — brings the stack up + runs tagged tests
# or just the stack, for manual poking:
make smoke-up
curl -s localhost:10054/metrics | grep '^ovs_' | head           # direct Prom
curl -s localhost:8889/metrics  | grep '^ovs_' | head           # via OTel pipeline
curl -s localhost:8888/metrics  | grep '^otelcol_receiver_'     # collector self-telemetry
make smoke-logs
make smoke-down
```

## Tinker — Grafana dashboards over the OTel stack

`make smoke-tinker` brings the same stack up *plus* a fully OTel-native observability backend so you can build dashboards by hand. Data flow is push-only end to end — no scrape jobs anywhere:

```
ovs-exporter ── OTLP ──▶ otel-collector ── OTLP push ──▶ ┌── Prometheus  (metrics, OTLP-receiver mode)
                                                         ├── Tempo       (traces)
                                                         └── Loki        (logs)
                                                                  ▲
                                                                  └── Grafana (PromQL / TraceQL / LogQL)
```

```bash
make smoke-tinker
open http://localhost:3000     # Grafana, anonymous Admin, no login
make smoke-down                # tears down profile services too
```

Pre-provisioned datasources (UIDs `prometheus` / `tempo` / `loki`, wired together so trace → log and trace → metric jumps work out of the box). No starter dashboards — `New → Dashboard` and explore. When you have a dashboard worth keeping, export the JSON into `examples/grafana/dashboards/`.

### Where to look in Grafana

| Use case | Datasource | Starting point |
|---|---|---|
| OVS bridge / port counts | Prometheus | `ovs_bridges_count`, `ovs_ports_count` |
| Datapath / upcall throughput | Prometheus | `rate(ovs_datapath_lookups_total[1m])`, `ovs_upcall_flows_current` |
| Exporter HTTP latency | Prometheus | `http_server_request_duration_*` (from otelhttp) |
| Spans from /metrics scrapes | Tempo | TraceQL `{ resource.service.name = "ovs-exporter" }` |
| Exporter logs | Loki | LogQL `{service_name="ovs-exporter"}` |

Resource attributes (`service.name`, `deployment.environment`) are promoted to Prometheus labels by `tinker/prometheus.yml` so you can filter without `target_info` joins.

### Exercise the mutation path manually

```bash
podman exec ovs ovs-vsctl add-br br-test
podman exec ovs ovs-vsctl add-port br-test eth-test \
    -- set interface eth-test type=internal
sleep 6   # wait one TTL cycle for the unixctl scraper to refresh
curl -s localhost:10054/metrics | \
    grep -E '^ovs_(bridges_count|interface|datapath|upcall)' | sort
```

## Caveats

- The exporter runs as `user: "0:0"` here because the OVS sockets are root-owned in the smoke image. **In production**, run the exporter as the distroless `nonroot` uid (`65532`) with `--group-add openvswitch` on the host. See `../../../examples/k8s/` for a reference manifest.
- `NET_ADMIN` cap on the OVS container lets `ovs-vswitchd` bind datapath state inside the container's net namespace.
- The tinker stack stores data in named volumes (`prometheus-data`, `tempo-data`, `loki-data`, `grafana-data`). `make smoke-down` deletes them. If you want dashboards to persist across restarts, export them via Grafana's UI rather than relying on the volume.
