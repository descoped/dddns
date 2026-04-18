package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/descoped/dddns/internal/config"
	"github.com/descoped/dddns/internal/constants"
)

// TestUpdateIntervalOrDefault_EmptyFallsBackToDefault guards the config-
// vs-default contract: an unset field must yield the package's
// DefaultUpdateInterval (otherwise cron mode would silently generate an
// empty schedule line).
func TestUpdateIntervalOrDefault_EmptyFallsBackToDefault(t *testing.T) {
	cfg := &config.Config{} // UpdateInterval unset
	if got := cfg.UpdateIntervalOrDefault(); got != config.DefaultUpdateInterval {
		t.Errorf("got %q, want DefaultUpdateInterval %q", got, config.DefaultUpdateInterval)
	}
}

// TestUpdateIntervalOrDefault_UserValueWins complements the default
// contract: when set, the user's crontab schedule passes through
// verbatim.
func TestUpdateIntervalOrDefault_UserValueWins(t *testing.T) {
	cfg := &config.Config{UpdateInterval: "*/15 * * * *"}
	if got := cfg.UpdateIntervalOrDefault(); got != "*/15 * * * *" {
		t.Errorf("got %q, want user-set schedule", got)
	}
}

// TestUpdateTimeoutOrDefault_Matrix is the branch matrix for the
// silently-tolerant duration parser. The documented contract is that
// Validate() surfaces parse errors at config-check time; this function
// must fall back to the default rather than return zero (which would
// be an unbounded Update).
func TestUpdateTimeoutOrDefault_Matrix(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  time.Duration
	}{
		{"empty returns default", "", config.DefaultUpdateTimeout},
		{"valid duration passes through", "45s", 45 * time.Second},
		{"minutes", "2m", 2 * time.Minute},
		{"malformed falls back to default", "not-a-duration", config.DefaultUpdateTimeout},
		{"zero falls back to default", "0s", config.DefaultUpdateTimeout},
		{"negative falls back to default", "-5s", config.DefaultUpdateTimeout},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{UpdateTimeout: tc.value}
			if got := cfg.UpdateTimeoutOrDefault(); got != tc.want {
				t.Errorf("UpdateTimeoutOrDefault(%q) = %v, want %v", tc.value, got, tc.want)
			}
		})
	}
}

// TestConfigValidate_UpdateTimeoutMalformed covers the Validate branch
// that IS strict (unlike UpdateTimeoutOrDefault). Validate is called at
// `dddns config check` time so operators catch malformed durations
// before the next cron tick silently reverts to the default.
func TestConfigValidate_UpdateTimeoutMalformed(t *testing.T) {
	cfg := &config.Config{
		AWSAccessKey:  "a",
		AWSSecretKey:  "s",
		HostedZoneID:  "Z",
		Hostname:      "h.example.com",
		TTL:           300,
		UpdateTimeout: "forty-five seconds", // not a Go duration
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate accepted a malformed update_timeout")
	}
	if !strings.Contains(err.Error(), "update_timeout") {
		t.Errorf("error should name update_timeout, got: %v", err)
	}
}

// TestConfigValidate_UpdateTimeoutNonPositive covers the duration-
// must-be-positive branch.
func TestConfigValidate_UpdateTimeoutNonPositive(t *testing.T) {
	cfg := &config.Config{
		AWSAccessKey:  "a",
		AWSSecretKey:  "s",
		HostedZoneID:  "Z",
		Hostname:      "h.example.com",
		TTL:           300,
		UpdateTimeout: "0s",
	}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "positive") {
		t.Errorf("Validate should reject non-positive update_timeout, got: %v", err)
	}
}

// TestSavePlaintext_RoundTrip verifies the plaintext save path produces
// a file Load() can read back. This closes a gap the earlier tests
// implicitly covered (by writing YAML by hand) but never exercised
// through SavePlaintext itself.
func TestSavePlaintext_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	in := &config.Config{
		AWSRegion:    "us-east-1",
		AWSAccessKey: "AKIAIOSFODNN7EXAMPLE",
		AWSSecretKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		HostedZoneID: "Z1ABCDEFGHIJKL",
		Hostname:     "test.example.com",
		TTL:          600,
		IPCacheFile:  filepath.Join(dir, "last-ip.txt"),
	}

	if err := config.SavePlaintext(in, path); err != nil {
		t.Fatalf("SavePlaintext: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if mode := info.Mode().Perm(); mode != constants.ConfigFilePerm {
		t.Errorf("plaintext save perms = %04o, want %04o", mode, constants.ConfigFilePerm)
	}

	config.SetActivePath(path)
	t.Cleanup(func() { config.SetActivePath("") })

	out, err := config.Load()
	if err != nil {
		t.Fatalf("Load after SavePlaintext: %v", err)
	}
	if out.AWSAccessKey != in.AWSAccessKey || out.Hostname != in.Hostname || out.TTL != 600 {
		t.Errorf("round-trip lost fields: %+v", out)
	}
}

// TestSavePlaintext_CreatesMissingParentDir covers the "config dir
// didn't exist yet" branch — matches MkdirAll behaviour. This is the
// fresh-install path where dddns is configured before any directory
// has been created.
func TestSavePlaintext_CreatesMissingParentDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "deeper", "config.yaml")

	cfg := &config.Config{
		AWSAccessKey: "AKIAIOSFODNN7EXAMPLE",
		AWSSecretKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		HostedZoneID: "Z1ABCDEFGHIJKL",
		Hostname:     "test.example.com",
		TTL:          300,
	}
	if err := config.SavePlaintext(cfg, path); err != nil {
		t.Fatalf("SavePlaintext on nested path: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("config not created: %v", err)
	}
}
