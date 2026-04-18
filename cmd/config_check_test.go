package cmd

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/descoped/dddns/internal/config"
)

// captureStdout redirects os.Stdout for the duration of fn and returns
// everything written to it. Used to assert on runConfigCheck's
// human-readable output, which goes through fmt.Println / fmt.Printf
// to the global stdout rather than a cobra-plumbed writer.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w

	done := make(chan []byte, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- b
	}()

	fn()

	_ = w.Close()
	os.Stdout = orig
	return string(<-done)
}

// writeValidConfig drops a minimally-valid config.yaml into dir with
// 0600 perms and activates it via config.SetActivePath. RFC 5737 /
// 2606 fixtures only.
func writeValidConfig(t *testing.T, dir string) string {
	t.Helper()
	p := filepath.Join(dir, "config.yaml")
	content := "" +
		"aws_region: \"us-east-1\"\n" +
		"aws_access_key: \"AKIAIOSFODNN7EXAMPLE\"\n" +
		"aws_secret_key: \"wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY\"\n" +
		"hosted_zone_id: \"Z1ABCDEFGHIJKL\"\n" +
		"hostname: \"test.example.com\"\n" +
		"ttl: 300\n" +
		"ip_cache_file: \"" + filepath.Join(dir, "last-ip.txt") + "\"\n"
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	config.SetActivePath(p)
	t.Cleanup(func() { config.SetActivePath("") })
	return p
}

// TestConfigCheck_AcceptsValidConfig is the happy-path invariant: a
// correctly-permissioned, well-formed config with every required field
// passes Load + Validate and runConfigCheck returns nil. The AWS probe
// at the tail will fail (no network in tests) but by design is fail-open
// and must not surface as an error.
func TestConfigCheck_AcceptsValidConfig(t *testing.T) {
	writeValidConfig(t, t.TempDir())

	var runErr error
	out := captureStdout(t, func() {
		runErr = runConfigCheck(nil, nil)
	})

	if runErr != nil {
		t.Fatalf("runConfigCheck returned error on valid config: %v", runErr)
	}
	if !strings.Contains(out, "Configuration is valid") {
		t.Errorf("stdout missing validation confirmation:\n%s", out)
	}
	if !strings.Contains(out, "Z1ABCDEFGHIJKL") || !strings.Contains(out, "test.example.com") {
		t.Errorf("stdout missing config summary fields:\n%s", out)
	}
}

// TestConfigCheck_RejectsWorldReadablePlaintext covers the security
// boundary: a plaintext config with overly-permissive perms (0644)
// must be refused at Load time. This is local-privilege-escalation
// territory on shared hosts.
func TestConfigCheck_RejectsWorldReadablePlaintext(t *testing.T) {
	dir := t.TempDir()
	p := writeValidConfig(t, dir)
	if err := os.Chmod(p, 0o644); err != nil {
		t.Fatalf("chmod: %v", err)
	}

	err := runConfigCheck(nil, nil)
	if err == nil {
		t.Fatal("runConfigCheck accepted 0644 plaintext config; expected rejection")
	}
	if !strings.Contains(err.Error(), "permission") && !strings.Contains(err.Error(), "must be 600") {
		t.Errorf("error should mention the permission issue, got: %v", err)
	}
}

// TestConfigCheck_RejectsMalformedYAML ensures a parse failure in Load
// surfaces to the caller rather than silently producing a zero-valued
// Config.
func TestConfigCheck_RejectsMalformedYAML(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(p, []byte("this: is: not: valid: yaml: ["), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	config.SetActivePath(p)
	t.Cleanup(func() { config.SetActivePath("") })

	err := runConfigCheck(nil, nil)
	if err == nil {
		t.Fatal("runConfigCheck accepted malformed YAML; expected parse error")
	}
	if !strings.Contains(err.Error(), "load") && !strings.Contains(err.Error(), "parse") && !strings.Contains(err.Error(), "yaml") {
		t.Errorf("error should mention the parse failure, got: %v", err)
	}
}

// TestConfigCheck_RejectsMissingRequiredField covers the Validate path
// specifically: a well-formed YAML that is missing hostname must fail
// loud. Prevents a silent regression where a field rename breaks
// Validate without breaking Load.
func TestConfigCheck_RejectsMissingRequiredField(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.yaml")
	// Valid YAML, valid perms, but no hostname.
	content := "" +
		"aws_region: \"us-east-1\"\n" +
		"aws_access_key: \"AKIAIOSFODNN7EXAMPLE\"\n" +
		"aws_secret_key: \"wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY\"\n" +
		"hosted_zone_id: \"Z1ABCDEFGHIJKL\"\n" +
		"ttl: 300\n"
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	config.SetActivePath(p)
	t.Cleanup(func() { config.SetActivePath("") })

	err := runConfigCheck(nil, nil)
	if err == nil {
		t.Fatal("runConfigCheck accepted config missing hostname; expected validation error")
	}
	if !strings.Contains(err.Error(), "hostname") {
		t.Errorf("error should name the missing field, got: %v", err)
	}
}

// TestConfigCheck_ReportsAWSFailureButDoesNotGate exercises the
// documented fail-open contract: the Route53 probe failing (network
// error, invalid creds) produces a stdout diagnostic but the command
// still returns nil so the operator can continue debugging.
func TestConfigCheck_ReportsAWSFailureButDoesNotGate(t *testing.T) {
	writeValidConfig(t, t.TempDir())

	var runErr error
	out := captureStdout(t, func() {
		runErr = runConfigCheck(nil, nil)
	})

	// The call to Route53 at the tail will fail (fake creds, or no
	// network) — runConfigCheck must still return nil.
	if runErr != nil {
		t.Fatalf("runConfigCheck returned error on AWS failure; expected fail-open: %v", runErr)
	}
	// Either the credential check succeeded against the real endpoint
	// (extremely unlikely with EXAMPLE keys) or the diagnostic landed on
	// stdout. Both are acceptable outcomes; the invariant is no error.
	if strings.Contains(out, "AWS credentials verified") {
		t.Log("AWS probe unexpectedly succeeded against the real Route53 endpoint — acceptable but unexpected.")
	} else if !strings.Contains(out, "AWS credential check failed") {
		t.Errorf("stdout missing credential-check diagnostic:\n%s", out)
	}
}
