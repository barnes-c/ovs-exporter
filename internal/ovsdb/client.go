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
package ovsdb

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/cenkalti/backoff/v5"
	"github.com/go-logr/logr"
	libovsdb "github.com/ovn-kubernetes/libovsdb/client"
	"go.opentelemetry.io/otel/trace"

	"github.com/barnes-c/ovs-exporter/internal/ovsdb/ovsmodel"
)

// Config configures the Open_vSwitch DB client.
type Config struct {
	// Endpoint is the libovsdb connection string, e.g.
	// "unix:/var/run/openvswitch/db.sock" or "tcp:host:6640".
	Endpoint string
	// ReconnectTimeout is the per-attempt connect timeout for the reconnect
	// loop. Defaults to 2s.
	ReconnectTimeout time.Duration
	// Logger receives wrapper-level log lines.
	Logger *slog.Logger
	// Tracer is used by the span helpers. Optional; when nil, no spans are
	// emitted.
	Tracer trace.Tracer
}

// Client is the wrapped libovsdb client.
type Client struct {
	cfg    Config
	log    *slog.Logger
	tracer trace.Tracer
	inner  libovsdb.Client
}

// Connect dials cfg.Endpoint, performs the OVSDB handshake, and establishes a
// MonitorAll so the in-process cache is populated and kept current. The
// client reconnects automatically on transport errors.
func Connect(ctx context.Context, cfg Config) (*Client, error) {
	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("ovsdb: Endpoint is required")
	}
	if cfg.Logger == nil {
		return nil, fmt.Errorf("ovsdb: Logger is required")
	}
	if cfg.ReconnectTimeout == 0 {
		cfg.ReconnectTimeout = 2 * time.Second
	}

	dbModel, err := ovsmodel.FullDatabaseModel()
	if err != nil {
		return nil, fmt.Errorf("ovsdb: build client db model: %w", err)
	}

	// Bridge our slog.Logger into libovsdb so reconnect attempts, timeouts,
	// and monitor updates are visible through the same log stream the rest
	// of the exporter uses.
	libovsdbLogger := logr.FromSlogHandler(cfg.Logger.With("component", "libovsdb").Handler())

	inner, err := libovsdb.NewOVSDBClient(
		dbModel,
		libovsdb.WithEndpoint(cfg.Endpoint),
		libovsdb.WithReconnect(cfg.ReconnectTimeout, backoff.NewExponentialBackOff()),
		libovsdb.WithLogger(&libovsdbLogger),
	)
	if err != nil {
		return nil, fmt.Errorf("ovsdb: construct client: %w", err)
	}

	c := &Client{cfg: cfg, log: cfg.Logger, tracer: cfg.Tracer, inner: inner}

	connectCtx, connectSpan := c.startSpan(ctx, "ovsdb.connect", attrOp("connect"))
	if err := inner.Connect(connectCtx); err != nil {
		endSpan(connectSpan, err)
		return nil, fmt.Errorf("ovsdb: connect to %s: %w", cfg.Endpoint, err)
	}
	endSpan(connectSpan, nil)

	monitorCtx, monitorSpan := c.startSpan(ctx, "ovsdb.monitor", attrOp("monitor_all"))
	if _, err := inner.MonitorAll(monitorCtx); err != nil {
		endSpan(monitorSpan, err)
		inner.Close()
		return nil, fmt.Errorf("ovsdb: MonitorAll: %w", err)
	}
	endSpan(monitorSpan, nil)

	cfg.Logger.Info("ovsdb client connected", "endpoint", cfg.Endpoint)
	return c, nil
}

// Close disconnects from the server and releases resources.
func (c *Client) Close() error {
	if c.inner == nil {
		return nil
	}
	c.inner.Close()
	return nil
}

// Connected reports whether the underlying libovsdb client currently has an
// active connection.
func (c *Client) Connected() bool {
	return c.inner != nil && c.inner.Connected()
}

// View returns a read-locked accessor over the in-process cache. Returns nil
// if the client has been closed.
func (c *Client) View() *OVSView {
	if c.inner == nil {
		return nil
	}
	return &OVSView{cache: c.inner.Cache()}
}
