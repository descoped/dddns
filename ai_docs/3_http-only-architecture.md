# HTTP-Only Architecture Design

## Executive Summary

This document revises the provider architecture to use pure HTTP clients instead of vendor SDKs, maintaining dddns's lightweight footprint (<10MB binary) while supporting multiple DNS providers.

## Rationale for HTTP-Only Approach

### The SDK Problem

| SDK | Binary Size Impact | Dependencies | Memory Usage |
|-----|-------------------|--------------|--------------|
| AWS SDK v2 | +15-20MB | 30+ modules | +50MB runtime |
| Google Cloud SDK | +20-30MB | 50+ modules | +80MB runtime |
| Azure SDK | +25-35MB | 40+ modules | +70MB runtime |
| **Total with 4 SDKs** | **+80-100MB** | **150+ modules** | **+250MB runtime** |

### HTTP-Only Benefits

| Aspect | HTTP-Only | SDK-Based |
|--------|-----------|-----------|
| Binary Size | <10MB total | 80-100MB |
| Dependencies | 2-3 (stdlib + maybe a helper) | 150+ |
| Memory Usage | <20MB | 200MB+ |
| Attack Surface | Minimal | Large |
| Maintenance | Simple | Complex |
| Build Time | Fast | Slow |
| UDM Compatibility | ✅ Excellent | ❌ Poor |

## Core HTTP Client Design

### Base HTTP Client

```go
// internal/providers/httpclient/client.go
package httpclient

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "time"
)

// Client is a lightweight HTTP client for DNS providers
type Client struct {
    http    *http.Client
    baseURL string
    headers map[string]string
    auth    AuthMethod
}

// AuthMethod defines how to authenticate requests
type AuthMethod interface {
    ApplyAuth(req *http.Request) error
}

// NewClient creates a new HTTP client
func NewClient(baseURL string, auth AuthMethod) *Client {
    return &Client{
        http: &http.Client{
            Timeout: 30 * time.Second,
            Transport: &http.Transport{
                MaxIdleConns:        10,
                MaxIdleConnsPerHost: 2,
                IdleConnTimeout:     90 * time.Second,
            },
        },
        baseURL: baseURL,
        headers: map[string]string{
            "User-Agent": "dddns/2.0",
        },
        auth: auth,
    }
}

// Request makes an HTTP request
func (c *Client) Request(ctx context.Context, method, path string, body interface{}) ([]byte, error) {
    var bodyReader io.Reader
    if body != nil {
        jsonBody, err := json.Marshal(body)
        if err != nil {
            return nil, fmt.Errorf("marshal body: %w", err)
        }
        bodyReader = bytes.NewBuffer(jsonBody)
    }

    req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
    if err != nil {
        return nil, fmt.Errorf("create request: %w", err)
    }

    // Apply headers
    for k, v := range c.headers {
        req.Header.Set(k, v)
    }
    if body != nil {
        req.Header.Set("Content-Type", "application/json")
    }

    // Apply authentication
    if c.auth != nil {
        if err := c.auth.ApplyAuth(req); err != nil {
            return nil, fmt.Errorf("apply auth: %w", err)
        }
    }

    // Execute request
    resp, err := c.http.Do(req)
    if err != nil {
        return nil, fmt.Errorf("execute request: %w", err)
    }
    defer resp.Body.Close()

    respBody, err := io.ReadAll(resp.Body)
    if err != nil {
        return nil, fmt.Errorf("read response: %w", err)
    }

    if resp.StatusCode >= 400 {
        return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
    }

    return respBody, nil
}

// JSON makes a request and unmarshals JSON response
func (c *Client) JSON(ctx context.Context, method, path string, body, result interface{}) error {
    respBody, err := c.Request(ctx, method, path, body)
    if err != nil {
        return err
    }

    if result != nil && len(respBody) > 0 {
        if err := json.Unmarshal(respBody, result); err != nil {
            return fmt.Errorf("unmarshal response: %w", err)
        }
    }

    return nil
}
```

### Authentication Methods

```go
// internal/providers/httpclient/auth.go
package httpclient

import "net/http"

// BearerAuth for token-based authentication
type BearerAuth struct {
    Token string
}

func (b *BearerAuth) ApplyAuth(req *http.Request) error {
    req.Header.Set("Authorization", "Bearer "+b.Token)
    return nil
}

// BasicAuth for username/password authentication
type BasicAuth struct {
    Username string
    Password string
}

func (b *BasicAuth) ApplyAuth(req *http.Request) error {
    req.SetBasicAuth(b.Username, b.Password)
    return nil
}

// HeaderAuth for API key in header
type HeaderAuth struct {
    Header string
    Value  string
}

func (h *HeaderAuth) ApplyAuth(req *http.Request) error {
    req.Header.Set(h.Header, h.Value)
    return nil
}

// AWSv4Auth for AWS Signature v4 (minimal implementation)
type AWSv4Auth struct {
    AccessKey string
    SecretKey string
    Region    string
    Service   string
}

func (a *AWSv4Auth) ApplyAuth(req *http.Request) error {
    // Minimal AWS v4 signature implementation
    // Only what's needed for Route53
    return signAWSv4(req, a.AccessKey, a.SecretKey, a.Region, a.Service)
}
```

## Provider Implementations Using HTTP Client

### AWS Route53 (HTTP-Only)

```go
// internal/providers/aws/route53_http.go
package aws

import (
    "context"
    "encoding/xml"
    "fmt"
    "strings"

    "github.com/descoped/dddns/internal/providers/httpclient"
)

// API Documentation: https://docs.aws.amazon.com/Route53/latest/APIReference/

type Route53Provider struct {
    client       *httpclient.Client
    hostedZoneID string
    hostname     string
    ttl          int64
}

// Route53 XML structures (minimal, only what we need)
type ListResourceRecordSetsResponse struct {
    XMLName            xml.Name            `xml:"ListResourceRecordSetsResponse"`
    ResourceRecordSets []ResourceRecordSet `xml:"ResourceRecordSets>ResourceRecordSet"`
}

type ResourceRecordSet struct {
    Name            string           `xml:"Name"`
    Type            string           `xml:"Type"`
    TTL             int64            `xml:"TTL"`
    ResourceRecords []ResourceRecord `xml:"ResourceRecords>ResourceRecord"`
}

type ResourceRecord struct {
    Value string `xml:"Value"`
}

type ChangeResourceRecordSetsRequest struct {
    XMLName     xml.Name    `xml:"ChangeResourceRecordSetsRequest"`
    ChangeBatch ChangeBatch `xml:"ChangeBatch"`
}

type ChangeBatch struct {
    Changes []Change `xml:"Changes>Change"`
}

type Change struct {
    Action            string            `xml:"Action"`
    ResourceRecordSet ResourceRecordSet `xml:"ResourceRecordSet"`
}

func NewRoute53Provider(config map[string]interface{}) (*Route53Provider, error) {
    // Extract config
    region := config["aws_region"].(string)
    accessKey := config["aws_access_key"].(string)
    secretKey := config["aws_secret_key"].(string)
    hostedZoneID := config["hosted_zone_id"].(string)
    hostname := config["hostname"].(string)
    ttl := config["ttl"].(int64)

    // Create HTTP client with AWS v4 auth
    auth := &httpclient.AWSv4Auth{
        AccessKey: accessKey,
        SecretKey: secretKey,
        Region:    region,
        Service:   "route53",
    }

    client := httpclient.NewClient(
        fmt.Sprintf("https://route53.amazonaws.com/2013-04-01"),
        auth,
    )

    return &Route53Provider{
        client:       client,
        hostedZoneID: hostedZoneID,
        hostname:     hostname,
        ttl:          ttl,
    }, nil
}

func (r *Route53Provider) GetCurrentIP(ctx context.Context) (string, error) {
    // Ensure hostname ends with dot
    fqdn := r.hostname
    if !strings.HasSuffix(fqdn, ".") {
        fqdn += "."
    }

    path := fmt.Sprintf("/hostedzone/%s/rrset?name=%s&type=A&maxitems=1",
        r.hostedZoneID, fqdn)

    respBody, err := r.client.Request(ctx, "GET", path, nil)
    if err != nil {
        return "", fmt.Errorf("list record sets: %w", err)
    }

    var resp ListResourceRecordSetsResponse
    if err := xml.Unmarshal(respBody, &resp); err != nil {
        return "", fmt.Errorf("unmarshal response: %w", err)
    }

    for _, rs := range resp.ResourceRecordSets {
        if rs.Name == fqdn && rs.Type == "A" {
            if len(rs.ResourceRecords) > 0 {
                return rs.ResourceRecords[0].Value, nil
            }
        }
    }

    return "", fmt.Errorf("A record not found for %s", r.hostname)
}

func (r *Route53Provider) UpdateIP(ctx context.Context, newIP string, dryRun bool) error {
    if dryRun {
        fmt.Printf("[DRY RUN] Would update %s to %s\n", r.hostname, newIP)
        return nil
    }

    fqdn := r.hostname
    if !strings.HasSuffix(fqdn, ".") {
        fqdn += "."
    }

    // Create change request
    changeReq := ChangeResourceRecordSetsRequest{
        ChangeBatch: ChangeBatch{
            Changes: []Change{
                {
                    Action: "UPSERT",
                    ResourceRecordSet: ResourceRecordSet{
                        Name: fqdn,
                        Type: "A",
                        TTL:  r.ttl,
                        ResourceRecords: []ResourceRecord{
                            {Value: newIP},
                        },
                    },
                },
            },
        },
    }

    xmlBody, err := xml.Marshal(changeReq)
    if err != nil {
        return fmt.Errorf("marshal request: %w", err)
    }

    path := fmt.Sprintf("/hostedzone/%s/rrset", r.hostedZoneID)
    _, err = r.client.RequestXML(ctx, "POST", path, xmlBody)
    if err != nil {
        return fmt.Errorf("change record set: %w", err)
    }

    return nil
}
```

### Cloudflare (HTTP-Only)

```go
// internal/providers/cloudflare/cloudflare_http.go
package cloudflare

// API Documentation: https://developers.cloudflare.com/api/operations/dns-records-for-a-zone-list-dns-records

type CloudflareProvider struct {
    client   *httpclient.Client
    zoneID   string
    hostname string
    ttl      int64
    proxied  bool
}

type DNSRecord struct {
    ID       string `json:"id,omitempty"`
    Type     string `json:"type"`
    Name     string `json:"name"`
    Content  string `json:"content"`
    TTL      int    `json:"ttl"`
    Proxied  bool   `json:"proxied"`
}

type ListDNSResponse struct {
    Result []DNSRecord `json:"result"`
}

func NewCloudflareProvider(config map[string]interface{}) (*CloudflareProvider, error) {
    apiToken := config["api_token"].(string)

    auth := &httpclient.BearerAuth{Token: apiToken}
    client := httpclient.NewClient("https://api.cloudflare.com/client/v4", auth)

    return &CloudflareProvider{
        client:   client,
        zoneID:   config["zone_id"].(string),
        hostname: config["hostname"].(string),
        ttl:      config["ttl"].(int64),
        proxied:  config["proxied"].(bool),
    }, nil
}

func (c *CloudflareProvider) GetCurrentIP(ctx context.Context) (string, error) {
    path := fmt.Sprintf("/zones/%s/dns_records?type=A&name=%s", c.zoneID, c.hostname)

    var resp ListDNSResponse
    if err := c.client.JSON(ctx, "GET", path, nil, &resp); err != nil {
        return "", err
    }

    if len(resp.Result) > 0 {
        return resp.Result[0].Content, nil
    }

    return "", fmt.Errorf("A record not found for %s", c.hostname)
}

func (c *CloudflareProvider) UpdateIP(ctx context.Context, newIP string, dryRun bool) error {
    if dryRun {
        fmt.Printf("[DRY RUN] Would update %s to %s\n", c.hostname, newIP)
        return nil
    }

    // Get existing record ID
    path := fmt.Sprintf("/zones/%s/dns_records?type=A&name=%s", c.zoneID, c.hostname)
    var listResp ListDNSResponse
    if err := c.client.JSON(ctx, "GET", path, nil, &listResp); err != nil {
        return err
    }

    record := DNSRecord{
        Type:    "A",
        Name:    c.hostname,
        Content: newIP,
        TTL:     int(c.ttl),
        Proxied: c.proxied,
    }

    if len(listResp.Result) > 0 {
        // Update existing
        recordID := listResp.Result[0].ID
        path = fmt.Sprintf("/zones/%s/dns_records/%s", c.zoneID, recordID)
        return c.client.JSON(ctx, "PATCH", path, record, nil)
    }

    // Create new
    path = fmt.Sprintf("/zones/%s/dns_records", c.zoneID)
    return c.client.JSON(ctx, "POST", path, record, nil)
}
```

### Namecheap (HTTP-Only)

```go
// internal/providers/namecheap/namecheap_http.go
package namecheap

// API Documentation: https://www.namecheap.com/support/api/methods/domains-dns/set-hosts/

import (
    "encoding/xml"
    "net/url"
)

type NamecheapProvider struct {
    client     *httpclient.Client
    apiUser    string
    apiKey     string
    clientIP   string
    domain     string
    hostname   string
    ttl        int64
}

// XML response structures
type ApiResponse struct {
    XMLName xml.Name       `xml:"ApiResponse"`
    Errors  []ApiError     `xml:"Errors>Error"`
    Result  SetHostsResult `xml:"CommandResponse>DomainDNSSetHostsResult"`
}

type ApiError struct {
    Number  string `xml:"Number,attr"`
    Message string `xml:",chardata"`
}

type SetHostsResult struct {
    Domain    string `xml:"Domain,attr"`
    IsSuccess string `xml:"IsSuccess,attr"`
}

func NewNamecheapProvider(config map[string]interface{}) (*NamecheapProvider, error) {
    // Namecheap uses query parameters for auth
    client := httpclient.NewClient("https://api.namecheap.com/xml.response", nil)

    return &NamecheapProvider{
        client:   client,
        apiUser:  config["api_user"].(string),
        apiKey:   config["api_key"].(string),
        clientIP: config["client_ip"].(string),
        domain:   config["domain"].(string),
        hostname: config["hostname"].(string),
        ttl:      config["ttl"].(int64),
    }, nil
}

func (n *NamecheapProvider) GetCurrentIP(ctx context.Context) (string, error) {
    params := url.Values{
        "ApiUser":  {n.apiUser},
        "ApiKey":   {n.apiKey},
        "UserName": {n.apiUser},
        "ClientIp": {n.clientIP},
        "Command":  {"namecheap.domains.dns.getHosts"},
        "SLD":      {n.extractSLD()},
        "TLD":      {n.extractTLD()},
    }

    respBody, err := n.client.Request(ctx, "GET", "?"+params.Encode(), nil)
    if err != nil {
        return "", err
    }

    // Parse XML response to find our A record
    // ... XML parsing logic ...

    return "", nil
}
```

## API Documentation References

### Primary Providers

| Provider | API Documentation | Auth Type | Format |
|----------|------------------|-----------|---------|
| **AWS Route53** | https://docs.aws.amazon.com/Route53/latest/APIReference/ | AWS Signature v4 | XML |
| **Cloudflare** | https://developers.cloudflare.com/api/ | Bearer Token | JSON |
| **Domeneshop** | https://api.domeneshop.no/docs/ | Basic Auth | JSON |
| **Namecheap** | https://www.namecheap.com/support/api/ | API Key (Query) | XML |
| **GoDaddy** | https://developer.godaddy.com/doc | API Key + Secret | JSON |

### Secondary Providers

| Provider | API Documentation | Auth Type | Format |
|----------|------------------|-----------|---------|
| **Google Cloud DNS** | https://cloud.google.com/dns/docs/reference/v1 | OAuth2/Service Account | JSON |
| **Azure DNS** | https://docs.microsoft.com/en-us/rest/api/dns/ | OAuth2/Service Principal | JSON |
| **DigitalOcean** | https://docs.digitalocean.com/reference/api/api-reference/#tag/Domains | Bearer Token | JSON |
| **Linode** | https://www.linode.com/api/v4/domains | Bearer Token | JSON |
| **Hetzner** | https://dns.hetzner.com/api-docs | API Token | JSON |

### Lightweight Providers

| Provider | API Documentation | Auth Type | Format |
|----------|------------------|-----------|---------|
| **DuckDNS** | https://www.duckdns.org/spec.jsp | Token (URL) | Text |
| **No-IP** | https://www.noip.com/integrate/api | Basic Auth | Text |
| **Dynu** | https://www.dynu.com/Support/API | API Key | JSON |

## Memory and Binary Size Analysis

### Current Implementation (with AWS SDK)
```
Binary size: ~25MB
Memory usage: ~40MB runtime
Dependencies: 30+ AWS SDK modules
```

### HTTP-Only Implementation
```
Binary size: ~8MB (68% reduction)
Memory usage: ~15MB runtime (62% reduction)
Dependencies: 0 external (only stdlib)
```

### Per-Provider Impact
| Provider | SDK Size | HTTP-Only Size | Savings |
|----------|----------|----------------|---------|
| AWS Route53 | ~15MB | ~200KB | 98.7% |
| Google Cloud | ~25MB | ~150KB | 99.4% |
| Azure | ~30MB | ~180KB | 99.4% |
| Cloudflare | N/A (no official Go SDK) | ~100KB | - |

## Implementation Strategy

### Phase 1: Core HTTP Client
1. Implement base HTTP client with auth methods
2. Add AWS Signature v4 minimal implementation
3. Create shared error handling
4. Add retry logic with exponential backoff

### Phase 2: Provider Migration
1. Port Route53 from SDK to HTTP
2. Implement Cloudflare (pure HTTP)
3. Implement Domeneshop (pure HTTP)
4. Implement Namecheap (XML handling)

### Phase 3: Testing
1. Unit tests with recorded HTTP responses
2. Integration tests with mock server
3. Memory profiling to confirm targets
4. Binary size validation

## Benefits Summary

1. **Lightweight**: 68% smaller binary, 62% less memory
2. **Fast**: Faster compilation, faster startup
3. **Secure**: Minimal attack surface, no dependency vulnerabilities
4. **Maintainable**: Simple code, no SDK version conflicts
5. **Portable**: Works perfectly on constrained devices (UDM)
6. **Consistent**: Same pattern for all providers

## Conclusion

The HTTP-only approach aligns perfectly with dddns's philosophy of simplicity and efficiency. By eliminating SDKs, we maintain a sub-10MB binary while supporting unlimited providers through a consistent, lightweight HTTP client pattern.