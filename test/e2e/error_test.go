//go:build e2e

package e2e

import (
	"testing"
	"time"
)

// TestE2E_InvalidCredentials_ShowsError verifies that invalid AWS credentials
// produce a visible error message, not a silent failure.
func TestE2E_InvalidCredentials_ShowsError(t *testing.T) {
	dir := uniqueDir(t)
	setupGoProject(t, dir)

	// Use bogus credentials and clear all other credential sources.
	out, err := runFKWithEnv(t, dir, 30*time.Second, map[string]string{
		"AWS_ACCESS_KEY_ID":     "TESTFAKEKEY000000000",
		"AWS_SECRET_ACCESS_KEY": "testFakeSecretKey00000000000000EXAMPLE",
		"AWS_PROFILE":           "",
		"AWS_SHARED_CREDENTIALS_FILE": "/dev/null",
		"AWS_CONFIG_FILE":             "/dev/null",
	}, "echo", "hello")

	// Should fail.
	if err == nil {
		t.Fatalf("expected error with bogus credentials, got success:\n%s", out)
	}

	// Should show an error message, not be silent.
	if out == "" {
		t.Fatal("output is empty — error was silently swallowed")
	}
	requireContainsAny(t, out, "error", "credentials", "denied", "invalid", "expired")
}

// TestE2E_NoCredentials_ShowsError verifies that missing credentials show
// a helpful error message with actionable fix instructions.
func TestE2E_NoCredentials_ShowsError(t *testing.T) {
	dir := uniqueDir(t)
	setupGoProject(t, dir)

	// Clear all AWS credential sources.
	emptyHome := t.TempDir()
	out, err := runFKWithEnv(t, dir, 30*time.Second, map[string]string{
		"AWS_ACCESS_KEY_ID":           "",
		"AWS_SECRET_ACCESS_KEY":       "",
		"AWS_SESSION_TOKEN":           "",
		"AWS_PROFILE":                 "",
		"AWS_WEB_IDENTITY_TOKEN_FILE": "",
		"AWS_SHARED_CREDENTIALS_FILE": "/dev/null",
		"AWS_CONFIG_FILE":             "/dev/null",
		"HOME":                        emptyHome,
	}, "echo", "hello")

	// Should fail.
	if err == nil {
		t.Fatalf("expected error with no credentials, got success:\n%s", out)
	}

	// Should show an error message, not be silent.
	if out == "" {
		t.Fatal("output is empty — error was silently swallowed")
	}
	requireContainsAny(t, out, "credentials", "yg configure", "AWS_ACCESS_KEY_ID")
}
