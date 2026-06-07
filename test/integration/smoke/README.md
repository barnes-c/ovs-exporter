# test/integration/smoke/

End-to-end test fixture: a real `ovs-vswitchd` + `ovsdb-server` alongside the exporter built from this checkout. Consumed by `../integration_test.go`.

## Run

From the repo root:

```bash
make test-integration    # the automated path — brings the stack up + runs tagged tests
# or just the stack, for manual poking:
make smoke-up
curl -s localhost:10054/metrics | grep '^ovs_' | head
make smoke-logs
make smoke-down
```

## Exercise the mutation path manually

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
