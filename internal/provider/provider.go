package provider

import "context"

// VMInfo represents a cloud VM's current state.
type VMInfo struct {
	InstanceID       string
	State            string // "running", "stopped", "terminated", "pending", "stopping", "shutting-down"
	PublicIP         string
	Region           string
	AvailabilityZone string
}

// CloudProvider defines the interface for cloud infrastructure operations.
// Implementations must be safe for concurrent use.
type CloudProvider interface {
	// AccountID returns the authenticated AWS account ID.
	AccountID(ctx context.Context) (string, error)

	// EnsureSecurityGroup creates the yeager security group if it doesn't exist.
	// Returns the security group ID. Idempotent.
	EnsureSecurityGroup(ctx context.Context) (string, error)

	// EnsureBucket creates the yeager S3 bucket if it doesn't exist.
	// Applies lifecycle policy (30-day expiration). Idempotent.
	EnsureBucket(ctx context.Context) error

	// CreateVM launches a new EC2 instance for the given project.
	CreateVM(ctx context.Context, opts CreateVMOpts) (VMInfo, error)

	// FindVM looks up the VM for a project by its hash tag.
	// Returns nil if no VM exists (not an error).
	FindVM(ctx context.Context, projectHash string) (*VMInfo, error)

	// StartVM starts a stopped instance.
	StartVM(ctx context.Context, instanceID string) error

	// StopVM stops a running instance.
	StopVM(ctx context.Context, instanceID string) error

	// TerminateVM terminates an instance.
	TerminateVM(ctx context.Context, instanceID string) error

	// WaitUntilRunning blocks until the instance is in "running" state.
	WaitUntilRunning(ctx context.Context, instanceID string) error

	// WaitUntilRunningWithProgress blocks until the instance is in "running" state,
	// calling progressCallback periodically to report elapsed time.
	WaitUntilRunningWithProgress(ctx context.Context, instanceID string, progressCallback ProgressCallback) error

	// Region returns the configured AWS region.
	Region() string

	// BucketName returns the yeager S3 bucket name.
	BucketName(ctx context.Context) (string, error)
}

// CreateVMOpts contains the parameters for creating a new VM.
type CreateVMOpts struct {
	ProjectHash     string
	ProjectPath     string
	Size            string // "small", "medium", "large", "xlarge"
	SecurityGroupID string
	UserData        string // base64-encoded cloud-init document (optional)
}
