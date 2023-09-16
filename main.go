package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/route53"
)

func main() {
	// Load AWS credentials from environment variables or AWS config file
	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		fmt.Println("Error loading AWS credentials:", err)
		os.Exit(1)
	}

	// Create a Route 53 client
	client := route53.NewFromConfig(cfg)

	// Specify the domain name for which you want to list SRV records
	domainName := "example.com" // Replace with your domain name

	// Create a request to list all resource record sets in the hosted zone
	listRecordsInput := &route53.ListResourceRecordSetsInput{
		HostedZoneId: aws.String("YOUR_HOSTED_ZONE_ID"), // Replace with your hosted zone ID
	}

	// Send the request to Route 53
	resp, err := client.ListResourceRecordSets(context.TODO(), listRecordsInput)
	if err != nil {
		fmt.Println("Error listing resource record sets:", err)
		os.Exit(1)
	}

	// Convert the RRType to a string for comparison
	targetRRType := "route53.RRTypeSrv"

	// Print the SRV records
	for _, recordSet := range resp.ResourceRecordSets {
		if strings.ToLower(string(recordSet.Type)) == string(targetRRType) && *recordSet.Name == domainName {
			fmt.Printf("Name: %s, Type: %s, TTL: %d\n", *recordSet.Name, targetRRType, *recordSet.TTL)
			for _, srvRecord := range recordSet.ResourceRecords {
				fmt.Printf("  Priority: %s, Weight: %s, Port: %s, Target: %s\n",
					*srvRecord.Value, *srvRecord.Value, *srvRecord.Value, *srvRecord.Value)
			}
		}
	}
}
