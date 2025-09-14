package dns

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	"github.com/aws/aws-sdk-go-v2/service/route53/types"
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
func NewRoute53Client(region, accessKey, secretKey, hostedZoneID, hostname string, ttl int64) (*Route53Client, error) {
	// Require explicit credentials for security
	if accessKey == "" || secretKey == "" {
		return nil, fmt.Errorf("AWS credentials are required in config file (aws_access_key and aws_secret_key)")
	}

	// Only use static credentials from config file
	cfg, err := config.LoadDefaultConfig(context.TODO(),
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

// GetCurrentIP retrieves the current IP address for the configured hostname
func (r *Route53Client) GetCurrentIP() (string, error) {
	ctx := context.TODO()
	// Ensure hostname ends with a dot for Route53
	fqdn := r.hostname
	if fqdn[len(fqdn)-1] != '.' {
		fqdn = fqdn + "."
	}

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

	//nolint:ST1005 // "A record" is a DNS term, not an article
	return "", fmt.Errorf("A record not found for %s", r.hostname)
}

// UpdateIP updates the A record with a new IP address
func (r *Route53Client) UpdateIP(newIP string, dryRun bool) error {
	if dryRun {
		fmt.Printf("[DRY RUN] Would update %s to %s\n", r.hostname, newIP)
		return nil
	}

	ctx := context.TODO()
	// Ensure hostname ends with a dot for Route53
	fqdn := r.hostname
	if fqdn[len(fqdn)-1] != '.' {
		fqdn = fqdn + "."
	}

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
