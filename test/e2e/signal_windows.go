//go:build e2e && windows

package e2e

import (
	"os"
)

// signalInterrupt returns os.Interrupt on Windows (SIGINT equivalent).
func signalInterrupt() os.Signal {
	return os.Interrupt
}
