// Command aws-lambda is the dddns deployment form that runs behind an
// API Gateway HTTP API on AWS Lambda. It receives dyndns v2 push
// requests from an upstream DDNS client (typically UniFi Dream's
// built-in inadyn, which can't reach a same-host loopback listener
// and needs a public HTTPS endpoint to push to) and performs the
// Route53 UPSERT, reusing the SigV4 signer and Route53Client from
// internal/dns.
//
// It does NOT depend on aws-sdk-go-v2. The single external runtime
// dependency added for this deployment form is github.com/aws/aws-lambda-go,
// which provides only the Lambda runtime wrapper + event struct
// types — no AWS API client code. SSM GetParameter is implemented
// here via the same hand-rolled SigV4 signer (just a different
// service name and endpoint). Total binary size stays under 10 MB.
//
// Configuration is entirely driven by environment variables — nothing
// is hard-coded to a particular account, region, hosted zone, or
// hostname. Tofu provides the values; Lambda surfaces them via the
// function's runtime env.
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"

	"github.com/descoped/dddns/internal/dns"
)

// config captures the per-deployment values the Lambda needs. All of
// these come from the Lambda function's env vars — set via
// terraform/opentofu at deploy time, never from a config file.
type config struct {
	region          string // AWS_REGION — provided by Lambda
	accessKey       string // AWS_ACCESS_KEY_ID — STS creds from exec role
	secretKey       string // AWS_SECRET_ACCESS_KEY — ditto
	sessionToken    string // AWS_SESSION_TOKEN — ditto
	hostedZoneID    string // HOSTED_ZONE_ID — Route53 zone to UPSERT into
	hostname        string // DDDNS_HOSTNAME — record name the handler accepts
	ssmSecretParam  string // SSM_SECRET_PARAM — SSM name holding the shared secret
	ttl             int64  // DDDNS_TTL — DNS TTL seconds (default 300)
	lookupTimeout   time.Duration
}

func loadConfig() (*config, error) {
	required := func(name string) (string, error) {
		v := os.Getenv(name)
		if v == "" {
			return "", fmt.Errorf("required env var %s is unset", name)
		}
		return v, nil
	}

	region, err := required("AWS_REGION")
	if err != nil {
		return nil, err
	}
	ak, err := required("AWS_ACCESS_KEY_ID")
	if err != nil {
		return nil, err
	}
	sk, err := required("AWS_SECRET_ACCESS_KEY")
	if err != nil {
		return nil, err
	}
	// AWS_SESSION_TOKEN is required on Lambda (the exec role hands out
	// temporary creds); we treat an empty value as a configuration error
	// so that a miswired test environment can't silently fall back to
	// unsigned requests.
	st, err := required("AWS_SESSION_TOKEN")
	if err != nil {
		return nil, err
	}
	zone, err := required("HOSTED_ZONE_ID")
	if err != nil {
		return nil, err
	}
	host, err := required("DDDNS_HOSTNAME")
	if err != nil {
		return nil, err
	}
	ssmParam, err := required("SSM_SECRET_PARAM")
	if err != nil {
		return nil, err
	}

	// Optional knobs.
	ttl := int64(300)
	if v := os.Getenv("DDDNS_TTL"); v != "" {
		var parsed int64
		if _, perr := fmt.Sscan(v, &parsed); perr == nil && parsed > 0 {
			ttl = parsed
		}
	}

	return &config{
		region:         region,
		accessKey:      ak,
		secretKey:      sk,
		sessionToken:   st,
		hostedZoneID:   zone,
		hostname:       host,
		ssmSecretParam: ssmParam,
		ttl:            ttl,
		lookupTimeout:  5 * time.Second,
	}, nil
}

func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("dddns-lambda: config load failed: %v", err)
	}

	route53, err := dns.NewRoute53Client(
		context.Background(),
		cfg.region, cfg.accessKey, cfg.secretKey, cfg.sessionToken,
		cfg.hostedZoneID, cfg.hostname, cfg.ttl,
	)
	if err != nil {
		log.Fatalf("dddns-lambda: Route53 client init failed: %v", err)
	}

	ssm := &ssmClient{
		region:       cfg.region,
		accessKey:    cfg.accessKey,
		secretKey:    cfg.secretKey,
		sessionToken: cfg.sessionToken,
		httpClient:   http.DefaultClient,
		now:          time.Now,
	}

	h := &handler{
		cfg:         cfg,
		route53:     route53,
		ssm:         ssm,
		secretCache: &secretCache{ttl: 60 * time.Second, now: time.Now},
	}
	lambda.Start(h.handle)
}

// invoke is a helper that lets tests drive handler.handle with a
// pre-built APIGatewayV2HTTPRequest. Kept in main.go so the bench /
// local debugging harness can exercise it without spinning up the
// full lambda.Start loop.
func invoke(ctx context.Context, h *handler, req events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
	return h.handle(ctx, req)
}
