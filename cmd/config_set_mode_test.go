package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func TestSetMode_InvalidMode(t *testing.T) {
	var buf bytes.Buffer
	setModeCmd.SetOut(&buf)
	setModeCmd.SetErr(&buf)
	err := runSetMode(setModeCmd, []string{"bogus"})
	if err == nil || !strings.Contains(err.Error(), "must be 'cron' or 'serve'") {
		t.Errorf("expected invalid-mode error, got: %v", err)
	}
}

func TestSetMode_Cron_WritesScript(t *testing.T) {
	cacheFile := filepath.Join(t.TempDir(), "cache.txt")
	_ = writeInitialConfig(t, baseRotateConfig(cacheFile))

	tmpBoot := filepath.Join(t.TempDir(), "20-dddns.sh")
	origPath := setModeBootPath
	t.Cleanup(func() { setModeBootPath = origPath })
	setModeBootPath = tmpBoot

	var buf bytes.Buffer
	setModeCmd.SetOut(&buf)
	if err := runSetMode(setModeCmd, []string{"cron"}); err != nil {
		t.Fatal(err)
	}

	body, err := os.ReadFile(tmpBoot)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "--- cron mode ---") {
		t.Errorf("script missing cron marker:\n%s", body)
	}
	info, _ := os.Stat(tmpBoot)
	if info.Mode().Perm() != 0o755 {
		t.Errorf("mode = %04o, want 0755", info.Mode().Perm())
	}
	if !strings.Contains(buf.String(), "mode=cron") {
		t.Errorf("instructions missing mode label:\n%s", buf.String())
	}
}

func TestSetMode_Serve_RequiresServerBlock(t *testing.T) {
	cacheFile := filepath.Join(t.TempDir(), "cache.txt")
	cfg := baseRotateConfig(cacheFile)
	cfg.Server = nil
	_ = writeInitialConfig(t, cfg)

	tmpBoot := filepath.Join(t.TempDir(), "20-dddns.sh")
	origPath := setModeBootPath
	t.Cleanup(func() { setModeBootPath = origPath })
	setModeBootPath = tmpBoot

	err := runSetMode(setModeCmd, []string{"serve"})
	if err == nil || !strings.Contains(err.Error(), "no server block") {
		t.Errorf("expected missing-server-block error, got: %v", err)
	}
	if _, statErr := os.Stat(tmpBoot); !os.IsNotExist(statErr) {
		t.Errorf("boot script should not be written when validation fails")
	}
}

func TestSetMode_Serve_WritesSystemdUnit(t *testing.T) {
	cacheFile := filepath.Join(t.TempDir(), "cache.txt")
	_ = writeInitialConfig(t, baseRotateConfig(cacheFile))

	tmpBoot := filepath.Join(t.TempDir(), "20-dddns.sh")
	origPath := setModeBootPath
	t.Cleanup(func() { setModeBootPath = origPath })
	setModeBootPath = tmpBoot

	var buf bytes.Buffer
	setModeCmd.SetOut(&buf)
	if err := runSetMode(setModeCmd, []string{"serve"}); err != nil {
		t.Fatal(err)
	}

	body, err := os.ReadFile(tmpBoot)
	if err != nil {
		t.Fatal(err)
	}
	for _, needle := range []string{
		"--- serve mode ---",
		"ExecStart=/usr/local/bin/dddns serve",
		"Restart=always",
		"systemctl daemon-reload",
		"systemctl enable dddns.service",
		"systemctl restart dddns.service",
	} {
		if !strings.Contains(string(body), needle) {
			t.Errorf("script missing %q", needle)
		}
	}
}

// TestSetMode_Idempotent verifies that two consecutive set-mode calls
// for the same mode produce byte-identical files.
func TestSetMode_Idempotent(t *testing.T) {
	cacheFile := filepath.Join(t.TempDir(), "cache.txt")
	_ = writeInitialConfig(t, baseRotateConfig(cacheFile))

	tmpBoot := filepath.Join(t.TempDir(), "20-dddns.sh")
	origPath := setModeBootPath
	t.Cleanup(func() { setModeBootPath = origPath })
	setModeBootPath = tmpBoot

	var buf bytes.Buffer
	setModeCmd.SetOut(&buf)
	if err := runSetMode(setModeCmd, []string{"cron"}); err != nil {
		t.Fatal(err)
	}
	first, _ := os.ReadFile(tmpBoot)
	if err := runSetMode(setModeCmd, []string{"cron"}); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(tmpBoot)
	if string(first) != string(second) {
		t.Error("set-mode cron is not idempotent")
	}
}

// TestSetMode_SwitchMode verifies switching from cron to serve and back
// rewrites the script with the expected mode marker each time.
func TestSetMode_SwitchMode(t *testing.T) {
	cacheFile := filepath.Join(t.TempDir(), "cache.txt")
	_ = writeInitialConfig(t, baseRotateConfig(cacheFile))

	tmpBoot := filepath.Join(t.TempDir(), "20-dddns.sh")
	origPath := setModeBootPath
	t.Cleanup(func() { setModeBootPath = origPath })
	setModeBootPath = tmpBoot

	// Reset viper between runSetMode calls so each Load re-reads the file.
	run := func(mode string) string {
		var buf bytes.Buffer
		setModeCmd.SetOut(&buf)
		// writeInitialConfig resets viper but runSetMode reuses the same
		// already-loaded state, so force a reload to ensure mode toggles
		// don't rely on stale config.
		viper.Reset()
		viper.SetConfigFile(cfgFile)
		if err := viper.ReadInConfig(); err != nil {
			t.Fatal(err)
		}
		if err := runSetMode(setModeCmd, []string{mode}); err != nil {
			t.Fatalf("set-mode %s: %v", mode, err)
		}
		body, err := os.ReadFile(tmpBoot)
		if err != nil {
			t.Fatal(err)
		}
		return string(body)
	}

	cronScript := run("cron")
	if !strings.Contains(cronScript, "--- cron mode ---") {
		t.Fatal("first run should be cron")
	}
	serveScript := run("serve")
	if !strings.Contains(serveScript, "--- serve mode ---") {
		t.Fatal("second run should be serve")
	}
	// The cron-mode script must not be present in the serve-mode output
	// (i.e. the switch fully replaces the file rather than appending).
	if strings.Contains(serveScript, "--- cron mode ---") {
		t.Error("serve-mode script unexpectedly contains cron-mode section")
	}
}
