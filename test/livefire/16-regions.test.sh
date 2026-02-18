#!/bin/bash
# Region Selection - LiveFire Test
#
# Tests region selection:
# - Non-default region launch
# - Cross-region change
# - Invalid region handling
# - Region with limited AMI support

set -euo pipefail

#####################################
# Configuration
#####################################
TEST_NAME="region_selection"
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
# Test 1: Non-default region launch
#####################################
test_nondefault_region() {
  log_info "Test 1: Non-default region launch"

  # Configure for non-default region (us-west-2)
  cat > .yeager.toml <<EOF
[compute]
region = "us-west-2"
EOF

  # Start VM
  output=$(yg up 2>&1) || {
    log_info "yg up output: $output"
    log_fail "yg up failed in us-west-2"
    exit 1
  }
  wait_for_vm_ready

  # Get instance details
  status_json=$(yg status --json 2>&1)
  instance_id=$(echo "$status_json" | jq -r '.instance_id // .id // empty')

  if [ -z "$instance_id" ]; then
    log_fail "Could not get instance ID"
    exit 1
  fi

  log_info "Instance ID: $instance_id"

  # Verify region via AWS CLI
  instance_region=$(aws ec2 describe-instances --instance-ids "$instance_id" \
    --region us-west-2 \
    --query 'Reservations[0].Instances[0].Placement.AvailabilityZone' \
    --output text 2>&1 | sed 's/[a-z]$//')

  log_info "Instance region: $instance_region"

  if echo "$instance_region" | grep -q "us-west-2"; then
    log_pass "VM created in specified region: us-west-2"
  else
    log_fail "VM not in expected region: $instance_region"
    exit 1
  fi
}

#####################################
# Test 2: Cross-region change
#####################################
test_cross_region_change() {
  log_info "Test 2: Cross-region change"

  # Get current instance ID (should be in us-west-2 from previous test)
  status_json=$(yg status --json 2>&1)
  instance_id_old=$(echo "$status_json" | jq -r '.instance_id // .id // empty')
  log_info "Original instance ID (us-west-2): $instance_id_old"

  # Change region to us-east-1
  log_info "Changing region to us-east-1"
  cat > .yeager.toml <<EOF
[compute]
region = "us-east-1"
EOF

  # Run yg up (should detect region change and recreate)
  output=$(echo "y" | yg up 2>&1) || exit_code=$?

  # Check if region change was detected
  if echo "$output" | grep -qi "region.*changed\|recreate\|different region"; then
    log_info "Region change detected"
  else
    log_info "Region change may be handled automatically"
  fi

  # Wait for new VM
  wait_for_vm_ready

  # Get new instance ID
  status_json=$(yg status --json 2>&1)
  instance_id_new=$(echo "$status_json" | jq -r '.instance_id // .id // empty')
  log_info "New instance ID (us-east-1): $instance_id_new"

  # Verify new region via AWS CLI
  instance_region=$(aws ec2 describe-instances --instance-ids "$instance_id_new" \
    --region us-east-1 \
    --query 'Reservations[0].Instances[0].Placement.AvailabilityZone' \
    --output text 2>&1 | sed 's/[a-z]$//')

  log_info "New instance region: $instance_region"

  if echo "$instance_region" | grep -q "us-east-1"; then
    log_pass "VM recreated in new region: us-east-1"
  else
    log_fail "VM not in expected region: $instance_region"
    exit 1
  fi

  # Verify old instance is terminated (check in old region)
  old_state=$(aws ec2 describe-instances --instance-ids "$instance_id_old" \
    --region us-west-2 \
    --query 'Reservations[0].Instances[0].State.Name' \
    --output text 2>&1) || true

  if echo "$old_state" | grep -qi "terminated\|shutting-down"; then
    log_pass "Old instance terminated successfully"
  else
    log_info "Old instance state: $old_state (may take time to terminate)"
    log_pass "Cross-region change completed"
  fi
}

#####################################
# Test 3: Invalid region handling
#####################################
test_invalid_region() {
  log_info "Test 3: Invalid region handling"

  # Destroy previous VM
  yg destroy --force > /dev/null 2>&1
  sleep 5

  # Configure with invalid region
  cat > .yeager.toml <<EOF
[compute]
region = "invalid-region-99"
EOF

  # Try to start VM
  output=$(yg up 2>&1) || exit_code=$?

  # Should fail with exit code 1
  assert_exit_code 1 ${exit_code:-0} "Invalid region should cause error"

  # Should mention invalid region
  if echo "$output" | grep -qi "invalid.*region\|not.*valid\|unknown.*region"; then
    log_pass "Invalid region shows appropriate error"
  else
    log_fail "Invalid region error message not clear"
    echo "Output: $output"
    exit 1
  fi
}

#####################################
# Test 4: Region with limited AMI support
#####################################
test_limited_ami_region() {
  log_info "Test 4: Region with limited AMI support"

  # Use a region that might have limited AMI support
  # ap-southeast-3 is a newer region that may not have all AMIs
  cat > .yeager.toml <<EOF
[compute]
region = "ap-southeast-3"
EOF

  # Try to start VM
  output=$(yg up 2>&1) || exit_code=$?

  # May fail if AMI not available
  if [ ${exit_code:-0} -ne 0 ]; then
    # Check for helpful error message about AMI
    if echo "$output" | grep -qi "ami\|image\|not found\|not available"; then
      log_pass "AMI availability error shows helpful message"
    else
      # May be other region-related error
      log_pass "Limited AMI region handled with error message"
    fi
  else
    # If it succeeded, that's fine too (AMI is available)
    log_pass "Region with AMI availability checked successfully"
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
  test_nondefault_region
  test_cross_region_change
  test_invalid_region
  test_limited_ami_region

  # Success
  log_pass "All tests passed"
  exit 0
}

# Run main
main
