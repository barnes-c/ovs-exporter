// Package ovsdb is the libovsdb client wrapper used by collectors to observe
// Open_vSwitch DB state. It hides three things from callers:
//
//   - Connection management: the underlying libovsdb client reconnects
//     automatically; the wrapper exposes only Connect / Close / Connected.
//   - Monitor lifecycle: MonitorAll is established once after Connect so the
//     in-process cache stays current via push updates from the server.
//   - Tracing: OTel spans around the Transact and MonitorAll RPCs.
//
// Collectors observe state through View(), which returns a read-locked
// snapshot accessor backed by the libovsdb cache. The accessor implements
// collector.OVSView so observe callbacks can iterate rows without touching
// the wire.
