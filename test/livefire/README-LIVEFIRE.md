# LiveFire Test Suite

## Overview

LiveFire tests validate the Yeager CLI against real AWS infrastructure with zero mocking. These tests simulate manual QA testing by running `yg` as a subprocess and verifying behavior through CLI output, file inspection, and AWS CLI queries.

## Test Files

### 01-configure.test.sh (8 scenarios)
Tests credential configuration and management:
1. First-time credential setup via CLI flags
2. Configure with invalid credentials format
3. Configure validates credentials against AWS
4. Configure with environment variables
5. Configure with custom region
6. Overwrite existing credentials
7. Configure shows current config status
8. Configure from AWS profile

**Runtime**: ~30 seconds (no AWS resources created)

### 02-grace-period.test.sh (7 scenarios)
Tests auto-stop daemon and grace period behavior:
1. VM auto-stops after grace period expires
2. VM stays warm during grace period
3. Multiple commands within grace period don't reset timer
4. Custom grace period config is respected
5. Grace period disabled stops VM immediately
6. Daemon survives SSH disconnects
7. Grace period daemon logs are accessible

**Runtime**: ~15-20 minutes (creates/stops multiple VMs)

## Running Tests

### Prerequisites
- AWS credentials configured: `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`
- `yg` binary in PATH
- `aws` CLI installed and configured
- Yeager IAM policy attached to credentials

### Run Individual Test
```bash
# Configure tests (fast, no AWS resources)
./test/livefire/01-configure.test.sh

# Grace period tests (slow, creates real VMs)
./test/livefire/02-grace-period.test.sh
```

### Run All Tests
```bash
cd test/livefire
for test in *.test.sh; do
  echo "Running $test..."
  ./"$test" || exit 1
done
```

### Debugging Failed Tests
Set `CLEANUP_ON_EXIT=false` in the script to preserve test artifacts:
```bash
# Edit script and change:
CLEANUP_ON_EXIT=false

# Or modify on the fly:
sed -i '' 's/CLEANUP_ON_EXIT=true/CLEANUP_ON_EXIT=false/' 01-configure.test.sh
./01-configure.test.sh
```

## Testing Principles

These tests follow strict manual QA simulation rules:

### ✅ ALWAYS
- Run `yg` as subprocess
- Parse stdout/stderr/exit codes
- Verify with file inspection (`cat`, `grep`, `test -f`)
- Use AWS CLI for verification (`aws ec2 describe-instances`)
- Use real AWS resources
- Clean up resources after tests

### ❌ NEVER
- Import internal Go packages
- Call Yeager functions directly
- Use AWS SDK in tests
- Mock AWS responses
- Inspect internal state/memory

## Cost Management

Grace period tests create real EC2 instances which cost money:
- Each test creates a fresh VM (~$0.03-0.10 per hour)
- Tests automatically terminate VMs on completion
- Total cost per full run: ~$0.50-1.00
- Use `CLEANUP_ON_EXIT=true` (default) to ensure cleanup

## Test Output

Tests use colored output:
- 🟢 `[PASS]` - Test passed
- 🔴 `[FAIL]` - Test failed (shows test dir and instance ID)
- 🟡 `[INFO]` - Informational message

Example:
```
[INFO] Starting LiveFire test suite: Configure Command
[INFO] Test 1: First-time credential setup via CLI flags
[PASS] Test 1: First-time credential setup via CLI flags
[INFO] Test 2: Configure with invalid credentials format
[PASS] Test 2: Configure with invalid credentials format
...
[PASS] All 8 configure tests passed
```

## Verification Methods

### Configuration Verification
```bash
# Check credentials file
cat ~/.aws/credentials | grep aws_access_key_id

# Run configure and parse output
yg configure --help
```

### VM State Verification
```bash
# Via yg CLI
yg status | grep -E "(running|stopped)"

# Via AWS CLI
aws ec2 describe-instances --instance-ids i-xxx \
  --query 'Reservations[0].Instances[0].State.Name'
```

### Grace Period Verification
```bash
# Check config
cat .yeager.toml | grep grace_period

# Monitor VM state over time
while true; do
  aws ec2 describe-instances --instance-ids i-xxx \
    --query 'Reservations[0].Instances[0].State.Name'
  sleep 5
done
```

## Troubleshooting

### Test hangs or times out
- Check AWS credentials are valid: `aws sts get-caller-identity`
- Verify VMs can be created: `aws ec2 describe-instances`
- Check AWS service health in your region

### VM not cleaning up
- Manually terminate: `yg destroy --force`
- Find orphaned instances: `aws ec2 describe-instances --filters "Name=tag:yeager,Values=true"`
- Terminate via AWS CLI: `aws ec2 terminate-instances --instance-ids i-xxx`

### Permission errors
- Verify IAM policy attached: See `yg configure` for required permissions
- Check policy includes: EC2, S3, STS, EC2 Instance Connect

## Adding New Tests

1. Copy `test-template.sh` from `_dev/TEST_COVERAGE_EXPANSION/`
2. Follow the pattern: setup → execute → verify → cleanup
3. Use assertion helpers: `assert_exit_code`, `assert_contains`, `assert_file_exists`
4. Always clean up AWS resources in `cleanup()` function
5. Test the "Golden Question": Could a QA tester verify this with only CLI access?

## References

- Testing Principles: `_dev/TEST_COVERAGE_EXPANSION/TESTING_PRINCIPLES.md`
- Test Template: `_dev/TEST_COVERAGE_EXPANSION/test-template.sh`
- Spec Documents: `_dev/TEST_COVERAGE_EXPANSION/*.spec.md`
