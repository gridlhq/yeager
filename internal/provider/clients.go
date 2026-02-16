package provider

import (
	"context"
	"fmt"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2instanceconnect"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	fkssh "github.com/gridlhq/yeager/internal/ssh"
	fkstorage "github.com/gridlhq/yeager/internal/storage"
)

// NewEC2InstanceConnectClient creates a real EC2 Instance Connect client.
func NewEC2InstanceConnectClient(ctx context.Context, region string) (fkssh.EC2InstanceConnectAPI, error) {
	opts := []func(*awsconfig.LoadOptions) error{}
	if region != "" {
		opts = append(opts, awsconfig.WithRegion(region))
	}
	cfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("loading AWS config: %w", err)
	}
	return ec2instanceconnect.NewFromConfig(cfg), nil
}

// NewS3ObjectClient creates a real S3 client that satisfies the storage.S3API interface.
func NewS3ObjectClient(ctx context.Context, region string) (fkstorage.S3API, error) {
	opts := []func(*awsconfig.LoadOptions) error{}
	if region != "" {
		opts = append(opts, awsconfig.WithRegion(region))
	}
	cfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("loading AWS config: %w", err)
	}
	return s3.NewFromConfig(cfg), nil
}
