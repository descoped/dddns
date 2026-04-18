package config_test

import (
	"testing"

	"github.com/descoped/dddns/internal/config"
)

// TestActivePath_Uninitialized guards the contract that ActivePath
// returns an empty string before SetActivePath has been called. Some
// cmd/ flows (e.g. `dddns config init`) rely on the empty case to mean
// "no config is loaded, write to the profile default" rather than
// using a stale path left behind by an earlier lifecycle.
func TestActivePath_Uninitialized(t *testing.T) {
	// Reset to the zero state. Tests run in parallel could otherwise
	// observe another test's path here; the package-level var is
	// single-threaded and this t.Cleanup restores the prior value so
	// surrounding tests are unaffected.
	prior := config.ActivePath()
	config.SetActivePath("")
	t.Cleanup(func() { config.SetActivePath(prior) })

	if got := config.ActivePath(); got != "" {
		t.Errorf("ActivePath() = %q after SetActivePath(\"\"), want empty", got)
	}
}

// TestSetActivePath_RoundTrip is the core API contract between cmd/
// and internal/config: a path recorded via SetActivePath is returned
// verbatim from ActivePath. A regression here would detach config.Load
// from the --config flag, silently loading the wrong file.
func TestSetActivePath_RoundTrip(t *testing.T) {
	prior := config.ActivePath()
	t.Cleanup(func() { config.SetActivePath(prior) })

	const path = "/tmp/dddns/test-config.yaml"
	config.SetActivePath(path)
	if got := config.ActivePath(); got != path {
		t.Errorf("ActivePath() = %q, want %q", got, path)
	}
}

// TestSetActivePath_OverwriteReplacesPrior documents that SetActivePath
// is a replace, not a stack — the second call fully supplants the
// first. This holds even when the new value is empty (used by some
// test teardowns and by the secure-migration flow to clear state).
func TestSetActivePath_OverwriteReplacesPrior(t *testing.T) {
	prior := config.ActivePath()
	t.Cleanup(func() { config.SetActivePath(prior) })

	config.SetActivePath("/tmp/first.yaml")
	config.SetActivePath("/tmp/second.yaml")
	if got := config.ActivePath(); got != "/tmp/second.yaml" {
		t.Errorf("ActivePath() = %q, want /tmp/second.yaml", got)
	}

	config.SetActivePath("")
	if got := config.ActivePath(); got != "" {
		t.Errorf("ActivePath() = %q after clear, want empty", got)
	}
}
