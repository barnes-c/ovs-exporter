package unixctl

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

// DiscoverSocket resolves the unix control socket for daemon under runDir.
// The conventional path is <runDir>/<daemon>.<pid>.ctl, where pid is read
// from <runDir>/<daemon>.pid. When the pid file is missing, unreadable, or
// points at a non-existent socket, DiscoverSocket falls back to a glob over
// <runDir>/<daemon>.*.ctl and returns the first match.
func DiscoverSocket(runDir, daemon string) (string, error) {
	if runDir == "" {
		return "", fmt.Errorf("unixctl: runDir is empty")
	}
	if daemon == "" {
		return "", fmt.Errorf("unixctl: daemon name is empty")
	}
	if path, err := socketFromPIDFile(runDir, daemon); err == nil {
		if _, statErr := os.Stat(path); statErr == nil {
			return path, nil
		}
	}
	matches, err := filepath.Glob(filepath.Join(runDir, daemon+".*.ctl"))
	if err != nil {
		return "", fmt.Errorf("unixctl: glob %s/%s.*.ctl: %w", runDir, daemon, err)
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("unixctl: no control socket found for %s in %s", daemon, runDir)
	}
	return matches[0], nil
}

func socketFromPIDFile(runDir, daemon string) (string, error) {
	pidPath := filepath.Join(runDir, daemon+".pid")
	raw, err := os.ReadFile(pidPath)
	if err != nil {
		return "", err
	}
	pid, err := strconv.Atoi(string(bytes.TrimSpace(raw)))
	if err != nil {
		return "", fmt.Errorf("unixctl: parse pid file %s: %w", pidPath, err)
	}
	return filepath.Join(runDir, fmt.Sprintf("%s.%d.ctl", daemon, pid)), nil
}
