package ovsdb

import (
	"context"
	"errors"
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
	Endpoint         string
	ReconnectTimeout time.Duration
	Logger           *slog.Logger
	Tracer           trace.Tracer
}

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

// Healthy returns nil when the client is initialised and connected,
// otherwise a descriptive error.
func (c *Client) Healthy() error {
	if c == nil || c.inner == nil {
		return errors.New("ovsdb: client not initialised")
	}
	if !c.inner.Connected() {
		return errors.New("ovsdb: client not connected")
	}
	return nil
}

func (c *Client) View() *OVSView {
	if c.inner == nil {
		return nil
	}
	return &OVSView{cache: c.inner.Cache()}
}
