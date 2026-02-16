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
			wantFix:     "aws configure",
		},
		{
			name:        "no IMDS role",
			err:         fmt.Errorf("no EC2 IMDS role found"),
			wantMessage: "no AWS credentials found",
			wantFix:     "aws configure",
		},
		{
			name:        "invalid client token",
			err:         fmt.Errorf("InvalidClientTokenId: The security token included in the request is invalid"),
			wantMessage: "AWS credentials are invalid",
			wantFix:     "aws sts get-caller-identity",
		},
		{
			name:        "signature mismatch",
			err:         fmt.Errorf("SignatureDoesNotMatch: the request signature"),
			wantMessage: "AWS credentials are invalid",
			wantFix:     "aws sts get-caller-identity",
		},
		{
			name:        "auth failure",
			err:         fmt.Errorf("AuthFailure: not authorized"),
			wantMessage: "AWS credentials are invalid",
			wantFix:     "aws sts get-caller-identity",
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
