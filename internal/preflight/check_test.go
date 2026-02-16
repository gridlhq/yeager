package preflight

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCheckRsync_Available(t *testing.T) {
	t.Parallel()
	// rsync is available on the dev machine (macOS/Linux).
	result := CheckRsync()
	assert.True(t, result.OK)
	assert.Equal(t, "rsync", result.Name)
	assert.Empty(t, result.Message)
}

func TestCheckAWSCredentials(t *testing.T) {
	t.Parallel()

	noFile := func(string) bool { return false }
	hasFile := func(string) bool { return true }

	tests := []struct {
		name      string
		env       map[string]string
		fileCheck func(string) bool
		homeDir   string
		wantOK    bool
		wantMsg   string
		wantFix   string
	}{
		{
			name:      "no credentials at all",
			env:       map[string]string{},
			fileCheck: noFile,
			homeDir:   "/home/test",
			wantOK:    false,
			wantMsg:   "no AWS credentials found",
			wantFix:   "run: aws configure (or set AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY environment variables)",
		},
		{
			name:      "has access key ID",
			env:       map[string]string{"AWS_ACCESS_KEY_ID": "AKIA123"},
			fileCheck: noFile,
			homeDir:   "/home/test",
			wantOK:    true,
		},
		{
			name:      "has profile",
			env:       map[string]string{"AWS_PROFILE": "dev"},
			fileCheck: noFile,
			homeDir:   "/home/test",
			wantOK:    true,
		},
		{
			name:      "session token alone is not sufficient",
			env:       map[string]string{"AWS_SESSION_TOKEN": "tok123"},
			fileCheck: noFile,
			homeDir:   "/home/test",
			wantOK:    false,
			wantMsg:   "no AWS credentials found",
			wantFix:   "run: aws configure (or set AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY environment variables)",
		},
		{
			name:      "has web identity token file",
			env:       map[string]string{"AWS_WEB_IDENTITY_TOKEN_FILE": "/var/token"},
			fileCheck: noFile,
			homeDir:   "/home/test",
			wantOK:    true,
		},
		{
			name:      "has credentials file",
			env:       map[string]string{},
			fileCheck: hasFile,
			homeDir:   "/home/test",
			wantOK:    true,
		},
		{
			name:      "has config file",
			env:       map[string]string{},
			fileCheck: func(path string) bool { return path == "/home/test/.aws/config" },
			homeDir:   "/home/test",
			wantOK:    true,
		},
		{
			name:      "empty access key ID not counted",
			env:       map[string]string{"AWS_ACCESS_KEY_ID": ""},
			fileCheck: noFile,
			homeDir:   "/home/test",
			wantOK:    false,
			wantMsg:   "no AWS credentials found",
			wantFix:   "run: aws configure (or set AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY environment variables)",
		},
		{
			name:      "empty home dir skips file check",
			env:       map[string]string{},
			fileCheck: noFile,
			homeDir:   "",
			wantOK:    false,
			wantMsg:   "no AWS credentials found",
			wantFix:   "run: aws configure (or set AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY environment variables)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			lookupEnv := func(key string) (string, bool) {
				v, ok := tt.env[key]
				return v, ok
			}
			result := CheckAWSCredentials(lookupEnv, tt.fileCheck, tt.homeDir)
			assert.Equal(t, "aws-credentials", result.Name)
			assert.Equal(t, tt.wantOK, result.OK)
			if !tt.wantOK {
				assert.Equal(t, tt.wantMsg, result.Message)
				assert.Equal(t, tt.wantFix, result.Fix)
			}
		})
	}
}

func TestRunAll_NoFailures(t *testing.T) {
	t.Parallel()
	// With valid env and rsync available, no failures expected.
	lookupEnv := func(key string) (string, bool) {
		if key == "AWS_ACCESS_KEY_ID" {
			return "AKIA123", true
		}
		return "", false
	}
	noFile := func(string) bool { return false }
	failures := RunAll(lookupEnv, noFile, "/home/test")
	assert.Empty(t, failures)
}

func TestRunAll_AWSFailure(t *testing.T) {
	t.Parallel()
	lookupEnv := func(string) (string, bool) { return "", false }
	noFile := func(string) bool { return false }
	failures := RunAll(lookupEnv, noFile, "/home/test")
	// rsync is available on dev, so only AWS should fail.
	assert.Len(t, failures, 1)
	assert.Equal(t, "aws-credentials", failures[0].Name)
}
