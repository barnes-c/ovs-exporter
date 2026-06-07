// Package unixctl is a minimal JSON-RPC 1.0 client for the unix control
// sockets exposed by OVS / OVN daemons (ovs-vswitchd, ovsdb-server,
// ovn-northd, ovn-controller). The wire format is concatenated JSON
// objects on the connection; each Call exchanges exactly one
// request/response pair before returning.
//
// The client is single-in-flight: concurrent Call invocations serialize on
// the same socket so request/response pairs cannot interleave. On any
// transport error the connection is dropped and the next Call re-dials —
// re-running DiscoverSocket so PID changes after a daemon restart are
// handled transparently.
package unixctl

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"
)

// Config configures a unixctl Client. Either SocketPath or both RunDir and
// Daemon must be set. When SocketPath is empty the client uses
// DiscoverSocket(RunDir, Daemon) on every (re)connect.
type Config struct {
	SocketPath  string
	RunDir      string
	Daemon      string
	DialTimeout time.Duration
	CallTimeout time.Duration
	Logger      *slog.Logger
}

// Client is a unixctl JSON-RPC client.
type Client struct {
	cfg    Config
	log    *slog.Logger
	mu     sync.Mutex
	conn   net.Conn
	dec    *json.Decoder
	enc    *json.Encoder
	nextID int64
}

// New validates cfg and returns an unconnected Client; the first Call dials
// the socket lazily.
func New(cfg Config) (*Client, error) {
	if cfg.SocketPath == "" && (cfg.RunDir == "" || cfg.Daemon == "") {
		return nil, fmt.Errorf("unixctl: SocketPath or (RunDir + Daemon) is required")
	}
	if cfg.Logger == nil {
		return nil, fmt.Errorf("unixctl: Logger is required")
	}
	if cfg.DialTimeout == 0 {
		cfg.DialTimeout = 2 * time.Second
	}
	if cfg.CallTimeout == 0 {
		cfg.CallTimeout = 5 * time.Second
	}
	return &Client{cfg: cfg, log: cfg.Logger}, nil
}

type request struct {
	Method string `json:"method"`
	Params []any  `json:"params"`
	ID     int64  `json:"id"`
}

type response struct {
	Result json.RawMessage `json:"result"`
	Error  json.RawMessage `json:"error"`
	ID     int64           `json:"id"`
}

// CallError is returned when the unixctl server replies with a non-null
// error field. Transport-level failures are returned as plain errors.
type CallError struct {
	Method string
	Cause  string
}

func (e *CallError) Error() string {
	return fmt.Sprintf("unixctl: %s: %s", e.Method, e.Cause)
}

// Call sends a JSON-RPC 1.0 request and returns the raw result payload. On
// any transport failure the underlying connection is reset so the next
// Call re-dials.
func (c *Client) Call(ctx context.Context, method string, params ...any) (json.RawMessage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.ensureConnectedLocked(ctx); err != nil {
		return nil, err
	}

	if params == nil {
		params = []any{}
	}
	c.nextID++
	req := request{Method: method, Params: params, ID: c.nextID}

	deadline := time.Now().Add(c.cfg.CallTimeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if err := c.conn.SetDeadline(deadline); err != nil {
		c.disconnectLocked()
		return nil, fmt.Errorf("unixctl: set deadline: %w", err)
	}

	if err := c.enc.Encode(req); err != nil {
		c.disconnectLocked()
		return nil, fmt.Errorf("unixctl: encode %s: %w", method, err)
	}

	var resp response
	if err := c.dec.Decode(&resp); err != nil {
		c.disconnectLocked()
		return nil, fmt.Errorf("unixctl: decode %s: %w", method, err)
	}

	if len(resp.Error) > 0 && string(resp.Error) != "null" {
		return nil, &CallError{Method: method, Cause: string(resp.Error)}
	}
	return resp.Result, nil
}

// Close releases the underlying socket connection.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.disconnectLocked()
	return nil
}

func (c *Client) ensureConnectedLocked(ctx context.Context) error {
	if c.conn != nil {
		return nil
	}
	socketPath := c.cfg.SocketPath
	if socketPath == "" {
		path, err := DiscoverSocket(c.cfg.RunDir, c.cfg.Daemon)
		if err != nil {
			return err
		}
		socketPath = path
	}
	dialer := net.Dialer{Timeout: c.cfg.DialTimeout}
	conn, err := dialer.DialContext(ctx, "unix", socketPath)
	if err != nil {
		return fmt.Errorf("unixctl: dial %s: %w", socketPath, err)
	}
	c.conn = conn
	c.dec = json.NewDecoder(conn)
	c.enc = json.NewEncoder(conn)
	return nil
}

func (c *Client) disconnectLocked() {
	if c.conn != nil {
		_ = c.conn.Close()
		c.conn = nil
		c.dec = nil
		c.enc = nil
	}
}
