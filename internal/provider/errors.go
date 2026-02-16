package provider

import (
	"strings"
)

// ClassifiedError wraps an AWS error with user-facing context.
type ClassifiedError struct {
	Message string // user-facing description
	Fix     string // actionable fix instruction
	Cause   error  // original error
}

func (e *ClassifiedError) Error() string {
	return e.Message
}

func (e *ClassifiedError) Unwrap() error {
	return e.Cause
}

// ClassifyAWSError examines an error from the AWS SDK and returns a
// ClassifiedError with actionable guidance, or nil if the error is not
// recognized and should be returned as-is.
func ClassifyAWSError(err error) *ClassifiedError {
	if err == nil {
		return nil
	}
	msg := err.Error()

	// Credential errors.
	if IsExpiredTokenError(err) {
		return &ClassifiedError{
			Message: "AWS credentials have expired",
			Fix:     "run: aws sso login (or refresh your credentials)",
			Cause:   err,
		}
	}
	if containsAny(msg, "NoCredentialProviders", "no EC2 IMDS role found") {
		return &ClassifiedError{
			Message: "no AWS credentials found",
			Fix:     "run: aws configure (or set AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY)",
			Cause:   err,
		}
	}
	if containsAny(msg, "InvalidClientTokenId", "SignatureDoesNotMatch", "AuthFailure") {
		return &ClassifiedError{
			Message: "AWS credentials are invalid",
			Fix:     "check your AWS credentials: aws sts get-caller-identity",
			Cause:   err,
		}
	}
	if containsAny(msg, "AccessDenied", "UnauthorizedOperation", "AccessDeniedException") {
		return &ClassifiedError{
			Message: "AWS permissions denied",
			Fix:     "your IAM user/role needs EC2, S3, STS, and EC2 Instance Connect permissions",
			Cause:   err,
		}
	}

	// Capacity errors.
	if containsAny(msg, "InsufficientInstanceCapacity", "InstanceLimitExceeded") {
		return &ClassifiedError{
			Message: "AWS capacity limit reached",
			Fix:     "try a different region (YEAGER_COMPUTE_REGION) or instance size (compute.size in .yeager.toml)",
			Cause:   err,
		}
	}
	if containsAny(msg, "VcpuLimitExceeded", "VpcLimitExceeded") {
		return &ClassifiedError{
			Message: "AWS account limit reached",
			Fix:     "request a limit increase in the AWS console, or terminate unused instances",
			Cause:   err,
		}
	}

	// Network/connectivity errors.
	if containsAny(msg, "RequestError", "connection refused", "no such host", "i/o timeout") {
		return &ClassifiedError{
			Message: "cannot reach AWS",
			Fix:     "check your internet connection and proxy settings",
			Cause:   err,
		}
	}

	return nil
}

func containsAny(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
