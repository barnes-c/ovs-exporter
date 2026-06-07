# examples/smoke/

Local docker-compose stack for end-to-end smoke testing: a real `ovs-vswitchd` + `ovsdb-server` alongside the exporter built from this checkout.

## Run

From the repo root:

```bash
make smoke-up
curl -s localhost:10054/metrics | grep '^ovs_' | head
make smoke-logs   # follow exporter + ovs logs
make smoke-down   # tear down + remove volumes
```

## Exercise the mutation path

```bash
podman exec ovs ovs-vsctl add-br br-test
podman exec ovs ovs-vsctl add-port br-test eth-test \
    -- set interface eth-test type=internal
sleep 6   # wait one TTL cycle for the unixctl scraper to refresh
curl -s localhost:10054/metrics | \
    grep -E '^ovs_(bridges_count|interface|datapath|upcall)' | sort
```

## Caveats

- The exporter runs as `user: "0:0"` here because the OVS sockets are root-owned in the smoke image. **In production**, run the exporter as the distroless `nonroot` uid (`65532`) with `--group-add openvswitch` on the host (k8s reference in `../k8s/`).
- `NET_ADMIN` cap on the OVS container lets `ovs-vswitchd` bind datapath state inside the container's net namespace.
