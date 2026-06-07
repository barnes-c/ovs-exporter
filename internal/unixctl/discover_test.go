package unixctl

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverSocket_FromPIDFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ovs-vswitchd.pid"), []byte("1234\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "ovs-vswitchd.1234.ctl")
	if err := os.WriteFile(want, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := DiscoverSocket(dir, "ovs-vswitchd")
	if err != nil {
		t.Fatalf("DiscoverSocket: %v", err)
	}
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestDiscoverSocket_StalePIDFile_FallsBackToGlob(t *testing.T) {
	dir := t.TempDir()
	// PID file points at a socket that doesn't exist.
	if err := os.WriteFile(filepath.Join(dir, "ovs-vswitchd.pid"), []byte("99999"), 0o644); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "ovs-vswitchd.42.ctl")
	if err := os.WriteFile(want, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := DiscoverSocket(dir, "ovs-vswitchd")
	if err != nil {
		t.Fatalf("DiscoverSocket: %v", err)
	}
	if got != want {
		t.Errorf("got %s, want %s (glob fallback)", got, want)
	}
}

func TestDiscoverSocket_NoPIDFile_FallsBackToGlob(t *testing.T) {
	dir := t.TempDir()
	want := filepath.Join(dir, "ovs-vswitchd.42.ctl")
	if err := os.WriteFile(want, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := DiscoverSocket(dir, "ovs-vswitchd")
	if err != nil {
		t.Fatalf("DiscoverSocket: %v", err)
	}
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestDiscoverSocket_NoSocketFound(t *testing.T) {
	dir := t.TempDir()
	if _, err := DiscoverSocket(dir, "ovs-vswitchd"); err == nil {
		t.Error("expected error when no socket exists")
	}
}

func TestDiscoverSocket_ValidatesInputs(t *testing.T) {
	if _, err := DiscoverSocket("", "ovs-vswitchd"); err == nil {
		t.Error("expected error for empty runDir")
	}
	if _, err := DiscoverSocket("/run/openvswitch", ""); err == nil {
		t.Error("expected error for empty daemon")
	}
}

func TestDiscoverSocket_MalformedPIDFile_FallsBackToGlob(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ovs-vswitchd.pid"), []byte("not-a-number"), 0o644); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "ovs-vswitchd.42.ctl")
	if err := os.WriteFile(want, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := DiscoverSocket(dir, "ovs-vswitchd")
	if err != nil {
		t.Fatalf("DiscoverSocket: %v", err)
	}
	if got != want {
		t.Errorf("got %s, want %s (malformed pid fallback)", got, want)
	}
}
