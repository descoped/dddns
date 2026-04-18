// Package dns provides a minimal Route53 REST client (no AWS SDK).
//
// This client issues AWS SigV4-signed HTTP requests directly to the Route53
// API (version 2013-04-01) for the two operations dddns needs: listing a
// single A record set and upserting an A record. The public signatures match
// the prior SDK-backed implementation so callers are unaffected.
package dns

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	dddnscfg "github.com/descoped/dddns/internal/config"
)

const (
	route53DefaultBaseURL = "https://route53.amazonaws.com"
	route53APIVersion     = "2013-04-01"
	route53Namespace      = "https://route53.amazonaws.com/doc/2013-04-01/"
	route53Service        = "route53"
	route53Region         = "us-east-1" // Route53 is a global service; SigV4 always uses us-east-1
)

// Route53Client issues signed HTTP requests to the Route53 REST API.
type Route53Client struct {
	accessKey    string
	secretKey    string
	hostedZoneID string
	hostname     string
	ttl          int64

	httpClient *http.Client // swappable for tests
	baseURL    string       // swappable for tests
	now        func() time.Time
}

// NewRoute53Client creates a Route53 client with the given static credentials.
//
// The region parameter is retained for API compatibility with earlier callers;
// Route53 is a global service so SigV4 signing always uses us-east-1 regardless
// of what the caller passes.
//
// ctx is accepted for API symmetry with the prior SDK-based constructor but
// is not currently used — construction is purely local (no network calls).
func NewRoute53Client(_ context.Context, _ /*region*/, accessKey, secretKey, hostedZoneID, hostname string, ttl int64) (*Route53Client, error) {
	if accessKey == "" || secretKey == "" {
		return nil, fmt.Errorf("AWS credentials are required in config file (aws_access_key and aws_secret_key)")
	}
	return &Route53Client{
		accessKey:    accessKey,
		secretKey:    secretKey,
		hostedZoneID: hostedZoneID,
		hostname:     hostname,
		ttl:          ttl,
		httpClient:   http.DefaultClient,
		baseURL:      route53DefaultBaseURL,
		now:          time.Now,
	}, nil
}

// NewFromConfig constructs a Route53Client from a fully-populated dddns Config.
func NewFromConfig(ctx context.Context, cfg *dddnscfg.Config) (*Route53Client, error) {
	return NewRoute53Client(ctx, cfg.AWSRegion, cfg.AWSAccessKey, cfg.AWSSecretKey, cfg.HostedZoneID, cfg.Hostname, cfg.TTL)
}

// fqdn returns the configured hostname in FQDN form (guaranteed trailing dot).
func (r *Route53Client) fqdn() string {
	if strings.HasSuffix(r.hostname, ".") {
		return r.hostname
	}
	return r.hostname + "."
}

// GetCurrentIP retrieves the current A record for the configured hostname.
func (r *Route53Client) GetCurrentIP(ctx context.Context) (string, error) {
	fqdn := r.fqdn()

	// Route53 API: GET /2013-04-01/hostedzone/{id}/rrset?name=X&type=A&maxitems=1
	// The `name` parameter is a cursor; the API returns the first record >= name.
	q := url.Values{
		"name":     {fqdn},
		"type":     {"A"},
		"maxitems": {"1"},
	}
	path := fmt.Sprintf("/%s/hostedzone/%s/rrset", route53APIVersion, r.hostedZoneID)
	endpoint := r.baseURL + path + "?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("build list request: %w", err)
	}

	respBody, err := r.do(req, emptyPayloadHash, nil)
	if err != nil {
		return "", fmt.Errorf("failed to list record sets: %w", err)
	}

	var parsed listResourceRecordSetsResponse
	if err := xml.Unmarshal(respBody, &parsed); err != nil {
		return "", fmt.Errorf("parse list response: %w", err)
	}

	for _, rs := range parsed.ResourceRecordSets {
		if rs.Name == fqdn && rs.Type == "A" && rs.ResourceRecords != nil && len(rs.ResourceRecords.ResourceRecord) > 0 {
			return rs.ResourceRecords.ResourceRecord[0].Value, nil
		}
	}
	return "", fmt.Errorf("A record not found for %s", r.hostname) //nolint:staticcheck // "A record" is a DNS term, not an article
}

// UpdateIP UPSERTs the A record with a new IP address.
// Callers are expected to handle dry-run short-circuits before invoking.
func (r *Route53Client) UpdateIP(ctx context.Context, newIP string) error {
	fqdn := r.fqdn()

	body := changeResourceRecordSetsRequest{
		Xmlns: route53Namespace,
		ChangeBatch: changeBatch{
			Changes: changes{
				Change: []change{
					{
						Action: "UPSERT",
						ResourceRecordSet: resourceRecordSet{
							Name: fqdn,
							Type: "A",
							TTL:  r.ttl,
							ResourceRecords: &resourceRecords{
								ResourceRecord: []resourceRecord{{Value: newIP}},
							},
						},
					},
				},
			},
		},
	}

	xmlBody, err := xml.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal change request: %w", err)
	}

	path := fmt.Sprintf("/%s/hostedzone/%s/rrset/", route53APIVersion, r.hostedZoneID)
	endpoint := r.baseURL + path

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(xmlBody))
	if err != nil {
		return fmt.Errorf("build change request: %w", err)
	}
	req.Header.Set("Content-Type", "application/xml")
	req.ContentLength = int64(len(xmlBody))

	payloadHash, _, err := hashBody(bytes.NewReader(xmlBody))
	if err != nil {
		return fmt.Errorf("hash body: %w", err)
	}

	if _, err := r.do(req, payloadHash, xmlBody); err != nil {
		return fmt.Errorf("failed to update A record: %w", err)
	}
	return nil
}

// do signs the request with SigV4 and executes it. On a non-2xx response it
// parses the error body and returns a descriptive error. The body argument
// allows callers to avoid re-reading req.Body for signing (the signer needs
// the payload hash, already computed by the caller).
func (r *Route53Client) do(req *http.Request, payloadHash string, _ []byte) ([]byte, error) {
	signRequest(req, r.accessKey, r.secretKey, route53Region, route53Service, payloadHash, r.now())

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	// Route53 responses are typically <10 KB (a single record's metadata).
	// Cap at 1 MiB so a compromised endpoint or MITM can't exhaust the
	// ~20 MB RAM budget on UDM / UDR devices by streaming a giant payload.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, parseAWSError(resp.StatusCode, body)
	}
	return body, nil
}

// parseAWSError extracts Code/Message from Route53 error XML bodies. The API
// uses ErrorResponse.Error for most errors and a flat root element for a few.
// We try both shapes and fall back to raw body on failure.
func parseAWSError(status int, body []byte) error {
	var wrapped struct {
		XMLName xml.Name `xml:"ErrorResponse"`
		Error   struct {
			Code    string `xml:"Code"`
			Message string `xml:"Message"`
		} `xml:"Error"`
	}
	if err := xml.Unmarshal(body, &wrapped); err == nil && wrapped.Error.Code != "" {
		return fmt.Errorf("route53 error (HTTP %d): %s: %s", status, wrapped.Error.Code, wrapped.Error.Message)
	}

	var flat struct {
		Code    string `xml:"Code"`
		Message string `xml:"Message"`
	}
	if err := xml.Unmarshal(body, &flat); err == nil && flat.Code != "" {
		return fmt.Errorf("route53 error (HTTP %d): %s: %s", status, flat.Code, flat.Message)
	}

	snippet := strings.TrimSpace(string(body))
	if len(snippet) > 256 {
		snippet = snippet[:256] + "..."
	}
	return fmt.Errorf("route53 error (HTTP %d): %s", status, snippet)
}

// --- XML request/response types ---

type changeResourceRecordSetsRequest struct {
	XMLName     xml.Name    `xml:"ChangeResourceRecordSetsRequest"`
	Xmlns       string      `xml:"xmlns,attr"`
	ChangeBatch changeBatch `xml:"ChangeBatch"`
}

type changeBatch struct {
	Changes changes `xml:"Changes"`
}

type changes struct {
	Change []change `xml:"Change"`
}

type change struct {
	Action            string            `xml:"Action"`
	ResourceRecordSet resourceRecordSet `xml:"ResourceRecordSet"`
}

type resourceRecordSet struct {
	Name            string           `xml:"Name"`
	Type            string           `xml:"Type"`
	TTL             int64            `xml:"TTL,omitempty"`
	ResourceRecords *resourceRecords `xml:"ResourceRecords,omitempty"`
}

type resourceRecords struct {
	ResourceRecord []resourceRecord `xml:"ResourceRecord"`
}

type resourceRecord struct {
	Value string `xml:"Value"`
}

type listResourceRecordSetsResponse struct {
	XMLName            xml.Name            `xml:"ListResourceRecordSetsResponse"`
	ResourceRecordSets []resourceRecordSet `xml:"ResourceRecordSets>ResourceRecordSet"`
}
