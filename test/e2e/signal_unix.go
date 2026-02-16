//go:build e2e && !windows

package e2e

import (
	"os"
	"syscall"
)

// signalInterrupt returns SIGINT for Unix systems.
func signalInterrupt() os.Signal {
	return syscall.SIGINT
}
