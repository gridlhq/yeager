package provider

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"errors"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

const (
	securityGroupName = "yeager-sg"
	securityGroupDesc = "Yeager remote execution - SSH access"
	managedTagKey     = "yeager:managed"
	managedTagValue   = "true"
	projectHashTagKey = "yeager:project-hash"
	projectPathTagKey = "yeager:project-path"
	createdTagKey     = "yeager:created"
	bucketPrefix      = "yeager-"
	lifecycleRuleID   = "yeager-expire-30d"

	waitTimeout = 5 * time.Minute
)

// instanceSizeMap maps yeager size names to EC2 instance types.
// Uses Graviton (arm64) instances for best price/performance.
var instanceSizeMap = map[string]ec2types.InstanceType{
	"small":  ec2types.InstanceTypeT4gSmall,
	"medium": ec2types.InstanceTypeT4gMedium,
	"large":  ec2types.InstanceTypeT4gLarge,
	"xlarge": ec2types.InstanceTypeT4gXlarge,
}

// InstanceTypeForSize returns the EC2 instance type for a yeager size string.
func InstanceTypeForSize(size string) (ec2types.InstanceType, error) {
	t, ok := instanceSizeMap[size]
	if !ok {
		return "", fmt.Errorf("unknown instance size %q (must be small, medium, large, or xlarge)", size)
	}
	return t, nil
}

// EC2API is the subset of the EC2 client used by AWSProvider.
type EC2API interface {
	DescribeSecurityGroups(ctx context.Context, params *ec2.DescribeSecurityGroupsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeSecurityGroupsOutput, error)
	CreateSecurityGroup(ctx context.Context, params *ec2.CreateSecurityGroupInput, optFns ...func(*ec2.Options)) (*ec2.CreateSecurityGroupOutput, error)
	AuthorizeSecurityGroupIngress(ctx context.Context, params *ec2.AuthorizeSecurityGroupIngressInput, optFns ...func(*ec2.Options)) (*ec2.AuthorizeSecurityGroupIngressOutput, error)
	CreateTags(ctx context.Context, params *ec2.CreateTagsInput, optFns ...func(*ec2.Options)) (*ec2.CreateTagsOutput, error)
	RunInstances(ctx context.Context, params *ec2.RunInstancesInput, optFns ...func(*ec2.Options)) (*ec2.RunInstancesOutput, error)
	DescribeInstances(ctx context.Context, params *ec2.DescribeInstancesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error)
	StartInstances(ctx context.Context, params *ec2.StartInstancesInput, optFns ...func(*ec2.Options)) (*ec2.StartInstancesOutput, error)
	StopInstances(ctx context.Context, params *ec2.StopInstancesInput, optFns ...func(*ec2.Options)) (*ec2.StopInstancesOutput, error)
	TerminateInstances(ctx context.Context, params *ec2.TerminateInstancesInput, optFns ...func(*ec2.Options)) (*ec2.TerminateInstancesOutput, error)
	DescribeImages(ctx context.Context, params *ec2.DescribeImagesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error)
}

// S3API is the subset of the S3 client used by AWSProvider.
type S3API interface {
	HeadBucket(ctx context.Context, params *s3.HeadBucketInput, optFns ...func(*s3.Options)) (*s3.HeadBucketOutput, error)
	CreateBucket(ctx context.Context, params *s3.CreateBucketInput, optFns ...func(*s3.Options)) (*s3.CreateBucketOutput, error)
	PutBucketLifecycleConfiguration(ctx context.Context, params *s3.PutBucketLifecycleConfigurationInput, optFns ...func(*s3.Options)) (*s3.PutBucketLifecycleConfigurationOutput, error)
}

// STSAPI is the subset of the STS client used by AWSProvider.
type STSAPI interface {
	GetCallerIdentity(ctx context.Context, params *sts.GetCallerIdentityInput, optFns ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error)
}

// InstanceWaiter waits for an instance to reach the running state.
type InstanceWaiter interface {
	Wait(ctx context.Context, params *ec2.DescribeInstancesInput, maxWaitDur time.Duration, optFns ...func(*ec2.InstanceRunningWaiterOptions)) error
}

// AWSProvider implements CloudProvider using AWS services.
type AWSProvider struct {
	ec2    EC2API
	s3     S3API
	sts    STSAPI
	waiter InstanceWaiter
	region string

	accountOnce sync.Once
	accountID   string
	accountErr  error
}

// NewAWSProvider creates an AWSProvider from the default AWS config.
// Uses standard credential resolution: env vars → ~/.aws/credentials → IAM role.
func NewAWSProvider(ctx context.Context, region string) (*AWSProvider, error) {
	opts := []func(*awsconfig.LoadOptions) error{}
	if region != "" {
		opts = append(opts, awsconfig.WithRegion(region))
	}

	cfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("loading AWS config: %w", err)
	}

	ec2Client := ec2.NewFromConfig(cfg)
	return &AWSProvider{
		ec2:    ec2Client,
		s3:     s3.NewFromConfig(cfg),
		sts:    sts.NewFromConfig(cfg),
		waiter: ec2.NewInstanceRunningWaiter(ec2Client),
		region: cfg.Region,
	}, nil
}

// NewAWSProviderFromClients creates an AWSProvider from pre-built clients.
// Used for testing with mocked interfaces.
func NewAWSProviderFromClients(ec2api EC2API, s3api S3API, stsapi STSAPI, waiter InstanceWaiter, region string) *AWSProvider {
	return &AWSProvider{
		ec2:    ec2api,
		s3:     s3api,
		sts:    stsapi,
		waiter: waiter,
		region: region,
	}
}

// Region returns the configured AWS region.
func (p *AWSProvider) Region() string {
	return p.region
}

// AccountID returns the authenticated AWS account ID. Cached after first call.
func (p *AWSProvider) AccountID(ctx context.Context) (string, error) {
	p.accountOnce.Do(func() {
		out, err := p.sts.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
		if err != nil {
			p.accountErr = fmt.Errorf("verifying AWS credentials: %w", err)
			return
		}
		p.accountID = aws.ToString(out.Account)
	})
	return p.accountID, p.accountErr
}

// BucketName returns the yeager S3 bucket name for the account.
func (p *AWSProvider) BucketName(ctx context.Context) (string, error) {
	acct, err := p.AccountID(ctx)
	if err != nil {
		return "", err
	}
	return bucketPrefix + acct, nil
}

// EnsureSecurityGroup creates the yeager-sg security group if it doesn't exist.
// Returns the security group ID. Idempotent.
func (p *AWSProvider) EnsureSecurityGroup(ctx context.Context) (string, error) {
	// Check if it already exists.
	desc, err := p.ec2.DescribeSecurityGroups(ctx, &ec2.DescribeSecurityGroupsInput{
		Filters: []ec2types.Filter{
			{Name: aws.String("group-name"), Values: []string{securityGroupName}},
		},
	})
	if err != nil {
		return "", fmt.Errorf("describing security groups: %w", err)
	}
	if len(desc.SecurityGroups) > 0 {
		sgID := aws.ToString(desc.SecurityGroups[0].GroupId)
		slog.Debug("security group already exists", "sg_id", sgID)
		return sgID, nil
	}

	// Create it.
	create, err := p.ec2.CreateSecurityGroup(ctx, &ec2.CreateSecurityGroupInput{
		GroupName:   aws.String(securityGroupName),
		Description: aws.String(securityGroupDesc),
		TagSpecifications: []ec2types.TagSpecification{
			{
				ResourceType: ec2types.ResourceTypeSecurityGroup,
				Tags: []ec2types.Tag{
					{Key: aws.String(managedTagKey), Value: aws.String(managedTagValue)},
				},
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("creating security group: %w", err)
	}
	sgID := aws.ToString(create.GroupId)

	// Add ingress rules: TCP 22 and TCP 443 from 0.0.0.0/0.
	_, err = p.ec2.AuthorizeSecurityGroupIngress(ctx, &ec2.AuthorizeSecurityGroupIngressInput{
		GroupId: aws.String(sgID),
		IpPermissions: []ec2types.IpPermission{
			{
				IpProtocol: aws.String("tcp"),
				FromPort:   aws.Int32(22),
				ToPort:     aws.Int32(22),
				IpRanges:   []ec2types.IpRange{{CidrIp: aws.String("0.0.0.0/0")}},
			},
			{
				IpProtocol: aws.String("tcp"),
				FromPort:   aws.Int32(443),
				ToPort:     aws.Int32(443),
				IpRanges:   []ec2types.IpRange{{CidrIp: aws.String("0.0.0.0/0")}},
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("authorizing security group ingress: %w", err)
	}

	slog.Debug("created security group", "sg_id", sgID)
	return sgID, nil
}

// EnsureBucket creates the yeager S3 bucket if it doesn't exist.
// Applies lifecycle policy (30-day expiration). Idempotent.
func (p *AWSProvider) EnsureBucket(ctx context.Context) error {
	bucket, err := p.BucketName(ctx)
	if err != nil {
		return err
	}

	// Check if it already exists.
	_, err = p.s3.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(bucket),
	})
	if err == nil {
		slog.Debug("bucket already exists", "bucket", bucket)
		return nil
	}

	// Create the bucket.
	createInput := &s3.CreateBucketInput{
		Bucket: aws.String(bucket),
	}
	// us-east-1 must not specify LocationConstraint.
	if p.region != "us-east-1" {
		createInput.CreateBucketConfiguration = &s3types.CreateBucketConfiguration{
			LocationConstraint: s3types.BucketLocationConstraint(p.region),
		}
	}
	if _, err := p.s3.CreateBucket(ctx, createInput); err != nil {
		// Bucket may already exist in another region — treat as success.
		var alreadyOwned *s3types.BucketAlreadyOwnedByYou
		if !errors.As(err, &alreadyOwned) {
			return fmt.Errorf("creating bucket %s: %w", bucket, err)
		}
		slog.Debug("bucket already exists (cross-region)", "bucket", bucket)
		return nil
	}

	// Apply lifecycle policy: 30-day expiration + abort incomplete multipart uploads.
	_, err = p.s3.PutBucketLifecycleConfiguration(ctx, &s3.PutBucketLifecycleConfigurationInput{
		Bucket: aws.String(bucket),
		LifecycleConfiguration: &s3types.BucketLifecycleConfiguration{
			Rules: []s3types.LifecycleRule{
				{
					ID:     aws.String(lifecycleRuleID),
					Status: s3types.ExpirationStatusEnabled,
					Filter: &s3types.LifecycleRuleFilter{Prefix: aws.String("")},
					Expiration: &s3types.LifecycleExpiration{
						Days: aws.Int32(30),
					},
					AbortIncompleteMultipartUpload: &s3types.AbortIncompleteMultipartUpload{
						DaysAfterInitiation: aws.Int32(1),
					},
				},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("setting lifecycle policy on bucket %s: %w", bucket, err)
	}

	slog.Debug("created bucket", "bucket", bucket)
	return nil
}

// LookupUbuntuAMI finds the latest Ubuntu 24.04 LTS AMI for arm64.
func (p *AWSProvider) LookupUbuntuAMI(ctx context.Context) (string, error) {
	out, err := p.ec2.DescribeImages(ctx, &ec2.DescribeImagesInput{
		Owners: []string{"099720109477"}, // Canonical
		Filters: []ec2types.Filter{
			{Name: aws.String("name"), Values: []string{"ubuntu/images/hvm-ssd-gp3/ubuntu-noble-24.04-arm64-server-*"}},
			{Name: aws.String("architecture"), Values: []string{"arm64"}},
			{Name: aws.String("state"), Values: []string{"available"}},
		},
	})
	if err != nil {
		return "", fmt.Errorf("looking up Ubuntu AMI: %w", err)
	}
	if len(out.Images) == 0 {
		return "", fmt.Errorf("no Ubuntu 24.04 arm64 AMI found in %s", p.region)
	}

	// Find the most recent by creation date.
	latest := out.Images[0]
	for _, img := range out.Images[1:] {
		if aws.ToString(img.CreationDate) > aws.ToString(latest.CreationDate) {
			latest = img
		}
	}

	amiID := aws.ToString(latest.ImageId)
	slog.Debug("found Ubuntu AMI", "ami_id", amiID, "name", aws.ToString(latest.Name))
	return amiID, nil
}

// CreateVM launches a new EC2 instance for the given project.
func (p *AWSProvider) CreateVM(ctx context.Context, opts CreateVMOpts) (VMInfo, error) {
	instanceType, err := InstanceTypeForSize(opts.Size)
	if err != nil {
		return VMInfo{}, err
	}

	amiID, err := p.LookupUbuntuAMI(ctx)
	if err != nil {
		return VMInfo{}, err
	}

	now := time.Now().UTC().Format(time.RFC3339)

	input := &ec2.RunInstancesInput{
		ImageId:          aws.String(amiID),
		InstanceType:     instanceType,
		MinCount:         aws.Int32(1),
		MaxCount:         aws.Int32(1),
		SecurityGroupIds: []string{opts.SecurityGroupID},
		TagSpecifications: []ec2types.TagSpecification{
			{
				ResourceType: ec2types.ResourceTypeInstance,
				Tags: []ec2types.Tag{
					{Key: aws.String(projectHashTagKey), Value: aws.String(opts.ProjectHash)},
					{Key: aws.String(projectPathTagKey), Value: aws.String(opts.ProjectPath)},
					{Key: aws.String(createdTagKey), Value: aws.String(now)},
					{Key: aws.String("Name"), Value: aws.String("yeager-" + opts.ProjectHash)},
				},
			},
		},
	}
	if opts.UserData != "" {
		input.UserData = aws.String(opts.UserData)
	}

	out, err := p.ec2.RunInstances(ctx, input)
	if err != nil {
		return VMInfo{}, fmt.Errorf("launching instance: %w", err)
	}

	if len(out.Instances) == 0 {
		return VMInfo{}, fmt.Errorf("RunInstances returned no instances")
	}

	inst := out.Instances[0]
	info := p.toVMInfo(inst)

	slog.Debug("launched instance", "instance_id", info.InstanceID, "state", info.State)
	return info, nil
}

// FindVM looks up the VM for a project by its hash tag.
// Returns nil if no VM exists. Filters out terminated instances.
func (p *AWSProvider) FindVM(ctx context.Context, projectHash string) (*VMInfo, error) {
	out, err := p.ec2.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		Filters: []ec2types.Filter{
			{Name: aws.String("tag:" + projectHashTagKey), Values: []string{projectHash}},
			{
				Name: aws.String("instance-state-name"),
				Values: []string{
					string(ec2types.InstanceStateNamePending),
					string(ec2types.InstanceStateNameRunning),
					string(ec2types.InstanceStateNameStopping),
					string(ec2types.InstanceStateNameStopped),
				},
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("describing instances: %w", err)
	}

	for _, res := range out.Reservations {
		for _, inst := range res.Instances {
			info := p.toVMInfo(inst)
			return &info, nil
		}
	}

	return nil, nil
}

// toVMInfo converts an EC2 instance into a VMInfo.
func (p *AWSProvider) toVMInfo(inst ec2types.Instance) VMInfo {
	info := VMInfo{
		InstanceID: aws.ToString(inst.InstanceId),
		PublicIP:   aws.ToString(inst.PublicIpAddress),
		Region:     p.region,
	}
	if inst.State != nil {
		info.State = string(inst.State.Name)
	}
	if inst.Placement != nil {
		info.AvailabilityZone = aws.ToString(inst.Placement.AvailabilityZone)
	}
	info.InstanceType = string(inst.InstanceType)
	return info
}

// StartVM starts a stopped instance.
func (p *AWSProvider) StartVM(ctx context.Context, instanceID string) error {
	_, err := p.ec2.StartInstances(ctx, &ec2.StartInstancesInput{
		InstanceIds: []string{instanceID},
	})
	if err != nil {
		return fmt.Errorf("starting instance %s: %w", instanceID, err)
	}
	slog.Debug("started instance", "instance_id", instanceID)
	return nil
}

// StopVM stops a running instance.
func (p *AWSProvider) StopVM(ctx context.Context, instanceID string) error {
	_, err := p.ec2.StopInstances(ctx, &ec2.StopInstancesInput{
		InstanceIds: []string{instanceID},
	})
	if err != nil {
		return fmt.Errorf("stopping instance %s: %w", instanceID, err)
	}
	slog.Debug("stopped instance", "instance_id", instanceID)
	return nil
}

// TerminateVM terminates an instance.
func (p *AWSProvider) TerminateVM(ctx context.Context, instanceID string) error {
	_, err := p.ec2.TerminateInstances(ctx, &ec2.TerminateInstancesInput{
		InstanceIds: []string{instanceID},
	})
	if err != nil {
		return fmt.Errorf("terminating instance %s: %w", instanceID, err)
	}
	slog.Debug("terminated instance", "instance_id", instanceID)
	return nil
}


// IsExpiredTokenError checks if an error is an expired credentials error.
func IsExpiredTokenError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "ExpiredToken") ||
		strings.Contains(msg, "ExpiredTokenException") ||
		strings.Contains(msg, "RequestExpired")
}
