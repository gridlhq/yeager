#!/bin/bash
# LiveFire Test: Help Command Functionality
#
# Tests help output and documentation discoverability.
# - NO code shortcuts, NO internal imports
# - Run yg as subprocess
# - Parse CLI output for help text
# - Verify command documentation

set -euo pipefail

#####################################
# Configuration
#####################################
TEST_NAME="09-help"
# Help tests don't need a full test directory or AWS resources
CLEANUP_ON_EXIT=true

#####################################
# Utilities
#####################################

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_pass() {
  echo -e "${GREEN}[PASS]${NC} $TEST_NAME: $1"
}

log_fail() {
  echo -e "${RED}[FAIL]${NC} $TEST_NAME: $1"
}

log_info() {
  echo -e "${YELLOW}[INFO]${NC} $TEST_NAME: $1"
}

# Cleanup function (minimal for help tests)
cleanup() {
  log_info "Cleanup complete (no resources to clean)"
}

# Register cleanup on exit
trap cleanup EXIT

# Assert functions
assert_exit_code() {
  local expected=$1
  local actual=$2
  local message=$3

  if [ "$actual" -ne "$expected" ]; then
    log_fail "$message (expected exit code $expected, got $actual)"
    exit 1
  fi
}

assert_contains() {
  local haystack=$1
  local needle=$2
  local message=$3

  if ! echo "$haystack" | grep -qi -- "$needle"; then
    log_fail "$message (expected to contain '$needle')"
    echo "Actual output:"
    echo "$haystack"
    exit 1
  fi
}

assert_not_contains() {
  local haystack=$1
  local needle=$2
  local message=$3

  if echo "$haystack" | grep -qi -- "$needle"; then
    log_fail "$message (should not contain '$needle')"
    echo "Actual output:"
    echo "$haystack"
    exit 1
  fi
}

#####################################
# Scenario 1: yg help shows all main commands
#####################################
test_help_shows_all_commands() {
  log_info "Test: yg help shows all main commands"

  # Execute: Run yg help (or yg --help)
  help_output=$(yg --help 2>&1) || exit_code=$?
  assert_exit_code 0 ${exit_code:-0} "yg --help should succeed"

  # Verify: All main commands are listed
  assert_contains "$help_output" "status" "help should list 'status' command"
  assert_contains "$help_output" "up" "help should list 'up' command"
  assert_contains "$help_output" "stop" "help should list 'stop' command"
  assert_contains "$help_output" "destroy" "help should list 'destroy' command"
  assert_contains "$help_output" "logs" "help should list 'logs' command"
  assert_contains "$help_output" "kill" "help should list 'kill' command"
  assert_contains "$help_output" "configure" "help should list 'configure' command"
  assert_contains "$help_output" "init" "help should list 'init' command"

  # Verify: Usage section exists
  assert_contains "$help_output" "Usage:" "help should show Usage section"

  # Verify: Commands section exists
  assert_contains "$help_output" "Commands:" "help should show Commands section"

  log_pass "Help shows all main commands"
}

#####################################
# Scenario 2: yg <command> --help shows detailed help
#####################################
test_command_specific_help() {
  log_info "Test: yg <command> --help shows detailed help"

  # Test 'status' command help
  status_help=$(yg status --help 2>&1) || exit_code=$?
  assert_exit_code 0 ${exit_code:-0} "yg status --help should succeed"

  assert_contains "$status_help" "status" "status help should mention the command"
  assert_contains "$status_help" "Usage:" "status help should show usage"

  # Test 'run' command help (implicit via root command)
  # Since run is the default action, test that help is available
  root_help=$(yg --help 2>&1)
  assert_contains "$root_help" "yg <command>" "help should show usage pattern"

  # Test 'up' command help
  up_help=$(yg up --help 2>&1) || exit_code=$?
  assert_exit_code 0 ${exit_code:-0} "yg up --help should succeed"
  assert_contains "$up_help" "up" "up help should mention the command"

  # Test 'stop' command help
  stop_help=$(yg stop --help 2>&1) || exit_code=$?
  assert_exit_code 0 ${exit_code:-0} "yg stop --help should succeed"
  assert_contains "$stop_help" "stop" "stop help should mention the command"

  # Test 'configure' command help
  configure_help=$(yg configure --help 2>&1) || exit_code=$?
  assert_exit_code 0 ${exit_code:-0} "yg configure --help should succeed"
  assert_contains "$configure_help" "configure" "configure help should mention the command"

  log_pass "Command-specific help works correctly"
}

#####################################
# Scenario 3: Help shows global flags
#####################################
test_help_shows_global_flags() {
  log_info "Test: Help shows global flags"

  # Execute: Run yg --help
  help_output=$(yg --help 2>&1) || exit_code=$?
  assert_exit_code 0 ${exit_code:-0} "yg --help should succeed"

  # Verify: Global flags are documented
  assert_contains "$help_output" "--json" "help should document --json flag"
  assert_contains "$help_output" "--quiet" "help should document --quiet flag"
  assert_contains "$help_output" "--verbose" "help should document --verbose flag"
  assert_contains "$help_output" "Flags:" "help should have Flags section"

  log_pass "Help shows global flags"
}

#####################################
# Scenario 4: Help includes examples and version
#####################################
test_help_includes_examples_and_version() {
  log_info "Test: Help includes examples and version"

  # Execute: Run yg --help
  help_output=$(yg --help 2>&1) || exit_code=$?
  assert_exit_code 0 ${exit_code:-0} "yg --help should succeed"

  # Verify: Examples section exists
  assert_contains "$help_output" "Examples:" "help should show Examples section"

  # Verify: Example commands are shown
  # Based on root.go, examples include basic commands
  assert_contains "$help_output" "yg" "help should show example commands"

  # Check version is available via --version flag
  version_output=$(yg --version 2>&1) || exit_code=$?
  assert_exit_code 0 ${exit_code:-0} "yg --version should succeed"

  # Version should contain some version identifier
  # Could be "dev", "0.x.x", etc.
  if [ -z "$version_output" ]; then
    log_fail "version output should not be empty"
    exit 1
  fi

  log_pass "Help includes examples and version is accessible"
}

#####################################
# Main Test Execution
#####################################
main() {
  log_info "Starting test suite: $TEST_NAME"

  # Check for required tools
  command -v yg >/dev/null 2>&1 || {
    log_fail "yg command not found in PATH"
    exit 1
  }

  # Run all tests
  test_help_shows_all_commands
  test_command_specific_help
  test_help_shows_global_flags
  test_help_includes_examples_and_version

  # Success
  log_pass "All help tests passed"
  exit 0
}

# Run main
main
