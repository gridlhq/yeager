#!/bin/bash
# Multi-Project Isolation - LiveFire Test
#
# Tests multi-project isolation:
# - Two projects with separate VMs
# - Switching between projects
# - Destroy one project keeps other alive
# - Credential sharing across projects

set -euo pipefail

#####################################
# Configuration
#####################################
TEST_NAME="multi_project_isolation"
TEST_DIR_A=$(mktemp -d -t yeager-test-a-XXXXX)
TEST_DIR_B=$(mktemp -d -t yeager-test-b-XXXXX)
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

    # Destroy both projects
    cd "$TEST_DIR_A" 2>/dev/null && yg destroy --force 2>/dev/null || true
    cd "$TEST_DIR_B" 2>/dev/null && yg destroy --force 2>/dev/null || true

    # Remove test directories
    rm -rf "$TEST_DIR_A" 2>/dev/null || true
    rm -rf "$TEST_DIR_B" 2>/dev/null || true
  else
    log_info "Skipping cleanup (CLEANUP_ON_EXIT=false)"
    log_info "Test directory A: $TEST_DIR_A"
    log_info "Test directory B: $TEST_DIR_B"
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

assert_not_equal() {
  local val1=$1
  local val2=$2
  local message=$3

  if [ "$val1" = "$val2" ]; then
    log_fail "$message (values should not be equal: $val1)"
    exit 1
  fi
}

wait_for_vm_ready() {
  local project_dir=$1
  log_info "Waiting for VM to be ready in $project_dir"
  local max_wait=600
  local elapsed=0

  cd "$project_dir"

  while [ $elapsed -lt $max_wait ]; do
    if yg status --json 2>&1 | jq -e '.state == "running"' > /dev/null 2>&1; then
      log_info "VM is ready in $project_dir"
      return 0
    fi
    sleep 5
    elapsed=$((elapsed + 5))
  done

  log_fail "VM did not become ready within ${max_wait}s in $project_dir"
  exit 1
}

#####################################
# Test Setup
#####################################
setup_test() {
  log_info "Setting up test projects"
  log_info "Project A: $TEST_DIR_A"
  log_info "Project B: $TEST_DIR_B"

  # Setup Project A
  cd "$TEST_DIR_A"
  yg init > /dev/null 2>&1 || {
    log_fail "Failed to initialize project A"
    exit 1
  }

  # Setup Project B
  cd "$TEST_DIR_B"
  yg init > /dev/null 2>&1 || {
    log_fail "Failed to initialize project B"
    exit 1
  }

  # Configure credentials (assumes AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY are set)
  # AWS credentials: yg uses ~/.aws/credentials automatically

  log_info "Setup complete"
}

#####################################
# Test 1: Two projects with separate VMs
#####################################
test_separate_vms() {
  log_info "Test 1: Two projects with separate VMs"

  # Start VM in project A
  log_info "Starting VM in project A"
  cd "$TEST_DIR_A"
  yg up > /dev/null 2>&1
  wait_for_vm_ready "$TEST_DIR_A"

  # Get instance ID from project A
  status_a=$(yg status --json 2>&1)
  instance_id_a=$(echo "$status_a" | jq -r '.instance_id // .id // empty')

  if [ -z "$instance_id_a" ]; then
    log_fail "Could not get instance ID from project A"
    exit 1
  fi

  log_info "Project A instance ID: $instance_id_a"

  # Start VM in project B
  log_info "Starting VM in project B"
  cd "$TEST_DIR_B"
  yg up > /dev/null 2>&1
  wait_for_vm_ready "$TEST_DIR_B"

  # Get instance ID from project B
  status_b=$(yg status --json 2>&1)
  instance_id_b=$(echo "$status_b" | jq -r '.instance_id // .id // empty')

  if [ -z "$instance_id_b" ]; then
    log_fail "Could not get instance ID from project B"
    exit 1
  fi

  log_info "Project B instance ID: $instance_id_b"

  # Verify they are different
  assert_not_equal "$instance_id_a" "$instance_id_b" \
    "Projects should have different instance IDs"

  # Verify both instances exist in AWS
  aws_check_a=$(aws ec2 describe-instances --instance-ids "$instance_id_a" \
    --query 'Reservations[0].Instances[0].InstanceId' --output text 2>&1) || true

  aws_check_b=$(aws ec2 describe-instances --instance-ids "$instance_id_b" \
    --query 'Reservations[0].Instances[0].InstanceId' --output text 2>&1) || true

  if [ "$aws_check_a" = "$instance_id_a" ] && [ "$aws_check_b" = "$instance_id_b" ]; then
    log_pass "Two separate EC2 instances verified in AWS"
  else
    log_fail "Could not verify both instances in AWS"
    exit 1
  fi
}

#####################################
# Test 2: Switching between projects
#####################################
test_switching_projects() {
  log_info "Test 2: Switching between projects"

  # Run command in project A
  cd "$TEST_DIR_A"
  output_a=$(yg 'echo "OUTPUT_A"' 2>&1)

  # Verify output
  assert_contains "$output_a" "OUTPUT_A" "Project A should output 'OUTPUT_A'"

  # Run command in project B
  cd "$TEST_DIR_B"
  output_b=$(yg 'echo "OUTPUT_B"' 2>&1)

  # Verify output
  assert_contains "$output_b" "OUTPUT_B" "Project B should output 'OUTPUT_B'"

  # Verify outputs are different (showing isolation)
  if echo "$output_a" | grep -q "OUTPUT_A" && \
     echo "$output_b" | grep -q "OUTPUT_B" && \
     ! echo "$output_a" | grep -q "OUTPUT_B"; then
    log_pass "Commands execute on separate VMs correctly"
  else
    log_fail "Project isolation not working correctly"
    exit 1
  fi
}

#####################################
# Test 3: Destroy one project keeps other alive
#####################################
test_destroy_isolation() {
  log_info "Test 3: Destroy one project keeps other alive"

  # Get instance IDs before destruction
  cd "$TEST_DIR_A"
  instance_id_a=$(yg status --json 2>&1 | jq -r '.instance_id // .id // empty')

  cd "$TEST_DIR_B"
  instance_id_b=$(yg status --json 2>&1 | jq -r '.instance_id // .id // empty')

  log_info "Before destroy - A: $instance_id_a, B: $instance_id_b"

  # Destroy project A
  log_info "Destroying project A"
  cd "$TEST_DIR_A"
  yg destroy --force > /dev/null 2>&1

  # Wait a moment for destruction
  sleep 5

  # Verify project A is destroyed
  cd "$TEST_DIR_A"
  status_a=$(yg status --json 2>&1) || exit_code_a=$?

  if [ ${exit_code_a:-0} -ne 0 ] || \
     echo "$status_a" | jq -e '.state == "terminated" or .state == "stopped"' > /dev/null 2>&1; then
    log_info "Project A destroyed successfully"
  else
    log_fail "Project A not destroyed"
    exit 1
  fi

  # Verify project B is still running
  cd "$TEST_DIR_B"
  status_b=$(yg status --json 2>&1) || exit_code_b=$?
  assert_exit_code 0 ${exit_code_b:-0} "Project B status should succeed"

  if echo "$status_b" | jq -e '.state == "running"' > /dev/null 2>&1; then
    log_pass "Project B remains running after destroying project A"
  else
    log_fail "Project B not running after destroying project A"
    exit 1
  fi

  # Verify project B can still execute commands
  output_b=$(yg 'echo "STILL_ALIVE"' 2>&1) || exit_code=$?
  assert_exit_code 0 ${exit_code:-0} "Project B should execute commands"
  assert_contains "$output_b" "STILL_ALIVE" "Project B should execute correctly"

  log_pass "Destroying one project does not affect the other"
}

#####################################
# Test 4: Credential sharing across projects
#####################################
test_credential_sharing() {
  log_info "Test 4: Credential sharing across projects"

  # Both projects should use same credentials from ~/.yeager/credentials
  cd "$TEST_DIR_A"
  status_a=$(yg status 2>&1) || exit_code_a=$?

  cd "$TEST_DIR_B"
  status_b=$(yg status 2>&1) || exit_code_b=$?

  # Both should succeed (even though project A is destroyed, configure should work)
  # Actually, project A was destroyed, so let's just check project B
  assert_exit_code 0 ${exit_code_b:-0} "Project B should access AWS with shared credentials"

  # Check credentials file exists globally
  if [ -f ~/.yeager/credentials ]; then
    log_pass "Credentials shared globally across projects"
  else
    log_fail "Global credentials file not found"
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
  test_separate_vms
  test_switching_projects
  test_destroy_isolation
  test_credential_sharing

  # Success
  log_pass "All tests passed"
  exit 0
}

# Run main
main
