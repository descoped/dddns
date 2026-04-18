package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/descoped/dddns/internal/constants"
)

// withConfigInitFlags installs cfgFile/forceInit/interactive for a test
// and restores the prior values on cleanup. Tests the non-interactive
// path only — interactive mode requires stdin stubbing that the plan
// explicitly excludes as test-ware.
func withConfigInitFlags(t *testing.T, path string, force, interactive bool) {
	t.Helper()
	origCfgFile, origForce, origInteractive := cfgFile, forceInit, interactiveConfigInitFlag()
	cfgFile = path
	forceInit = force
	setInteractive(interactive)
	t.Cleanup(func() {
		cfgFile = origCfgFile
		forceInit = origForce
		setInteractive(origInteractive)
	})
}

// interactiveConfigInitFlag and setInteractive isolate access to the
// package-level `interactive` var so renames in the command layer
// don't silently break this test file.
func interactiveConfigInitFlag() bool { return interactive }
func setInteractive(v bool)           { interactive = v }

// TestConfigInit_NonInteractiveCreatesAt0600 covers the invariant:
// non-interactive init creates the config file with the standard
// plaintext permission (0600). A regression to 0644 or 0660 would
// silently widen the attack surface.
func TestConfigInit_NonInteractiveCreatesAt0600(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	withConfigInitFlags(t, path, false, false)

	_ = captureStdout(t, func() {
		if err := runConfigInit(nil, nil); err != nil {
			t.Fatalf("runConfigInit: %v", err)
		}
	})

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("config file missing: %v", err)
	}
	if mode := info.Mode().Perm(); mode != constants.ConfigFilePerm {
		t.Errorf("config file perms = %04o, want %04o", mode, constants.ConfigFilePerm)
	}
}

// TestConfigInit_RefusesOverwriteWithoutForce is the first-time-setup
// safety net: running `dddns config init` a second time must not
// silently clobber an existing config that may hold working AWS
// credentials.
func TestConfigInit_RefusesOverwriteWithoutForce(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	// Pre-existing file — non-empty marker to detect accidental overwrite.
	const marker = "# previously-authored config — do not clobber\n"
	if err := os.WriteFile(path, []byte(marker), constants.ConfigFilePerm); err != nil {
		t.Fatalf("seed existing: %v", err)
	}
	withConfigInitFlags(t, path, false, false)

	err := runConfigInit(nil, nil)
	if err == nil {
		t.Fatal("runConfigInit silently overwrote an existing config without --force")
	}

	// Marker must still be there — nothing was written.
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("read existing after refusal: %v", readErr)
	}
	if string(data) != marker {
		t.Error("existing config was modified despite refusal to overwrite")
	}
}

// TestConfigInit_ForceOverwritesExisting verifies the complementary
// branch: `--force` must actually overwrite. A regression where
// forceInit is accidentally gated by an earlier check would surface
// as users unable to regenerate their config.
func TestConfigInit_ForceOverwritesExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("# stale\n"), constants.ConfigFilePerm); err != nil {
		t.Fatalf("seed: %v", err)
	}
	withConfigInitFlags(t, path, true, false)

	_ = captureStdout(t, func() {
		if err := runConfigInit(nil, nil); err != nil {
			t.Fatalf("runConfigInit with --force: %v", err)
		}
	})

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read after force: %v", err)
	}
	// The template always writes `# dddns Configuration` as the first
	// comment; its absence means the overwrite didn't happen.
	if string(data) == "# stale\n" {
		t.Fatal("config still contains stale content; --force did not overwrite")
	}
}
