# examples/k8s/

Reference Kubernetes manifests for running ovs-exporter on hypervisor nodes that are also k8s workers (typical OpenStack-on-k8s, kubevirt, or kube-ovn-style deployments).

## What's here

| File | Purpose |
|---|---|
| [`daemonset.yaml`](daemonset.yaml) | Runs ovs-exporter on every node matching the selector. Mounts `/var/run/openvswitch` read-only from the host. |
| [`service.yaml`](service.yaml) | Headless Service over the DaemonSet pods. Required so the Prometheus Operator can discover scrape endpoints. |
| [`servicemonitor.yaml`](servicemonitor.yaml) | Prometheus Operator CRD that wires the Service into your Prometheus scrape config. |

## Three things you must edit before applying

1. **`nodeSelector`** in `daemonset.yaml` — match the label your fleet uses to mark hypervisors. Common values: `node-role.kubernetes.io/compute: ""`, `openstack.org/compute: "true"`. Without this, the DaemonSet lands on every node including ones without ovs-vswitchd.

2. **`supplementalGroups`** in `daemonset.yaml` — set to the host's `openvswitch` group GID. The container runs as uid 65532 (nonroot distroless); the OVS unix sockets in `/var/run/openvswitch/` are group-owned by `openvswitch`. Without the supplemental GID, the container cannot read the sockets and you'll see `ovsdb: dial: permission denied` in the logs. Check the GID:

   ```bash
   ssh hypervisor "getent group openvswitch"
   # → openvswitch:x:388:
   ```

   RHEL family is typically 388; Debian is typically 116. EDIT the manifest to match.

3. **`release` label** in `servicemonitor.yaml` — the Prometheus Operator's `serviceMonitorSelector` decides which ServiceMonitors a given Prometheus instance picks up. Match what your install uses (default for `kube-prometheus-stack` is `release: kube-prometheus-stack`).

   Check:
   ```bash
   kubectl -n monitoring get prometheus -o yaml | grep -A5 serviceMonitorSelector
   ```

## Apply

```bash
kubectl apply -f service.yaml
kubectl apply -f daemonset.yaml
kubectl apply -f servicemonitor.yaml
```

Verify:

```bash
kubectl -n monitoring get ds ovs-exporter
kubectl -n monitoring get pods -l app.kubernetes.io/name=ovs-exporter
kubectl -n monitoring logs -l app.kubernetes.io/name=ovs-exporter --tail=20

# Sanity scrape from inside the cluster:
kubectl -n monitoring port-forward ds/ovs-exporter 10054:10054
curl -s localhost:10054/metrics | grep '^ovs_' | head
```

## Image pinning

The DaemonSet pins to `ghcr.io/barnes-c/ovs-exporter:v0.1.0`. Bump explicitly — `:latest` floating tags don't work with the immutable-image pattern Kubernetes assumes for rollouts. You can also pin to a SHA digest for full reproducibility:

```yaml
image: ghcr.io/barnes-c/ovs-exporter@sha256:<digest>
```

## Hardening

The pod spec ships with:
- `runAsNonRoot: true`, uid 65532
- `readOnlyRootFilesystem: true`
- `allowPrivilegeEscalation: false`
- `capabilities.drop: [ALL]`
- `seccompProfile.type: RuntimeDefault`
- Only `supplementalGroups` is escalated (read access to OVS sockets), nothing else.

If your cluster has Pod Security Standards enforced at the namespace level, the manifest should pass `restricted` mode out of the box.

## Why hostNetwork?

The DaemonSet uses `hostNetwork: true` so each pod binds `:10054` directly on the node IP, rather than going through `kube-proxy` for every Prometheus scrape. Saves a hop and makes the metric endpoint reachable from the host's node IP if the operator wants to scrape from outside the cluster. The tradeoff is that the host's port 10054 must be free — adjust `--web.listen-address` if it's taken.
