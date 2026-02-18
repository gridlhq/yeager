#!/bin/bash
# LiveFire Test: Idle Monitoring Functionality
#
# Tests idle monitoring behavior - how yeager stops VMs after inactivity.
# - NO code shortcuts, NO internal imports
# - Run yg as subprocess
# - Parse CLI output
# - Verify with file inspection and AWS CLI

set -euo pipefail

#####################################
# Configuration
#####################################
TEST_NAME="06-idle"
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

assert_json_value() {
  local json=$1
  local jq_query=$2
  local expected=$3
  local message=$4

  local actual=$(echo "$json" | jq -r "$jq_query")

  if [ "$actual" != "$expected" ]; then
    log_fail "$message (expected '$expected', got '$actual')"
    echo "Full JSON:"
    echo "$json" | jq .
    exit 1
  fi
}

# Wait for VM to reach a specific state
wait_for_vm_state() {
  local target_state=$1
  local max_wait=${2:-300}  # default 5 minutes
  local elapsed=0

  log_info "Waiting for VM to reach state: $target_state"

  while [ $elapsed -lt $max_wait ]; do
    status_json=$(yg status --json 2>&1) || true
    current_state=$(echo "$status_json" | jq -r '.state' 2>/dev/null || echo "unknown")

    if [ "$current_state" = "$target_state" ]; then
      log_info "VM reached state: $target_state (after ${elapsed}s)"
      return 0
    fi

    sleep 5
    elapsed=$((elapsed + 5))
  done

  log_fail "Timeout waiting for VM to reach state: $target_state (waited ${elapsed}s)"
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
# Scenario 1: VM stops after grace period when idle_stop enabled
#####################################
test_idle_stop_after_grace_period() {
  log_info "Test: VM stops after grace period when idle_stop enabled"

  # Setup: Configure idle_stop and grace_period in .yeager.toml
  cat > .yeager.toml <<EOF

idle_stop = true
grace_period = "1m"
EOF

  # Start VM
  up_output=$(yg up 2>&1) || exit_code=$?
  assert_exit_code 0 ${exit_code:-0} "yg up should succeed"

  # Verify VM is running
  wait_for_vm_state "running"

  # Wait for grace period to elapse (1m + 30s buffer for monitoring)
  log_info "Waiting 90 seconds for grace period to elapse..."
  sleep 90

  # Verify VM has stopped (or is stopping)
  status_json=$(yg status --json 2>&1)
  current_state=$(echo "$status_json" | jq -r '.state')

  if [ "$current_state" != "stopped" ] && [ "$current_state" != "stopping" ]; then
    log_fail "VM should be stopped or stopping after grace period (state: $current_state)"
    exit 1
  fi

  log_pass "VM stopped after grace period as expected"
}

#####################################
# Scenario 2: VM stays running when idle_stop disabled
#####################################
test_no_idle_stop_when_disabled() {
  log_info "Test: VM stays running when idle_stop disabled"

  # Setup: Ensure idle_stop is disabled (default or explicit)
  cat > .yeager.toml <<EOF

idle_stop = false
grace_period = "1m"
EOF

  # Start VM
  up_output=$(yg up 2>&1) || exit_code=$?
  assert_exit_code 0 ${exit_code:-0} "yg up should succeed"

  # Verify VM is running
  wait_for_vm_state "running"

  # Wait longer than grace period
  log_info "Waiting 90 seconds (beyond grace period) to verify VM stays running..."
  sleep 90

  # Verify VM is still running
  status_json=$(yg status --json 2>&1)
  assert_json_value "$status_json" ".state" "running" \
    "VM should still be running when idle_stop is disabled"

  log_pass "VM stays running when idle_stop disabled"
}

#####################################
# Scenario 3: Activity resets grace period timer
#####################################
test_activity_resets_grace_period() {
  log_info "Test: Activity resets grace period timer"

  # Setup: Configure short grace period
  cat > .yeager.toml <<EOF

idle_stop = true
grace_period = "2m"
EOF

  # Start VM
  up_output=$(yg up 2>&1) || exit_code=$?
  assert_exit_code 0 ${exit_code:-0} "yg up should succeed"

  # Verify VM is running
  wait_for_vm_state "running"

  # Wait 60 seconds (half the grace period)
  log_info "Waiting 60 seconds (half grace period)..."
  sleep 60

  # Run a command to reset the timer
  log_info "Running command to reset grace period timer..."
  run_output=$(yg echo "keep-alive" 2>&1) || exit_code=$?
  assert_exit_code 0 ${exit_code:-0} "yg should succeed"
  assert_contains "$run_output" "keep-alive" "command should execute"

  # Wait another 60 seconds
  log_info "Waiting another 60 seconds..."
  sleep 60

  # VM should still be running (timer was reset by the command)
  status_json=$(yg status --json 2>&1)
  current_state=$(echo "$status_json" | jq -r '.state')

  if [ "$current_state" != "running" ]; then
    log_fail "VM should still be running after activity reset timer (state: $current_state)"
    exit 1
  fi

  log_pass "Activity successfully resets grace period timer"
}

#####################################
# Scenario 4: Grace period configuration is respected
#####################################
test_grace_period_configuration() {
  log_info "Test: Grace period configuration is respected"

  # Setup: Configure specific grace period
  cat > .yeager.toml <<EOF

idle_stop = true
grace_period = "30s"
EOF

  # Start VM
  up_output=$(yg up 2>&1) || exit_code=$?
  assert_exit_code 0 ${exit_code:-0} "yg up should succeed"

  # Verify VM is running
  wait_for_vm_state "running"

  # Wait less than grace period
  log_info "Waiting 15 seconds (less than grace period)..."
  sleep 15

  # VM should still be running
  status_json=$(yg status --json 2>&1)
  assert_json_value "$status_json" ".state" "running" \
    "VM should still be running before grace period expires"

  # Wait for grace period to complete (15s more + 30s buffer)
  log_info "Waiting 45 more seconds for grace period to elapse..."
  sleep 45

  # Verify VM has stopped
  status_json=$(yg status --json 2>&1)
  current_state=$(echo "$status_json" | jq -r '.state')

  if [ "$current_state" != "stopped" ] && [ "$current_state" != "stopping" ]; then
    log_fail "VM should be stopped after grace period (state: $current_state)"
    exit 1
  fi

  log_pass "Grace period configuration is respected"
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

  command -v jq >/dev/null 2>&1 || {
    log_fail "jq not found (required for JSON parsing)"
    exit 1
  }

  # Run each test in isolation
  # Note: Each test starts fresh with setup_test

  # Test 1
  setup_test
  test_idle_stop_after_grace_period
  cleanup
  TEST_DIR=$(mktemp -d -t yeager-test-XXXXX)

  # Test 2
  setup_test
  test_no_idle_stop_when_disabled
  cleanup
  TEST_DIR=$(mktemp -d -t yeager-test-XXXXX)

  # Test 3
  setup_test
  test_activity_resets_grace_period
  cleanup
  TEST_DIR=$(mktemp -d -t yeager-test-XXXXX)

  # Test 4
  setup_test
  test_grace_period_configuration
  cleanup
  TEST_DIR=$(mktemp -d -t yeager-test-XXXXX)

  # Success
  log_pass "All idle tests passed"
  exit 0
}

# Run main
main
