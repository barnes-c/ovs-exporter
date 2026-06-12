# OVS Exporter

[![GitHub Release](https://img.shields.io/github/v/release/barnes-c/ovs-exporter)](https://github.com/barnes-c/ovs-exporter/releases/latest)
[![Build Status](https://github.com/barnes-c/ovs-exporter/actions/workflows/ci.yml/badge.svg)](https://github.com/barnes-c/ovs-exporter/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/barnes-c/ovs-exporter)](https://goreportcard.com/report/github.com/barnes-c/ovs-exporter)
[![GHCR](https://img.shields.io/badge/ghcr.io-barnes--c%2Fovs--exporter-blue?logo=github)](https://github.com/barnes-c/ovs-exporter/pkgs/container/ovs-exporter)
[![Docker Hub](https://img.shields.io/docker/pulls/barnesbiz/ovs-exporter?logo=docker)](https://hub.docker.com/r/barnesbiz/ovs-exporter)

OTel-native Prometheus exporter for [Open vSwitch (OVS)](https://www.openvswitch.org/).

Listens on port **10054** by default. Scrapes data from `ovsdb` (via libovsdb) and `ovs-vswitchd` (via the unixctl socket), exposes metrics at `/metrics`, and can additionally push metrics, traces, and logs to an OTLP endpoint.

## Quick start

Docker (mount the OVS sockets read-only):

```sh
docker run --rm -p 10054:10054 \
  -v /var/run/openvswitch:/var/run/openvswitch:ro \
  ghcr.io/barnes-c/ovs-exporter:latest
```

Binary:

```sh
make build
./ovs-exporter --ovs.db-socket=unix:/var/run/openvswitch/db.sock
```

Scrape: `curl localhost:10054/metrics`. Probes: `/healthz`, `/readyz`.

See [`examples/`](examples/) for systemd, Kubernetes DaemonSet + ServiceMonitor, and OTel Collector configs.

## Configuration

Key flags (full list via `--help`):

|          Flag          |               Default               |                Purpose                 |
| ---------------------- | ----------------------------------- | -------------------------------------- |
| `--web.listen-address` | `:10054`                            | Listen address                         |
| `--web.telemetry-path` | `/metrics`                          | Prometheus endpoint                    |
| `--web.prometheus`     | `true`                              | Disable for OTLP-push-only deployments |
| `--ovs.db-socket`      | `unix:/var/run/openvswitch/db.sock` | libovsdb endpoint                      |
| `--ovs.run-dir`        | `/var/run/openvswitch`              | unixctl socket directory               |
| `--cache.ttl`          | `15s`                               | unixctl scrape interval                |

### OTel exporters

Metrics, traces, and logs each have their own pipeline. Each can be set to `otlp`, `console`, or `none` (default off). `/metrics` is always served unless `--web.prometheus=false`.

|            Flag            |              Env              |                               Values                               |
| -------------------------- | ----------------------------- | ------------------------------------------------------------------ |
| `--otel.metrics-exporter`  | `OTEL_METRICS_EXPORTER`       | `otlp`, `console`, `none`                                          |
| `--otel.traces-exporter`   | `OTEL_TRACES_EXPORTER`        | `otlp`, `console`, `none`                                          |
| `--otel.logs-exporter`     | `OTEL_LOGS_EXPORTER`          | `otlp`, `console`, `none`                                          |
| `--otel.otlp.endpoint`     | `OTEL_EXPORTER_OTLP_ENDPOINT` | e.g. `localhost:4317`                                              |
| `--otel.otlp.protocol`     | `OTEL_EXPORTER_OTLP_PROTOCOL` | `grpc`, `http/protobuf`                                            |
| `--otel.otlp.interval`     | `OTEL_METRIC_EXPORT_INTERVAL` | OTLP push interval (default `15s`)                                 |
| `--otel.trace-sample-rate` | —                             | `0 < rate <= 1` (default `1.0`)                                    |
| `--otel.service-name`      | `OTEL_SERVICE_NAME`           | resource `service.name`                                            |
| `--otel.config-file`       | `OTEL_CONFIG_FILE`            | Declarative `otelconf` YAML (overrides all other `--otel.*` flags) |

Traces cover the scrape pipeline (`ovsdb`, `unixctl-ovs`, HTTP handler). Logs are emitted via the OTel logs SDK when an exporter is set.

## Metrics

All instruments use the OTel-spec naming convention (`ovs.<area>.<name>`).

|                                Metric                                |  Type   | Source  |
| -------------------------------------------------------------------- | ------- | ------- |
| `ovs.bridge.ports.count`                                             | gauge   | ovsdb   |
| `ovs.bridges.count`                                                  | gauge   | ovsdb   |
| `ovs.coverage.events`                                                | counter | unixctl |
| `ovs.datapath.cache.hit`                                             | counter | unixctl |
| `ovs.datapath.flows`                                                 | gauge   | unixctl |
| `ovs.datapath.interface.info`                                        | gauge   | unixctl |
| `ovs.datapath.lookups`                                               | counter | unixctl |
| `ovs.datapath.masks.hit`                                             | counter | unixctl |
| `ovs.interface.{errors,drops,collisions}`                            | counter | ovsdb   |
| `ovs.interface.{rx,tx}.{bytes,packets}`                              | counter | ovsdb   |
| `ovs.interface.admin_state`                                          | gauge   | ovsdb   |
| `ovs.interface.external_ids`                                         | gauge   | ovsdb   |
| `ovs.interface.if_index`                                             | gauge   | ovsdb   |
| `ovs.interface.info`                                                 | gauge   | ovsdb   |
| `ovs.interface.ingress_policing.{rate,burst,kpkts_rate,kpkts_burst}` | gauge   | ovsdb   |
| `ovs.interface.link_speed`                                           | gauge   | ovsdb   |
| `ovs.interface.link_state`                                           | gauge   | ovsdb   |
| `ovs.interface.mtu`                                                  | gauge   | ovsdb   |
| `ovs.interface.of_port`                                              | gauge   | ovsdb   |
| `ovs.interface.options`                                              | gauge   | ovsdb   |
| `ovs.interface.status`                                               | gauge   | ovsdb   |
| `ovs.memory.usage`                                                   | gauge   | unixctl |
| `ovs.ports.count`                                                    | gauge   | ovsdb   |
| `ovs.upcall.dump.duration`                                           | gauge   | unixctl |
| `ovs.upcall.flows.{current,max,limit}`                               | gauge   | unixctl |
| `ovs.upcall.handler.keys`                                            | gauge   | unixctl |

See [`collector/`](collector/) for the per-collector implementation.

## Development

```sh
make all              # fmt, vet, lint, build, test
make test             # go test -race ./...
make test-integration # smoke stack + integration tests (podman/docker)
make snapshot         # local goreleaser build
```

## License

[Apache-2.0](LICENSE)
