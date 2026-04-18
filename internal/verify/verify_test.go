package verify

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/descoped/dddns/internal/config"
)

// fakeRoute53 is a minimal route53Getter stub. ip is returned on success;
// err (when non-nil) takes precedence and is returned as-is.
type fakeRoute53 struct {
	ip  string
	err error
}

func (f *fakeRoute53) GetCurrentIP(_ context.Context) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.ip, nil
}

// swapHooks installs the given replacements for the package-level test
// hooks and returns a cleanup function that restores the originals. Use
// t.Cleanup to ensure restoration even on test panic.
func swapHooks(
	t *testing.T,
	pub func(ctx context.Context) (string, error),
	r53 func(ctx context.Context, cfg *config.Config) (route53Getter, error),
	std func(ctx context.Context, host string) ([]net.IPAddr, error),
	named func(ctx context.Context, hostname, server string) (string, error),
) {
	t.Helper()
	origPub, origR53, origStd, origNamed := fetchPublicIP, newRoute53, stdLookup, queryNamed
	fetchPublicIP, newRoute53, stdLookup, queryNamed = pub, r53, std, named
	t.Cleanup(func() {
		fetchPublicIP, newRoute53, stdLookup, queryNamed = origPub, origR53, origStd, origNamed
	})
}

// testCfg returns a minimal Config that Run can consume. Fields below
// satisfy the Route53 constructor's contract; production values are
// irrelevant because newRoute53 is stubbed.
func testCfg() *config.Config {
	return &config.Config{
		AWSRegion:    "us-east-1",
		AWSAccessKey: "AKIAIOSFODNN7EXAMPLE",
		AWSSecretKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		HostedZoneID: "Z1ABCDEFGHIJKL",
		Hostname:     "test.example.com",
		TTL:          300,
	}
}

// TestRun_HappyPath exercises the orchestration invariant: when every
// sub-step succeeds, the Report reflects each observed IP and no error
// fields are populated.
func TestRun_HappyPath(t *testing.T) {
	const publicIP = "203.0.113.10" // RFC 5737 TEST-NET-3

	swapHooks(t,
		func(_ context.Context) (string, error) { return publicIP, nil },
		func(_ context.Context, _ *config.Config) (route53Getter, error) {
			return &fakeRoute53{ip: publicIP}, nil
		},
		func(_ context.Context, _ string) ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP(publicIP)}}, nil
		},
		func(_ context.Context, _, _ string) (string, error) { return publicIP, nil },
	)

	rep, err := Run(context.Background(), testCfg())
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if rep.PublicIP != publicIP {
		t.Errorf("PublicIP = %q, want %q", rep.PublicIP, publicIP)
	}
	if rep.Route53IP != publicIP || rep.Route53Error != nil {
		t.Errorf("Route53 = %q / err=%v, want %q / nil", rep.Route53IP, rep.Route53Error, publicIP)
	}
	if rep.StdlibIP != publicIP || rep.StdlibError != nil {
		t.Errorf("Stdlib = %q / err=%v, want %q / nil", rep.StdlibIP, rep.StdlibError, publicIP)
	}
	if len(rep.Resolvers) != len(namedResolvers) {
		t.Fatalf("Resolvers len = %d, want %d", len(rep.Resolvers), len(namedResolvers))
	}
	for i, r := range rep.Resolvers {
		if r.IP != publicIP || r.Error != nil {
			t.Errorf("Resolvers[%d] = %q / err=%v, want %q / nil", i, r.IP, r.Error, publicIP)
		}
		if r.Name != namedResolvers[i].Name || r.Server != namedResolvers[i].Address {
			t.Errorf("Resolvers[%d] metadata = (%q,%q), want (%q,%q)",
				i, r.Name, r.Server, namedResolvers[i].Name, namedResolvers[i].Address)
		}
	}
}

// TestRun_PublicIPFailurePropagates guards the contract that a public-IP
// lookup failure is the ONE condition that returns a non-nil error from
// Run. Without public IP there's nothing to compare against, so partial
// results would be misleading.
func TestRun_PublicIPFailurePropagates(t *testing.T) {
	wantErr := errors.New("checkip unreachable")

	swapHooks(t,
		func(_ context.Context) (string, error) { return "", wantErr },
		// The rest should never be called. Use panicking stubs to prove it.
		func(_ context.Context, _ *config.Config) (route53Getter, error) {
			t.Fatal("newRoute53 called after public-IP failure")
			return nil, nil
		},
		func(_ context.Context, _ string) ([]net.IPAddr, error) {
			t.Fatal("stdLookup called after public-IP failure")
			return nil, nil
		},
		func(_ context.Context, _, _ string) (string, error) {
			t.Fatal("queryNamed called after public-IP failure")
			return "", nil
		},
	)

	rep, err := Run(context.Background(), testCfg())
	if rep != nil {
		t.Errorf("Run returned non-nil report on public-IP failure: %+v", rep)
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("Run err = %v, want %v", err, wantErr)
	}
}

// TestRun_Route53ErrorFoldedIntoReport verifies the partial-results
// invariant: Route53 failures don't abort the flow, they land in
// Report.Route53Error and the other sub-steps still run.
func TestRun_Route53ErrorFoldedIntoReport(t *testing.T) {
	const publicIP = "198.51.100.5" // RFC 5737 TEST-NET-2
	r53Err := errors.New("route53 client construction failed")

	swapHooks(t,
		func(_ context.Context) (string, error) { return publicIP, nil },
		func(_ context.Context, _ *config.Config) (route53Getter, error) { return nil, r53Err },
		func(_ context.Context, _ string) ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP(publicIP)}}, nil
		},
		func(_ context.Context, _, _ string) (string, error) { return publicIP, nil },
	)

	rep, err := Run(context.Background(), testCfg())
	if err != nil {
		t.Fatalf("Run returned error despite Route53 failure: %v", err)
	}
	if !errors.Is(rep.Route53Error, r53Err) {
		t.Errorf("Route53Error = %v, want %v", rep.Route53Error, r53Err)
	}
	if rep.Route53IP != "" {
		t.Errorf("Route53IP = %q on constructor failure, want empty", rep.Route53IP)
	}
	// Stdlib and resolvers must still have populated.
	if rep.StdlibIP != publicIP {
		t.Errorf("StdlibIP = %q; Route53 failure should not abort stdlib lookup", rep.StdlibIP)
	}
	if len(rep.Resolvers) != len(namedResolvers) {
		t.Errorf("Resolvers len = %d; Route53 failure should not abort resolver sweep", len(rep.Resolvers))
	}
}

// TestRun_Route53GetCurrentIPError verifies that a successfully-constructed
// client whose GetCurrentIP then fails lands the error in the report,
// distinct from the constructor-failed case above.
func TestRun_Route53GetCurrentIPError(t *testing.T) {
	const publicIP = "203.0.113.42"
	getErr := errors.New("A record not found")

	swapHooks(t,
		func(_ context.Context) (string, error) { return publicIP, nil },
		func(_ context.Context, _ *config.Config) (route53Getter, error) {
			return &fakeRoute53{err: getErr}, nil
		},
		func(_ context.Context, _ string) ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP(publicIP)}}, nil
		},
		func(_ context.Context, _, _ string) (string, error) { return publicIP, nil },
	)

	rep, err := Run(context.Background(), testCfg())
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !errors.Is(rep.Route53Error, getErr) {
		t.Errorf("Route53Error = %v, want %v", rep.Route53Error, getErr)
	}
	if rep.Route53IP != "" {
		t.Errorf("Route53IP = %q on GetCurrentIP failure, want empty", rep.Route53IP)
	}
}

// TestRun_StdlibLookupErrorFoldedIntoReport mirrors the Route53-failure
// contract for the stdlib path: error is recorded, the rest of the flow
// completes.
func TestRun_StdlibLookupErrorFoldedIntoReport(t *testing.T) {
	const publicIP = "203.0.113.1"
	lookupErr := errors.New("DNS server unreachable")

	swapHooks(t,
		func(_ context.Context) (string, error) { return publicIP, nil },
		func(_ context.Context, _ *config.Config) (route53Getter, error) {
			return &fakeRoute53{ip: publicIP}, nil
		},
		func(_ context.Context, _ string) ([]net.IPAddr, error) {
			return nil, lookupErr
		},
		func(_ context.Context, _, _ string) (string, error) { return publicIP, nil },
	)

	rep, err := Run(context.Background(), testCfg())
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !errors.Is(rep.StdlibError, lookupErr) {
		t.Errorf("StdlibError = %v, want %v", rep.StdlibError, lookupErr)
	}
	if rep.StdlibIP != "" {
		t.Errorf("StdlibIP = %q on lookup failure, want empty", rep.StdlibIP)
	}
	// Route53 and named resolvers still ran.
	if rep.Route53IP != publicIP {
		t.Errorf("Route53IP = %q; stdlib failure should not abort Route53 path", rep.Route53IP)
	}
	if len(rep.Resolvers) != len(namedResolvers) {
		t.Errorf("Resolvers len = %d; stdlib failure should not abort resolver sweep", len(rep.Resolvers))
	}
}

// TestRun_StdlibSkipsIPv6 verifies that when DefaultResolver returns a
// mix of IPv6 and IPv4 addresses, Run picks the first IPv4 for StdlibIP.
// This is the non-obvious branch in the lookup-result loop.
func TestRun_StdlibSkipsIPv6(t *testing.T) {
	const wantIPv4 = "203.0.113.77"

	swapHooks(t,
		func(_ context.Context) (string, error) { return wantIPv4, nil },
		func(_ context.Context, _ *config.Config) (route53Getter, error) {
			return &fakeRoute53{ip: wantIPv4}, nil
		},
		func(_ context.Context, _ string) ([]net.IPAddr, error) {
			return []net.IPAddr{
				{IP: net.ParseIP("2001:db8::1")}, // RFC 3849 documentation prefix
				{IP: net.ParseIP(wantIPv4)},
			}, nil
		},
		func(_ context.Context, _, _ string) (string, error) { return wantIPv4, nil },
	)

	rep, err := Run(context.Background(), testCfg())
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if rep.StdlibIP != wantIPv4 {
		t.Errorf("StdlibIP = %q, want %q (first IPv4 even after IPv6)", rep.StdlibIP, wantIPv4)
	}
}

// TestRun_ZeroResolversReturnEmpty covers the "lookup succeeded but
// returned no A records" branch: queryNamed returns ("", nil). Those
// results land in the report as IP="" Error=nil — distinct from timeout.
func TestRun_ZeroResolversReturnEmpty(t *testing.T) {
	const publicIP = "203.0.113.99"

	swapHooks(t,
		func(_ context.Context) (string, error) { return publicIP, nil },
		func(_ context.Context, _ *config.Config) (route53Getter, error) {
			return &fakeRoute53{ip: publicIP}, nil
		},
		func(_ context.Context, _ string) ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP(publicIP)}}, nil
		},
		func(_ context.Context, _, _ string) (string, error) { return "", nil }, // NODATA: success with no records
	)

	rep, err := Run(context.Background(), testCfg())
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	for i, r := range rep.Resolvers {
		if r.IP != "" || r.Error != nil {
			t.Errorf("Resolvers[%d] = (%q, %v), want (\"\", nil) for NODATA", i, r.IP, r.Error)
		}
	}
}

// TestRun_PerResolverTimeout covers the timeout branch: queryNamed returns
// context.DeadlineExceeded for a single resolver, which must land in that
// entry's Error field without contaminating other resolvers' results.
func TestRun_PerResolverTimeout(t *testing.T) {
	const publicIP = "203.0.113.50"
	timeoutErr := context.DeadlineExceeded

	swapHooks(t,
		func(_ context.Context) (string, error) { return publicIP, nil },
		func(_ context.Context, _ *config.Config) (route53Getter, error) {
			return &fakeRoute53{ip: publicIP}, nil
		},
		func(_ context.Context, _ string) ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP(publicIP)}}, nil
		},
		func(_ context.Context, _, server string) (string, error) {
			if server == namedResolvers[1].Address {
				return "", timeoutErr
			}
			return publicIP, nil
		},
	)

	rep, err := Run(context.Background(), testCfg())
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	for i, r := range rep.Resolvers {
		if i == 1 {
			if !errors.Is(r.Error, timeoutErr) {
				t.Errorf("Resolvers[%d].Error = %v, want %v", i, r.Error, timeoutErr)
			}
			if r.IP != "" {
				t.Errorf("Resolvers[%d].IP = %q on timeout, want empty", i, r.IP)
			}
			continue
		}
		if r.Error != nil || r.IP != publicIP {
			t.Errorf("Resolvers[%d] = (%q, %v), want (%q, nil) for non-failing resolver", i, r.IP, r.Error, publicIP)
		}
	}
}

// TestRun_IPMismatchAcrossSourcesIsReported captures the "different
// resolvers see different IPs" scenario — this is precisely what
// `dddns verify` exists to surface. The report should record each IP
// distinctly; it's the CLI formatter's job (not Run's) to flag the
// mismatch.
func TestRun_IPMismatchAcrossSourcesIsReported(t *testing.T) {
	const publicIP = "203.0.113.10"
	const route53IP = "203.0.113.11" // one octet off — propagation lag
	const stdlibIP = "203.0.113.12"

	swapHooks(t,
		func(_ context.Context) (string, error) { return publicIP, nil },
		func(_ context.Context, _ *config.Config) (route53Getter, error) {
			return &fakeRoute53{ip: route53IP}, nil
		},
		func(_ context.Context, _ string) ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP(stdlibIP)}}, nil
		},
		func(_ context.Context, _, server string) (string, error) {
			switch server {
			case namedResolvers[0].Address:
				return publicIP, nil
			case namedResolvers[1].Address:
				return route53IP, nil
			default:
				return stdlibIP, nil
			}
		},
	)

	rep, err := Run(context.Background(), testCfg())
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if rep.PublicIP != publicIP {
		t.Errorf("PublicIP = %q, want %q", rep.PublicIP, publicIP)
	}
	if rep.Route53IP != route53IP {
		t.Errorf("Route53IP = %q, want %q", rep.Route53IP, route53IP)
	}
	if rep.StdlibIP != stdlibIP {
		t.Errorf("StdlibIP = %q, want %q", rep.StdlibIP, stdlibIP)
	}
	wantResolverIPs := []string{publicIP, route53IP, stdlibIP}
	for i, want := range wantResolverIPs {
		if rep.Resolvers[i].IP != want {
			t.Errorf("Resolvers[%d].IP = %q, want %q", i, rep.Resolvers[i].IP, want)
		}
	}
}

// TestQueryResolver_InvalidServerReturnsError exercises the live
// queryResolver against an unroutable RFC 5737 TEST-NET-1 address.
// The 2-second internal timeout guarantees the test completes quickly.
// Covers the err != nil branch at line 119 of verify.go.
func TestQueryResolver_InvalidServerReturnsError(t *testing.T) {
	// Fabricated TEST-NET-1 address on a non-standard port — guaranteed
	// unroutable on any well-behaved network.
	const bogusServer = "192.0.2.1:53535"

	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ip, err := queryResolver(ctx, "test.example.com", bogusServer)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatalf("queryResolver(%s) unexpectedly succeeded with IP %q", bogusServer, ip)
	}
	if elapsed > 4*time.Second {
		t.Errorf("queryResolver took %v — internal 2s timeout should cap it well below 4s", elapsed)
	}
}

// Build-time check: fakeRoute53 must satisfy route53Getter.
var _ route53Getter = (*fakeRoute53)(nil)
