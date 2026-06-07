package unixctl

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// fakeServer listens on a unix socket and replies to JSON-RPC 1.0 requests
// by routing each method to the supplied handler. It tolerates the client
// hanging up between calls (one connection may serve multiple requests).
type fakeServer struct {
	t        *testing.T
	listener net.Listener
	path     string
	handlers map[string]func(request) response

	mu       sync.Mutex
	requests []request
}

func startFakeServer(t *testing.T, handlers map[string]func(request) response) *fakeServer {
	t.Helper()
	// macOS limits sun_path to 104 bytes; t.TempDir() embeds the test name
	// which blows that for some of our longer tests. Use a short stable
	// root instead.
	dir, err := os.MkdirTemp("", "ux-")
	if err != nil {
		t.Fatalf("mkdir tmp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	path := filepath.Join(dir, "f.ctl")
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	s := &fakeServer{t: t, listener: ln, path: path, handlers: handlers}
	t.Cleanup(func() { _ = ln.Close() })
	go s.serve()
	return s
}

func (s *fakeServer) serve() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		go s.handle(conn)
	}
}

func (s *fakeServer) handle(conn net.Conn) {
	defer conn.Close()
	dec := json.NewDecoder(conn)
	enc := json.NewEncoder(conn)
	for {
		var req request
		if err := dec.Decode(&req); err != nil {
			return
		}
		s.mu.Lock()
		s.requests = append(s.requests, req)
		s.mu.Unlock()
		handler, ok := s.handlers[req.Method]
		var resp response
		if !ok {
			resp = response{Error: json.RawMessage(`"unknown method"`)}
		} else {
			resp = handler(req)
		}
		resp.ID = req.ID
		if err := enc.Encode(resp); err != nil {
			return
		}
	}
}

func (s *fakeServer) seenRequests() []request {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]request, len(s.requests))
	copy(out, s.requests)
	return out
}

func TestNew_ValidatesConfig(t *testing.T) {
	if _, err := New(Config{Logger: discardLogger()}); err == nil {
		t.Error("expected error for missing SocketPath/RunDir+Daemon")
	}
	if _, err := New(Config{SocketPath: "/tmp/x.ctl"}); err == nil {
		t.Error("expected error for missing Logger")
	}
	if _, err := New(Config{SocketPath: "/tmp/x.ctl", Logger: discardLogger()}); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestClient_Call_ReturnsResult(t *testing.T) {
	srv := startFakeServer(t, map[string]func(request) response{
		"coverage/show": func(request) response {
			return response{Result: json.RawMessage(`"coverage text"`)}
		},
	})
	c, err := New(Config{SocketPath: srv.path, Logger: discardLogger()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	result, err := c.Call(context.Background(), "coverage/show")
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if string(result) != `"coverage text"` {
		t.Errorf("result = %s, want %s", string(result), `"coverage text"`)
	}
	reqs := srv.seenRequests()
	if len(reqs) != 1 {
		t.Fatalf("server saw %d requests, want 1", len(reqs))
	}
	if reqs[0].Method != "coverage/show" {
		t.Errorf("method = %s, want coverage/show", reqs[0].Method)
	}
	if reqs[0].Params == nil {
		t.Error("params should be encoded as [], not omitted")
	}
}

func TestClient_Call_WithParams(t *testing.T) {
	srv := startFakeServer(t, map[string]func(request) response{
		"memory/show": func(request) response {
			return response{Result: json.RawMessage(`"ok"`)}
		},
	})
	c, err := New(Config{SocketPath: srv.path, Logger: discardLogger()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	if _, err := c.Call(context.Background(), "memory/show", "verbose", "json"); err != nil {
		t.Fatalf("Call: %v", err)
	}
	reqs := srv.seenRequests()
	if len(reqs) != 1 || len(reqs[0].Params) != 2 {
		t.Fatalf("params not forwarded: reqs=%+v", reqs)
	}
	if reqs[0].Params[0] != "verbose" || reqs[0].Params[1] != "json" {
		t.Errorf("params = %v, want [verbose json]", reqs[0].Params)
	}
}

func TestClient_Call_PropagatesServerError(t *testing.T) {
	srv := startFakeServer(t, map[string]func(request) response{
		"broken": func(request) response {
			return response{Error: json.RawMessage(`"boom"`)}
		},
	})
	c, err := New(Config{SocketPath: srv.path, Logger: discardLogger()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	_, err = c.Call(context.Background(), "broken")
	var cerr *CallError
	if !errors.As(err, &cerr) {
		t.Fatalf("err = %v, want *CallError", err)
	}
	if cerr.Method != "broken" {
		t.Errorf("CallError.Method = %s, want broken", cerr.Method)
	}
}

func TestClient_Call_ReconnectsAfterDrop(t *testing.T) {
	var calls int
	srv := startFakeServer(t, map[string]func(request) response{
		"coverage/show": func(request) response {
			calls++
			return response{Result: json.RawMessage(`"ok"`)}
		},
	})
	c, err := New(Config{SocketPath: srv.path, Logger: discardLogger()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	if _, err := c.Call(context.Background(), "coverage/show"); err != nil {
		t.Fatalf("first Call: %v", err)
	}

	// Simulate the server killing the connection.
	c.mu.Lock()
	c.disconnectLocked()
	c.mu.Unlock()

	if _, err := c.Call(context.Background(), "coverage/show"); err != nil {
		t.Fatalf("second Call (after reconnect): %v", err)
	}
	if calls != 2 {
		t.Errorf("server saw %d calls, want 2", calls)
	}
}

func TestClient_Call_DialFailure(t *testing.T) {
	c, err := New(Config{SocketPath: "/nonexistent/path/x.ctl", Logger: discardLogger()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	if _, err := c.Call(context.Background(), "coverage/show"); err == nil {
		t.Error("expected dial error")
	}
}
