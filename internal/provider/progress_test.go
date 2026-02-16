package provider

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWaitUntilRunningWithProgress verifies that WaitUntilRunningWithProgress
// calls the progress callback during the wait period.
func TestWaitUntilRunningWithProgress(t *testing.T) {
	t.Parallel()

	t.Run("reports progress during wait", func(t *testing.T) {
		t.Parallel()

		// Mock waiter that takes 25ms to complete
		waiter := &mockWaiter{
			waitFn: func(ctx context.Context, params *ec2.DescribeInstancesInput, maxWaitDur time.Duration, optFns ...func(*ec2.InstanceRunningWaiterOptions)) error {
				time.Sleep(25 * time.Millisecond)
				return nil
			},
		}

		p := newTestProvider(nil, nil, stsWithAccount("123"), waiter)

		var progressCalls int
		progressCallback := func(elapsed time.Duration) {
			progressCalls++
		}

		// Use a shorter interval for testing (10ms instead of 10s)
		err := p.WaitUntilRunningWithProgressInterval(context.Background(), "i-12345", progressCallback, 10*time.Millisecond)
		require.NoError(t, err)

		// Should have called progress callback at least once (25ms wait / 10ms interval = 2+ calls)
		assert.Greater(t, progressCalls, 0, "progress callback should be called during wait")
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		t.Parallel()

		waiter := &mockWaiter{
			waitFn: func(ctx context.Context, params *ec2.DescribeInstancesInput, maxWaitDur time.Duration, optFns ...func(*ec2.InstanceRunningWaiterOptions)) error {
				<-ctx.Done()
				return ctx.Err()
			},
		}

		p := newTestProvider(nil, nil, stsWithAccount("123"), waiter)

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		err := p.WaitUntilRunningWithProgress(ctx, "i-12345", nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "context canceled")
	})

	t.Run("works without progress callback", func(t *testing.T) {
		t.Parallel()

		waiter := &mockWaiter{
			waitFn: func(ctx context.Context, params *ec2.DescribeInstancesInput, maxWaitDur time.Duration, optFns ...func(*ec2.InstanceRunningWaiterOptions)) error {
				return nil
			},
		}

		p := newTestProvider(nil, nil, stsWithAccount("123"), waiter)

		// Should not panic when callback is nil
		err := p.WaitUntilRunningWithProgress(context.Background(), "i-12345", nil)
		require.NoError(t, err)
	})
}

// TestProgressMessage tests the progress message formatting.
func TestProgressMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		elapsed time.Duration
		want    string
	}{
		{"5 seconds", 5 * time.Second, "waiting for VM to be ready (5s elapsed)..."},
		{"30 seconds", 30 * time.Second, "waiting for VM to be ready (30s elapsed)..."},
		{"90 seconds", 90 * time.Second, "waiting for VM to be ready (1m 30s elapsed)..."},
		{"2 minutes", 2 * time.Minute, "waiting for VM to be ready (2m 0s elapsed)..."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := FormatProgressMessage(tt.elapsed)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestFormatDurationForProgress tests duration formatting for progress messages.
func TestFormatDurationForProgress(t *testing.T) {
	t.Parallel()

	tests := []struct {
		duration time.Duration
		want     string
	}{
		{0, "0s"},
		{5 * time.Second, "5s"},
		{30 * time.Second, "30s"},
		{60 * time.Second, "1m 0s"},
		{90 * time.Second, "1m 30s"},
		{2 * time.Minute, "2m 0s"},
		{125 * time.Second, "2m 5s"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			t.Parallel()
			got := FormatDurationForProgress(tt.duration)
			assert.Equal(t, tt.want, got)
		})
	}
}
