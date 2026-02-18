# Running the New LiveFire Tests

## Quick Start

### Run All New Tests
```bash
# From project root
go test -tags livefire -v -count=1 -timeout 30m ./test/livefire/... \
  -run "TestLiveFire/(Large_file|\.gitignore|Symlink|Permission|Binary|UTF-8|Deeply_nested|Status)"
```

### Run Only File Sync Tests (8 scenarios)
```bash
LIVEFIRE_TAGS=@sync go test -tags livefire -v -count=1 -timeout 30m ./test/livefire/...
```

### Run Only Status Tests (4 scenarios)
```bash
LIVEFIRE_TAGS=@status go test -tags livefire -v -count=1 -timeout 30m ./test/livefire/...
```

### Run Single Scenario
```bash
# Example: Large file sync
go test -tags livefire -v -count=1 -timeout 30m ./test/livefire/... \
  -run "TestLiveFire/Large_file_sync"
```

## Prerequisites

1. **Build the binary**:
   ```bash
   make build
   ```

2. **AWS Credentials**:
   ```bash
   export AWS_ACCESS_KEY_ID=your_key
   export AWS_SECRET_ACCESS_KEY=your_secret
   # OR use AWS profile
   export AWS_PROFILE=your_profile
   ```

3. **AWS Permissions**: Ensure your IAM user/role has yeager policy attached

## Expected Output

### Successful Run
```
Feature: File Sync Edge Cases

  Scenario: Large file sync (500MB)                    # features/08-file-sync.feature:9
    Given the shared project directory                 # steps_test.go:56 -> theSharedProjectDirectory
    And the VM is running                              # steps_test.go:126 -> theVMIsRunning
    Given I create a 500MB file named "large.bin"      # steps_filesync_test.go:43 -> iCreateLargeFile
    When I run "yg run 'ls -lh large.bin...'"          # steps_test.go:225 -> iRun
    Then the exit code should be 0                     # steps_test.go:272 -> exitCodeShouldBe
    And the output should contain "large.bin"          # steps_test.go:293 -> outputShouldContain

8 scenarios (8 passed)
48 steps (48 passed)
```

## Test Details

### File Sync Tests (08-file-sync.feature)

| # | Scenario | Duration | Cost Impact |
|---|----------|----------|-------------|
| 1 | Large file sync (500MB) | ~60s | Minimal (data transfer) |
| 2 | .gitignore exclusions | ~10s | Minimal |
| 3 | Symlink handling | ~10s | Minimal |
| 4 | Permission preservation | ~10s | Minimal |
| 5 | Binary file sync | ~10s | Minimal |
| 6 | UTF-8 filename handling | ~15s | Minimal |
| 7 | Deeply nested directory | ~10s | Minimal |
| 8 | Large file with checksum | ~30s | Minimal |

**Total estimated time**: ~3-5 minutes (using shared VM)

### Status Tests (10-status.feature)

| # | Scenario | Duration | Cost Impact |
|---|----------|----------|-------------|
| 1 | Status JSON schema | ~5s | None (uses shared VM) |
| 2 | VM transition states | ~120s | Moderate (boots new VM) |
| 3 | Corrupted state file | ~10s | Minimal |
| 4 | No VM provisioned | ~5s | None (no VM) |

**Total estimated time**: ~2-3 minutes

## Cost Optimization

The tests are designed to minimize AWS costs:

1. **Shared VM**: File sync tests use a single shared VM
2. **Sequential execution**: No parallel VMs
3. **Small instance**: Uses smallest instance size
4. **Quick cleanup**: Automatic VM destruction after tests

**Estimated cost per full run**: $0.05 - $0.10 (depending on instance hours)

## Troubleshooting

### Test Hangs on Large File
```bash
# Reduce file size in the feature file temporarily
# Edit features/08-file-sync.feature line 10:
# Change: I create a 500MB file named "large.bin"
# To:     I create a 50MB file named "large.bin"
```

### AWS Timeout
```bash
# Increase timeout
go test -tags livefire -v -count=1 -timeout 60m ./test/livefire/...
```

### VM Not Cleaning Up
```bash
# Manually destroy
cd /tmp/livefire-*
yg destroy --force
```

### Step Not Found Error
```bash
# Recompile tests
go test -tags livefire -c ./test/livefire/
# If compilation fails, check steps_filesync_test.go for syntax errors
```

## Debugging

### Enable Verbose Output
```bash
LIVEFIRE_VERBOSE=1 go test -tags livefire -v -count=1 ./test/livefire/...
```

### Preserve Test Directory
```bash
# Tests clean up automatically. To inspect failure:
# 1. Note the temp directory from test output
# 2. cd to that directory before cleanup
# 3. Run: yg status --json > debug.json
```

### Check VM State During Test
```bash
# In another terminal while test is running:
aws ec2 describe-instances --filters "Name=tag:Name,Values=yeager-*" \
  --query 'Reservations[].Instances[].[InstanceId,State.Name,PublicIpAddress]'
```

## Integration with CI

### GitHub Actions Example
```yaml
- name: Run LiveFire Tests
  run: |
    make build
    go test -tags livefire -v -count=1 -timeout 30m ./test/livefire/... \
      -run "TestLiveFire/(Large_file|Status)"
  env:
    AWS_ACCESS_KEY_ID: ${{ secrets.AWS_ACCESS_KEY_ID }}
    AWS_SECRET_ACCESS_KEY: ${{ secrets.AWS_SECRET_ACCESS_KEY }}
```

### Cost Control in CI
```yaml
# Run only on main branch or tags
on:
  push:
    branches: [main]
    tags: ['v*']
  # Or run nightly
  schedule:
    - cron: '0 2 * * *'  # 2 AM UTC daily
```

## Validation Checklist

Before committing:

- [ ] Tests compile: `go test -tags livefire -c ./test/livefire/`
- [ ] No syntax errors in feature files
- [ ] All 12 scenarios are defined (8 + 4)
- [ ] Step definitions exist for all Gherkin steps
- [ ] Tests follow TESTING_PRINCIPLES.md (no internal imports)
- [ ] Executable permissions set on feature files

## Support

If tests fail:
1. Check IMPLEMENTATION_SUMMARY.md for design details
2. Review TESTING_PRINCIPLES.md for requirements
3. Examine test output for specific failure
4. Verify AWS credentials and permissions
5. Check that `yg` binary is built and in PATH
