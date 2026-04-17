package bootscript

import (
	"strings"
	"testing"
)

func TestGenerate_Cron(t *testing.T) {
	out, err := Generate(DefaultUnifiParams("cron"))
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, out, []string{
		"#!/bin/sh",
		"BINARY=\"/data/dddns/dddns\"",
		"--- cron mode ---",
		// Switching-from-serve guard.
		"systemctl stop dddns.service",
		"systemctl disable dddns.service",
		`rm -f "$SYSTEMD_UNIT"`,
		// Cron install.
		"*/30 * * * * root /usr/local/bin/dddns update --quiet",
		"/var/log/dddns.log",
		"/etc/init.d/cron restart",
	})
	mustNotContain(t, out, []string{
		// Cron mode must not start the daemon.
		"systemctl enable dddns.service",
		"systemctl restart dddns.service",
		"ExecStart=/usr/local/bin/dddns serve",
	})
}

func TestGenerate_Serve(t *testing.T) {
	out, err := Generate(DefaultUnifiParams("serve"))
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, out, []string{
		"#!/bin/sh",
		"BINARY=\"/data/dddns/dddns\"",
		"--- serve mode ---",
		// Switching-from-cron guard.
		`rm -f "$CRON_PATH"`,
		// systemd unit install.
		`cat > "$SYSTEMD_UNIT" <<'UNIT'`,
		"[Service]",
		"ExecStart=/usr/local/bin/dddns serve",
		"Restart=always",
		"RestartSec=5",
		"NoNewPrivileges=true",
		"ProtectSystem=strict",
		"ReadWritePaths=/data/.dddns /var/log",
		"systemctl daemon-reload",
		"systemctl enable dddns.service",
		"systemctl restart dddns.service",
	})
	// The cron entry content and the while-loop shell supervisor must
	// not leak into serve mode.
	mustNotContain(t, out, []string{
		"update --quiet",
		`cat > "$CRON_PATH"`,
		"while true; do",
		`pkill -f "dddns serve"`,
	})
}

func TestGenerate_InvalidMode(t *testing.T) {
	if _, err := Generate(Params{Mode: "bogus"}); err == nil {
		t.Error("expected error for invalid mode")
	}
}

// TestGenerate_Idempotent ensures two calls with the same params return
// byte-identical output — a re-run of `dddns config set-mode` should
// produce the same script and therefore no diff on disk.
func TestGenerate_Idempotent(t *testing.T) {
	for _, mode := range []string{"cron", "serve"} {
		a, _ := Generate(DefaultUnifiParams(mode))
		b, _ := Generate(DefaultUnifiParams(mode))
		if a != b {
			t.Errorf("Generate(%q) is not idempotent", mode)
		}
	}
}

func TestGenerate_CustomPaths(t *testing.T) {
	p := Params{
		Mode:           "cron",
		BinaryPath:     "/opt/dddns/bin/dddns",
		ConfigDir:      "/opt/dddns/etc",
		CronEntryPath:  "/opt/dddns/etc/cron",
		UpdateLogPath:  "/opt/dddns/log/update.log",
		UpdateInterval: "*/15 * * * *",
	}
	out, err := Generate(p)
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, out, []string{
		"/opt/dddns/bin/dddns",
		"/opt/dddns/etc",
		"/opt/dddns/etc/cron",
		"/opt/dddns/log/update.log",
		"*/15 * * * *",
	})
}

func mustContain(t *testing.T, s string, needles []string) {
	t.Helper()
	for _, n := range needles {
		if !strings.Contains(s, n) {
			t.Errorf("output missing %q\n---\n%s", n, s)
		}
	}
}

func mustNotContain(t *testing.T, s string, needles []string) {
	t.Helper()
	for _, n := range needles {
		if strings.Contains(s, n) {
			t.Errorf("output unexpectedly contains %q", n)
		}
	}
}
