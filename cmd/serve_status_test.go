package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/descoped/dddns/internal/config"
	"github.com/descoped/dddns/internal/constants"
	"github.com/descoped/dddns/internal/server"
	"github.com/spf13/cobra"
)

// newServeStatusCmdWithBuffer constructs a cobra.Command whose stdout
// writes into the returned buffer. runServeStatus uses
// cmd.OutOrStdout(), so this captures its output cleanly without
// touching the process-level os.Stdout.
func newServeStatusCmdWithBuffer() (*cobra.Command, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	cmd := &cobra.Command{}
	cmd.SetOut(buf)
	return cmd, buf
}

// writeServeConfig drops a config.yaml whose IPCacheFile points into
// dir so that server.StatusPath resolves to dir/serve-status.json.
// Activates it via SetActivePath and returns the expected status path.
func writeServeConfig(t *testing.T, dir string) string {
	t.Helper()
	configPath := filepath.Join(dir, "config.yaml")
	content := "" +
		"aws_region: \"us-east-1\"\n" +
		"aws_access_key: \"AKIAIOSFODNN7EXAMPLE\"\n" +
		"aws_secret_key: \"wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY\"\n" +
		"hosted_zone_id: \"Z1ABCDEFGHIJKL\"\n" +
		"hostname: \"test.example.com\"\n" +
		"ttl: 300\n" +
		"ip_cache_file: \"" + filepath.Join(dir, "last-ip.txt") + "\"\n"
	if err := os.WriteFile(configPath, []byte(content), constants.ConfigFilePerm); err != nil {
		t.Fatalf("write config: %v", err)
	}
	config.SetActivePath(configPath)
	t.Cleanup(func() { config.SetActivePath("") })
	return filepath.Join(dir, "serve-status.json")
}

// TestServeStatus_ReadsWellFormedSnapshot covers the happy path: a
// valid serve-status.json is read and each field surfaces in the
// formatted output. This is the exact flow an operator runs after a
// push arrives.
func TestServeStatus_ReadsWellFormedSnapshot(t *testing.T) {
	dir := t.TempDir()
	statusPath := writeServeConfig(t, dir)

	snap := server.StatusSnapshot{
		LastRequestAt:   time.Date(2026, 4, 18, 12, 34, 56, 0, time.UTC),
		LastRemoteAddr:  "127.0.0.1:54321",
		LastAuthOutcome: "ok",
		LastAction:      "good 203.0.113.10",
	}
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(statusPath, data, 0o600); err != nil {
		t.Fatalf("write status: %v", err)
	}

	cmd, buf := newServeStatusCmdWithBuffer()
	if err := runServeStatus(cmd, nil); err != nil {
		t.Fatalf("runServeStatus: %v", err)
	}
	out := buf.String()
	for _, want := range []string{
		statusPath,
		"2026-04-18T12:34:56Z",
		"127.0.0.1:54321",
		"ok",
		"good 203.0.113.10",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n---\n%s", want, out)
		}
	}
}

// TestServeStatus_ReportsAbsentFileClearly covers the "never received a
// request" case. The operator should see a diagnostic that names the
// expected file and hints that the server has not run yet — not a
// cryptic os.ErrNotExist stack.
func TestServeStatus_ReportsAbsentFileClearly(t *testing.T) {
	dir := t.TempDir()
	writeServeConfig(t, dir)

	cmd, _ := newServeStatusCmdWithBuffer()
	err := runServeStatus(cmd, nil)
	if err == nil {
		t.Fatal("runServeStatus returned nil for absent status file")
	}
	if !strings.Contains(err.Error(), "no status is recorded") && !strings.Contains(err.Error(), "not") {
		t.Errorf("error message should hint at 'no status recorded', got: %v", err)
	}
}

// TestServeStatus_RejectsMalformedJSON covers the "disk corruption or
// partial write after power loss" case. runServeStatus must surface
// the parse error rather than pretending the snapshot was read.
func TestServeStatus_RejectsMalformedJSON(t *testing.T) {
	dir := t.TempDir()
	statusPath := writeServeConfig(t, dir)

	if err := os.WriteFile(statusPath, []byte("{this is not: valid json"), 0o600); err != nil {
		t.Fatalf("write malformed: %v", err)
	}

	cmd, _ := newServeStatusCmdWithBuffer()
	err := runServeStatus(cmd, nil)
	if err == nil {
		t.Fatal("runServeStatus accepted malformed JSON as a valid snapshot")
	}
	if !strings.Contains(err.Error(), "parse") && !strings.Contains(err.Error(), "status") {
		t.Errorf("error should cite parse/status failure, got: %v", err)
	}
}
