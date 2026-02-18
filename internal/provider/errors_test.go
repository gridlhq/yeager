package provider

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClassifyAWSError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		err         error
		wantNil     bool
		wantMessage string
		wantFix     string
	}{
		{
			name:    "nil error",
			err:     nil,
			wantNil: true,
		},
		{
			name:    "unrecognized error passes through",
			err:     fmt.Errorf("something unexpected"),
			wantNil: true,
		},
		{
			name:        "expired token",
			err:         fmt.Errorf("operation error STS: GetCallerIdentity, ExpiredToken: token has expired"),
			wantMessage: "AWS credentials have expired",
			wantFix:     "aws sso login",
		},
		{
			name:        "expired token exception variant",
			err:         fmt.Errorf("ExpiredTokenException: token is expired"),
			wantMessage: "AWS credentials have expired",
			wantFix:     "aws sso login",
		},
		{
			name:        "no credential providers",
			err:         fmt.Errorf("NoCredentialProviders: no valid providers in chain"),
			wantMessage: "no AWS credentials found",
			wantFix:     "yg configure",
		},
		{
			name:        "no IMDS role",
			err:         fmt.Errorf("no EC2 IMDS role found"),
			wantMessage: "no AWS credentials found",
			wantFix:     "yg configure",
		},
		{
			name:        "invalid client token",
			err:         fmt.Errorf("InvalidClientTokenId: The security token included in the request is invalid"),
			wantMessage: "AWS credentials are invalid",
			wantFix:     "yg configure",
		},
		{
			name:        "signature mismatch",
			err:         fmt.Errorf("SignatureDoesNotMatch: the request signature"),
			wantMessage: "AWS credentials are invalid",
			wantFix:     "yg configure",
		},
		{
			name:        "auth failure",
			err:         fmt.Errorf("AuthFailure: not authorized"),
			wantMessage: "AWS credentials are invalid",
			wantFix:     "yg configure",
		},
		{
			name:        "access denied",
			err:         fmt.Errorf("AccessDenied: User is not authorized to perform ec2:RunInstances"),
			wantMessage: "AWS permissions denied",
			wantFix:     "EC2, S3, STS, and EC2 Instance Connect permissions",
		},
		{
			name:        "unauthorized operation",
			err:         fmt.Errorf("UnauthorizedOperation: You are not authorized"),
			wantMessage: "AWS permissions denied",
			wantFix:     "EC2, S3, STS, and EC2 Instance Connect permissions",
		},
		{
			name:        "insufficient capacity",
			err:         fmt.Errorf("InsufficientInstanceCapacity: no capacity"),
			wantMessage: "AWS capacity limit reached",
			wantFix:     "different region",
		},
		{
			name:        "instance limit exceeded",
			err:         fmt.Errorf("InstanceLimitExceeded: too many instances"),
			wantMessage: "AWS capacity limit reached",
			wantFix:     "different region",
		},
		{
			name:        "vCPU limit exceeded",
			err:         fmt.Errorf("VcpuLimitExceeded: vCPU limit"),
			wantMessage: "AWS account limit reached",
			wantFix:     "limit increase",
		},
		{
			name:        "VPC limit exceeded",
			err:         fmt.Errorf("VpcLimitExceeded: too many VPCs"),
			wantMessage: "AWS account limit reached",
			wantFix:     "limit increase",
		},
		{
			name:        "connection refused",
			err:         fmt.Errorf("RequestError: send request failed: connection refused"),
			wantMessage: "cannot reach AWS",
			wantFix:     "internet connection",
		},
		{
			name:        "no such host",
			err:         fmt.Errorf("no such host"),
			wantMessage: "cannot reach AWS",
			wantFix:     "internet connection",
		},
		{
			name:        "i/o timeout",
			err:         fmt.Errorf("i/o timeout"),
			wantMessage: "cannot reach AWS",
			wantFix:     "internet connection",
		},
		{
			name:        "throttling - RequestLimitExceeded",
			err:         fmt.Errorf("RequestLimitExceeded: Rate exceeded"),
			wantMessage: "request rate limit",
			wantFix:     "wait a moment",
		},
		{
			name:        "throttling - Throttling",
			err:         fmt.Errorf("Throttling: Rate exceeded"),
			wantMessage: "request rate limit",
			wantFix:     "wait a moment",
		},
		{
			name:        "AMI not found",
			err:         fmt.Errorf("InvalidAMIID.NotFound: The image id 'ami-xxx' does not exist"),
			wantMessage: "AMI not found",
			wantFix:     "different region",
		},
		{
			name:        "invalid subnet",
			err:         fmt.Errorf("InvalidSubnetID.NotFound: subnet-xxxx does not exist"),
			wantMessage: "VPC or subnet",
			wantFix:     "default VPC",
		},
		{
			name:        "opt in required",
			err:         fmt.Errorf("OptInRequired: region is not enabled"),
			wantMessage: "region not enabled",
			wantFix:     "enable the region",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ClassifyAWSError(tt.err)
			if tt.wantNil {
				assert.Nil(t, got)
				return
			}
			require.NotNil(t, got, "expected ClassifiedError, got nil")
			assert.Contains(t, got.Message, tt.wantMessage)
			assert.Contains(t, got.Fix, tt.wantFix)
			assert.Equal(t, tt.err, got.Unwrap(), "Unwrap should return original error")
		})
	}
}

func TestClassifiedError_Error(t *testing.T) {
	t.Parallel()

	ce := &ClassifiedError{
		Message: "test message",
		Fix:     "try this",
		Cause:   fmt.Errorf("original"),
	}
	assert.Equal(t, "test message", ce.Error())
}
