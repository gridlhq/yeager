#!/bin/bash
# LiveFire Test: Monitor Daemon Functionality
#
# Tests background monitoring daemon behavior.
# - NO code shortcuts, NO internal imports
# - Run yg as subprocess
# - Parse CLI output and process status
# - Verify daemon lifecycle with ps/pgrep

set -euo pipefail

#####################################
# Configuration
#####################################
TEST_NAME="07-monitor"
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

    # Stop any monitor daemon processes
    pkill -f "yg monitor-daemon" 2>/dev/null || true

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

assert_process_exists() {
  local pattern=$1
  local message=$2

  if ! pgrep -f "$pattern" > /dev/null 2>&1; then
    log_fail "$message (process not found: $pattern)"
    echo "Running processes:"
    ps aux | grep "$pattern" | grep -v grep || true
    exit 1
  fi
}

assert_process_not_exists() {
  local pattern=$1
  local message=$2

  if pgrep -f "$pattern" > /dev/null 2>&1; then
    log_fail "$message (process still running: $pattern)"
    echo "Running processes:"
    ps aux | grep "$pattern" | grep -v grep || true
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
# Scenario 1: Daemon process starts with grace period monitoring
#####################################
test_daemon_starts_with_monitoring() {
  log_info "Test: Daemon process starts with grace period monitoring"

  # Setup: Configure grace period to trigger daemon
  cat > .yeager.toml <<EOF

idle_stop = true
grace_period = "5m"
EOF

  # Start VM (this should spawn the monitor daemon)
  up_output=$(yg up 2>&1) || exit_code=$?
  assert_exit_code 0 ${exit_code:-0} "yg up should succeed"

  # Verify VM is running
  wait_for_vm_state "running"

  # Wait a moment for daemon to start
  sleep 3

  # Verify monitor daemon process is running
  # The daemon is spawned as "yg monitor-daemon --project-hash ... --state-dir ... --grace-period ..."
  assert_process_exists "yg monitor-daemon" \
    "Monitor daemon process should be running"

  log_pass "Monitor daemon starts successfully with grace period monitoring"
}

#####################################
# Scenario 2: Daemon survives yg command exit
#####################################
test_daemon_survives_parent_exit() {
  log_info "Test: Daemon survives parent command exit"

  # Setup: Configure grace period
  cat > .yeager.toml <<EOF

idle_stop = true
grace_period = "5m"
EOF

  # Start VM in background (simulates terminal close)
  yg up > /dev/null 2>&1 &
  UP_PID=$!

  # Wait for VM to be ready
  wait_for_vm_state "running"

  # Wait for parent yg up process to complete
  wait $UP_PID || true

  # Wait a moment
  sleep 2

  # Verify monitor daemon is still running after parent exited
  assert_process_exists "yg monitor-daemon" \
    "Monitor daemon should survive parent process exit"

  log_pass "Monitor daemon survives parent process exit"
}

#####################################
# Scenario 3: Daemon stops VM after grace period expires
#####################################
test_daemon_stops_vm_after_grace_period() {
  log_info "Test: Daemon stops VM after grace period expires"

  # Setup: Configure short grace period for testing
  cat > .yeager.toml <<EOF

idle_stop = true
grace_period = "1m"
EOF

  # Start VM
  up_output=$(yg up 2>&1) || exit_code=$?
  assert_exit_code 0 ${exit_code:-0} "yg up should succeed"

  # Verify VM is running
  wait_for_vm_state "running"

  # Verify daemon is running
  sleep 3
  assert_process_exists "yg monitor-daemon" \
    "Monitor daemon should be running"

  # Wait for grace period to expire (1m + 45s buffer)
  log_info "Waiting 105 seconds for grace period to expire..."
  sleep 105

  # Verify VM has been stopped by daemon
  status_json=$(yg status --json 2>&1)
  current_state=$(echo "$status_json" | jq -r '.state')

  if [ "$current_state" != "stopped" ] && [ "$current_state" != "stopping" ]; then
    log_fail "Daemon should have stopped VM after grace period (state: $current_state)"
    exit 1
  fi

  log_pass "Daemon successfully stops VM after grace period"
}

#####################################
# Scenario 4: No daemon when idle_stop disabled
#####################################
test_no_daemon_when_idle_stop_disabled() {
  log_info "Test: No daemon spawned when idle_stop disabled"

  # Setup: Disable idle_stop
  cat > .yeager.toml <<EOF

idle_stop = false
grace_period = "5m"
EOF

  # Kill any existing daemons
  pkill -f "yg monitor-daemon" 2>/dev/null || true
  sleep 1

  # Start VM
  up_output=$(yg up 2>&1) || exit_code=$?
  assert_exit_code 0 ${exit_code:-0} "yg up should succeed"

  # Verify VM is running
  wait_for_vm_state "running"

  # Wait a moment
  sleep 3

  # Verify no monitor daemon is running
  if pgrep -f "yg monitor-daemon" > /dev/null 2>&1; then
    log_fail "Monitor daemon should NOT be running when idle_stop is disabled"
    exit 1
  fi

  log_pass "No daemon spawned when idle_stop disabled (as expected)"
}

#####################################
# Scenario 5: Daemon cleans up after VM destroyed
#####################################
test_daemon_cleanup_on_destroy() {
  log_info "Test: Daemon cleans up after VM destroyed"

  # Setup: Configure grace period
  cat > .yeager.toml <<EOF

idle_stop = true
grace_period = "5m"
EOF

  # Start VM
  up_output=$(yg up 2>&1) || exit_code=$?
  assert_exit_code 0 ${exit_code:-0} "yg up should succeed"

  # Verify VM is running
  wait_for_vm_state "running"

  # Verify daemon is running
  sleep 3
  assert_process_exists "yg monitor-daemon" \
    "Monitor daemon should be running"

  # Destroy VM
  log_info "Destroying VM..."
  destroy_output=$(yg destroy --force 2>&1) || exit_code=$?
  assert_exit_code 0 ${exit_code:-0} "yg destroy should succeed"

  # Wait for daemon to detect and exit
  log_info "Waiting for daemon to detect VM destruction..."
  sleep 10

  # Verify daemon has stopped
  assert_process_not_exists "yg monitor-daemon" \
    "Monitor daemon should stop after VM destroyed"

  log_pass "Daemon cleans up successfully after VM destroyed"
}

#####################################
# Scenario 6: Daemon continues monitoring across stop/start cycles
#####################################
test_daemon_monitors_across_stop_start() {
  log_info "Test: Daemon monitors VM across stop/start cycles"

  # Setup: Configure grace period
  cat > .yeager.toml <<EOF

idle_stop = true
grace_period = "10m"
EOF

  # Start VM
  up_output=$(yg up 2>&1) || exit_code=$?
  assert_exit_code 0 ${exit_code:-0} "yg up should succeed"
  wait_for_vm_state "running"

  # Verify daemon is running
  sleep 3
  assert_process_exists "yg monitor-daemon" \
    "Monitor daemon should be running after up"

  # Manually stop VM
  log_info "Stopping VM manually..."
  stop_output=$(yg stop 2>&1) || exit_code=$?
  assert_exit_code 0 ${exit_code:-0} "yg stop should succeed"
  wait_for_vm_state "stopped"

  # Daemon should still be present (monitoring for state changes)
  # Or it may have exited - this is implementation dependent
  # We'll just verify it doesn't crash
  sleep 2

  # Restart VM
  log_info "Restarting VM..."
  up_output=$(yg up 2>&1) || exit_code=$?
  assert_exit_code 0 ${exit_code:-0} "yg up should succeed (restart)"
  wait_for_vm_state "running"

  # Verify daemon is running again
  sleep 3
  assert_process_exists "yg monitor-daemon" \
    "Monitor daemon should be running after restart"

  log_pass "Daemon handles stop/start cycles correctly"
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

  command -v pgrep >/dev/null 2>&1 || {
    log_fail "pgrep not found (required for process checking)"
    exit 1
  }

  # Run each test in isolation
  # Note: Each test starts fresh with setup_test

  # Test 1
  setup_test
  test_daemon_starts_with_monitoring
  cleanup
  TEST_DIR=$(mktemp -d -t yeager-test-XXXXX)

  # Test 2
  setup_test
  test_daemon_survives_parent_exit
  cleanup
  TEST_DIR=$(mktemp -d -t yeager-test-XXXXX)

  # Test 3
  setup_test
  test_daemon_stops_vm_after_grace_period
  cleanup
  TEST_DIR=$(mktemp -d -t yeager-test-XXXXX)

  # Test 4
  setup_test
  test_no_daemon_when_idle_stop_disabled
  cleanup
  TEST_DIR=$(mktemp -d -t yeager-test-XXXXX)

  # Test 5
  setup_test
  test_daemon_cleanup_on_destroy
  cleanup
  TEST_DIR=$(mktemp -d -t yeager-test-XXXXX)

  # Test 6
  setup_test
  test_daemon_monitors_across_stop_start
  cleanup
  TEST_DIR=$(mktemp -d -t yeager-test-XXXXX)

  # Success
  log_pass "All monitor daemon tests passed"
  exit 0
}

# Run main
main
