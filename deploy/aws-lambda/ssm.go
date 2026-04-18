package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/descoped/dddns/internal/dns"
)

// ssmClient is a minimal AWS Systems Manager Parameter Store client —
// just enough to fetch a single SecureString parameter for the shared
// secret. Avoids pulling in aws-sdk-go-v2 and its reflective transport
// machinery (several MB of binary size + noticeably slower cold start).
//
// The wire protocol is AWS JSON 1.1 over POST:
//
//	POST / HTTP/1.1
//	Host: ssm.<region>.amazonaws.com
//	X-Amz-Target: AmazonSSM.GetParameter
//	Content-Type: application/x-amz-json-1.1
//	{"Name": "...", "WithDecryption": true}
//
// Signing is standard SigV4; the session token from the Lambda exec
// role flows through via the extended signer in internal/dns/sigv4.go.
type ssmClient struct {
	region       string
	accessKey    string
	secretKey    string
	sessionToken string

	httpClient *http.Client
	now        func() time.Time
	endpoint   string // blank = derive from region
}

// getParameterRequest / getParameterResponse mirror the AWS JSON 1.1
// shape. Only the fields we actually use are declared — parameters
// we don't read (ARN, Version, LastModifiedDate, DataType) are
// intentionally absent so the decoder ignores them.
type getParameterRequest struct {
	Name           string `json:"Name"`
	WithDecryption bool   `json:"WithDecryption"`
}

type getParameterResponse struct {
	Parameter struct {
		Name  string `json:"Name"`
		Value string `json:"Value"`
		Type  string `json:"Type"`
	} `json:"Parameter"`
}

// getParameter fetches a single SSM parameter value. WithDecryption
// is hard-coded true — SecureString is the only reason dddns uses
// SSM at all.
func (c *ssmClient) getParameter(ctx context.Context, name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("ssm.getParameter: empty parameter name")
	}

	body, err := json.Marshal(getParameterRequest{Name: name, WithDecryption: true})
	if err != nil {
		return "", fmt.Errorf("marshal GetParameter request: %w", err)
	}

	endpoint := c.endpoint
	if endpoint == "" {
		endpoint = fmt.Sprintf("https://ssm.%s.amazonaws.com/", c.region)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build GetParameter request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "AmazonSSM.GetParameter")
	req.ContentLength = int64(len(body))

	// Compute payload hash for SigV4 and sign via the shared signer
	// (same signer the Route53 client uses; just a different service
	// name + region here).
	sum := sha256.Sum256(body)
	payloadHash := hex.EncodeToString(sum[:])
	dns.SignRequest(req, c.accessKey, c.secretKey, c.sessionToken, c.region, "ssm", payloadHash, c.now())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("GetParameter HTTP: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// SSM responses for a single parameter are <1 KB. Cap at 64 KB as
	// belt-and-braces against a compromised / misrouted endpoint
	// streaming a giant body into a memory-constrained Lambda.
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return "", fmt.Errorf("read GetParameter body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GetParameter %s: %s", resp.Status, string(raw))
	}

	var out getParameterResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("decode GetParameter response: %w", err)
	}
	if out.Parameter.Value == "" {
		return "", fmt.Errorf("GetParameter: empty value for %q", name)
	}
	return out.Parameter.Value, nil
}
