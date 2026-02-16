package provider

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
)

// ProgressCallback is called periodically during WaitUntilRunning to report progress.
// elapsed is the time since the wait started.
type ProgressCallback func(elapsed time.Duration)

// WaitUntilRunningWithProgress blocks until the instance is in "running" state,
// calling progressCallback periodically (every 10s) to report elapsed time.
func (p *AWSProvider) WaitUntilRunningWithProgress(ctx context.Context, instanceID string, progressCallback ProgressCallback) error {
	return p.WaitUntilRunningWithProgressInterval(ctx, instanceID, progressCallback, 10*time.Second)
}

// WaitUntilRunningWithProgressInterval is like WaitUntilRunningWithProgress but allows
// customizing the progress callback interval (useful for testing).
func (p *AWSProvider) WaitUntilRunningWithProgressInterval(ctx context.Context, instanceID string, progressCallback ProgressCallback, interval time.Duration) error {
	start := time.Now()
	done := make(chan error, 1)

	// Start the waiter in a goroutine
	go func() {
		err := p.waiter.Wait(ctx, instanceInputForID(instanceID), waitTimeout)
		if err != nil {
			done <- fmt.Errorf("waiting for instance %s to be running: %w", instanceID, err)
		} else {
			done <- nil
		}
	}()

	// Progress ticker
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case err := <-done:
			return err
		case <-ticker.C:
			if progressCallback != nil {
				progressCallback(time.Since(start))
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// FormatProgressMessage formats a progress message for the VM wait.
func FormatProgressMessage(elapsed time.Duration) string {
	return fmt.Sprintf("waiting for VM to be ready (%s elapsed)...", FormatDurationForProgress(elapsed))
}

// FormatDurationForProgress formats a duration for progress messages.
func FormatDurationForProgress(d time.Duration) string {
	d = d.Truncate(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	minutes := int(d.Minutes())
	seconds := int(d.Seconds()) - minutes*60
	return fmt.Sprintf("%dm %ds", minutes, seconds)
}

// instanceInputForID creates a DescribeInstancesInput for a single instance ID.
func instanceInputForID(instanceID string) *ec2.DescribeInstancesInput {
	return &ec2.DescribeInstancesInput{
		InstanceIds: []string{instanceID},
	}
}

// WaitUntilRunning is the standard version without progress callback (for backward compatibility).
func (p *AWSProvider) WaitUntilRunning(ctx context.Context, instanceID string) error {
	return p.WaitUntilRunningWithProgress(ctx, instanceID, nil)
}
