//go:build livefire

package livefire

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

// ---------------------------------------------------------------------------
// Scenario context — fresh per scenario, passed via context.Context
// ---------------------------------------------------------------------------

type scenarioCtxKey struct{}

type scenarioCtx struct {
	projectDir    string
	output        string
	exitCode      int
	err           error
	envOverrides  map[string]string
	useShared     bool   // true when using lfShared.projectDir (don't delete on cleanup)
	capturedRunID string // run ID extracted from status output for parameterized steps
}

func scFrom(ctx context.Context) *scenarioCtx {
	return ctx.Value(scenarioCtxKey{}).(*scenarioCtx)
}

func scTo(ctx context.Context, sc *scenarioCtx) context.Context {
	return context.WithValue(ctx, scenarioCtxKey{}, sc)
}

// ---------------------------------------------------------------------------
// GIVEN steps
// ---------------------------------------------------------------------------

func aTemporaryProjectDirectory(ctx context.Context) (context.Context, error) {
	sc := scFrom(ctx)
	dir, err := os.MkdirTemp("", "lf-scenario-*")
	if err != nil {
		return ctx, err
	}
	sc.projectDir = dir
	return ctx, nil
}

func theSharedProjectDirectory(ctx context.Context) (context.Context, error) {
	sc := scFrom(ctx)
	if lfShared.projectDir == "" {
		return ctx, fmt.Errorf("shared project directory not initialized")
	}
	sc.projectDir = lfShared.projectDir
	sc.useShared = true
	return ctx, nil
}

func aGoProjectInDir(ctx context.Context) (context.Context, error) {
	sc := scFrom(ctx)
	return ctx, setupGoProjectFiles(sc.projectDir)
}

func aNodeProjectInDir(ctx context.Context) (context.Context, error) {
	sc := scFrom(ctx)
	return ctx, setupNodeProjectFiles(sc.projectDir)
}

func aPythonProjectInDir(ctx context.Context) (context.Context, error) {
	sc := scFrom(ctx)
	return ctx, setupPythonProjectFiles(sc.projectDir)
}

func aRustProjectInDir(ctx context.Context) (context.Context, error) {
	sc := scFrom(ctx)
	return ctx, setupRustProjectFiles(sc.projectDir)
}

func aFileExistsInDir(ctx context.Context, filename string) (context.Context, error) {
	sc := scFrom(ctx)
	path := filepath.Join(sc.projectDir, filename)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return ctx, err
	}
	return ctx, os.WriteFile(path, []byte("# existing\n"), 0o644)
}

func awsCredsInvalid(ctx context.Context) (context.Context, error) {
	sc := scFrom(ctx)
	sc.envOverrides = map[string]string{
		"AWS_ACCESS_KEY_ID":           "TESTFAKEKEY000000000",
		"AWS_SECRET_ACCESS_KEY":       "testFakeSecretKey00000000000000EXAMPLE",
		"AWS_PROFILE":                 "",
		"AWS_SHARED_CREDENTIALS_FILE": "/dev/null",
		"AWS_CONFIG_FILE":             "/dev/null",
	}
	return ctx, nil
}

func awsCredsRemoved(ctx context.Context) (context.Context, error) {
	sc := scFrom(ctx)
	emptyDir, err := os.MkdirTemp("", "lf-empty-home-*")
	if err != nil {
		return ctx, err
	}
	sc.envOverrides = map[string]string{
		"AWS_ACCESS_KEY_ID":           "",
		"AWS_SECRET_ACCESS_KEY":       "",
		"AWS_SESSION_TOKEN":           "",
		"AWS_PROFILE":                 "",
		"AWS_WEB_IDENTITY_TOKEN_FILE": "",
		"AWS_SHARED_CREDENTIALS_FILE": "/dev/null",
		"AWS_CONFIG_FILE":             "/dev/null",
		"HOME":                        emptyDir,
	}
	return ctx, nil
}

func theVMIsRunning(ctx context.Context) (context.Context, error) {
	sc := scFrom(ctx)
	// Quick status check — if already running, nothing to do.
	out, _, _ := runYG(sc.projectDir, 30*time.Second, nil, "status")
	if strings.Contains(out, "running") {
		return ctx, nil
	}
	// Boot the VM.
	_, code, err := runYG(sc.projectDir, 10*time.Minute, nil, "up")
	if err != nil {
		return ctx, fmt.Errorf("failed to boot VM: %w", err)
	}
	if code != 0 {
		return ctx, fmt.Errorf("yg up exited %d", code)
	}
	return ctx, nil
}

func iWaitForStatusToContain(ctx context.Context, want string, timeoutSecs int) (context.Context, error) {
	sc := scFrom(ctx)
	deadline := time.Now().Add(time.Duration(timeoutSecs) * time.Second)
	for time.Now().Before(deadline) {
		out, _, _ := runYG(sc.projectDir, 30*time.Second, nil, "status")
		if strings.Contains(out, want) {
			sc.output = out
			sc.exitCode = 0
			sc.err = nil
			return ctx, nil
		}
		time.Sleep(5 * time.Second)
	}
	return ctx, fmt.Errorf("status did not contain %q within %d seconds", want, timeoutSecs)
}

func iHaveRun(ctx context.Context, command string) (context.Context, error) {
	sc := scFrom(ctx)
	args := parseCommand(command)
	out, code, err := runYG(sc.projectDir, 5*time.Minute, nil, args...)
	if err != nil {
		return ctx, fmt.Errorf("prerequisite %q failed: %w\n%s", command, err, truncateOutput(out))
	}
	if code != 0 {
		return ctx, fmt.Errorf("prerequisite %q exited %d\n%s", command, code, truncateOutput(out))
	}
	return ctx, nil
}

func theConfigFileContains(ctx context.Context, content *godog.DocString) (context.Context, error) {
	sc := scFrom(ctx)
	path := filepath.Join(sc.projectDir, ".yeager.toml")
	return ctx, os.WriteFile(path, []byte(content.Content), 0o644)
}

func theFileExistsWithContent(ctx context.Context, filename, content string) (context.Context, error) {
	sc := scFrom(ctx)
	path := filepath.Join(sc.projectDir, filename)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return ctx, err
	}
	return ctx, os.WriteFile(path, []byte(content), 0o644)
}

func theDirectoryExists(ctx context.Context, dirname string) (context.Context, error) {
	sc := scFrom(ctx)
	path := filepath.Join(sc.projectDir, dirname)
	return ctx, os.MkdirAll(path, 0o755)
}

func iWaitSeconds(ctx context.Context, seconds int) (context.Context, error) {
	time.Sleep(time.Duration(seconds) * time.Second)
	return ctx, nil
}

// captureLastRunIDFromStatus runs "yg status", parses out the most recent run ID
// (8 hex chars), and stores it in the scenario context for subsequent steps.
func captureLastRunIDFromStatus(ctx context.Context) (context.Context, error) {
	sc := scFrom(ctx)
	out, code, err := runYG(sc.projectDir, 30*time.Second, nil, "status")
	if err != nil {
		return ctx, fmt.Errorf("status failed: %w\n%s", err, truncateOutput(out))
	}
	if code != 0 {
		return ctx, fmt.Errorf("status exited %d\n%s", code, truncateOutput(out))
	}

	// Run IDs are 8 hex chars — find the last one in the "recent runs:" section.
	re := regexp.MustCompile(`\b([0-9a-f]{8})\b`)
	matches := re.FindAllString(out, -1)
	if len(matches) == 0 {
		return ctx, fmt.Errorf("no run ID found in status output\n%s", truncateOutput(out))
	}
	sc.capturedRunID = matches[len(matches)-1]
	return ctx, nil
}

// ---------------------------------------------------------------------------
// WHEN steps
// ---------------------------------------------------------------------------

func iRun(ctx context.Context, command string) (context.Context, error) {
	sc := scFrom(ctx)
	args := parseCommand(command)
	sc.output, sc.exitCode, sc.err = runYG(sc.projectDir, 30*time.Second, sc.envOverrides, args...)
	return ctx, nil
}

func iRunWithTimeout(ctx context.Context, command string, timeoutSecs int) (context.Context, error) {
	sc := scFrom(ctx)
	args := parseCommand(command)
	sc.output, sc.exitCode, sc.err = runYG(
		sc.projectDir,
		time.Duration(timeoutSecs)*time.Second,
		sc.envOverrides,
		args...,
	)
	return ctx, nil
}

func iRunLogsWithCapturedRunID(ctx context.Context) (context.Context, error) {
	sc := scFrom(ctx)
	if sc.capturedRunID == "" {
		return ctx, fmt.Errorf("no captured run ID — did you forget the capture step?")
	}
	sc.output, sc.exitCode, sc.err = runYG(
		sc.projectDir, 60*time.Second, sc.envOverrides,
		"logs", sc.capturedRunID,
	)
	return ctx, nil
}

func iRunKillWithCapturedRunID(ctx context.Context) (context.Context, error) {
	sc := scFrom(ctx)
	if sc.capturedRunID == "" {
		return ctx, fmt.Errorf("no captured run ID — did you forget the capture step?")
	}
	sc.output, sc.exitCode, sc.err = runYG(
		sc.projectDir, 60*time.Second, sc.envOverrides,
		"kill", sc.capturedRunID,
	)
	return ctx, nil
}

// ---------------------------------------------------------------------------
// THEN steps
// ---------------------------------------------------------------------------

func exitCodeShouldBe(ctx context.Context, expected int) error {
	sc := scFrom(ctx)
	if sc.err != nil {
		return fmt.Errorf("command error: %v\nOutput:\n%s", sc.err, truncateOutput(sc.output))
	}
	if sc.exitCode != expected {
		return fmt.Errorf("expected exit code %d, got %d\nOutput:\n%s",
			expected, sc.exitCode, truncateOutput(sc.output))
	}
	return nil
}

func exitCodeShouldNotBe(ctx context.Context, expected int) error {
	sc := scFrom(ctx)
	if sc.exitCode == expected {
		return fmt.Errorf("expected exit code NOT %d, but got %d\nOutput:\n%s",
			expected, sc.exitCode, truncateOutput(sc.output))
	}
	return nil
}

func outputShouldContain(ctx context.Context, expected string) error {
	sc := scFrom(ctx)
	if !strings.Contains(sc.output, expected) {
		return fmt.Errorf("output does not contain %q\nOutput:\n%s",
			expected, truncateOutput(sc.output))
	}
	return nil
}

func outputShouldNotContain(ctx context.Context, expected string) error {
	sc := scFrom(ctx)
	if strings.Contains(sc.output, expected) {
		return fmt.Errorf("output should not contain %q but it does\nOutput:\n%s",
			expected, truncateOutput(sc.output))
	}
	return nil
}

func outputShouldNotBeEmpty(ctx context.Context) error {
	sc := scFrom(ctx)
	if strings.TrimSpace(sc.output) == "" {
		return fmt.Errorf("output is empty")
	}
	return nil
}

func outputShouldContainOneOf(ctx context.Context, table *godog.Table) error {
	sc := scFrom(ctx)
	lower := strings.ToLower(sc.output)
	for _, row := range table.Rows[1:] { // skip header row
		keyword := strings.ToLower(row.Cells[0].Value)
		if strings.Contains(lower, keyword) {
			return nil
		}
	}
	var keywords []string
	for _, row := range table.Rows[1:] {
		keywords = append(keywords, row.Cells[0].Value)
	}
	return fmt.Errorf("output contains none of %v\nOutput:\n%s",
		keywords, truncateOutput(sc.output))
}

func fileShouldExistInDir(ctx context.Context, filename string) error {
	sc := scFrom(ctx)
	path := filepath.Join(sc.projectDir, filename)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("file %q does not exist in %s", filename, sc.projectDir)
	}
	return nil
}

func outputShouldBeValidJSONLines(ctx context.Context) error {
	sc := scFrom(ctx)
	lines := strings.Split(strings.TrimSpace(sc.output), "\n")
	parsed := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var obj map[string]interface{}
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			return fmt.Errorf("line is not valid JSON: %s\nParse error: %v\nFull output:\n%s",
				line, err, truncateOutput(sc.output))
		}
		parsed++
	}
	if parsed == 0 {
		return fmt.Errorf("output contains no JSON lines\nOutput:\n%s", truncateOutput(sc.output))
	}
	return nil
}

func outputShouldMatchPattern(ctx context.Context, pattern string) error {
	sc := scFrom(ctx)
	matched, err := regexp.MatchString(pattern, sc.output)
	if err != nil {
		return fmt.Errorf("invalid regex %q: %w", pattern, err)
	}
	if !matched {
		return fmt.Errorf("output does not match pattern %q\nOutput:\n%s",
			pattern, truncateOutput(sc.output))
	}
	return nil
}

func jsonOutputFieldShouldMatch(ctx context.Context, jqPath, pattern string) error {
	sc := scFrom(ctx)
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(sc.output), &data); err != nil {
		return fmt.Errorf("output is not valid JSON: %w\nOutput:\n%s", err, truncateOutput(sc.output))
	}

	// Simple jq-style path resolver (supports .field and nested .field.subfield)
	value, err := resolveJSONPath(data, jqPath)
	if err != nil {
		return fmt.Errorf("failed to resolve path %q: %w\nJSON:\n%s", jqPath, err, truncateOutput(sc.output))
	}

	valueStr := fmt.Sprintf("%v", value)
	matched, err := regexp.MatchString(pattern, valueStr)
	if err != nil {
		return fmt.Errorf("invalid regex %q: %w", pattern, err)
	}
	if !matched {
		return fmt.Errorf("JSON field %q value %q does not match pattern %q\nFull JSON:\n%s",
			jqPath, valueStr, pattern, truncateOutput(sc.output))
	}
	return nil
}

func jsonOutputFieldShouldBe(ctx context.Context, jqPath, expected string) error {
	sc := scFrom(ctx)
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(sc.output), &data); err != nil {
		return fmt.Errorf("output is not valid JSON: %w\nOutput:\n%s", err, truncateOutput(sc.output))
	}

	value, err := resolveJSONPath(data, jqPath)
	if err != nil {
		return fmt.Errorf("failed to resolve path %q: %w\nJSON:\n%s", jqPath, err, truncateOutput(sc.output))
	}

	valueStr := fmt.Sprintf("%v", value)
	if valueStr != expected {
		return fmt.Errorf("JSON field %q value %q does not equal %q\nFull JSON:\n%s",
			jqPath, valueStr, expected, truncateOutput(sc.output))
	}
	return nil
}

func jsonOutputFieldShouldStartWith(ctx context.Context, jqPath, prefix string) error {
	sc := scFrom(ctx)
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(sc.output), &data); err != nil {
		return fmt.Errorf("output is not valid JSON: %w\nOutput:\n%s", err, truncateOutput(sc.output))
	}

	value, err := resolveJSONPath(data, jqPath)
	if err != nil {
		return fmt.Errorf("failed to resolve path %q: %w\nJSON:\n%s", jqPath, err, truncateOutput(sc.output))
	}

	valueStr := fmt.Sprintf("%v", value)
	if !strings.HasPrefix(valueStr, prefix) {
		return fmt.Errorf("JSON field %q value %q does not start with %q\nFull JSON:\n%s",
			jqPath, valueStr, prefix, truncateOutput(sc.output))
	}
	return nil
}

func localFileShouldExist(ctx context.Context, filePath string) error {
	sc := scFrom(ctx)
	path := filePath
	if !strings.HasPrefix(path, "/") {
		path = filepath.Join(sc.projectDir, filePath)
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("file %q does not exist", filePath)
	}
	return nil
}

func localFileShouldContain(ctx context.Context, filePath, expected string) error {
	sc := scFrom(ctx)
	path := filePath
	if !strings.HasPrefix(path, "/") {
		path = filepath.Join(sc.projectDir, filePath)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read file %q: %w", filePath, err)
	}
	if !strings.Contains(string(content), expected) {
		return fmt.Errorf("file %q does not contain %q\nContent:\n%s",
			filePath, expected, truncateOutput(string(content)))
	}
	return nil
}

func localFileShouldHaveSize(ctx context.Context, filePath string, size int64) error {
	sc := scFrom(ctx)
	path := filePath
	if !strings.HasPrefix(path, "/") {
		path = filepath.Join(sc.projectDir, filePath)
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("failed to stat file %q: %w", filePath, err)
	}
	if info.Size() != size {
		return fmt.Errorf("file %q has size %d, expected %d", filePath, info.Size(), size)
	}
	return nil
}

// resolveJSONPath is a simple jq-style path resolver for .field and nested .field.subfield
func resolveJSONPath(data map[string]interface{}, path string) (interface{}, error) {
	// Remove leading dot if present
	path = strings.TrimPrefix(path, ".")
	parts := strings.Split(path, ".")

	var current interface{} = data
	for _, part := range parts {
		if part == "" {
			continue
		}
		m, ok := current.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("cannot traverse non-object at %q", part)
		}
		var exists bool
		current, exists = m[part]
		if !exists {
			return nil, fmt.Errorf("field %q not found", part)
		}
	}
	return current, nil
}

// ---------------------------------------------------------------------------
// Initializers — wire steps to godog
// ---------------------------------------------------------------------------

func InitializeScenario(sc *godog.ScenarioContext) {
	// Fresh context per scenario.
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return scTo(ctx, &scenarioCtx{
			envOverrides: make(map[string]string),
		}), nil
	})

	// Cleanup temp directories (never the shared dir).
	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		tc := scFrom(ctx)
		if tc.projectDir != "" && !tc.useShared {
			os.RemoveAll(tc.projectDir)
		}
		return ctx, nil
	})

	// ── GIVEN ──
	sc.Given(`^a temporary project directory$`, aTemporaryProjectDirectory)
	sc.Given(`^the shared project directory$`, theSharedProjectDirectory)
	sc.Given(`^a Go project in the project directory$`, aGoProjectInDir)
	sc.Given(`^a Node project in the project directory$`, aNodeProjectInDir)
	sc.Given(`^a Python project in the project directory$`, aPythonProjectInDir)
	sc.Given(`^a Rust project in the project directory$`, aRustProjectInDir)
	sc.Given(`^a "([^"]*)" file exists in the project directory$`, aFileExistsInDir)
	sc.Given(`^the AWS credentials are set to invalid values$`, awsCredsInvalid)
	sc.Given(`^all AWS credential sources are removed$`, awsCredsRemoved)
	sc.Given(`^the VM is running$`, theVMIsRunning)
	sc.Given(`^I wait for VM status to contain "([^"]*)" within (\d+) seconds$`, iWaitForStatusToContain)
	sc.Given(`^I have run "([^"]*)"$`, iHaveRun)
	sc.Given(`^I capture the last run ID from status$`, captureLastRunIDFromStatus)
	sc.Given(`^the config file contains:$`, theConfigFileContains)
	sc.Given(`^the file "([^"]*)" exists with content "([^"]*)"$`, theFileExistsWithContent)
	sc.Given(`^the directory "([^"]*)" exists$`, theDirectoryExists)
	// File sync steps
	sc.Given(`^I create a (\d+)MB file named "([^"]*)"$`, iCreateLargeFile)
	sc.Given(`^I create a "([^"]*)" file containing "([^"]*)" and "([^"]*)"$`, iCreateGitignoreWithPatterns)
	sc.Given(`^I create file "([^"]*)" with content "([^"]*)"$`, iCreateFileWithContent)
	sc.Given(`^I create symlink "([^"]*)" pointing to "([^"]*)"$`, iCreateSymlink)
	sc.Given(`^I create file "([^"]*)" with mode (\d+)$`, iCreateFileWithMode)
	sc.Given(`^I create a binary file "([^"]*)"$`, iCreateBinaryFile)
	sc.Given(`^I compute checksum of "([^"]*)" as "([^"]*)"$`, iComputeChecksumAs)
	sc.Given(`^I create nested directories (\d+) levels deep$`, iCreateNestedDirectories)

	// ── WHEN ──
	sc.When(`^I corrupt the state file at "([^"]*)"$`, iCorruptStateFile)
	sc.When(`^I run "([^"]*)"$`, iRun)
	sc.When(`^I run "([^"]*)" with a (\d+) second timeout$`, iRunWithTimeout)
	sc.When(`^I run logs with the captured run ID$`, iRunLogsWithCapturedRunID)
	sc.When(`^I run kill with the captured run ID$`, iRunKillWithCapturedRunID)
	sc.When(`^I wait (\d+) seconds$`, iWaitSeconds)

	// ── THEN ──
	sc.Then(`^the exit code should be (\d+)$`, exitCodeShouldBe)
	sc.Then(`^the exit code should not be (\d+)$`, exitCodeShouldNotBe)
	sc.Then(`^the output should contain "([^"]*)"$`, outputShouldContain)
	sc.Then(`^the output should not contain "([^"]*)"$`, outputShouldNotContain)
	sc.Then(`^the output should not be empty$`, outputShouldNotBeEmpty)
	sc.Then(`^the output should contain one of:$`, outputShouldContainOneOf)
	sc.Then(`^the output should be valid JSON lines$`, outputShouldBeValidJSONLines)
	sc.Then(`^the file "([^"]*)" should exist in the project directory$`, fileShouldExistInDir)
	sc.Then(`^the output should match pattern "([^"]*)"$`, outputShouldMatchPattern)
	sc.Then(`^the JSON output field "([^"]*)" should match "([^"]*)"$`, jsonOutputFieldShouldMatch)
	sc.Then(`^the JSON output field "([^"]*)" should be "([^"]*)"$`, jsonOutputFieldShouldBe)
	sc.Then(`^the JSON output field "([^"]*)" should start with "([^"]*)"$`, jsonOutputFieldShouldStartWith)
	sc.Then(`^the local file "([^"]*)" should exist$`, localFileShouldExist)
	sc.Then(`^the local file "([^"]*)" should contain "([^"]*)"$`, localFileShouldContain)
	sc.Then(`^the local file "([^"]*)" should have size (\d+)$`, localFileShouldHaveSize)
	// File sync and status steps
	sc.Then(`^the output should be valid JSON$`, outputShouldBeValidJSON)
	sc.Then(`^the JSON output should have field "([^"]*)"$`, jsonOutputShouldHaveField)
	sc.Then(`^the output should contain the stored checksum$`, outputShouldContainStoredChecksum)
}

func InitializeTestSuite(ctx *godog.TestSuiteContext) {
	ctx.BeforeSuite(func() {
		lfShared.binary = findBinary()
		fmt.Printf("Live Fire: using binary %s\n", lfShared.binary)
		if err := setupSharedProject(); err != nil {
			panic(fmt.Sprintf("Live Fire: failed to set up shared project: %v", err))
		}
		fmt.Printf("Live Fire: shared project at %s\n", lfShared.projectDir)
	})

	ctx.AfterSuite(func() {
		fmt.Println("Live Fire: cleaning up...")
		destroySharedProject()
	})
}
