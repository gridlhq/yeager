//go:build integration

package provider

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Integration tests require real AWS credentials.
// Run with: go test -tags integration -v ./internal/provider/
//
// These tests create real AWS resources. They clean up after themselves,
// but interrupted runs may leave resources behind.
//
// Required environment:
//   - AWS credentials (env vars, ~/.aws/credentials, or IAM role)
//   - Default VPC in the target region

func integrationProvider(t *testing.T) *AWSProvider {
	t.Helper()

	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "us-east-1"
	}

	p, err := NewAWSProvider(context.Background(), region)
	require.NoError(t, err)

	// Verify credentials work.
	acct, err := p.AccountID(context.Background())
	require.NoError(t, err)
	require.NotEmpty(t, acct)
	t.Logf("AWS account: %s, region: %s", acct, p.Region())

	return p
}

func TestIntegrationSecurityGroup(t *testing.T) {
	p := integrationProvider(t)
	ctx := context.Background()

	// First call — create or find existing.
	sgID1, err := p.EnsureSecurityGroup(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, sgID1)
	t.Logf("security group: %s", sgID1)

	// Second call — idempotent, should return same ID.
	sgID2, err := p.EnsureSecurityGroup(ctx)
	require.NoError(t, err)
	assert.Equal(t, sgID1, sgID2, "EnsureSecurityGroup should be idempotent")
}

func TestIntegrationS3Bucket(t *testing.T) {
	p := integrationProvider(t)
	ctx := context.Background()

	// First call — create or find existing.
	err := p.EnsureBucket(ctx)
	require.NoError(t, err)

	bucket, err := p.BucketName(ctx)
	require.NoError(t, err)
	t.Logf("bucket: %s", bucket)

	// Second call — idempotent.
	err = p.EnsureBucket(ctx)
	require.NoError(t, err)
}

func TestIntegrationEC2Lifecycle(t *testing.T) {
	p := integrationProvider(t)
	ctx := context.Background()

	// Ensure security group exists.
	sgID, err := p.EnsureSecurityGroup(ctx)
	require.NoError(t, err)

	projectHash := "integration-test-" + time.Now().Format("20060102150405")

	// Create instance.
	info, err := p.CreateVM(ctx, CreateVMOpts{
		ProjectHash:     projectHash,
		ProjectPath:     "/tmp/yeager-integration-test",
		Size:            "small",
		SecurityGroupID: sgID,
	})
	require.NoError(t, err)
	require.NotEmpty(t, info.InstanceID)
	t.Logf("created instance: %s", info.InstanceID)

	// Ensure cleanup.
	defer func() {
		t.Logf("terminating instance: %s", info.InstanceID)
		err := p.TerminateVM(ctx, info.InstanceID)
		if err != nil {
			t.Logf("cleanup failed: %v", err)
		}
	}()

	// Wait for running.
	err = p.WaitUntilRunning(ctx, info.InstanceID)
	require.NoError(t, err)
	t.Log("instance is running")

	// Find by project hash.
	found, err := p.FindVM(ctx, projectHash)
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, info.InstanceID, found.InstanceID)
	assert.Equal(t, "running", found.State)
	t.Logf("found instance: %s (state: %s, ip: %s)", found.InstanceID, found.State, found.PublicIP)

	// Terminate.
	err = p.TerminateVM(ctx, info.InstanceID)
	require.NoError(t, err)
	t.Log("instance terminated")

	// After termination, FindVM should not find it (filtered out).
	// Give AWS a moment to register the state change.
	time.Sleep(5 * time.Second)
	found, err = p.FindVM(ctx, projectHash)
	require.NoError(t, err)
	// May or may not be nil depending on how fast AWS processes the termination.
	if found != nil {
		t.Logf("instance still visible in state: %s (this is normal — AWS is processing)", found.State)
	}
}
