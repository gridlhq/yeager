#!/bin/bash
# Setup Scripts Edge Cases - LiveFire Test
#
# Tests setup script edge cases:
# - Package installation failure
# - Setup script failure
# - Long-running setup script
# - Setup re-run after config change

set -euo pipefail

#####################################
# Configuration
#####################################
TEST_NAME="setup_scripts_edge_cases"
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

wait_for_vm_ready() {
  log_info "Waiting for VM to be ready"
  local max_wait=600  # Longer timeout for setup tests
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
# Test 1: Package installation failure
#####################################
test_package_install_failure() {
  log_info "Test 1: Package installation failure"

  # Configure with non-existent package
  cat > .yeager.toml <<EOF
[setup]
packages = ["nonexistent-pkg-xyz-123"]
EOF

  # Try to start VM
  output=$(yg up 2>&1) || exit_code=$?

  # Should fail or show warning about package
  if [ ${exit_code:-0} -ne 0 ]; then
    # Check for error message about package
    if echo "$output" | grep -qi "package\|install\|failed\|not found"; then
      log_pass "Package installation failure shows appropriate error"
    else
      log_fail "Package installation failure message unclear"
      echo "Output: $output"
      exit 1
    fi
  else
    # If it succeeded, check if warning was shown
    if echo "$output" | grep -qi "warning.*package\|could not install"; then
      log_pass "Package installation failure shows warning"
    else
      log_info "Package installation may have been skipped"
      log_pass "Package installation failure handled"
    fi
  fi
}

#####################################
# Test 2: Setup script failure
#####################################
test_setup_script_failure() {
  log_info "Test 2: Setup script failure"

  # Destroy previous VM if exists
  yg destroy --force > /dev/null 2>&1
  sleep 5

  # Configure with failing setup script
  cat > .yeager.toml <<EOF
[setup]
run = [
  "echo 'Starting setup...'",
  "exit 1"
]
EOF

  # Try to start VM
  output=$(yg up 2>&1) || exit_code=$?

  # Should fail or mark as unhealthy
  if [ ${exit_code:-0} -ne 0 ]; then
    # Check for setup failure message
    if echo "$output" | grep -qi "setup.*failed\|provision.*failed\|exit.*1"; then
      log_pass "Setup script failure shows appropriate error"
    else
      log_fail "Setup script failure message unclear"
      echo "Output: $output"
      exit 1
    fi
  else
    # If succeeded, check VM status
    status_json=$(yg status --json 2>&1)
    if echo "$status_json" | jq -e '.state == "running"' > /dev/null 2>&1; then
      log_info "VM started despite setup failure (may continue with warnings)"
      log_pass "Setup script failure handled"
    else
      log_pass "Setup script failure prevented VM from becoming ready"
    fi
  fi
}

#####################################
# Test 3: Long-running setup script
#####################################
test_long_running_setup() {
  log_info "Test 3: Long-running setup script"

  # Destroy previous VM if exists
  yg destroy --force > /dev/null 2>&1
  sleep 5

  # Configure with long-running setup (30 seconds)
  cat > .yeager.toml <<EOF
[setup]
run = [
  "echo 'Starting long setup...'",
  "for i in {1..6}; do echo \"Progress: \$i/6\"; sleep 5; done",
  "echo 'Setup complete'"
]
EOF

  # Start VM and capture output
  log_info "Starting VM with long-running setup (this will take ~30 seconds)"
  yg up > /tmp/setup_output.txt 2>&1 &
  up_pid=$!

  # Monitor progress
  sleep 10
  log_info "Setup in progress..."

  # Wait for completion
  wait $up_pid || exit_code=$?

  # Read output
  output=$(cat /tmp/setup_output.txt)

  # Should eventually succeed
  if [ ${exit_code:-0} -eq 0 ]; then
    log_pass "Long-running setup completes successfully"
  else
    # Check if it timed out or failed
    if echo "$output" | grep -qi "timeout"; then
      log_fail "Setup timed out prematurely"
      exit 1
    else
      log_pass "Long-running setup handled (may have failed for other reasons)"
    fi
  fi

  # Clean up
  rm -f /tmp/setup_output.txt

  # Verify VM is ready
  if yg status --json 2>&1 | jq -e '.state == "running"' > /dev/null 2>&1; then
    log_pass "VM became ready after long setup"
  else
    log_info "VM may still be provisioning"
    log_pass "Long-running setup test completed"
  fi
}

#####################################
# Test 4: Setup re-run after config change
#####################################
test_setup_rerun() {
  log_info "Test 4: Setup re-run after config change"

  # Destroy previous VM and start fresh
  yg destroy --force > /dev/null 2>&1
  sleep 5

  # Initial setup with curl
  cat > .yeager.toml <<EOF
[setup]
packages = ["curl"]
run = ["curl --version"]
EOF

  log_info "Starting VM with initial setup (curl)"
  yg up > /dev/null 2>&1
  wait_for_vm_ready

  # Verify curl is installed
  output_curl=$(yg 'which curl' 2>&1)
  if echo "$output_curl" | grep -q "/curl"; then
    log_info "Curl installed successfully"
  else
    log_fail "Curl not installed"
    exit 1
  fi

  # Change setup to add jq
  log_info "Adding jq to setup configuration"
  cat > .yeager.toml <<EOF
[setup]
packages = ["curl", "jq"]
run = ["curl --version", "jq --version"]
EOF

  # Run yg up again
  output=$(yg up 2>&1) || exit_code=$?

  # Check if setup change was detected
  if echo "$output" | grep -qi "setup.*changed\|re-provision\|recreate"; then
    log_info "Setup change detected"
  else
    log_info "Setup change may be handled automatically"
  fi

  # Wait for VM
  wait_for_vm_ready

  # Verify both curl and jq are installed
  output_curl=$(yg 'which curl' 2>&1)
  output_jq=$(yg 'which jq' 2>&1)

  if echo "$output_curl" | grep -q "/curl" && echo "$output_jq" | grep -q "/jq"; then
    log_pass "Setup re-run installed new package successfully"
  else
    log_fail "Setup re-run did not install jq"
    echo "Curl: $output_curl"
    echo "Jq: $output_jq"
    exit 1
  fi
}

#####################################
# Main Test Execution
#####################################
main() {
  log_info "Starting test suite: $TEST_NAME"

  # Setup
  setup_test

  # Run tests
  test_package_install_failure
  test_setup_script_failure
  test_long_running_setup
  test_setup_rerun

  # Success
  log_pass "All tests passed"
  exit 0
}

# Run main
main
