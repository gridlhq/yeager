package cli

import (
	"context"
	"log/slog"
	"time"

	"github.com/gridlhq/yeager/internal/provider"
)

const defaultPollInterval = 30 * time.Second

// IdleMonitor watches for idle VMs and stops them after the configured timeout.
type IdleMonitor struct {
	idleTimeout  time.Duration
	pollInterval time.Duration
	instanceID   string
	provider     provider.CloudProvider
	connectSSH   SSHClientFactory
	listRuns     ListRunsFunc
	vmInfo       *provider.VMInfo

	// For testing: override the clock.
	now func() time.Time
}

// IdleMonitorOpts configures the idle monitor.
type IdleMonitorOpts struct {
	IdleTimeout  time.Duration
	PollInterval time.Duration // 0 = use default (30s)
	InstanceID   string
	Provider     provider.CloudProvider
	ConnectSSH   SSHClientFactory
	ListRuns     ListRunsFunc
	VMInfo       *provider.VMInfo
}

// NewIdleMonitor creates an idle monitor. Returns nil if timeout is zero (disabled).
func NewIdleMonitor(opts IdleMonitorOpts) *IdleMonitor {
	if opts.IdleTimeout <= 0 {
		return nil
	}
	poll := opts.PollInterval
	if poll <= 0 {
		poll = defaultPollInterval
	}
	return &IdleMonitor{
		idleTimeout:  opts.IdleTimeout,
		pollInterval: poll,
		instanceID:   opts.InstanceID,
		provider:     opts.Provider,
		connectSSH:   opts.ConnectSSH,
		listRuns:     opts.ListRuns,
		vmInfo:       opts.VMInfo,
		now:          time.Now,
	}
}

// Start runs the idle monitor in a background goroutine.
// Returns a channel that is closed when the monitor exits (VM stopped or ctx cancelled).
// The caller should select on the returned channel to keep the process alive.
func (m *IdleMonitor) Start(ctx context.Context) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		m.run(ctx)
	}()
	return done
}

// CheckAndStop does a single check: if no active runs, stops the VM immediately.
// This is used at process exit when the idle monitor can't run as a long-lived goroutine.
// Returns true if the VM was stopped.
func (m *IdleMonitor) CheckAndStop(ctx context.Context) bool {
	hasActive, err := m.checkActiveRuns(ctx)
	if err != nil {
		slog.Debug("idle check: SSH failed, skipping stop", "error", err)
		return false
	}
	if hasActive {
		slog.Debug("idle check: active runs found, VM stays running")
		return false
	}
	slog.Info("idle check: no active runs, stopping VM", "instance", m.instanceID)
	if err := m.provider.StopVM(ctx, m.instanceID); err != nil {
		slog.Debug("idle check: stop failed", "error", err)
		return false
	}
	return true
}

// run is the main loop of the idle monitor.
func (m *IdleMonitor) run(ctx context.Context) {
	idleSince := m.now()

	ticker := time.NewTicker(m.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			hasActive, err := m.checkActiveRuns(ctx)
			if err != nil {
				slog.Debug("idle monitor: check failed", "error", err)
				// On SSH error, assume runs may still be active — reset idle timer.
				// Without a successful check, we can't confirm the VM is idle.
				idleSince = m.now()
				continue
			}

			now := m.now()
			if hasActive {
				idleSince = now
				continue
			}

			// No active runs. Check if we've been idle long enough.
			if now.Sub(idleSince) >= m.idleTimeout {
				slog.Info("idle monitor: stopping VM", "instance", m.instanceID,
					"idle_for", now.Sub(idleSince).Truncate(time.Second))
				if err := m.provider.StopVM(ctx, m.instanceID); err != nil {
					slog.Debug("idle monitor: stop failed", "error", err)
				}
				return
			}
		}
	}
}

// checkActiveRuns connects via SSH and checks for active marker files.
func (m *IdleMonitor) checkActiveRuns(ctx context.Context) (bool, error) {
	client, err := m.connectSSH(ctx, m.vmInfo)
	if err != nil {
		return false, err
	}
	if client != nil {
		defer client.Close()
	}

	runs, err := m.listRuns(client)
	if err != nil {
		return false, err
	}

	return len(runs) > 0, nil
}
