package preflight

import (
	"fmt"
	"os/exec"
	"runtime"
)

// Result is the outcome of a single preflight check.
type Result struct {
	Name    string // e.g. "rsync"
	OK      bool
	Message string // user-facing error description
	Fix     string // actionable fix instruction
}

// CheckRsync verifies that rsync is installed and available on PATH.
func CheckRsync() Result {
	r := Result{Name: "rsync"}
	_, err := exec.LookPath("rsync")
	if err != nil {
		r.OK = false
		r.Message = "rsync is not installed"
		switch runtime.GOOS {
		case "darwin":
			r.Fix = "install: brew install rsync"
		case "linux":
			r.Fix = "install: sudo apt install rsync (or your distro's package manager)"
		default:
			r.Fix = "install rsync and ensure it is on your PATH"
		}
		return r
	}
	r.OK = true
	return r
}

// CheckAWSCredentials verifies that AWS credentials are configured.
// It checks for the environment variables and config files that the AWS SDK resolves.
// This is a fast, offline check — it does not call AWS APIs.
func CheckAWSCredentials(lookupEnv func(string) (string, bool), fileExists func(string) bool, homeDir string) Result {
	r := Result{Name: "aws-credentials"}

	// Check environment variables first (highest priority in SDK resolution).
	if key, ok := lookupEnv("AWS_ACCESS_KEY_ID"); ok && key != "" {
		r.OK = true
		return r
	}
	if profile, ok := lookupEnv("AWS_PROFILE"); ok && profile != "" {
		r.OK = true
		return r
	}
	if webIdentity, ok := lookupEnv("AWS_WEB_IDENTITY_TOKEN_FILE"); ok && webIdentity != "" {
		r.OK = true
		return r
	}

	// Check for credential files.
	if homeDir != "" {
		credPath := fmt.Sprintf("%s/.aws/credentials", homeDir)
		configPath := fmt.Sprintf("%s/.aws/config", homeDir)
		if fileExists(credPath) || fileExists(configPath) {
			r.OK = true
			return r
		}
	}

	r.OK = false
	r.Message = "no AWS credentials found"
	r.Fix = "run: aws configure (or set AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY environment variables)"
	return r
}

// RunAll runs all preflight checks and returns any failures.
func RunAll(lookupEnv func(string) (string, bool), fileExists func(string) bool, homeDir string) []Result {
	checks := []Result{
		CheckRsync(),
		CheckAWSCredentials(lookupEnv, fileExists, homeDir),
	}

	var failures []Result
	for _, c := range checks {
		if !c.OK {
			failures = append(failures, c)
		}
	}
	return failures
}
