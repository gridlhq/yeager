package cli

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/gridlhq/yeager/internal/config"
	fkexec "github.com/gridlhq/yeager/internal/exec"
	"github.com/gridlhq/yeager/internal/output"
	"github.com/gridlhq/yeager/internal/preflight"
	"github.com/gridlhq/yeager/internal/project"
	"github.com/gridlhq/yeager/internal/provider"
	fkssh "github.com/gridlhq/yeager/internal/ssh"
	"github.com/gridlhq/yeager/internal/state"
	fkstorage "github.com/gridlhq/yeager/internal/storage"
	fksync "github.com/gridlhq/yeager/internal/sync"
	gossh "golang.org/x/crypto/ssh"
)

// SSHConnectorFactory creates an SSH connector for a given VM.
type SSHConnectorFactory func(ctx context.Context, region, az string) (*fkssh.Connector, error)

// SSHClientFactory creates an SSH client connection to the VM.
// Combines connector creation + ConnectWithFallback into one step for testability.
type SSHClientFactory func(ctx context.Context, vmInfo *provider.VMInfo) (*gossh.Client, error)

// StorageFactory creates a storage store for a given bucket.
type StorageFactory func(ctx context.Context) (*fkstorage.Store, error)

// SyncFunc runs rsync to sync project files to a VM and returns transfer stats.
type SyncFunc func(ctx context.Context, cc *cmdContext, vmInfo *provider.VMInfo) (*fksync.SyncResult, error)

// ExecFunc runs a command on the remote VM via an SSH client.
type ExecFunc func(client *gossh.Client, opts fkexec.RunOpts, stdout, stderr io.Writer) (*fkexec.RunResult, error)

// ListRunsFunc queries active runs on the VM via SSH.
type ListRunsFunc func(client *gossh.Client) ([]fkexec.ActiveRun, error)

// IsRunActiveFunc checks if a tmux session exists for a run.
type IsRunActiveFunc func(client *gossh.Client, runID fkexec.RunID) (bool, error)

// TailLogFunc streams the tmux log for an active run.
type TailLogFunc func(client *gossh.Client, runID fkexec.RunID, stdout io.Writer) error

// ReadRemoteFileFunc reads a file from the VM over SSH.
type ReadRemoteFileFunc func(client *gossh.Client, remotePath string) ([]byte, error)

// cmdContext holds the resolved context for a CLI command.
// Created once per command invocation, not shared between commands.
type cmdContext struct {
	Project  project.Project
	Config   config.Config
	Provider provider.CloudProvider
	State    *state.Store
	Output   *output.Writer

	// Factories for execution pipeline (set in resolveCmdContext, overridable in tests).
	NewSSHConnector SSHConnectorFactory
	ConnectSSH      SSHClientFactory
	NewStorage      StorageFactory
	RunSync         SyncFunc
	RunExec         ExecFunc
	ListRuns        ListRunsFunc
	IsRunActive     IsRunActiveFunc
	TailLog         TailLogFunc
	ReadRemoteFile  ReadRemoteFileFunc
}

// resolveCmdContext builds the full context needed by VM-interacting commands.
func resolveCmdContext(ctx context.Context, mode output.Mode) (*cmdContext, error) {
	w := output.New(mode)

	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("getting working directory: %w", err)
	}

	proj, err := project.Resolve(cwd)
	if err != nil {
		return nil, fmt.Errorf("resolving project: %w", err)
	}

	cfg, _, err := config.Load(cwd)
	if err != nil {
		w.Error("invalid configuration", "check .yeager.toml syntax, or delete it and run: yg init")
		return nil, err
	}

	// Preflight checks — detect missing prerequisites with actionable errors.
	homeDir, _ := os.UserHomeDir()
	if failures := preflight.RunAll(os.LookupEnv, fileExists, homeDir); len(failures) > 0 {
		for _, f := range failures {
			w.Error(f.Message, f.Fix)
		}
		return nil, fmt.Errorf("preflight checks failed")
	}

	prov, err := provider.NewAWSProvider(ctx, cfg.Compute.Region)
	if err != nil {
		printClassifiedError(w, err)
		return nil, err
	}

	store, err := state.NewStore("")
	if err != nil {
		return nil, fmt.Errorf("initializing state store: %w", err)
	}

	cc := &cmdContext{
		Project:  proj,
		Config:   cfg,
		Provider: prov,
		State:    store,
		Output:   w,
	}

	// Set default factories that create real AWS-backed clients.
	cc.NewSSHConnector = defaultSSHConnectorFactory(prov)
	cc.NewStorage = defaultStorageFactory(prov)
	cc.RunSync = defaultSyncFunc
	cc.RunExec = fkexec.Run
	cc.ListRuns = fkexec.ListRuns
	cc.IsRunActive = fkexec.IsRunActive
	cc.TailLog = fkexec.TailLog
	cc.ReadRemoteFile = fkexec.ReadRemoteFile
	cc.ConnectSSH = defaultConnectSSH(cc)

	return cc, nil
}

// defaultSSHConnectorFactory creates an SSH connector using EC2 Instance Connect.
func defaultSSHConnectorFactory(prov *provider.AWSProvider) SSHConnectorFactory {
	return func(ctx context.Context, region, az string) (*fkssh.Connector, error) {
		ic, err := provider.NewEC2InstanceConnectClient(ctx, region)
		if err != nil {
			return nil, fmt.Errorf("creating EC2 Instance Connect client: %w", err)
		}
		return fkssh.NewConnector(ic, region, az), nil
	}
}

// defaultConnectSSH creates an SSH client via EC2 Instance Connect.
func defaultConnectSSH(cc *cmdContext) SSHClientFactory {
	return func(ctx context.Context, vmInfo *provider.VMInfo) (*gossh.Client, error) {
		connector, err := cc.NewSSHConnector(ctx, vmInfo.Region, vmInfo.AvailabilityZone)
		if err != nil {
			return nil, err
		}
		return connector.ConnectWithFallback(ctx, vmInfo.InstanceID, vmInfo.PublicIP)
	}
}

// fileExists returns true if a file exists at the given path.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// printClassifiedError checks if an error is a recognized AWS error and prints
// an actionable error message. Returns true if a classified message was printed.
func printClassifiedError(w *output.Writer, err error) bool {
	if ce := provider.ClassifyAWSError(err); ce != nil {
		w.Error(ce.Message, ce.Fix)
		return true
	}
	return false
}

// defaultStorageFactory creates a storage store using the provider's S3 client.
func defaultStorageFactory(prov *provider.AWSProvider) StorageFactory {
	return func(ctx context.Context) (*fkstorage.Store, error) {
		bucketName, err := prov.BucketName(ctx)
		if err != nil {
			return nil, err
		}
		s3Client, err := provider.NewS3ObjectClient(ctx, prov.Region())
		if err != nil {
			return nil, fmt.Errorf("creating S3 client: %w", err)
		}
		return fkstorage.NewStore(s3Client, bucketName), nil
	}
}
