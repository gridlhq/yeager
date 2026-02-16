package provider

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mock implementations ---

type mockEC2 struct {
	describeSecurityGroupsFn     func(ctx context.Context, params *ec2.DescribeSecurityGroupsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeSecurityGroupsOutput, error)
	createSecurityGroupFn        func(ctx context.Context, params *ec2.CreateSecurityGroupInput, optFns ...func(*ec2.Options)) (*ec2.CreateSecurityGroupOutput, error)
	authorizeSecurityGroupIngressFn func(ctx context.Context, params *ec2.AuthorizeSecurityGroupIngressInput, optFns ...func(*ec2.Options)) (*ec2.AuthorizeSecurityGroupIngressOutput, error)
	createTagsFn                 func(ctx context.Context, params *ec2.CreateTagsInput, optFns ...func(*ec2.Options)) (*ec2.CreateTagsOutput, error)
	runInstancesFn               func(ctx context.Context, params *ec2.RunInstancesInput, optFns ...func(*ec2.Options)) (*ec2.RunInstancesOutput, error)
	describeInstancesFn          func(ctx context.Context, params *ec2.DescribeInstancesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error)
	startInstancesFn             func(ctx context.Context, params *ec2.StartInstancesInput, optFns ...func(*ec2.Options)) (*ec2.StartInstancesOutput, error)
	stopInstancesFn              func(ctx context.Context, params *ec2.StopInstancesInput, optFns ...func(*ec2.Options)) (*ec2.StopInstancesOutput, error)
	terminateInstancesFn         func(ctx context.Context, params *ec2.TerminateInstancesInput, optFns ...func(*ec2.Options)) (*ec2.TerminateInstancesOutput, error)
	describeImagesFn             func(ctx context.Context, params *ec2.DescribeImagesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error)
}

func (m *mockEC2) DescribeSecurityGroups(ctx context.Context, params *ec2.DescribeSecurityGroupsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeSecurityGroupsOutput, error) {
	return m.describeSecurityGroupsFn(ctx, params, optFns...)
}
func (m *mockEC2) CreateSecurityGroup(ctx context.Context, params *ec2.CreateSecurityGroupInput, optFns ...func(*ec2.Options)) (*ec2.CreateSecurityGroupOutput, error) {
	return m.createSecurityGroupFn(ctx, params, optFns...)
}
func (m *mockEC2) AuthorizeSecurityGroupIngress(ctx context.Context, params *ec2.AuthorizeSecurityGroupIngressInput, optFns ...func(*ec2.Options)) (*ec2.AuthorizeSecurityGroupIngressOutput, error) {
	return m.authorizeSecurityGroupIngressFn(ctx, params, optFns...)
}
func (m *mockEC2) CreateTags(ctx context.Context, params *ec2.CreateTagsInput, optFns ...func(*ec2.Options)) (*ec2.CreateTagsOutput, error) {
	return m.createTagsFn(ctx, params, optFns...)
}
func (m *mockEC2) RunInstances(ctx context.Context, params *ec2.RunInstancesInput, optFns ...func(*ec2.Options)) (*ec2.RunInstancesOutput, error) {
	return m.runInstancesFn(ctx, params, optFns...)
}
func (m *mockEC2) DescribeInstances(ctx context.Context, params *ec2.DescribeInstancesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
	return m.describeInstancesFn(ctx, params, optFns...)
}
func (m *mockEC2) StartInstances(ctx context.Context, params *ec2.StartInstancesInput, optFns ...func(*ec2.Options)) (*ec2.StartInstancesOutput, error) {
	return m.startInstancesFn(ctx, params, optFns...)
}
func (m *mockEC2) StopInstances(ctx context.Context, params *ec2.StopInstancesInput, optFns ...func(*ec2.Options)) (*ec2.StopInstancesOutput, error) {
	return m.stopInstancesFn(ctx, params, optFns...)
}
func (m *mockEC2) TerminateInstances(ctx context.Context, params *ec2.TerminateInstancesInput, optFns ...func(*ec2.Options)) (*ec2.TerminateInstancesOutput, error) {
	return m.terminateInstancesFn(ctx, params, optFns...)
}
func (m *mockEC2) DescribeImages(ctx context.Context, params *ec2.DescribeImagesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error) {
	return m.describeImagesFn(ctx, params, optFns...)
}

type mockS3 struct {
	headBucketFn                      func(ctx context.Context, params *s3.HeadBucketInput, optFns ...func(*s3.Options)) (*s3.HeadBucketOutput, error)
	createBucketFn                    func(ctx context.Context, params *s3.CreateBucketInput, optFns ...func(*s3.Options)) (*s3.CreateBucketOutput, error)
	putBucketLifecycleConfigurationFn func(ctx context.Context, params *s3.PutBucketLifecycleConfigurationInput, optFns ...func(*s3.Options)) (*s3.PutBucketLifecycleConfigurationOutput, error)
}

func (m *mockS3) HeadBucket(ctx context.Context, params *s3.HeadBucketInput, optFns ...func(*s3.Options)) (*s3.HeadBucketOutput, error) {
	return m.headBucketFn(ctx, params, optFns...)
}
func (m *mockS3) CreateBucket(ctx context.Context, params *s3.CreateBucketInput, optFns ...func(*s3.Options)) (*s3.CreateBucketOutput, error) {
	return m.createBucketFn(ctx, params, optFns...)
}
func (m *mockS3) PutBucketLifecycleConfiguration(ctx context.Context, params *s3.PutBucketLifecycleConfigurationInput, optFns ...func(*s3.Options)) (*s3.PutBucketLifecycleConfigurationOutput, error) {
	return m.putBucketLifecycleConfigurationFn(ctx, params, optFns...)
}

type mockSTS struct {
	getCallerIdentityFn func(ctx context.Context, params *sts.GetCallerIdentityInput, optFns ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error)
}

func (m *mockSTS) GetCallerIdentity(ctx context.Context, params *sts.GetCallerIdentityInput, optFns ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error) {
	return m.getCallerIdentityFn(ctx, params, optFns...)
}

type mockWaiter struct {
	waitFn func(ctx context.Context, params *ec2.DescribeInstancesInput, maxWaitDur time.Duration, optFns ...func(*ec2.InstanceRunningWaiterOptions)) error
}

func (m *mockWaiter) Wait(ctx context.Context, params *ec2.DescribeInstancesInput, maxWaitDur time.Duration, optFns ...func(*ec2.InstanceRunningWaiterOptions)) error {
	return m.waitFn(ctx, params, maxWaitDur, optFns...)
}

// --- Helpers ---

func newTestProvider(ec2mock *mockEC2, s3mock *mockS3, stsMock *mockSTS, waiterMock *mockWaiter) *AWSProvider {
	return NewAWSProviderFromClients(ec2mock, s3mock, stsMock, waiterMock, "us-east-1")
}

func stsWithAccount(accountID string) *mockSTS {
	return &mockSTS{
		getCallerIdentityFn: func(ctx context.Context, params *sts.GetCallerIdentityInput, optFns ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error) {
			return &sts.GetCallerIdentityOutput{Account: aws.String(accountID)}, nil
		},
	}
}

func stsWithError(err error) *mockSTS {
	return &mockSTS{
		getCallerIdentityFn: func(ctx context.Context, params *sts.GetCallerIdentityInput, optFns ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error) {
			return nil, err
		},
	}
}

// --- Tests ---

func TestInstanceTypeForSize(t *testing.T) {
	t.Parallel()
	tests := []struct {
		size    string
		want    ec2types.InstanceType
		wantErr bool
	}{
		{"small", ec2types.InstanceTypeT4gSmall, false},
		{"medium", ec2types.InstanceTypeT4gMedium, false},
		{"large", ec2types.InstanceTypeT4gLarge, false},
		{"xlarge", ec2types.InstanceTypeT4gXlarge, false},
		{"invalid", "", true},
		{"", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.size, func(t *testing.T) {
			t.Parallel()
			got, err := InstanceTypeForSize(tt.size)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestAccountID(t *testing.T) {
	t.Parallel()

	t.Run("returns account ID from STS", func(t *testing.T) {
		t.Parallel()
		p := newTestProvider(nil, nil, stsWithAccount("123456789012"), nil)
		acct, err := p.AccountID(context.Background())
		require.NoError(t, err)
		assert.Equal(t, "123456789012", acct)
	})

	t.Run("caches account ID", func(t *testing.T) {
		t.Parallel()
		calls := 0
		stsMock := &mockSTS{
			getCallerIdentityFn: func(ctx context.Context, params *sts.GetCallerIdentityInput, optFns ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error) {
				calls++
				return &sts.GetCallerIdentityOutput{Account: aws.String("123456789012")}, nil
			},
		}
		p := newTestProvider(nil, nil, stsMock, nil)
		_, _ = p.AccountID(context.Background())
		_, _ = p.AccountID(context.Background())
		assert.Equal(t, 1, calls, "STS should be called only once")
	})

	t.Run("returns error on credential failure", func(t *testing.T) {
		t.Parallel()
		p := newTestProvider(nil, nil, stsWithError(fmt.Errorf("ExpiredTokenException: token has expired")), nil)
		_, err := p.AccountID(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "verifying AWS credentials")
	})
}

func TestBucketName(t *testing.T) {
	t.Parallel()

	t.Run("returns yeager-{account-id}", func(t *testing.T) {
		t.Parallel()
		p := newTestProvider(nil, nil, stsWithAccount("123456789012"), nil)
		name, err := p.BucketName(context.Background())
		require.NoError(t, err)
		assert.Equal(t, "yeager-123456789012", name)
	})

	t.Run("propagates STS error", func(t *testing.T) {
		t.Parallel()
		p := newTestProvider(nil, nil, stsWithError(fmt.Errorf("no credentials")), nil)
		_, err := p.BucketName(context.Background())
		require.Error(t, err)
	})
}

func TestEnsureSecurityGroup(t *testing.T) {
	t.Parallel()

	t.Run("skips creation when group exists", func(t *testing.T) {
		t.Parallel()
		ec2Mock := &mockEC2{
			describeSecurityGroupsFn: func(ctx context.Context, params *ec2.DescribeSecurityGroupsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeSecurityGroupsOutput, error) {
				// Verify we're filtering by the correct name.
				require.Len(t, params.Filters, 1)
				assert.Equal(t, "group-name", aws.ToString(params.Filters[0].Name))
				assert.Equal(t, []string{"yeager-sg"}, params.Filters[0].Values)
				return &ec2.DescribeSecurityGroupsOutput{
					SecurityGroups: []ec2types.SecurityGroup{
						{GroupId: aws.String("sg-existing123")},
					},
				}, nil
			},
		}
		p := newTestProvider(ec2Mock, nil, nil, nil)
		sgID, err := p.EnsureSecurityGroup(context.Background())
		require.NoError(t, err)
		assert.Equal(t, "sg-existing123", sgID)
	})

	t.Run("creates group with correct ingress rules when absent", func(t *testing.T) {
		t.Parallel()
		var createdGroupID string
		var ingressRules []ec2types.IpPermission

		ec2Mock := &mockEC2{
			describeSecurityGroupsFn: func(ctx context.Context, params *ec2.DescribeSecurityGroupsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeSecurityGroupsOutput, error) {
				return &ec2.DescribeSecurityGroupsOutput{SecurityGroups: nil}, nil
			},
			createSecurityGroupFn: func(ctx context.Context, params *ec2.CreateSecurityGroupInput, optFns ...func(*ec2.Options)) (*ec2.CreateSecurityGroupOutput, error) {
				assert.Equal(t, "yeager-sg", aws.ToString(params.GroupName))
				// Verify managed tag at creation time.
				require.Len(t, params.TagSpecifications, 1)
				tags := params.TagSpecifications[0].Tags
				require.Len(t, tags, 1)
				assert.Equal(t, "yeager:managed", aws.ToString(tags[0].Key))
				assert.Equal(t, "true", aws.ToString(tags[0].Value))
				createdGroupID = "sg-new456"
				return &ec2.CreateSecurityGroupOutput{GroupId: aws.String(createdGroupID)}, nil
			},
			authorizeSecurityGroupIngressFn: func(ctx context.Context, params *ec2.AuthorizeSecurityGroupIngressInput, optFns ...func(*ec2.Options)) (*ec2.AuthorizeSecurityGroupIngressOutput, error) {
				assert.Equal(t, "sg-new456", aws.ToString(params.GroupId))
				ingressRules = params.IpPermissions
				return &ec2.AuthorizeSecurityGroupIngressOutput{}, nil
			},
		}

		p := newTestProvider(ec2Mock, nil, nil, nil)
		sgID, err := p.EnsureSecurityGroup(context.Background())
		require.NoError(t, err)
		assert.Equal(t, "sg-new456", sgID)

		// Verify both port 22 and 443 rules.
		require.Len(t, ingressRules, 2)

		assert.Equal(t, int32(22), aws.ToInt32(ingressRules[0].FromPort))
		assert.Equal(t, int32(22), aws.ToInt32(ingressRules[0].ToPort))
		assert.Equal(t, "0.0.0.0/0", aws.ToString(ingressRules[0].IpRanges[0].CidrIp))

		assert.Equal(t, int32(443), aws.ToInt32(ingressRules[1].FromPort))
		assert.Equal(t, int32(443), aws.ToInt32(ingressRules[1].ToPort))
		assert.Equal(t, "0.0.0.0/0", aws.ToString(ingressRules[1].IpRanges[0].CidrIp))
	})

	t.Run("propagates describe error", func(t *testing.T) {
		t.Parallel()
		ec2Mock := &mockEC2{
			describeSecurityGroupsFn: func(ctx context.Context, params *ec2.DescribeSecurityGroupsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeSecurityGroupsOutput, error) {
				return nil, fmt.Errorf("access denied")
			},
		}
		p := newTestProvider(ec2Mock, nil, nil, nil)
		_, err := p.EnsureSecurityGroup(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "describing security groups")
	})

	t.Run("propagates create error", func(t *testing.T) {
		t.Parallel()
		ec2Mock := &mockEC2{
			describeSecurityGroupsFn: func(ctx context.Context, params *ec2.DescribeSecurityGroupsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeSecurityGroupsOutput, error) {
				return &ec2.DescribeSecurityGroupsOutput{SecurityGroups: nil}, nil
			},
			createSecurityGroupFn: func(ctx context.Context, params *ec2.CreateSecurityGroupInput, optFns ...func(*ec2.Options)) (*ec2.CreateSecurityGroupOutput, error) {
				return nil, fmt.Errorf("quota exceeded")
			},
		}
		p := newTestProvider(ec2Mock, nil, nil, nil)
		_, err := p.EnsureSecurityGroup(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "creating security group")
	})
}

func TestEnsureBucket(t *testing.T) {
	t.Parallel()

	t.Run("skips creation when bucket exists", func(t *testing.T) {
		t.Parallel()
		createCalled := false
		s3Mock := &mockS3{
			headBucketFn: func(ctx context.Context, params *s3.HeadBucketInput, optFns ...func(*s3.Options)) (*s3.HeadBucketOutput, error) {
				assert.Equal(t, "yeager-123456789012", aws.ToString(params.Bucket))
				return &s3.HeadBucketOutput{}, nil
			},
			createBucketFn: func(ctx context.Context, params *s3.CreateBucketInput, optFns ...func(*s3.Options)) (*s3.CreateBucketOutput, error) {
				createCalled = true
				return nil, nil
			},
		}
		p := newTestProvider(nil, s3Mock, stsWithAccount("123456789012"), nil)
		err := p.EnsureBucket(context.Background())
		require.NoError(t, err)
		assert.False(t, createCalled, "should not call CreateBucket when bucket exists")
	})

	t.Run("creates bucket with lifecycle when absent", func(t *testing.T) {
		t.Parallel()
		var lifecycleRules []s3types.LifecycleRule
		s3Mock := &mockS3{
			headBucketFn: func(ctx context.Context, params *s3.HeadBucketInput, optFns ...func(*s3.Options)) (*s3.HeadBucketOutput, error) {
				return nil, fmt.Errorf("NotFound")
			},
			createBucketFn: func(ctx context.Context, params *s3.CreateBucketInput, optFns ...func(*s3.Options)) (*s3.CreateBucketOutput, error) {
				assert.Equal(t, "yeager-123456789012", aws.ToString(params.Bucket))
				// us-east-1 should not have LocationConstraint.
				assert.Nil(t, params.CreateBucketConfiguration)
				return &s3.CreateBucketOutput{}, nil
			},
			putBucketLifecycleConfigurationFn: func(ctx context.Context, params *s3.PutBucketLifecycleConfigurationInput, optFns ...func(*s3.Options)) (*s3.PutBucketLifecycleConfigurationOutput, error) {
				lifecycleRules = params.LifecycleConfiguration.Rules
				return &s3.PutBucketLifecycleConfigurationOutput{}, nil
			},
		}
		p := newTestProvider(nil, s3Mock, stsWithAccount("123456789012"), nil)
		err := p.EnsureBucket(context.Background())
		require.NoError(t, err)

		require.Len(t, lifecycleRules, 1)
		assert.Equal(t, "yeager-expire-30d", aws.ToString(lifecycleRules[0].ID))
		assert.Equal(t, s3types.ExpirationStatusEnabled, lifecycleRules[0].Status)
		assert.Equal(t, int32(30), aws.ToInt32(lifecycleRules[0].Expiration.Days))
		assert.Equal(t, int32(1), aws.ToInt32(lifecycleRules[0].AbortIncompleteMultipartUpload.DaysAfterInitiation))
	})

	t.Run("sets LocationConstraint for non-us-east-1 regions", func(t *testing.T) {
		t.Parallel()
		var locationConstraint s3types.BucketLocationConstraint
		s3Mock := &mockS3{
			headBucketFn: func(ctx context.Context, params *s3.HeadBucketInput, optFns ...func(*s3.Options)) (*s3.HeadBucketOutput, error) {
				return nil, fmt.Errorf("NotFound")
			},
			createBucketFn: func(ctx context.Context, params *s3.CreateBucketInput, optFns ...func(*s3.Options)) (*s3.CreateBucketOutput, error) {
				require.NotNil(t, params.CreateBucketConfiguration)
				locationConstraint = params.CreateBucketConfiguration.LocationConstraint
				return &s3.CreateBucketOutput{}, nil
			},
			putBucketLifecycleConfigurationFn: func(ctx context.Context, params *s3.PutBucketLifecycleConfigurationInput, optFns ...func(*s3.Options)) (*s3.PutBucketLifecycleConfigurationOutput, error) {
				return &s3.PutBucketLifecycleConfigurationOutput{}, nil
			},
		}
		stsMock := stsWithAccount("123456789012")
		p := NewAWSProviderFromClients(nil, s3Mock, stsMock, nil, "eu-west-1")
		err := p.EnsureBucket(context.Background())
		require.NoError(t, err)
		assert.Equal(t, s3types.BucketLocationConstraint("eu-west-1"), locationConstraint)
	})

	t.Run("propagates CreateBucket error", func(t *testing.T) {
		t.Parallel()
		s3Mock := &mockS3{
			headBucketFn: func(ctx context.Context, params *s3.HeadBucketInput, optFns ...func(*s3.Options)) (*s3.HeadBucketOutput, error) {
				return nil, fmt.Errorf("NotFound")
			},
			createBucketFn: func(ctx context.Context, params *s3.CreateBucketInput, optFns ...func(*s3.Options)) (*s3.CreateBucketOutput, error) {
				return nil, fmt.Errorf("access denied")
			},
		}
		p := newTestProvider(nil, s3Mock, stsWithAccount("123456789012"), nil)
		err := p.EnsureBucket(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "creating bucket")
	})

	t.Run("propagates lifecycle policy error", func(t *testing.T) {
		t.Parallel()
		s3Mock := &mockS3{
			headBucketFn: func(ctx context.Context, params *s3.HeadBucketInput, optFns ...func(*s3.Options)) (*s3.HeadBucketOutput, error) {
				return nil, fmt.Errorf("NotFound")
			},
			createBucketFn: func(ctx context.Context, params *s3.CreateBucketInput, optFns ...func(*s3.Options)) (*s3.CreateBucketOutput, error) {
				return &s3.CreateBucketOutput{}, nil
			},
			putBucketLifecycleConfigurationFn: func(ctx context.Context, params *s3.PutBucketLifecycleConfigurationInput, optFns ...func(*s3.Options)) (*s3.PutBucketLifecycleConfigurationOutput, error) {
				return nil, fmt.Errorf("lifecycle policy error")
			},
		}
		p := newTestProvider(nil, s3Mock, stsWithAccount("123456789012"), nil)
		err := p.EnsureBucket(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "setting lifecycle policy")
	})
}

func TestCreateVM(t *testing.T) {
	t.Parallel()

	t.Run("sends correct RunInstances params", func(t *testing.T) {
		t.Parallel()
		ec2Mock := &mockEC2{
			describeImagesFn: func(ctx context.Context, params *ec2.DescribeImagesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error) {
				// Verify canonical owner and arm64 filter.
				assert.Contains(t, params.Owners, "099720109477")
				return &ec2.DescribeImagesOutput{
					Images: []ec2types.Image{
						{ImageId: aws.String("ami-test123"), CreationDate: aws.String("2024-01-01T00:00:00Z"), Name: aws.String("ubuntu-noble")},
					},
				}, nil
			},
			runInstancesFn: func(ctx context.Context, params *ec2.RunInstancesInput, optFns ...func(*ec2.Options)) (*ec2.RunInstancesOutput, error) {
				assert.Equal(t, "ami-test123", aws.ToString(params.ImageId))
				assert.Equal(t, ec2types.InstanceTypeT4gMedium, params.InstanceType)
				assert.Equal(t, int32(1), aws.ToInt32(params.MinCount))
				assert.Equal(t, int32(1), aws.ToInt32(params.MaxCount))
				assert.Equal(t, []string{"sg-test"}, params.SecurityGroupIds)

				// Verify tags.
				require.Len(t, params.TagSpecifications, 1)
				tags := params.TagSpecifications[0].Tags
				tagMap := make(map[string]string)
				for _, tag := range tags {
					tagMap[aws.ToString(tag.Key)] = aws.ToString(tag.Value)
				}
				assert.Equal(t, "abc123", tagMap["yeager:project-hash"])
				assert.Equal(t, "/home/user/myproject", tagMap["yeager:project-path"])
				assert.Contains(t, tagMap, "yeager:created")
				assert.Equal(t, "yeager-abc123", tagMap["Name"])

				return &ec2.RunInstancesOutput{
					Instances: []ec2types.Instance{
						{
							InstanceId:      aws.String("i-new789"),
							State:           &ec2types.InstanceState{Name: ec2types.InstanceStateNamePending},
							PublicIpAddress: aws.String("1.2.3.4"),
						},
					},
				}, nil
			},
		}

		p := newTestProvider(ec2Mock, nil, nil, nil)
		info, err := p.CreateVM(context.Background(), CreateVMOpts{
			ProjectHash:     "abc123",
			ProjectPath:     "/home/user/myproject",
			Size:            "medium",
			SecurityGroupID: "sg-test",
		})
		require.NoError(t, err)
		assert.Equal(t, "i-new789", info.InstanceID)
		assert.Equal(t, "pending", info.State)
		assert.Equal(t, "1.2.3.4", info.PublicIP)
		assert.Equal(t, "us-east-1", info.Region)
	})

	t.Run("passes UserData when provided", func(t *testing.T) {
		t.Parallel()
		var capturedUserData *string
		ec2Mock := &mockEC2{
			describeImagesFn: func(ctx context.Context, params *ec2.DescribeImagesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error) {
				return &ec2.DescribeImagesOutput{
					Images: []ec2types.Image{
						{ImageId: aws.String("ami-test"), CreationDate: aws.String("2024-01-01T00:00:00Z"), Name: aws.String("test")},
					},
				}, nil
			},
			runInstancesFn: func(ctx context.Context, params *ec2.RunInstancesInput, optFns ...func(*ec2.Options)) (*ec2.RunInstancesOutput, error) {
				capturedUserData = params.UserData
				return &ec2.RunInstancesOutput{
					Instances: []ec2types.Instance{
						{
							InstanceId: aws.String("i-ud001"),
							State:      &ec2types.InstanceState{Name: ec2types.InstanceStateNamePending},
						},
					},
				}, nil
			},
		}

		p := newTestProvider(ec2Mock, nil, nil, nil)
		_, err := p.CreateVM(context.Background(), CreateVMOpts{
			ProjectHash:     "abc123",
			ProjectPath:     "/test",
			Size:            "small",
			SecurityGroupID: "sg-test",
			UserData:        "dGVzdC1jbG91ZC1pbml0",
		})
		require.NoError(t, err)
		require.NotNil(t, capturedUserData)
		assert.Equal(t, "dGVzdC1jbG91ZC1pbml0", aws.ToString(capturedUserData))
	})

	t.Run("omits UserData when empty", func(t *testing.T) {
		t.Parallel()
		ec2Mock := &mockEC2{
			describeImagesFn: func(ctx context.Context, params *ec2.DescribeImagesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error) {
				return &ec2.DescribeImagesOutput{
					Images: []ec2types.Image{
						{ImageId: aws.String("ami-test"), CreationDate: aws.String("2024-01-01T00:00:00Z"), Name: aws.String("test")},
					},
				}, nil
			},
			runInstancesFn: func(ctx context.Context, params *ec2.RunInstancesInput, optFns ...func(*ec2.Options)) (*ec2.RunInstancesOutput, error) {
				assert.Nil(t, params.UserData, "UserData should be nil when not provided")
				return &ec2.RunInstancesOutput{
					Instances: []ec2types.Instance{
						{
							InstanceId: aws.String("i-noud001"),
							State:      &ec2types.InstanceState{Name: ec2types.InstanceStateNamePending},
						},
					},
				}, nil
			},
		}

		p := newTestProvider(ec2Mock, nil, nil, nil)
		_, err := p.CreateVM(context.Background(), CreateVMOpts{
			ProjectHash:     "abc123",
			ProjectPath:     "/test",
			Size:            "small",
			SecurityGroupID: "sg-test",
		})
		require.NoError(t, err)
	})

	t.Run("returns error for invalid size", func(t *testing.T) {
		t.Parallel()
		p := newTestProvider(nil, nil, nil, nil)
		_, err := p.CreateVM(context.Background(), CreateVMOpts{Size: "tiny"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown instance size")
	})

	t.Run("returns error when no AMI found", func(t *testing.T) {
		t.Parallel()
		ec2Mock := &mockEC2{
			describeImagesFn: func(ctx context.Context, params *ec2.DescribeImagesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error) {
				return &ec2.DescribeImagesOutput{Images: nil}, nil
			},
		}
		p := newTestProvider(ec2Mock, nil, nil, nil)
		_, err := p.CreateVM(context.Background(), CreateVMOpts{Size: "medium"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no Ubuntu 24.04 arm64 AMI found")
	})
}

func TestEnsureSecurityGroup_AuthorizeIngressError(t *testing.T) {
	t.Parallel()

	ec2Mock := &mockEC2{
		describeSecurityGroupsFn: func(ctx context.Context, params *ec2.DescribeSecurityGroupsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeSecurityGroupsOutput, error) {
			return &ec2.DescribeSecurityGroupsOutput{SecurityGroups: nil}, nil
		},
		createSecurityGroupFn: func(ctx context.Context, params *ec2.CreateSecurityGroupInput, optFns ...func(*ec2.Options)) (*ec2.CreateSecurityGroupOutput, error) {
			return &ec2.CreateSecurityGroupOutput{GroupId: aws.String("sg-new789")}, nil
		},
		authorizeSecurityGroupIngressFn: func(ctx context.Context, params *ec2.AuthorizeSecurityGroupIngressInput, optFns ...func(*ec2.Options)) (*ec2.AuthorizeSecurityGroupIngressOutput, error) {
			return nil, fmt.Errorf("ingress authorization failed")
		},
	}
	p := newTestProvider(ec2Mock, nil, nil, nil)
	_, err := p.EnsureSecurityGroup(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "authorizing security group ingress")
}

func TestCreateVM_RunInstancesError(t *testing.T) {
	t.Parallel()

	ec2Mock := &mockEC2{
		describeImagesFn: func(ctx context.Context, params *ec2.DescribeImagesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error) {
			return &ec2.DescribeImagesOutput{
				Images: []ec2types.Image{
					{ImageId: aws.String("ami-test"), CreationDate: aws.String("2024-01-01T00:00:00Z"), Name: aws.String("test")},
				},
			}, nil
		},
		runInstancesFn: func(ctx context.Context, params *ec2.RunInstancesInput, optFns ...func(*ec2.Options)) (*ec2.RunInstancesOutput, error) {
			return nil, fmt.Errorf("InsufficientInstanceCapacity")
		},
	}
	p := newTestProvider(ec2Mock, nil, nil, nil)
	_, err := p.CreateVM(context.Background(), CreateVMOpts{Size: "medium", ProjectHash: "abc", ProjectPath: "/test", SecurityGroupID: "sg-test"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "launching instance")
}

func TestCreateVM_EmptyInstances(t *testing.T) {
	t.Parallel()

	ec2Mock := &mockEC2{
		describeImagesFn: func(ctx context.Context, params *ec2.DescribeImagesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error) {
			return &ec2.DescribeImagesOutput{
				Images: []ec2types.Image{
					{ImageId: aws.String("ami-test"), CreationDate: aws.String("2024-01-01T00:00:00Z"), Name: aws.String("test")},
				},
			}, nil
		},
		runInstancesFn: func(ctx context.Context, params *ec2.RunInstancesInput, optFns ...func(*ec2.Options)) (*ec2.RunInstancesOutput, error) {
			return &ec2.RunInstancesOutput{Instances: nil}, nil
		},
	}
	p := newTestProvider(ec2Mock, nil, nil, nil)
	_, err := p.CreateVM(context.Background(), CreateVMOpts{Size: "medium", ProjectHash: "abc", ProjectPath: "/test", SecurityGroupID: "sg-test"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "RunInstances returned no instances")
}

func TestFindVM_DescribeError(t *testing.T) {
	t.Parallel()

	ec2Mock := &mockEC2{
		describeInstancesFn: func(ctx context.Context, params *ec2.DescribeInstancesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
			return nil, fmt.Errorf("throttling exception")
		},
	}
	p := newTestProvider(ec2Mock, nil, nil, nil)
	_, err := p.FindVM(context.Background(), "abc123")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "describing instances")
}

func TestLookupUbuntuAMI_DescribeImagesError(t *testing.T) {
	t.Parallel()

	ec2Mock := &mockEC2{
		describeImagesFn: func(ctx context.Context, params *ec2.DescribeImagesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error) {
			return nil, fmt.Errorf("access denied to DescribeImages")
		},
	}
	p := newTestProvider(ec2Mock, nil, nil, nil)
	_, err := p.LookupUbuntuAMI(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "looking up Ubuntu AMI")
}

func TestCreateVM_AMILookupError(t *testing.T) {
	t.Parallel()

	ec2Mock := &mockEC2{
		describeImagesFn: func(ctx context.Context, params *ec2.DescribeImagesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error) {
			return nil, fmt.Errorf("AMI lookup failed")
		},
	}
	p := newTestProvider(ec2Mock, nil, nil, nil)
	_, err := p.CreateVM(context.Background(), CreateVMOpts{Size: "medium", ProjectHash: "abc", ProjectPath: "/test", SecurityGroupID: "sg-test"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "AMI lookup failed")
}

func TestFindVM(t *testing.T) {
	t.Parallel()

	t.Run("finds running instance by project hash", func(t *testing.T) {
		t.Parallel()
		ec2Mock := &mockEC2{
			describeInstancesFn: func(ctx context.Context, params *ec2.DescribeInstancesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
				// Verify filter on project hash tag.
				foundHashFilter := false
				for _, f := range params.Filters {
					if aws.ToString(f.Name) == "tag:yeager:project-hash" {
						assert.Equal(t, []string{"abc123"}, f.Values)
						foundHashFilter = true
					}
				}
				assert.True(t, foundHashFilter, "should filter by project hash tag")

				return &ec2.DescribeInstancesOutput{
					Reservations: []ec2types.Reservation{
						{
							Instances: []ec2types.Instance{
								{
									InstanceId:      aws.String("i-found001"),
									State:           &ec2types.InstanceState{Name: ec2types.InstanceStateNameRunning},
									PublicIpAddress: aws.String("5.6.7.8"),
									Placement:       &ec2types.Placement{AvailabilityZone: aws.String("us-east-1a")},
								},
							},
						},
					},
				}, nil
			},
		}
		p := newTestProvider(ec2Mock, nil, nil, nil)
		info, err := p.FindVM(context.Background(), "abc123")
		require.NoError(t, err)
		require.NotNil(t, info)
		assert.Equal(t, "i-found001", info.InstanceID)
		assert.Equal(t, "running", info.State)
		assert.Equal(t, "5.6.7.8", info.PublicIP)
		assert.Equal(t, "us-east-1a", info.AvailabilityZone)
	})

	t.Run("returns nil when no VM exists", func(t *testing.T) {
		t.Parallel()
		ec2Mock := &mockEC2{
			describeInstancesFn: func(ctx context.Context, params *ec2.DescribeInstancesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
				return &ec2.DescribeInstancesOutput{Reservations: nil}, nil
			},
		}
		p := newTestProvider(ec2Mock, nil, nil, nil)
		info, err := p.FindVM(context.Background(), "nonexistent")
		require.NoError(t, err)
		assert.Nil(t, info)
	})

	t.Run("filters out terminated instances", func(t *testing.T) {
		t.Parallel()
		ec2Mock := &mockEC2{
			describeInstancesFn: func(ctx context.Context, params *ec2.DescribeInstancesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
				// Verify the state filter excludes terminated/shutting-down.
				for _, f := range params.Filters {
					if aws.ToString(f.Name) == "instance-state-name" {
						assert.NotContains(t, f.Values, "terminated")
						assert.NotContains(t, f.Values, "shutting-down")
						assert.Contains(t, f.Values, "running")
						assert.Contains(t, f.Values, "stopped")
						assert.Contains(t, f.Values, "pending")
						assert.Contains(t, f.Values, "stopping")
					}
				}
				return &ec2.DescribeInstancesOutput{Reservations: nil}, nil
			},
		}
		p := newTestProvider(ec2Mock, nil, nil, nil)
		_, _ = p.FindVM(context.Background(), "abc123")
	})
}

func TestStartVM(t *testing.T) {
	t.Parallel()

	t.Run("calls StartInstances with correct ID", func(t *testing.T) {
		t.Parallel()
		ec2Mock := &mockEC2{
			startInstancesFn: func(ctx context.Context, params *ec2.StartInstancesInput, optFns ...func(*ec2.Options)) (*ec2.StartInstancesOutput, error) {
				assert.Equal(t, []string{"i-start001"}, params.InstanceIds)
				return &ec2.StartInstancesOutput{}, nil
			},
		}
		p := newTestProvider(ec2Mock, nil, nil, nil)
		err := p.StartVM(context.Background(), "i-start001")
		require.NoError(t, err)
	})

	t.Run("propagates error", func(t *testing.T) {
		t.Parallel()
		ec2Mock := &mockEC2{
			startInstancesFn: func(ctx context.Context, params *ec2.StartInstancesInput, optFns ...func(*ec2.Options)) (*ec2.StartInstancesOutput, error) {
				return nil, fmt.Errorf("IncorrectInstanceState")
			},
		}
		p := newTestProvider(ec2Mock, nil, nil, nil)
		err := p.StartVM(context.Background(), "i-start001")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "starting instance")
	})
}

func TestStopVM(t *testing.T) {
	t.Parallel()

	t.Run("calls StopInstances with correct ID", func(t *testing.T) {
		t.Parallel()
		ec2Mock := &mockEC2{
			stopInstancesFn: func(ctx context.Context, params *ec2.StopInstancesInput, optFns ...func(*ec2.Options)) (*ec2.StopInstancesOutput, error) {
				assert.Equal(t, []string{"i-stop001"}, params.InstanceIds)
				return &ec2.StopInstancesOutput{}, nil
			},
		}
		p := newTestProvider(ec2Mock, nil, nil, nil)
		err := p.StopVM(context.Background(), "i-stop001")
		require.NoError(t, err)
	})

	t.Run("propagates error", func(t *testing.T) {
		t.Parallel()
		ec2Mock := &mockEC2{
			stopInstancesFn: func(ctx context.Context, params *ec2.StopInstancesInput, optFns ...func(*ec2.Options)) (*ec2.StopInstancesOutput, error) {
				return nil, fmt.Errorf("IncorrectInstanceState")
			},
		}
		p := newTestProvider(ec2Mock, nil, nil, nil)
		err := p.StopVM(context.Background(), "i-stop001")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "stopping instance")
	})
}

func TestTerminateVM(t *testing.T) {
	t.Parallel()

	t.Run("calls TerminateInstances with correct ID", func(t *testing.T) {
		t.Parallel()
		ec2Mock := &mockEC2{
			terminateInstancesFn: func(ctx context.Context, params *ec2.TerminateInstancesInput, optFns ...func(*ec2.Options)) (*ec2.TerminateInstancesOutput, error) {
				assert.Equal(t, []string{"i-term001"}, params.InstanceIds)
				return &ec2.TerminateInstancesOutput{}, nil
			},
		}
		p := newTestProvider(ec2Mock, nil, nil, nil)
		err := p.TerminateVM(context.Background(), "i-term001")
		require.NoError(t, err)
	})

	t.Run("propagates error", func(t *testing.T) {
		t.Parallel()
		ec2Mock := &mockEC2{
			terminateInstancesFn: func(ctx context.Context, params *ec2.TerminateInstancesInput, optFns ...func(*ec2.Options)) (*ec2.TerminateInstancesOutput, error) {
				return nil, fmt.Errorf("unauthorized operation")
			},
		}
		p := newTestProvider(ec2Mock, nil, nil, nil)
		err := p.TerminateVM(context.Background(), "i-term001")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "terminating instance")
	})
}

func TestWaitUntilRunning(t *testing.T) {
	t.Parallel()

	t.Run("delegates to waiter with correct params", func(t *testing.T) {
		t.Parallel()
		waiterMock := &mockWaiter{
			waitFn: func(ctx context.Context, params *ec2.DescribeInstancesInput, maxWaitDur time.Duration, optFns ...func(*ec2.InstanceRunningWaiterOptions)) error {
				assert.Equal(t, []string{"i-wait001"}, params.InstanceIds)
				assert.Equal(t, waitTimeout, maxWaitDur)
				return nil
			},
		}
		p := newTestProvider(nil, nil, nil, waiterMock)
		err := p.WaitUntilRunning(context.Background(), "i-wait001")
		require.NoError(t, err)
	})

	t.Run("propagates waiter error", func(t *testing.T) {
		t.Parallel()
		waiterMock := &mockWaiter{
			waitFn: func(ctx context.Context, params *ec2.DescribeInstancesInput, maxWaitDur time.Duration, optFns ...func(*ec2.InstanceRunningWaiterOptions)) error {
				return fmt.Errorf("exceeded max wait time")
			},
		}
		p := newTestProvider(nil, nil, nil, waiterMock)
		err := p.WaitUntilRunning(context.Background(), "i-wait001")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "waiting for instance")
	})
}

func TestLookupUbuntuAMI(t *testing.T) {
	t.Parallel()

	t.Run("returns latest AMI by creation date", func(t *testing.T) {
		t.Parallel()
		ec2Mock := &mockEC2{
			describeImagesFn: func(ctx context.Context, params *ec2.DescribeImagesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error) {
				return &ec2.DescribeImagesOutput{
					Images: []ec2types.Image{
						{ImageId: aws.String("ami-old"), CreationDate: aws.String("2024-01-01T00:00:00Z"), Name: aws.String("old")},
						{ImageId: aws.String("ami-newest"), CreationDate: aws.String("2024-06-15T00:00:00Z"), Name: aws.String("newest")},
						{ImageId: aws.String("ami-mid"), CreationDate: aws.String("2024-03-01T00:00:00Z"), Name: aws.String("mid")},
					},
				}, nil
			},
		}
		p := newTestProvider(ec2Mock, nil, nil, nil)
		amiID, err := p.LookupUbuntuAMI(context.Background())
		require.NoError(t, err)
		assert.Equal(t, "ami-newest", amiID)
	})

	t.Run("filters for canonical owner and arm64", func(t *testing.T) {
		t.Parallel()
		ec2Mock := &mockEC2{
			describeImagesFn: func(ctx context.Context, params *ec2.DescribeImagesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error) {
				assert.Equal(t, []string{"099720109477"}, params.Owners)
				foundArch := false
				for _, f := range params.Filters {
					if aws.ToString(f.Name) == "architecture" {
						assert.Equal(t, []string{"arm64"}, f.Values)
						foundArch = true
					}
				}
				assert.True(t, foundArch)
				return &ec2.DescribeImagesOutput{
					Images: []ec2types.Image{
						{ImageId: aws.String("ami-test"), CreationDate: aws.String("2024-01-01T00:00:00Z"), Name: aws.String("test")},
					},
				}, nil
			},
		}
		p := newTestProvider(ec2Mock, nil, nil, nil)
		_, err := p.LookupUbuntuAMI(context.Background())
		require.NoError(t, err)
	})

	t.Run("returns error when no AMIs found", func(t *testing.T) {
		t.Parallel()
		ec2Mock := &mockEC2{
			describeImagesFn: func(ctx context.Context, params *ec2.DescribeImagesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error) {
				return &ec2.DescribeImagesOutput{Images: nil}, nil
			},
		}
		p := newTestProvider(ec2Mock, nil, nil, nil)
		_, err := p.LookupUbuntuAMI(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no Ubuntu 24.04 arm64 AMI found")
	})
}

func TestIsExpiredTokenError(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error", nil, false},
		{"ExpiredToken", fmt.Errorf("ExpiredToken: ..."), true},
		{"ExpiredTokenException", fmt.Errorf("ExpiredTokenException: token has expired"), true},
		{"RequestExpired", fmt.Errorf("RequestExpired: request expired"), true},
		{"unrelated error", fmt.Errorf("connection timeout"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, IsExpiredTokenError(tt.err))
		})
	}
}

func TestRegion(t *testing.T) {
	t.Parallel()
	p := NewAWSProviderFromClients(nil, nil, nil, nil, "ap-southeast-2")
	assert.Equal(t, "ap-southeast-2", p.Region())
}
