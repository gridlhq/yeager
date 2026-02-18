#!/bin/bash
# Kill Command Edge Cases - LiveFire Test
#
# Tests kill command behavior:
# - Killing already-finished commands
# - Killing during file sync
# - Force flag (no confirmation)
# - Duplicate kill attempts

set -euo pipefail

#####################################
# Configuration
#####################################
TEST_NAME="kill_edge_cases"
TEST_DIR=$(mktemp -d -t yeager-test-XXXXX)
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

# Cleanup function
cleanup() {
  if [ "$CLEANUP_ON_EXIT" = true ]; then
    log_info "Cleaning up test resources"

    # Destroy Yeager VM
    cd "$TEST_DIR" 2>/dev/null && yg destroy --force 2>/dev/null || true

    # Remove test directory
    rm -rf "$TEST_DIR" 2>/dev/null || true
  else
    log_info "Skipping cleanup (CLEANUP_ON_EXIT=false)"
    log_info "Test directory: $TEST_DIR"
  fi
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

  if ! echo "$haystack" | grep -q -- "$needle"; then
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

  if echo "$haystack" | grep -q -- "$needle"; then
    log_fail "$message (should not contain '$needle')"
    echo "Actual output:"
    echo "$haystack"
    exit 1
  fi
}

wait_for_vm_ready() {
  log_info "Waiting for VM to be ready"
  local max_wait=600
  local elapsed=0

  while [ $elapsed -lt $max_wait ]; do
    if yg status --json 2>&1 | jq -e '.state == "running"' > /dev/null 2>&1; then
      log_info "VM is ready"
      return 0
    fi
    sleep 5
    elapsed=$((elapsed + 5))
  done

  log_fail "VM did not become ready within ${max_wait}s"
  exit 1
}

#####################################
# Test Setup
#####################################
setup_test() {
  log_info "Setting up test in $TEST_DIR"

  # Change to test directory
  cd "$TEST_DIR"

  # Initialize Yeager project
  yg init > /dev/null 2>&1 || {
    log_fail "Failed to initialize Yeager project"
    exit 1
  }

  # Configure credentials (assumes AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY are set)
  # AWS credentials: yg uses ~/.aws/credentials automatically

  log_info "Setup complete"
}

#####################################
# Test 1: Kill already-finished command
#####################################
test_kill_already_finished() {
  log_info "Test 1: Kill already-finished command"

  # Start VM
  yg up > /dev/null 2>&1
  wait_for_vm_ready

  # Run a command that completes quickly
  log_info "Running quick command that will finish"
  yg 'echo "done" && sleep 1' > /dev/null 2>&1

  # Wait for command to finish
  sleep 3

  # Try to kill the finished run
  output=$(yg kill 2>&1) || exit_code=$?

  # Kill may exit 0 (graceful no-op) or non-zero. Either is acceptable
  # as long as it doesn't crash. Check for informative message.
  if echo "$output" | grep -qi "no active\|nothing\|cancel\|kill\|not found\|session\|finished\|completed\|already"; then
    log_pass "Kill already-finished command shows appropriate message"
  elif [ ${exit_code:-0} -eq 0 ]; then
    log_pass "Kill already-finished command returned gracefully"
  else
    log_fail "Kill already-finished command produced unexpected output"
    echo "Output: $output"
    exit 1
  fi
}

#####################################
# Test 2: Kill during file sync
#####################################
test_kill_during_sync() {
  log_info "Test 2: Kill during file sync"

  # Create a large file to sync (to make sync take time)
  log_info "Creating large test file"
  dd if=/dev/zero of=largefile.bin bs=1M count=100 2>/dev/null || true

  # Start command with file sync in background
  log_info "Starting command with file sync"
  yg 'sleep 60' > /dev/null 2>&1 &
  run_pid=$!

  # Give it a moment to start syncing
  sleep 5

  # Kill with force (no confirmation)
  output=$(yg kill 2>&1) || exit_code=$?

  # Should succeed
  if [ ${exit_code:-0} -eq 0 ]; then
    # Check that kill was acknowledged
    if echo "$output" | grep -qi "kill\|termin\|stop"; then
      log_pass "Kill during sync succeeds"
    else
      log_info "Kill command output: $output"
      log_pass "Kill during sync completed (no explicit message)"
    fi
  else
    log_fail "Kill during sync failed"
    echo "Output: $output"
    exit 1
  fi

  # Clean up background process if still running
  kill $run_pid 2>/dev/null || true
  wait $run_pid 2>/dev/null || true
}

#####################################
# Test 3: Kill does not prompt for confirmation
#####################################
test_kill_force_flag() {
  log_info "Test 3: Kill does not prompt for confirmation"

  # Start a long-running command
  log_info "Starting long-running command"
  yg 'sleep 120' > /dev/null 2>&1 &
  run_pid=$!

  # Give it time to start
  sleep 3

  # Kill should succeed without prompting
  log_info "Killing running command"
  output=$(yg kill 2>&1) || exit_code=$?

  # Should succeed
  assert_exit_code 0 ${exit_code:-0} "Kill should succeed"

  # Output should not contain confirmation prompts
  assert_not_contains "$output" "Are you sure\|confirm\|y/n" \
    "Kill should not prompt for confirmation"

  log_pass "Kill does not prompt for confirmation"

  # Clean up background process
  kill $run_pid 2>/dev/null || true
  wait $run_pid 2>/dev/null || true
}

#####################################
# Test 4: Kill same run multiple times
#####################################
test_kill_duplicate() {
  log_info "Test 4: Kill same run multiple times"

  # Start a long-running command
  log_info "Starting long-running command"
  yg 'sleep 120' > /dev/null 2>&1 &
  run_pid=$!

  # Give it time to start
  sleep 3

  # Kill once
  log_info "First kill attempt"
  output1=$(yg kill 2>&1) || exit_code1=$?
  assert_exit_code 0 ${exit_code1:-0} "First kill should succeed"

  # Wait a moment
  sleep 2

  # Try to kill again
  log_info "Second kill attempt (duplicate)"
  output2=$(yg kill 2>&1) || exit_code2=$?

  # Second kill may exit 0 (no-op) or non-zero (already dead). Either is fine.
  if echo "$output2" | grep -qi "already\|finished\|not found\|killed\|cancel\|no active\|nothing\|session"; then
    log_pass "Duplicate kill shows appropriate message"
  elif [ ${exit_code2:-0} -eq 0 ]; then
    log_pass "Duplicate kill returned gracefully"
  else
    log_fail "Duplicate kill produced unexpected output"
    echo "Output: $output2"
    exit 1
  fi

  # Clean up background process
  kill $run_pid 2>/dev/null || true
  wait $run_pid 2>/dev/null || true
}

#####################################
# Main Test Execution
#####################################
main() {
  log_info "Starting test suite: $TEST_NAME"

  # Setup
  setup_test

  # Run tests
  test_kill_already_finished
  test_kill_force_flag
  test_kill_duplicate
  test_kill_during_sync

  # Success
  log_pass "All tests passed"
  exit 0
}

# Run main
main
