package dns

import (
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	"github.com/aws/aws-sdk-go-v2/service/route53/types"
)

// Mock Route53 client for testing
type mockRoute53Client struct {
	listResourceRecordSetsFunc   func(ctx context.Context, params *route53.ListResourceRecordSetsInput, optFns ...func(*route53.Options)) (*route53.ListResourceRecordSetsOutput, error)
	changeResourceRecordSetsFunc func(ctx context.Context, params *route53.ChangeResourceRecordSetsInput, optFns ...func(*route53.Options)) (*route53.ChangeResourceRecordSetsOutput, error)
}

func (m *mockRoute53Client) ListResourceRecordSets(ctx context.Context, params *route53.ListResourceRecordSetsInput, optFns ...func(*route53.Options)) (*route53.ListResourceRecordSetsOutput, error) {
	if m.listResourceRecordSetsFunc != nil {
		return m.listResourceRecordSetsFunc(ctx, params, optFns...)
	}
	return &route53.ListResourceRecordSetsOutput{
		ResourceRecordSets: []types.ResourceRecordSet{
			{
				Name: aws.String("test.example.com."),
				Type: types.RRTypeA,
				ResourceRecords: []types.ResourceRecord{
					{Value: aws.String("1.2.3.4")},
				},
			},
		},
	}, nil
}

func (m *mockRoute53Client) ChangeResourceRecordSets(ctx context.Context, params *route53.ChangeResourceRecordSetsInput, optFns ...func(*route53.Options)) (*route53.ChangeResourceRecordSetsOutput, error) {
	if m.changeResourceRecordSetsFunc != nil {
		return m.changeResourceRecordSetsFunc(ctx, params, optFns...)
	}
	return &route53.ChangeResourceRecordSetsOutput{
		ChangeInfo: &types.ChangeInfo{
			Id:     aws.String("test-change-id"),
			Status: types.ChangeStatusPending,
		},
	}, nil
}

func TestRoute53Client_GetCurrentIP(t *testing.T) {
	client := &Route53Client{
		client:       &mockRoute53Client{},
		hostedZoneID: "Z123456",
		hostname:     "test.example.com",
		ttl:          300,
	}

	ip, err := client.GetCurrentIP()
	if err != nil {
		t.Fatalf("GetCurrentIP failed: %v", err)
	}

	if ip != "1.2.3.4" {
		t.Errorf("Expected IP 1.2.3.4, got %s", ip)
	}
}

func TestRoute53Client_GetCurrentIP_NotFound(t *testing.T) {
	mockClient := &mockRoute53Client{
		listResourceRecordSetsFunc: func(ctx context.Context, params *route53.ListResourceRecordSetsInput, optFns ...func(*route53.Options)) (*route53.ListResourceRecordSetsOutput, error) {
			return &route53.ListResourceRecordSetsOutput{
				ResourceRecordSets: []types.ResourceRecordSet{},
			}, nil
		},
	}

	client := &Route53Client{
		client:       mockClient,
		hostedZoneID: "Z123456",
		hostname:     "test.example.com",
		ttl:          300,
	}

	_, err := client.GetCurrentIP()
	if err == nil {
		t.Error("Expected error for not found record, got nil")
	}
}

func TestRoute53Client_GetCurrentIP_Error(t *testing.T) {
	mockClient := &mockRoute53Client{
		listResourceRecordSetsFunc: func(ctx context.Context, params *route53.ListResourceRecordSetsInput, optFns ...func(*route53.Options)) (*route53.ListResourceRecordSetsOutput, error) {
			return nil, fmt.Errorf("AWS error")
		},
	}

	client := &Route53Client{
		client:       mockClient,
		hostedZoneID: "Z123456",
		hostname:     "test.example.com",
		ttl:          300,
	}

	_, err := client.GetCurrentIP()
	if err == nil {
		t.Error("Expected error from AWS, got nil")
	}
}

func TestRoute53Client_UpdateIP(t *testing.T) {
	client := &Route53Client{
		client:       &mockRoute53Client{},
		hostedZoneID: "Z123456",
		hostname:     "test.example.com",
		ttl:          300,
	}

	err := client.UpdateIP("5.6.7.8", false)
	if err != nil {
		t.Fatalf("UpdateIP failed: %v", err)
	}
}

func TestRoute53Client_UpdateIP_DryRun(t *testing.T) {
	mockClient := &mockRoute53Client{
		changeResourceRecordSetsFunc: func(ctx context.Context, params *route53.ChangeResourceRecordSetsInput, optFns ...func(*route53.Options)) (*route53.ChangeResourceRecordSetsOutput, error) {
			t.Fatal("ChangeResourceRecordSets should not be called in dry run mode")
			return nil, nil
		},
	}

	client := &Route53Client{
		client:       mockClient,
		hostedZoneID: "Z123456",
		hostname:     "test.example.com",
		ttl:          300,
	}

	err := client.UpdateIP("5.6.7.8", true)
	if err != nil {
		t.Fatalf("UpdateIP dry run failed: %v", err)
	}
}

// TestRoute53Client_GetCurrentIP_EmptyHostname verifies that an empty
// hostname does not panic. Config.Validate() should catch this earlier,
// but the Route53 client must not assume — the prior `fqdn[len(fqdn)-1]`
// indexing crashed at runtime on empty strings.
func TestRoute53Client_GetCurrentIP_EmptyHostname(t *testing.T) {
	client := &Route53Client{
		client:       &mockRoute53Client{},
		hostedZoneID: "Z123456",
		hostname:     "",
		ttl:          300,
	}
	// Must not panic. Empty is still not a valid lookup key so we expect
	// an error (from the "A record not found" path), but not a crash.
	_, err := client.GetCurrentIP()
	if err == nil {
		t.Error("Expected error for empty hostname, got nil")
	}
}

// TestRoute53Client_UpdateIP_EmptyHostname verifies that UpdateIP also
// handles an empty hostname without panicking.
func TestRoute53Client_UpdateIP_EmptyHostname(t *testing.T) {
	client := &Route53Client{
		client:       &mockRoute53Client{},
		hostedZoneID: "Z123456",
		hostname:     "",
		ttl:          300,
	}
	// Must not panic.
	_ = client.UpdateIP("1.2.3.4", false)
}

// TestRoute53Client_AlreadyDottedHostname verifies that a hostname that
// already ends with "." is passed through unchanged (no double-dot).
func TestRoute53Client_AlreadyDottedHostname(t *testing.T) {
	var captured string
	mockClient := &mockRoute53Client{
		listResourceRecordSetsFunc: func(ctx context.Context, params *route53.ListResourceRecordSetsInput, optFns ...func(*route53.Options)) (*route53.ListResourceRecordSetsOutput, error) {
			captured = *params.StartRecordName
			return &route53.ListResourceRecordSetsOutput{
				ResourceRecordSets: []types.ResourceRecordSet{
					{
						Name: aws.String("test.example.com."),
						Type: types.RRTypeA,
						ResourceRecords: []types.ResourceRecord{
							{Value: aws.String("1.2.3.4")},
						},
					},
				},
			}, nil
		},
	}
	client := &Route53Client{
		client:       mockClient,
		hostedZoneID: "Z123456",
		hostname:     "test.example.com.", // already dotted
		ttl:          300,
	}
	if _, err := client.GetCurrentIP(); err != nil {
		t.Fatalf("GetCurrentIP failed: %v", err)
	}
	if captured != "test.example.com." {
		t.Errorf("expected StartRecordName=%q (no double-dot), got %q", "test.example.com.", captured)
	}
}

func TestRoute53Client_UpdateIP_Error(t *testing.T) {
	mockClient := &mockRoute53Client{
		changeResourceRecordSetsFunc: func(ctx context.Context, params *route53.ChangeResourceRecordSetsInput, optFns ...func(*route53.Options)) (*route53.ChangeResourceRecordSetsOutput, error) {
			return nil, fmt.Errorf("AWS update error")
		},
	}

	client := &Route53Client{
		client:       mockClient,
		hostedZoneID: "Z123456",
		hostname:     "test.example.com",
		ttl:          300,
	}

	err := client.UpdateIP("5.6.7.8", false)
	if err == nil {
		t.Error("Expected error from AWS update, got nil")
	}
}
