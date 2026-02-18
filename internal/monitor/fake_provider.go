package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gridlhq/yeager/internal/provider"
)

// FakeProviderState tracks operations performed by the fake provider.
// It's written to a file so it can be shared across processes.
type FakeProviderState struct {
	StopCalled bool      `json:"stop_called"`
	StopTime   time.Time `json:"stop_time"`
	StopError  string    `json:"stop_error,omitempty"`
}

// fakeProvider is a file-based provider for testing that persists state to disk.
type fakeProvider struct {
	stateFile string
}

func newFakeProvider(stateDir string) *fakeProvider {
	return &fakeProvider{
		stateFile: filepath.Join(stateDir, "fake_provider_state.json"),
	}
}

func (f *fakeProvider) StopVM(ctx context.Context, instanceID string) error {
	state := FakeProviderState{
		StopCalled: true,
		StopTime:   time.Now(),
	}

	data, err := json.Marshal(state)
	if err != nil {
		return err
	}

	return os.WriteFile(f.stateFile, data, 0o644)
}

func (f *fakeProvider) GetState() (*FakeProviderState, error) {
	data, err := os.ReadFile(f.stateFile)
	if os.IsNotExist(err) {
		return &FakeProviderState{}, nil
	}
	if err != nil {
		return nil, err
	}

	var state FakeProviderState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("unmarshaling fake provider state: %w", err)
	}

	return &state, nil
}

// Implement remaining CloudProvider interface methods as no-ops for testing.
func (f *fakeProvider) AccountID(ctx context.Context) (string, error) {
	return "123456789012", nil
}

func (f *fakeProvider) EnsureSecurityGroup(ctx context.Context) (string, error) {
	return "sg-fake", nil
}

func (f *fakeProvider) EnsureBucket(ctx context.Context) error {
	return nil
}

func (f *fakeProvider) CreateVM(ctx context.Context, opts provider.CreateVMOpts) (provider.VMInfo, error) {
	return provider.VMInfo{}, fmt.Errorf("not implemented")
}

func (f *fakeProvider) FindVM(ctx context.Context, projectHash string) (*provider.VMInfo, error) {
	return nil, nil
}

func (f *fakeProvider) StartVM(ctx context.Context, instanceID string) error {
	return nil
}

func (f *fakeProvider) TerminateVM(ctx context.Context, instanceID string) error {
	return nil
}

func (f *fakeProvider) WaitUntilRunning(ctx context.Context, instanceID string) error {
	return nil
}

func (f *fakeProvider) WaitUntilRunningWithProgress(ctx context.Context, instanceID string, progressCallback provider.ProgressCallback) error {
	return nil
}

func (f *fakeProvider) Region() string {
	return "us-east-1"
}

func (f *fakeProvider) BucketName(ctx context.Context) (string, error) {
	return "yeager-test-bucket", nil
}
