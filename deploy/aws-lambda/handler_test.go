package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
)

// stubRoute53 records the IPs it was asked to publish. Mirrors the
// dnsClient interface exactly so it drops into handler.route53.
type stubRoute53 struct {
	mu      sync.Mutex
	pushed  []string
	failErr error
}

func (s *stubRoute53) UpdateIP(_ context.Context, ip string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failErr != nil {
		return s.failErr
	}
	s.pushed = append(s.pushed, ip)
	return nil
}

func (s *stubRoute53) last() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.pushed) == 0 {
		return ""
	}
	return s.pushed[len(s.pushed)-1]
}

// Fixtures — all use RFC 5737 TEST-NET-3 addresses and RFC 2606
// example.com domains so nothing resembles a real deployment.
const (
	testZoneID   = "Z1ABCDEFGHIJKL"
	testHostname = "home.example.com"
	testSecret   = "correct-horse-battery-staple-for-tests-only"
	testSourceIP = "203.0.113.42"
)

// newTestHandler wires a *handler with a stub Route53 client and an
// httptest server standing in for SSM. The Route53 stub accepts any
// UpdateIP call and records the IP; the SSM stub returns testSecret
// as the parameter value. Tests can override the SSM behaviour.
func newTestHandler(t *testing.T, ssmHandler http.HandlerFunc) (*handler, *stubRoute53) {
	t.Helper()

	if ssmHandler == nil {
		ssmHandler = defaultSSMStub
	}
	ssmSrv := httptest.NewServer(ssmHandler)
	t.Cleanup(ssmSrv.Close)

	cfg := &config{
		region:         "us-east-1",
		accessKey:      "AKIATEST",
		secretKey:      "SECRETTEST",
		sessionToken:   "SESSIONTOKENTEST",
		hostedZoneID:   testZoneID,
		hostname:       testHostname,
		ssmSecretParam: "/dddns/test/shared_secret",
		ttl:            300,
		lookupTimeout:  5 * time.Second,
	}

	r53 := &stubRoute53{}
	ssm := &ssmClient{
		region:       cfg.region,
		accessKey:    cfg.accessKey,
		secretKey:    cfg.secretKey,
		sessionToken: cfg.sessionToken,
		httpClient:   ssmSrv.Client(),
		endpoint:     ssmSrv.URL,
		now:          time.Now,
	}

	return &handler{
		cfg:         cfg,
		route53:     r53,
		ssm:         ssm,
		secretCache: &secretCache{ttl: time.Minute, now: time.Now},
	}, r53
}

func defaultSSMStub(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	// Sanity-check request shape.
	if got, want := r.Header.Get("X-Amz-Target"), "AmazonSSM.GetParameter"; got != want {
		http.Error(w, fmt.Sprintf("unexpected target %q", got), http.StatusBadRequest)
		return
	}
	if !strings.Contains(string(body), `"WithDecryption":true`) {
		http.Error(w, "expected WithDecryption:true", http.StatusBadRequest)
		return
	}
	resp := getParameterResponse{}
	resp.Parameter.Name = "/dddns/test/shared_secret"
	resp.Parameter.Value = testSecret
	resp.Parameter.Type = "SecureString"
	w.Header().Set("Content-Type", "application/x-amz-json-1.1")
	_ = json.NewEncoder(w).Encode(resp)
}

func mkRequest(auth, hostname, sourceIP string) events.APIGatewayV2HTTPRequest {
	headers := map[string]string{}
	if auth != "" {
		headers["Authorization"] = auth
	}
	req := events.APIGatewayV2HTTPRequest{
		Headers: headers,
		QueryStringParameters: map[string]string{
			"hostname": hostname,
			"myip":     "198.51.100.1", // deliberately NOT what we expect — handler must ignore this and use SourceIP
		},
	}
	req.RequestContext.HTTP.SourceIP = sourceIP
	return req
}

func basicAuth(user, pass string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass))
}

func TestHandler_HappyPath_PublishesSourceIP(t *testing.T) {
	h, _ := newTestHandler(t, nil)

	resp, err := h.handle(context.Background(), mkRequest(basicAuth("dddns", testSecret), testHostname, testSourceIP))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if got := strings.TrimSpace(resp.Body); got != "good "+testSourceIP {
		t.Errorf("body = %q, want 'good %s'", got, testSourceIP)
	}
	if resp.StatusCode != 200 {
		t.Errorf("status = %d", resp.StatusCode)
	}
}

func TestHandler_WrongSecret_Badauth(t *testing.T) {
	h, _ := newTestHandler(t, nil)

	resp, _ := h.handle(context.Background(), mkRequest(basicAuth("dddns", "wrong-secret"), testHostname, testSourceIP))
	if got := strings.TrimSpace(resp.Body); got != "badauth" {
		t.Errorf("body = %q, want 'badauth'", got)
	}
}

func TestHandler_MissingAuth_Badauth(t *testing.T) {
	h, _ := newTestHandler(t, nil)

	resp, _ := h.handle(context.Background(), mkRequest("", testHostname, testSourceIP))
	if got := strings.TrimSpace(resp.Body); got != "badauth" {
		t.Errorf("body = %q, want 'badauth'", got)
	}
}

func TestHandler_MalformedAuth_Badauth(t *testing.T) {
	h, _ := newTestHandler(t, nil)

	// Not Base64, not colon-separated.
	resp, _ := h.handle(context.Background(), mkRequest("Basic garbage!!", testHostname, testSourceIP))
	if got := strings.TrimSpace(resp.Body); got != "badauth" {
		t.Errorf("body = %q, want 'badauth'", got)
	}
}

func TestHandler_WrongHostname_Nohost(t *testing.T) {
	h, _ := newTestHandler(t, nil)

	resp, _ := h.handle(context.Background(), mkRequest(basicAuth("dddns", testSecret), "other.example.com", testSourceIP))
	if got := strings.TrimSpace(resp.Body); got != "nohost" {
		t.Errorf("body = %q, want 'nohost'", got)
	}
}

func TestHandler_HostnameCaseInsensitive(t *testing.T) {
	h, _ := newTestHandler(t, nil)

	resp, _ := h.handle(context.Background(), mkRequest(basicAuth("dddns", testSecret), strings.ToUpper(testHostname), testSourceIP))
	if got := strings.TrimSpace(resp.Body); got != "good "+testSourceIP {
		t.Errorf("body = %q — RFC 1035 case-insensitive match should have hit", got)
	}
}

func TestHandler_EmptyHostname_Notfqdn(t *testing.T) {
	h, _ := newTestHandler(t, nil)

	resp, _ := h.handle(context.Background(), mkRequest(basicAuth("dddns", testSecret), "", testSourceIP))
	if got := strings.TrimSpace(resp.Body); got != "notfqdn" {
		t.Errorf("body = %q, want 'notfqdn'", got)
	}
}

func TestHandler_MissingSourceIP_Dnserr(t *testing.T) {
	h, _ := newTestHandler(t, nil)

	// No sourceIP set. Under real API Gateway this can't happen; tests
	// verify the fail-closed branch fires when it somehow does.
	resp, _ := h.handle(context.Background(), mkRequest(basicAuth("dddns", testSecret), testHostname, ""))
	if got := strings.TrimSpace(resp.Body); !strings.HasPrefix(got, "dnserr") {
		t.Errorf("body = %q, want dnserr prefix", got)
	}
}

func TestHandler_MyipParamIgnored(t *testing.T) {
	// Specifically: the myip query param in mkRequest is 198.51.100.1
	// (RFC 5737 TEST-NET-2) but the handler MUST publish SourceIP
	// (203.0.113.42, TEST-NET-3) instead. If it ever publishes the
	// myip value, the body won't match.
	h, _ := newTestHandler(t, nil)

	resp, _ := h.handle(context.Background(), mkRequest(basicAuth("dddns", testSecret), testHostname, testSourceIP))
	body := strings.TrimSpace(resp.Body)
	if !strings.Contains(body, testSourceIP) {
		t.Errorf("body %q missing source IP %s", body, testSourceIP)
	}
	if strings.Contains(body, "198.51.100.1") {
		t.Errorf("body %q leaked myip query param — handler is trusting client-supplied value", body)
	}
}

func TestSecretCache_SingleRoundTrip(t *testing.T) {
	// Verify the cache TTL suppresses duplicate SSM fetches — critical
	// for cost (SSM GetParameter is $0.05 per 10k requests, which adds
	// up on a request-flooding day without caching).
	var calls int
	ssmHandler := func(w http.ResponseWriter, r *http.Request) {
		calls++
		defaultSSMStub(w, r)
	}
	h, _ := newTestHandler(t, ssmHandler)

	req := mkRequest(basicAuth("dddns", testSecret), testHostname, testSourceIP)

	// Three back-to-back invocations — expect exactly one SSM call.
	for i := 0; i < 3; i++ {
		if _, err := h.handle(context.Background(), req); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 1 {
		t.Errorf("ssm.getParameter called %d times, want 1 — cache not honouring TTL", calls)
	}
}

func TestParseBasicAuth(t *testing.T) {
	cases := []struct {
		name, header, user, pass string
		ok                       bool
	}{
		{"standard", "Basic " + base64.StdEncoding.EncodeToString([]byte("u:p")), "u", "p", true},
		{"case-insensitive prefix not supported", "basic " + base64.StdEncoding.EncodeToString([]byte("u:p")), "", "", false},
		{"empty", "", "", "", false},
		{"bearer token", "Bearer xyz", "", "", false},
		{"missing colon", "Basic " + base64.StdEncoding.EncodeToString([]byte("nocolon")), "", "", false},
		{"invalid base64", "Basic %%%", "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			u, p, ok := parseBasicAuth(map[string]string{"Authorization": tc.header})
			if ok != tc.ok || u != tc.user || p != tc.pass {
				t.Errorf("parseBasicAuth(%q) = (%q, %q, %v), want (%q, %q, %v)",
					tc.header, u, p, ok, tc.user, tc.pass, tc.ok)
			}
		})
	}
}

func TestParseBasicAuth_LowercaseHeaderKey(t *testing.T) {
	// API Gateway normalizes header keys to lowercase before handing
	// them to the Lambda. Verify the lookup is case-insensitive.
	h := map[string]string{"authorization": basicAuth("dddns", testSecret)}
	u, p, ok := parseBasicAuth(h)
	if !ok || u != "dddns" || p != testSecret {
		t.Errorf("lowercase header key not matched: got (%q, %q, %v)", u, p, ok)
	}
}
