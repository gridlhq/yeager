#!/bin/bash
# Dry-run verification that the test is properly structured
# This doesn't require AWS credentials, just checks test logic

set -euo pipefail

echo "=== Verifying 19-no-aws-cli.test.sh structure ==="
echo ""

# Check test file exists
if [ ! -f "test/livefire/19-no-aws-cli.test.sh" ]; then
  echo "❌ FAIL: Test file not found"
  exit 1
fi
echo "✓ Test file exists"

# Check it's executable
if [ ! -x "test/livefire/19-no-aws-cli.test.sh" ]; then
  echo "❌ FAIL: Test file not executable"
  exit 1
fi
echo "✓ Test file is executable"

# Check syntax
if ! bash -n test/livefire/19-no-aws-cli.test.sh; then
  echo "❌ FAIL: Syntax error in test"
  exit 1
fi
echo "✓ Syntax is valid"

# Check for required functions
required_functions=(
  "hide_aws_cli"
  "restore_aws_cli"
  "check_real_aws_cli"
  "test_configure_without_aws_cli"
  "test_vm_up_without_aws_cli"
  "test_status_without_aws_cli"
  "test_run_without_aws_cli"
  "test_destroy_without_aws_cli"
  "test_graceful_aws_cli_detection"
)

for func in "${required_functions[@]}"; do
  if ! grep -q "^${func}()" test/livefire/19-no-aws-cli.test.sh; then
    echo "❌ FAIL: Missing function: $func"
    exit 1
  fi
  echo "✓ Function defined: $func"
done

# Check that test hides AWS CLI in PATH
if ! grep -q 'export PATH="\$fake_bin:\$PATH"' test/livefire/19-no-aws-cli.test.sh; then
  echo "❌ FAIL: Test doesn't hide AWS CLI in PATH"
  exit 1
fi
echo "✓ Test hides AWS CLI in PATH"

# Check that test restores PATH
if ! grep -q 'export PATH="\$ORIGINAL_PATH"' test/livefire/19-no-aws-cli.test.sh; then
  echo "❌ FAIL: Test doesn't restore original PATH"
  exit 1
fi
echo "✓ Test restores original PATH"

# Check for cleanup trap
if ! grep -q 'trap cleanup EXIT' test/livefire/19-no-aws-cli.test.sh; then
  echo "❌ FAIL: Missing cleanup trap"
  exit 1
fi
echo "✓ Cleanup trap registered"

# Verify test uses yg status --json (not aws CLI)
if ! grep -q 'yg status --json' test/livefire/19-no-aws-cli.test.sh; then
  echo "❌ FAIL: Test doesn't use 'yg status --json'"
  exit 1
fi
echo "✓ Test uses 'yg status --json' for verification"

# Check that test doesn't have hard dependency on aws CLI
aws_cli_calls=$(grep -c 'aws ec2\|aws sts' test/livefire/19-no-aws-cli.test.sh || true)
if [ "$aws_cli_calls" -gt 2 ]; then
  echo "⚠️  WARNING: Test has $aws_cli_calls calls to aws CLI (should be minimal or none)"
else
  echo "✓ Test doesn't depend on aws CLI ($aws_cli_calls conditional calls)"
fi

echo ""
echo "=== Test Structure Verification: PASSED ==="
echo ""
echo "Test is ready to run with:"
echo "  export AWS_ACCESS_KEY_ID=xxx"
echo "  export AWS_SECRET_ACCESS_KEY=xxx"
echo "  ./test/livefire/19-no-aws-cli.test.sh"
echo ""
echo "The test will:"
echo "  1. Hide aws CLI from PATH"
echo "  2. Verify all yg commands work without it"
echo "  3. Verify status through 'yg status --json' only"
echo "  4. Restore PATH and clean up"
