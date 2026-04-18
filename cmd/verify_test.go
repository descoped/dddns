package cmd

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/descoped/dddns/internal/verify"
)

// TestFormatVerifyReport_AllConsistent is the happy-path invariant:
// every source reports the same IP as the public one; the summary
// line reads "up to date" and every check gets a ✓. This is what
// users see after a successful `dddns update`.
func TestFormatVerifyReport_AllConsistent(t *testing.T) {
	const ip = "203.0.113.10"
	report := &verify.Report{
		PublicIP:  ip,
		Route53IP: ip,
		StdlibIP:  ip,
		Resolvers: []verify.ResolverResult{
			{Name: "Google", IP: ip},
			{Name: "Cloudflare", IP: ip},
		},
	}

	buf := &bytes.Buffer{}
	formatVerifyReport(buf, report, 300)
	out := buf.String()

	mustContainAll(t, out, []string{
		"Your public IP:     " + ip,
		"Route53 record:     " + ip + " ✓",
		"Public DNS lookup:  " + ip + " ✓",
		"Google: " + ip + " ✓",
		"Cloudflare: " + ip + " ✓",
		"✓ Route53 record is up to date",
		"300 seconds",
	})
	if strings.Contains(out, "mismatch") || strings.Contains(out, "FAILED") {
		t.Errorf("consistent report rendered mismatch/FAILED:\n%s", out)
	}
}

// TestFormatVerifyReport_Route53Mismatch verifies the summary flips
// to "doesn't match" and instructs the user to run update. The exact
// text is part of the user-contract — copy/paste in README.
func TestFormatVerifyReport_Route53Mismatch(t *testing.T) {
	report := &verify.Report{
		PublicIP:  "203.0.113.10",
		Route53IP: "198.51.100.5", // stale
		StdlibIP:  "198.51.100.5",
		Resolvers: []verify.ResolverResult{
			{Name: "Google", IP: "198.51.100.5"},
		},
	}

	buf := &bytes.Buffer{}
	formatVerifyReport(buf, report, 300)
	out := buf.String()

	mustContainAll(t, out, []string{
		"Route53 record:     198.51.100.5 ✗ (mismatch)",
		"Public DNS lookup:  198.51.100.5 ✗ (mismatch)",
		"Google: 198.51.100.5 ✗",
		"doesn't match current IP",
		"Run 'dddns update' to fix this",
	})
}

// TestFormatVerifyReport_Route53Missing covers the "no record yet"
// case a fresh operator sees before the first update. The summary
// must tell them what to do.
func TestFormatVerifyReport_Route53Missing(t *testing.T) {
	report := &verify.Report{
		PublicIP:     "203.0.113.10",
		Route53Error: errors.New("A record not found for test.example.com"),
	}

	buf := &bytes.Buffer{}
	formatVerifyReport(buf, report, 300)
	out := buf.String()

	mustContainAll(t, out, []string{
		"Route53 record:     NOT FOUND",
		"A record not found",
		"No Route53 record found - run 'dddns update' to create it",
	})
}

// TestFormatVerifyReport_StdlibFailureAndEmpty covers the two non-
// error Stdlib branches: hard error and "NO A RECORD" (lookup
// succeeded, zero answers). Both must appear distinctly in the output
// so the operator can tell a resolver outage from a missing record.
func TestFormatVerifyReport_StdlibFailureAndEmpty(t *testing.T) {
	t.Run("failure", func(t *testing.T) {
		report := &verify.Report{
			PublicIP:    "203.0.113.10",
			Route53IP:   "203.0.113.10",
			StdlibError: errors.New("DNS timeout"),
		}
		buf := &bytes.Buffer{}
		formatVerifyReport(buf, report, 60)
		if !strings.Contains(buf.String(), "Public DNS lookup:  FAILED (DNS timeout)") {
			t.Errorf("expected Public DNS lookup FAILED line, got:\n%s", buf.String())
		}
	})

	t.Run("empty_record", func(t *testing.T) {
		report := &verify.Report{
			PublicIP:  "203.0.113.10",
			Route53IP: "203.0.113.10",
			StdlibIP:  "", // no v4 answer
		}
		buf := &bytes.Buffer{}
		formatVerifyReport(buf, report, 60)
		if !strings.Contains(buf.String(), "Public DNS lookup:  NO A RECORD") {
			t.Errorf("expected Public DNS lookup NO A RECORD line, got:\n%s", buf.String())
		}
	})
}

// TestFormatVerifyReport_PerResolverOutcomes covers all three branches
// of the resolver loop: FAILED, NO RECORD, and IP match/mismatch.
func TestFormatVerifyReport_PerResolverOutcomes(t *testing.T) {
	report := &verify.Report{
		PublicIP: "203.0.113.10",
		Resolvers: []verify.ResolverResult{
			{Name: "Ok", IP: "203.0.113.10"},
			{Name: "Stale", IP: "198.51.100.5"},
			{Name: "Broken", Error: errors.New("timeout")},
			{Name: "Empty", IP: ""},
		},
	}

	buf := &bytes.Buffer{}
	formatVerifyReport(buf, report, 60)
	out := buf.String()

	mustContainAll(t, out, []string{
		"Ok: 203.0.113.10 ✓",
		"Stale: 198.51.100.5 ✗",
		"Broken: FAILED",
		"Empty: NO RECORD",
	})
}
