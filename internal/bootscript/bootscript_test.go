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
		"pkill -f \"dddns serve\"", // stop serve loop before installing cron
		"*/30 * * * * root /usr/local/bin/dddns update --quiet",
		"/var/log/dddns.log",
		"/etc/init.d/cron restart",
	})
	mustNotContain(t, out, []string{
		"dddns serve >> /var/log/dddns-server.log",
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
		"rm -f \"$CRON_PATH\"",
		"pkill -f \"dddns serve\"",
		"while true; do",
		"/usr/local/bin/dddns serve >> /var/log/dddns-server.log",
		"sleep 5",
	})
	// The common header references CRON_PATH so removing a stale cron
	// entry can work; what must NOT be present in serve mode is the
	// cron installation (the `cat > "$CRON_PATH"` heredoc).
	mustNotContain(t, out, []string{
		"update --quiet",
		`cat > "$CRON_PATH"`,
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
		Mode:           "serve",
		BinaryPath:     "/opt/dddns/bin/dddns",
		ConfigDir:      "/opt/dddns/etc",
		CronEntryPath:  "/opt/dddns/etc/cron",
		UpdateLogPath:  "/opt/dddns/log/update.log",
		ServerLogPath:  "/opt/dddns/log/server.log",
		UpdateInterval: "*/15 * * * *",
	}
	out, err := Generate(p)
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, out, []string{
		"/opt/dddns/bin/dddns",
		"/opt/dddns/etc",
		"/opt/dddns/log/server.log",
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
