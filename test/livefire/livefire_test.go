//go:build livefire

package livefire

import (
	"os"
	"testing"

	"github.com/cucumber/godog"
	"github.com/cucumber/godog/colors"
)

// TestLiveFire runs all Live Fire BDD scenarios.
//
// Live Fire tests exercise every CLI interaction path against real AWS
// infrastructure — no mocks, no fakes. They create actual EC2 instances,
// run real commands, and verify real output.
//
// Prerequisites:
//   - Built binary: make build
//   - Valid AWS credentials with yeager IAM policy
//   - Internet access
//
// Run all scenarios:
//
//	make livefire
//
// Run offline-only scenarios (no AWS):
//
//	make livefire-offline
//
// Run a single scenario by name:
//
//	go test -tags livefire -v -count=1 -timeout 30m ./test/livefire/... -run "TestLiveFire/First_command"
//
// Filter by tag via environment variable:
//
//	LIVEFIRE_TAGS=@offline go test -tags livefire -v -count=1 ./test/livefire/...
func TestLiveFire(t *testing.T) {
	opts := godog.Options{
		Output: colors.Colored(os.Stdout),
		Format: "pretty",
		Paths: []string{
			"features/smoke.feature",
			"features/credentials.feature",
			"features/command_execution.feature",
			"features/logs_and_history.feature",
			"features/flags.feature",
			"features/kill.feature",
			"features/vm_lifecycle.feature",
			"features/config_behavior.feature",
			"features/artifacts.feature",
			"features/08-file-sync.feature",
			"features/10-status.feature",
		},
		TestingT:    t,
		Concurrency: 0, // sequential — AWS scenarios share VM state
	}

	// Allow tag filtering via env var: LIVEFIRE_TAGS=@offline
	if tags := os.Getenv("LIVEFIRE_TAGS"); tags != "" {
		opts.Tags = tags
	}

	suite := godog.TestSuite{
		Name:                 "livefire",
		ScenarioInitializer:  InitializeScenario,
		TestSuiteInitializer: InitializeTestSuite,
		Options:              &opts,
	}

	if suite.Run() != 0 {
		t.Fatal("Live Fire tests failed")
	}
}
