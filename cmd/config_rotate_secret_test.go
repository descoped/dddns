package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/descoped/dddns/internal/config"
	"github.com/spf13/viper"
)

const rotateTestSecret = "initial-secret-value"

func baseRotateConfig(cacheFile string) *config.Config {
	return &config.Config{
		AWSRegion:    "us-east-1",
		AWSAccessKey: "AKIATEST",
		AWSSecretKey: "SECRETTEST",
		HostedZoneID: "Z123",
		Hostname:     "test.example.com",
		TTL:          300,
		IPCacheFile:  cacheFile,
		Server: &config.ServerConfig{
			Bind:         "127.0.0.1:53353",
			SharedSecret: rotateTestSecret,
			AllowedCIDRs: []string{"127.0.0.0/8"},
		},
	}
}

// writeInitialConfig writes the passed cfg as plaintext YAML to
// tmpDir/config.yaml, mirrors cfgFile and viper to point at it, and
// returns the path. Cleanup restores global state.
func writeInitialConfig(t *testing.T, cfg *config.Config) string {
	t.Helper()
	tmp := t.TempDir()
	path := filepath.Join(tmp, "config.yaml")
	if err := config.SavePlaintext(cfg, path); err != nil {
		t.Fatal(err)
	}

	origCfgFile := cfgFile
	t.Cleanup(func() { cfgFile = origCfgFile; viper.Reset() })
	cfgFile = path
	viper.Reset()
	viper.SetConfigFile(path)
	if err := viper.ReadInConfig(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRotateSecret_PlaintextRoundTrip(t *testing.T) {
	cacheFile := filepath.Join(t.TempDir(), "cache.txt")
	path := writeInitialConfig(t, baseRotateConfig(cacheFile))

	var buf bytes.Buffer
	rotateSecretCmd.SetOut(&buf)
	if err := runRotateSecret(rotateSecretCmd, nil); err != nil {
		t.Fatalf("runRotateSecret failed: %v", err)
	}

	// Output must include the new secret.
	if !strings.Contains(buf.String(), "New shared secret generated") {
		t.Errorf("output missing framed header:\n%s", buf.String())
	}

	// Reload and verify the secret changed to the one in the output.
	viper.Reset()
	viper.SetConfigFile(path)
	if err := viper.ReadInConfig(); err != nil {
		t.Fatal(err)
	}
	cfgAfter, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfgAfter.Server == nil {
		t.Fatal("Server block lost after rotation")
	}
	if cfgAfter.Server.SharedSecret == rotateTestSecret {
		t.Error("SharedSecret did not change")
	}
	if len(cfgAfter.Server.SharedSecret) != 64 {
		t.Errorf("SharedSecret length = %d, want 64 hex chars", len(cfgAfter.Server.SharedSecret))
	}
	// The rest of the config must be untouched.
	if cfgAfter.Hostname != "test.example.com" || cfgAfter.Server.Bind != "127.0.0.1:53353" {
		t.Errorf("non-secret fields changed after rotation: %+v", cfgAfter)
	}
}

func TestRotateSecret_TwoCallsProduceDifferentSecrets(t *testing.T) {
	cacheFile := filepath.Join(t.TempDir(), "cache.txt")
	path := writeInitialConfig(t, baseRotateConfig(cacheFile))

	run := func() string {
		var buf bytes.Buffer
		rotateSecretCmd.SetOut(&buf)
		if err := runRotateSecret(rotateSecretCmd, nil); err != nil {
			t.Fatalf("runRotateSecret failed: %v", err)
		}
		viper.Reset()
		viper.SetConfigFile(path)
		if err := viper.ReadInConfig(); err != nil {
			t.Fatal(err)
		}
		c, err := config.Load()
		if err != nil {
			t.Fatal(err)
		}
		return c.Server.SharedSecret
	}

	first := run()
	second := run()
	if first == second {
		t.Error("two rotations produced the same secret")
	}
}

func TestRotateSecret_RequiresServerBlockOrInit(t *testing.T) {
	cacheFile := filepath.Join(t.TempDir(), "cache.txt")
	cfg := baseRotateConfig(cacheFile)
	cfg.Server = nil
	path := writeInitialConfig(t, cfg)

	// Without --init, the call must error.
	rotateSecretInit = false
	var buf bytes.Buffer
	rotateSecretCmd.SetOut(&buf)
	if err := runRotateSecret(rotateSecretCmd, nil); err == nil {
		t.Error("expected error when server block is absent and --init not set")
	}

	// With --init, the call succeeds and creates a default server block.
	rotateSecretInit = true
	t.Cleanup(func() { rotateSecretInit = false })
	buf.Reset()
	if err := runRotateSecret(rotateSecretCmd, nil); err != nil {
		t.Fatalf("runRotateSecret --init failed: %v", err)
	}
	// Reload and verify the server block was created with the loopback
	// default bind and CIDR.
	viper.Reset()
	viper.SetConfigFile(path)
	if err := viper.ReadInConfig(); err != nil {
		t.Fatal(err)
	}
	c, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.Server == nil {
		t.Fatal("--init should have created Server block")
	}
	if c.Server.Bind != defaultBind {
		t.Errorf("Bind = %q, want %q", c.Server.Bind, defaultBind)
	}
	if len(c.Server.AllowedCIDRs) != 1 || c.Server.AllowedCIDRs[0] != defaultAllowedCIDR {
		t.Errorf("AllowedCIDRs = %v, want [%q]", c.Server.AllowedCIDRs, defaultAllowedCIDR)
	}
	if len(c.Server.SharedSecret) != 64 {
		t.Errorf("SharedSecret length = %d, want 64", len(c.Server.SharedSecret))
	}
}

func TestRotateSecret_SecureConfig(t *testing.T) {
	// Build a baseline config and save it via SaveSecure directly.
	tmp := t.TempDir()
	cacheFile := filepath.Join(tmp, "cache.txt")
	securePath := filepath.Join(tmp, "config.secure")
	initial := baseRotateConfig(cacheFile)
	if err := config.SaveSecure(initial, securePath); err != nil {
		t.Fatal(err)
	}

	// Point both cfgFile and viper's "config" key at the secure path.
	// config.Load checks viper.IsSet("config") to resolve .secure files
	// without a concrete viper.SetConfigFile + ReadInConfig (.secure is
	// not YAML).
	origCfgFile := cfgFile
	t.Cleanup(func() { cfgFile = origCfgFile; viper.Reset() })
	cfgFile = securePath
	viper.Reset()
	viper.Set("config", securePath)

	var buf bytes.Buffer
	rotateSecretCmd.SetOut(&buf)
	if err := runRotateSecret(rotateSecretCmd, nil); err != nil {
		t.Fatalf("runRotateSecret against .secure failed: %v", err)
	}

	// The file must still parse as SecureConfig and decrypt to a
	// different shared secret.
	after, err := config.LoadSecure(securePath)
	if err != nil {
		t.Fatalf("LoadSecure after rotation failed: %v", err)
	}
	if after.Server == nil {
		t.Fatal("Server lost after secure-config rotation")
	}
	if after.Server.SharedSecret == rotateTestSecret {
		t.Error("SharedSecret did not change after secure rotation")
	}

	// And the raw file must not contain the plaintext secret.
	raw, err := os.ReadFile(securePath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), after.Server.SharedSecret) {
		t.Error("rotated secret appears in plaintext in .secure file")
	}
}

func TestGenerateSecret_LengthAndUniqueness(t *testing.T) {
	a, err := generateSecret()
	if err != nil {
		t.Fatal(err)
	}
	b, err := generateSecret()
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != 64 || len(b) != 64 {
		t.Errorf("secret lengths = %d, %d, want 64", len(a), len(b))
	}
	if a == b {
		t.Error("two calls produced the same secret")
	}
}
