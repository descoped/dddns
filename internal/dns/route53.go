package dns

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	"github.com/aws/aws-sdk-go-v2/service/route53/types"

	dddnscfg "github.com/descoped/dddns/internal/config"
)

// route53API interface for mocking
type route53API interface {
	ListResourceRecordSets(ctx context.Context, params *route53.ListResourceRecordSetsInput, optFns ...func(*route53.Options)) (*route53.ListResourceRecordSetsOutput, error)
	ChangeResourceRecordSets(ctx context.Context, params *route53.ChangeResourceRecordSetsInput, optFns ...func(*route53.Options)) (*route53.ChangeResourceRecordSetsOutput, error)
}

// Route53Client wraps the AWS Route53 client with our config
type Route53Client struct {
	client       route53API
	hostedZoneID string
	hostname     string
	ttl          int64
}

// NewRoute53Client creates a new Route53 client
// It ONLY uses static credentials from config for security (no env vars or IAM roles)
func NewRoute53Client(ctx context.Context, region, accessKey, secretKey, hostedZoneID, hostname string, ttl int64) (*Route53Client, error) {
	// Require explicit credentials for security
	if accessKey == "" || secretKey == "" {
		return nil, fmt.Errorf("AWS credentials are required in config file (aws_access_key and aws_secret_key)")
	}

	// Only use static credentials from config file
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
	)

	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	client := route53.NewFromConfig(cfg)

	return &Route53Client{
		client:       client,
		hostedZoneID: hostedZoneID,
		hostname:     hostname,
		ttl:          ttl,
	}, nil
}

// NewFromConfig constructs a Route53Client from a fully-populated dddns
// Config. It is the preferred constructor for production code; tests that
// need to inject a mock HTTP layer should use NewRoute53Client directly.
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

// GetCurrentIP retrieves the current IP address for the configured hostname.
// ctx is honored on the Route53 API call.
func (r *Route53Client) GetCurrentIP(ctx context.Context) (string, error) {
	fqdn := r.fqdn()

	input := &route53.ListResourceRecordSetsInput{
		HostedZoneId:    aws.String(r.hostedZoneID),
		StartRecordName: aws.String(fqdn),
		StartRecordType: types.RRTypeA,
		MaxItems:        aws.Int32(1),
	}

	resp, err := r.client.ListResourceRecordSets(ctx, input)
	if err != nil {
		return "", fmt.Errorf("failed to list record sets: %w", err)
	}

	// Check if we found the record
	for _, recordSet := range resp.ResourceRecordSets {
		if *recordSet.Name == fqdn && recordSet.Type == types.RRTypeA {
			if len(recordSet.ResourceRecords) > 0 {
				return *recordSet.ResourceRecords[0].Value, nil
			}
		}
	}

	return "", fmt.Errorf("A record not found for %s", r.hostname) //nolint:staticcheck // "A record" is a DNS term, not an article
}

// UpdateIP updates the A record with a new IP address.
// ctx is honored on the Route53 API call. Callers are expected to handle
// dry-run short-circuits before invoking UpdateIP (see internal/updater).
func (r *Route53Client) UpdateIP(ctx context.Context, newIP string) error {
	fqdn := r.fqdn()

	input := &route53.ChangeResourceRecordSetsInput{
		HostedZoneId: aws.String(r.hostedZoneID),
		ChangeBatch: &types.ChangeBatch{
			Changes: []types.Change{
				{
					Action: types.ChangeActionUpsert,
					ResourceRecordSet: &types.ResourceRecordSet{
						Name: aws.String(fqdn),
						Type: types.RRTypeA,
						TTL:  aws.Int64(r.ttl),
						ResourceRecords: []types.ResourceRecord{
							{
								Value: aws.String(newIP),
							},
						},
					},
				},
			},
		},
	}

	_, err := r.client.ChangeResourceRecordSets(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to update A record: %w", err)
	}

	return nil
}
