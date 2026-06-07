package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/procfs"

	"github.com/barnes-c/ovs-exporter/internal/otel"
)

const testListenAddress = "localhost:11054"

var testBinary = "./ovs-exporter"

// setupOTelForTest builds the OTel pipeline with all push exporters set to "none"
// so the test never tries to dial an OTLP collector.
func setupOTelForTest(t *testing.T) *otel.Result {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	res, err := otel.Setup(context.Background(), logger, otel.Config{
		ServiceName:    "ovs-exporter-test",
		ServiceVersion: "test",
	})
	if err != nil {
		t.Fatalf("otel.Setup: %v", err)
	}
	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithCancel(context.Background())
		defer cancel()
		_ = res.Shutdown(shutdownCtx)
	})
	return res
}

func TestBuildHandlerRoutes(t *testing.T) {
	res := setupOTelForTest(t)
	h, err := buildHandler(res, "/metrics", nil)
	if err != nil {
		t.Fatalf("buildHandler: %v", err)
	}

	tests := []struct {
		path       string
		wantCode   int
		wantSubstr string
	}{
		{"/healthz", http.StatusOK, "ok"},
		{"/readyz", http.StatusOK, ""},
		{"/metrics", http.StatusOK, "target_info"},
		{"/", http.StatusOK, "OVS Exporter"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			h.ServeHTTP(rec, req)
			if rec.Code != tt.wantCode {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantCode)
			}
			if !strings.Contains(rec.Body.String(), tt.wantSubstr) {
				t.Errorf("body missing %q; got:\n%s", tt.wantSubstr, rec.Body.String())
			}
		})
	}
}

// TestBuildHandlerMetricsAtRoot covers the edge case where the operator sets
// --web.telemetry-path=/, which suppresses the landing page.
func TestBuildHandlerMetricsAtRoot(t *testing.T) {
	res := setupOTelForTest(t)
	h, err := buildHandler(res, "/", nil)
	if err != nil {
		t.Fatalf("buildHandler: %v", err)
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "target_info") {
		t.Errorf("expected metrics at /, got body:\n%s", rec.Body.String())
	}
}

// TestFileDescriptorLeak boots the compiled binary and scrapes /metrics
// repeatedly, asserting the open-FD count does not grow. Catches leaks in
// HTTP response handling, libovsdb reconnects, unixctl sockets, etc. Skipped
// on platforms without /proc.
func TestFileDescriptorLeak(t *testing.T) {
	if _, err := os.Stat(testBinary); err != nil {
		t.Skipf("ovs-exporter binary not available, run `make build` first: %s", err)
	}
	fs, err := procfs.NewDefaultFS()
	if err != nil {
		t.Skipf("proc filesystem not available: %s", err)
	}
	if _, err := fs.Stat(); err != nil {
		t.Errorf("unable to read process stats: %s", err)
	}

	cmd := exec.Command(testBinary, "--web.listen-address", testListenAddress)
	test := func(pid int) error {
		if err := queryExporter(testListenAddress); err != nil {
			return err
		}
		proc, err := procfs.NewProc(pid)
		if err != nil {
			return err
		}
		fdsBefore, err := proc.FileDescriptors()
		if err != nil {
			return err
		}
		for range 5 {
			if err := queryExporter(testListenAddress); err != nil {
				return err
			}
		}
		fdsAfter, err := proc.FileDescriptors()
		if err != nil {
			return err
		}
		if want, have := len(fdsBefore), len(fdsAfter); want != have {
			return fmt.Errorf("want %d open file descriptors after metrics scrape, have %d", want, have)
		}
		return nil
	}

	if err := startBinaryAndRun(cmd, testListenAddress, test); err != nil {
		t.Error(err)
	}
}

// queryExporter GETs /metrics and asserts a 200 response.
func queryExporter(address string) error {
	resp, err := http.Get(fmt.Sprintf("http://%s/metrics", address))
	if err != nil {
		return err
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if err := resp.Body.Close(); err != nil {
		return err
	}
	if want, have := http.StatusOK, resp.StatusCode; want != have {
		return fmt.Errorf("want /metrics status code %d, have %d. Body:\n%s", want, have, b)
	}
	return nil
}

// startBinaryAndRun starts a subprocess, waits for it to become reachable,
// runs fn with the subprocess PID, then kills the subprocess.
func startBinaryAndRun(cmd *exec.Cmd, address string, fn func(pid int) error) error {
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start command: %s", err)
	}
	time.Sleep(50 * time.Millisecond)
	for i := range 10 {
		if err := queryExporter(address); err == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
		if cmd.Process == nil || i == 9 {
			return fmt.Errorf("can't start command")
		}
	}

	errc := make(chan error)
	go func(pid int) {
		errc <- fn(pid)
	}(cmd.Process.Pid)

	err := <-errc
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	return err
}
