package main

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-lambda-go/events"
)

// dnsClient is the subset of the Route53 client the Lambda handler
// needs. *dns.Route53Client satisfies it; tests use a stub.
type dnsClient interface {
	UpdateIP(ctx context.Context, ip string) error
}

// handler owns the per-invocation flow. Constructed once at Lambda
// init by main; lambda.Start routes every request through handle.
type handler struct {
	cfg         *config
	route53     dnsClient
	ssm         *ssmClient
	secretCache *secretCache
}

// secretCache holds the SSM-fetched shared secret between invocations.
// Lambda container reuse means we pay the SSM GetParameter round trip
// at most every `ttl` seconds instead of every request — which keeps
// the steady-state cost low while still picking up rotations promptly.
// Rotations propagate within `ttl` automatically; during the overlap
// window the old secret also still works (UniFi UI caches on its side
// too), so we don't strictly need to invalidate on auth failure.
type secretCache struct {
	mu        sync.Mutex
	value     string
	fetchedAt time.Time
	ttl       time.Duration
	now       func() time.Time
}

func (c *secretCache) get(ctx context.Context, ssm *ssmClient, name string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.value != "" && c.now().Sub(c.fetchedAt) < c.ttl {
		return c.value, nil
	}
	v, err := ssm.getParameter(ctx, name)
	if err != nil {
		// Surface the freshest SSM error even if we have a stale cached
		// value — refusing to auth when SSM is unreachable is the
		// fail-closed posture we want. UniFi will retry.
		return "", err
	}
	c.value = v
	c.fetchedAt = c.now()
	return v, nil
}

// handle implements the dyndns v2 protocol
// (https://help.dyn.com/remote-access-api/perform-update/). The
// response body is a plain-text diagnostic ("good <ip>" / "nohost" /
// "badauth" / "dnserr" / "nochg <ip>"), HTTP 200 in all cases —
// dyndns clients inspect the body, not the status code.
func (h *handler) handle(ctx context.Context, req events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
	// L6 — the only IP we will publish is the TCP source IP that
	// API Gateway recorded. The myip query parameter is ignored
	// entirely (same posture as serve-mode on the listener: the
	// client's claim is not authoritative). On API Gateway HTTP
	// API, RequestContext.HTTP.SourceIP is populated from the TCP
	// connection peer, so this is ground truth.
	sourceIP := req.RequestContext.HTTP.SourceIP
	if sourceIP == "" {
		// Should never happen under API Gateway HTTP API — if it
		// does, we're in a misconfigured environment and blind
		// publishing is unsafe. Fail closed.
		return dyndns("dnserr no source ip"), nil
	}
	if net.ParseIP(sourceIP) == nil {
		return dyndns("dnserr source ip unparseable"), nil
	}

	// Auth — Basic, constant-time compare against SSM-stored secret.
	user, pass, ok := parseBasicAuth(req.Headers)
	if !ok {
		return dyndns("badauth"), nil
	}
	_ = user // UniFi's inadyn sends a username; we ignore it and auth on the secret only.

	expected, err := h.secretCache.get(ctx, h.ssm, h.cfg.ssmSecretParam)
	if err != nil {
		log.Printf("ssm fetch failed: %v", err)
		return dyndns("dnserr ssm"), nil
	}
	if subtle.ConstantTimeCompare([]byte(pass), []byte(expected)) != 1 {
		return dyndns("badauth"), nil
	}

	// Hostname match — RFC 1035 case-insensitive. Mirror serve-mode
	// behaviour from internal/server/handler.go.
	hostname := strings.TrimSpace(req.QueryStringParameters["hostname"])
	if hostname == "" {
		return dyndns("notfqdn"), nil
	}
	if !strings.EqualFold(hostname, h.cfg.hostname) {
		return dyndns("nohost"), nil
	}

	// Route53 UPSERT. Bound by the handler's context (API Gateway
	// applies its own 30s integration timeout; we add a shorter
	// deadline so a hung Route53 call can't eat our execution
	// budget).
	upctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := h.route53.UpdateIP(upctx, sourceIP); err != nil {
		log.Printf("route53 update failed: %v", err)
		return dyndns("dnserr " + err.Error()), nil
	}

	return dyndns("good " + sourceIP), nil
}

// dyndns builds a minimal HTTP 200 text/plain response.
func dyndns(body string) events.APIGatewayV2HTTPResponse {
	return events.APIGatewayV2HTTPResponse{
		StatusCode: 200,
		Headers:    map[string]string{"Content-Type": "text/plain; charset=utf-8"},
		Body:       body + "\n",
	}
}

// parseBasicAuth extracts user+pass from the standard Basic header.
// Case-insensitive header lookup — API Gateway normalizes header keys
// to lowercase, but tests may populate them either way.
func parseBasicAuth(headers map[string]string) (user, pass string, ok bool) {
	var raw string
	for k, v := range headers {
		if strings.EqualFold(k, "Authorization") {
			raw = v
			break
		}
	}
	if !strings.HasPrefix(raw, "Basic ") {
		return "", "", false
	}
	decoded, err := base64.StdEncoding.DecodeString(raw[len("Basic "):])
	if err != nil {
		return "", "", false
	}
	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	return parts[0], parts[1], true
}

