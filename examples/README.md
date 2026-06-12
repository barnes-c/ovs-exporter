# examples/

Reference deployment material. Answers "how do I run this in production?"

|              Path              |                                        Purpose                                         |
| ------------------------------ | -------------------------------------------------------------------------------------- |
| [`systemd/`](systemd/)         | systemd unit, hardening profile, and env file for RPM/DEB host installs                |
| [`k8s/`](k8s/)                 | DaemonSet + Service + ServiceMonitor for cluster deployments                           |
| [`otel-config/`](otel-config/) | Declarative OTel SDK YAML (otelconf) for richer pipelines than `OTEL_*` env vars allow |
| [`grafana/`](grafana/)         | Sample dashboards in OTel-spec metric naming                                           |
