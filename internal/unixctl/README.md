# unixctl

Package unixctl is a minimal JSON-RPC 1.0 client for the unix control sockets exposed by OVS daemons (ovs-vswitchd, ovsdb-server).
The wire format is concatenated JSON objects on the connection; each Call exchanges exactly one request/response pair before returning.

The client is single-in-flight: concurrent Call invocations serialize on the same socket so request/response pairs cannot interleave.
On any transport error the connection is dropped and the next Call re-dials — re-running DiscoverSocket so PID changes after a daemon restart are handled transparently.
