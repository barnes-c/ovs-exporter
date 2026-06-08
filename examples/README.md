# examples/

Reference deployment material — answers "how do I run this in production?". **Not run by CI**, these are starting points operators copy + adapt.

| Path | Purpose |
|---|---|
| [`systemd/`](systemd/) | systemd unit, hardening profile, and env file for RPM/DEB host installs |
| [`k8s/`](k8s/) | DaemonSet + Service + ServiceMonitor for cluster deployments |
| [`grafana/`](grafana/) | Sample dashboards in OTel-spec metric naming (TBD) |

> Contributor end-to-end test infrastructure (docker-compose with a live OVS) lives under [`test/integration/smoke/`](../test/integration/smoke/), not here — that's not a deployment reference.
