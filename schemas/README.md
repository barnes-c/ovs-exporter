# OVS Schemas

This directory contains the Open vSwitch (OVS) database schema used by the exporter to understand and monitor the OVSDB.

## vswitch.ovsschema

The `vswitch.ovsschema` file is the schema definition for the OVS database. It describes all tables, columns, and their types.

**Source:** This is a vendored floor version from OVS 3.1, downloaded from the [Open vSwitch upstream repository](https://raw.githubusercontent.com/openvswitch/ovs/main/vswitchd/vswitch.ovsschema).

**Usage:** The schema is used by the [libovsdb](https://github.com/ovn-kubernetes/libovsdb) client library to model the database and is referenced by the [internal/ovsdb](../internal/ovsdb) package when building the database model.

## Schema Drift Monitoring

A [weekly workflow](../.github/workflows/schema-watch.yml) runs every Monday to check for schema drift between our vendored version and the upstream main branch. If new columns are added upstream, the workflow opens an issue so we can evaluate whether to expose metrics for them.
