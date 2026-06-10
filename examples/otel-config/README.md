# examples/otel-config/

Declarative YAML configuration for the OTel SDK pipeline. Use this **instead of** `--otel.*` flags when you want a single, fleet-portable config file.

## When to use this

Pick YAML over flags when:

- You're running ovs-exporter alongside other OTel-instrumented services and want one shared config for all of them (the schema is language-agnostic).
- You need richer pipelines than the flags expose: multiple processors, custom views, exemplar filters, parent-based samplers, or composite propagators.
- You're standardizing your fleet on OTel's [declarative configuration spec](https://opentelemetry.io/docs/specs/otel/configuration/file-configuration/).

Stick with flags for simple cases — flags cover the common "scrape `/metrics`, push to one OTLP collector" path with less ceremony.

## How activation works

```bash
# Flag
ovs-exporter --otel.config-file=/etc/ovs-exporter/otel-config.yaml

# Environment (per the spec)
export OTEL_CONFIG_FILE=/etc/ovs-exporter/otel-config.yaml
ovs-exporter
```

When `--otel.config-file` is set, **all other `--otel.*` flags are ignored** with a startup warning listing them. This matches the OTel spec rule: *"When `OTEL_CONFIG_FILE` is set, all other environment variables besides those referenced in the configuration file for environment variable substitution MUST be ignored."*

Non-OTel flags (`--web.listen-address`, `--collector.*`, `--ovs.*`) continue to work as normal.

## The Prometheus reader carve-out

`/metrics` is always served by ovs-exporter from its built-in Prometheus reader — that's the product. The YAML config **must not declare a pull reader** (e.g. `meter_provider.readers[].pull.exporter.prometheus`). If it does, the exporter refuses to start with a clear error.

Push-based metric readers (`periodic` with `otlp_grpc` / `otlp_http`) work as expected and run alongside the Prometheus reader.

> This is a current limitation of `go.opentelemetry.io/contrib/otelconf` (v0.24.0, alpha): the package does not expose the constructed reader list, so we can't extract the YAML-declared Prometheus reader's HTTP handler and mount it on our own server. Tracked in code as a `TODO(otelconf):` comment in `internal/otel/config_file.go`. When upstream stabilizes, this carve-out will be removed.

## Files

| File | Purpose |
|---|---|
| [`full-pipeline.yaml`](full-pipeline.yaml) | Reference config: OTLP push for metrics + traces + logs, 10% head sampling, tracecontext/baggage propagation. Edit endpoints and ratios to taste. |

## References

- [OTel declarative configuration spec](https://opentelemetry.io/docs/specs/otel/configuration/file-configuration/)
- [Schema](https://github.com/open-telemetry/opentelemetry-configuration) — JSON Schema for validation in your editor
- [`go.opentelemetry.io/contrib/otelconf`](https://pkg.go.dev/go.opentelemetry.io/contrib/otelconf) — the Go SDK parser we use
