package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/descoped/dddns/internal/config"
	"github.com/descoped/dddns/internal/constants"
)

// updateFlagState snapshots every package-level flag runUpdate reads.
// Used by withUpdateFlags to restore state on test cleanup.
type updateFlagState struct {
	cfg     string
	force   bool
	dryRun  bool
	ip      string
	quiet   bool
	verbose bool
}

func snapshotUpdateFlags() updateFlagState {
	return updateFlagState{cfgFile, forceUpdate, dryRun, customIP, quiet, verbose}
}

func restoreUpdateFlags(s updateFlagState) {
	cfgFile, forceUpdate, dryRun, customIP, quiet, verbose = s.cfg, s.force, s.dryRun, s.ip, s.quiet, s.verbose
}

// setUpdateFlags installs IP + quiet for a test and restores priors
// on cleanup. Kept narrow: runUpdate reads several flags and leaking
// any across tests would produce nondeterministic failures.
func setUpdateFlags(t *testing.T, ip string) {
	t.Helper()
	prior := snapshotUpdateFlags()
	cfgFile = ""
	forceUpdate = false
	dryRun = false
	customIP = ip
	quiet = true
	verbose = false
	t.Cleanup(func() { restoreUpdateFlags(prior) })
}

func writeUpdateConfig(t *testing.T, dir string) {
	t.Helper()
	path := filepath.Join(dir, "config.yaml")
	content := "" +
		"aws_region: \"us-east-1\"\n" +
		"aws_access_key: \"AKIAIOSFODNN7EXAMPLE\"\n" +
		"aws_secret_key: \"wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY\"\n" +
		"hosted_zone_id: \"Z1ABCDEFGHIJKL\"\n" +
		"hostname: \"test.example.com\"\n" +
		"ttl: 300\n" +
		"ip_cache_file: \"" + filepath.Join(dir, "last-ip.txt") + "\"\n"
	if err := os.WriteFile(path, []byte(content), constants.ConfigFilePerm); err != nil {
		t.Fatalf("write config: %v", err)
	}
	config.SetActivePath(path)
	t.Cleanup(func() { config.SetActivePath("") })
}

// TestRunUpdate_RejectsPrivateIPFromFlag covers the security-boundary
// branch: --ip runs through myip.ValidatePublicIP, which rejects
// RFC1918 addresses. A regression here would let an operator publish
// 192.168.x.y to Route53 — useless and confusing.
func TestRunUpdate_RejectsPrivateIPFromFlag(t *testing.T) {
	writeUpdateConfig(t, t.TempDir())
	setUpdateFlags(t, "192.168.1.1")

	err := runUpdate(nil, nil)
	if err == nil {
		t.Fatal("runUpdate accepted an RFC1918 --ip value")
	}
	if !strings.Contains(err.Error(), "invalid --ip") {
		t.Errorf("error should cite invalid --ip, got: %v", err)
	}
}

// TestRunUpdate_RejectsMalformedIPFromFlag covers the other validation
// branch — non-IP strings must fail loud rather than reaching Route53
// with garbage.
func TestRunUpdate_RejectsMalformedIPFromFlag(t *testing.T) {
	writeUpdateConfig(t, t.TempDir())
	setUpdateFlags(t, "not-an-ip")

	err := runUpdate(nil, nil)
	if err == nil {
		t.Fatal("runUpdate accepted a malformed --ip value")
	}
	if !strings.Contains(err.Error(), "invalid --ip") {
		t.Errorf("error should cite invalid --ip, got: %v", err)
	}
}

// TestRunUpdate_PropagatesValidateError confirms Config.Validate
// failures surface with the 'invalid configuration' prefix — the text
// operators grep for when diagnosing a misconfigured install.
func TestRunUpdate_PropagatesValidateError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	// Valid YAML + perms, but no hostname — Validate will reject.
	content := "" +
		"aws_region: \"us-east-1\"\n" +
		"aws_access_key: \"AKIAIOSFODNN7EXAMPLE\"\n" +
		"aws_secret_key: \"wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY\"\n" +
		"hosted_zone_id: \"Z1ABCDEFGHIJKL\"\n" +
		"ttl: 300\n"
	if err := os.WriteFile(path, []byte(content), constants.ConfigFilePerm); err != nil {
		t.Fatalf("write: %v", err)
	}
	config.SetActivePath(path)
	t.Cleanup(func() { config.SetActivePath("") })
	setUpdateFlags(t, "")

	err := runUpdate(nil, nil)
	if err == nil {
		t.Fatal("runUpdate accepted a config missing required fields")
	}
	if !strings.Contains(err.Error(), "invalid configuration") {
		t.Errorf("error prefix should be 'invalid configuration', got: %v", err)
	}
}
