# examples/systemd/

Reference systemd unit for host installations (RPM / DEB). Shipped automatically by goreleaser-built packages.

## Install via RPM (RHEL family)

```bash
sudo dnf install ovs-exporter
sudo systemctl enable --now ovs-exporter
sudo systemctl status ovs-exporter
```

The package places files at:

- `/usr/bin/ovs-exporter` — the binary
- `/usr/lib/systemd/system/ovs-exporter.service` — this unit
- `/etc/sysconfig/ovs-exporter` — env file (see [`sysconfig.ovs-exporter`](sysconfig.ovs-exporter))

## Install via DEB (Debian / Ubuntu)

Same as above but with `apt install` and env file at `/etc/default/ovs-exporter`.

## Configure

Edit the env file:

```bash
# RHEL family
sudoedit /etc/sysconfig/ovs-exporter
# Debian family
sudoedit /etc/default/ovs-exporter
```

Set `OPTIONS=` to whatever flags you need. Example: push to OTLP in addition to `/metrics`:

```ini
OPTIONS="--otel.metrics-exporter=otlp \
         --otel.otlp.endpoint=otel-collector.monitoring:4317"
```

Then reload:

```bash
sudo systemctl restart ovs-exporter
sudo journalctl -u ovs-exporter -f
```

## Pre-requisites

The unit assumes:

- A user `ovs-exporter` exists. If the package didn't create one (e.g. you built from source), `sudo useradd --system --no-create-home --shell /usr/sbin/nologin ovs-exporter` is enough.
- The user is a member of the `openvswitch` group — that's what gates access to `/var/run/openvswitch/{db.sock,ovs-vswitchd.<pid>.ctl}`. The unit's `SupplementaryGroups=openvswitch` handles this without needing the user to be a primary member.

## Hardening

The unit ships with a tight `systemd-analyze security` profile (~ score 1.5 / 10.0 means very safe). Key restrictions:

- `ProtectSystem=strict` — `/usr` is read-only, `/etc` and `/var` only writable as declared
- `RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6` — no raw sockets, no netlink
- `SystemCallFilter=@system-service ~@privileged @resources @debug @mount` — explicit syscall allowlist
- `MemoryDenyWriteExecute=true`, `LockPersonality=true`, `NoNewPrivileges=true`

If you need to loosen any of these (e.g. for a debugger), drop in `/etc/systemd/system/ovs-exporter.service.d/local.conf` rather than editing the shipped unit — `daemon-reload` then takes care of the override.
