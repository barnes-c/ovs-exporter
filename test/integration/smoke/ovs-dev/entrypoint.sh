#!/bin/sh
# Bring up ovsdb-server + ovs-vswitchd in the foreground for the smoke
# stack. ovs-ctl's start/stop targets background the daemons, so we run
# them directly and wait. SIGTERM propagation gives clean shutdown.

set -eu

DBDIR=/var/lib/openvswitch
RUNDIR=/var/run/openvswitch
LOGDIR=/var/log/openvswitch
DBPATH="$DBDIR/conf.db"
SCHEMA=/usr/share/openvswitch/vswitch.ovsschema

mkdir -p "$DBDIR" "$RUNDIR" "$LOGDIR"

if [ ! -f "$DBPATH" ]; then
    ovsdb-tool create "$DBPATH" "$SCHEMA"
fi

ovsdb-server \
    --remote=punix:"$RUNDIR/db.sock" \
    --remote=db:Open_vSwitch,Open_vSwitch,manager_options \
    --pidfile="$RUNDIR/ovsdb-server.pid" \
    --log-file="$LOGDIR/ovsdb-server.log" \
    --detach --no-chdir

ovs-vsctl --no-wait init

ovs-vswitchd \
    --pidfile="$RUNDIR/ovs-vswitchd.pid" \
    --log-file="$LOGDIR/ovs-vswitchd.log" \
    --mlockall \
    --detach --no-chdir

trap 'kill $(cat "$RUNDIR/ovs-vswitchd.pid" "$RUNDIR/ovsdb-server.pid" 2>/dev/null) 2>/dev/null; exit 0' TERM INT

tail -F "$LOGDIR/ovs-vswitchd.log" "$LOGDIR/ovsdb-server.log" &
wait $!
