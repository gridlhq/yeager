# LiveFire Test Scenarios

## Quick Reference

### Config Behavior Tests (12 scenarios)

```bash
# Run all config behavior tests
LIVEFIRE_TAGS=@config make livefire
```

| # | Scenario | What It Tests | Duration |
|---|----------|--------------|----------|
| 1 | Compute size "small" | t3.small instance provisioning | ~8 min |
| 2 | Compute size "xlarge" | c5.2xlarge/c6i.2xlarge provisioning | ~8 min |
| 3 | Custom region | eu-west-1 region configuration | ~8 min |
| 4 | idle_stop enabled | VM stops after 90s grace period | ~8 min |
| 5 | idle_stop disabled | VM stays running after commands | ~8 min |
| 6 | Setup packages | jq and htop installation | ~10 min |
| 7 | Setup run commands | Shell commands during provisioning | ~8 min |
| 8 | Sync include pattern | Only *.go and *.md files synced | ~8 min |
| 9 | Sync exclude pattern | node_modules/ and *.log excluded | ~8 min |
| 10 | Artifacts path | dist/ and *.zip collection | ~8 min |
| 11 | Default config | VM runs with default settings | ~8 min |

**Total Duration**: ~90 minutes
**Total Cost**: ~$1.20

---

### Artifact Tests (5 scenarios)

```bash
# Run all artifact tests
LIVEFIRE_TAGS=@artifacts make livefire
```

| # | Scenario | What It Tests | Duration |
|---|----------|--------------|----------|
| 1 | Single file artifact | output.txt download | ~7 min |
| 2 | Directory artifact | dist/ directory structure | ~7 min |
| 3 | Glob pattern | **/*.log pattern matching | ~7 min |
| 4 | Large artifact | 100MB file handling | ~8 min |
| 5 | Missing artifact warnings | Graceful warning messages | ~7 min |

**Total Duration**: ~35 minutes
**Total Cost**: ~$0.50

---

## Running Individual Scenarios

### By Name
```bash
go test -tags livefire -v -count=1 -timeout 60m ./test/livefire/... \
  -run "TestLiveFire/Compute_size_\"small\"_provisions_correct_instance_type"
```

### By Feature File
```bash
# Config behavior only
LIVEFIRE_TAGS=@config go test -tags livefire -v -count=1 -timeout 90m ./test/livefire/...

# Artifacts only
LIVEFIRE_TAGS=@artifacts go test -tags livefire -v -count=1 -timeout 40m ./test/livefire/...
```

---

## Scenario Details

### Config: Compute Size "small"

**Purpose**: Verify that `size = "small"` provisions t3.small instance

**Config**:
```toml
[compute]
size = "small"
```

**Assertions**:
- Exit code 0 on `yg up`
- `yg status --json` shows instance_type matches `(t3\.small|t3a\.small)`

**AWS Resources**: 1 t3.small EC2 instance

---

### Config: Compute Size "xlarge"

**Purpose**: Verify that `size = "xlarge"` provisions c5.2xlarge or c6i.2xlarge

**Config**:
```toml
[compute]
size = "xlarge"
```

**Assertions**:
- Exit code 0 on `yg up`
- `yg status --json` shows instance_type matches `(c5\.2xlarge|c6i\.2xlarge|c5n\.2xlarge)`

**AWS Resources**: 1 c5.2xlarge EC2 instance

---

### Config: Custom Region

**Purpose**: Verify region configuration is respected

**Config**:
```toml
[compute]
region = "eu-west-1"
```

**Assertions**:
- Exit code 0 on `yg up`
- `yg status --json` shows availability_zone starts with "eu-west-1"

**AWS Resources**: 1 t3.small EC2 instance in eu-west-1

---

### Config: idle_stop Enabled

**Purpose**: Verify VM stops after grace period expires

**Config**:
```toml
[lifecycle]
idle_stop = true
grace_period = "90s"
```

**Steps**:
1. Start VM with `yg up`
2. Run command `yg run echo test`
3. Wait 120 seconds
4. Check status

**Assertions**:
- `yg status --json` shows state = "stopped"

**AWS Resources**: 1 t3.small EC2 instance (stopped)

---

### Config: idle_stop Disabled

**Purpose**: Verify VM stays running when idle_stop is false

**Config**:
```toml
[lifecycle]
idle_stop = false
```

**Steps**:
1. Start VM with `yg up`
2. Run command `yg run echo test`
3. Wait 150 seconds
4. Check status

**Assertions**:
- `yg status --json` shows state = "running"

**AWS Resources**: 1 t3.small EC2 instance (running)

---

### Config: Setup Packages

**Purpose**: Verify packages are installed during provisioning

**Config**:
```toml
[setup]
packages = ["jq", "htop"]
```

**Assertions**:
- `yg run which jq` returns `/usr/bin/jq`
- `yg run which htop` returns `/usr/bin/htop`

**AWS Resources**: 1 t3.small EC2 instance with packages

---

### Config: Setup Run Commands

**Purpose**: Verify shell commands execute during provisioning

**Config**:
```toml
[setup]
run = ["echo HELLO > /tmp/test.txt", "mkdir -p /tmp/workspace"]
```

**Assertions**:
- `yg run cat /tmp/test.txt` contains "HELLO"
- `yg run ls -d /tmp/workspace` succeeds

**AWS Resources**: 1 t3.small EC2 instance

---

### Config: Sync Include Pattern

**Purpose**: Verify include patterns filter synced files

**Config**:
```toml
[sync]
include = ["*.go", "*.md"]
```

**Setup Files**:
- test.py
- README.md
- main.go

**Assertions**:
- `yg run test -f main.go` → success (file exists)
- `yg run test -f README.md` → success (file exists)
- `yg run test -f test.py` → failure (file not synced)

**AWS Resources**: 1 t3.small EC2 instance

---

### Config: Sync Exclude Pattern

**Purpose**: Verify exclude patterns prevent file sync

**Config**:
```toml
[sync]
exclude = ["node_modules/", "*.log"]
```

**Setup Files**:
- node_modules/ directory
- debug.log

**Assertions**:
- `yg run test -d node_modules` → failure (not synced)
- `yg run test -f debug.log` → failure (not synced)

**AWS Resources**: 1 t3.small EC2 instance

---

### Config: Artifacts Path

**Purpose**: Verify artifacts are collected after commands

**Config**:
```toml
[artifacts]
path = ["dist/", "*.zip"]
```

**Assertions**:
- Local file `artifacts/dist/app.txt` exists with "BUILT"
- Local file `artifacts/output.zip` exists with "ZIP"

**AWS Resources**: 1 t3.small EC2 instance

---

### Artifact: Single File

**Purpose**: Verify single file artifact download

**Config**:
```toml
[artifacts]
path = ["output.txt"]
```

**Assertions**:
- Local file `artifacts/output.txt` exists
- Contains "RESULT"

**AWS Resources**: 1 t3.small EC2 instance

---

### Artifact: Directory

**Purpose**: Verify directory structure is preserved

**Config**:
```toml
[artifacts]
path = ["dist/"]
```

**Assertions**:
- `artifacts/dist/file.txt` exists with "OK"
- `artifacts/dist/other.txt` exists with "TWO"

**AWS Resources**: 1 t3.small EC2 instance

---

### Artifact: Glob Pattern

**Purpose**: Verify glob patterns match nested files

**Config**:
```toml
[artifacts]
path = ["**/*.log"]
```

**Assertions**:
- `artifacts/a/test.log` exists with "LOG1"
- `artifacts/a/b/debug.log` exists with "LOG2"

**AWS Resources**: 1 t3.small EC2 instance

---

### Artifact: Large File

**Purpose**: Verify large file (100MB) handling

**Config**:
```toml
[artifacts]
path = ["large.bin"]
```

**Assertions**:
- `artifacts/large.bin` exists
- File size = 104857600 bytes (100MB)

**AWS Resources**: 1 t3.small EC2 instance

---

### Artifact: Missing Warnings

**Purpose**: Verify missing artifacts show warnings without failing

**Config**:
```toml
[artifacts]
path = ["nonexistent.txt", "also-missing.zip"]
```

**Assertions**:
- Exit code 0 (warnings, not errors)
- Output contains "warning" or "not found" or "missing"

**AWS Resources**: 1 t3.small EC2 instance

---

## Debugging Failed Tests

### View full output
```bash
go test -tags livefire -v -count=1 -timeout 60m ./test/livefire/... \
  -run "TestLiveFire/Compute_size" 2>&1 | tee test.log
```

### Check VM state manually
```bash
# After test failure, check state file
cat /tmp/livefire-*/. yeager/state.json

# Check AWS console for lingering resources
aws ec2 describe-instances --filters "Name=tag:yeager,Values=true"
```

### Clean up manually
```bash
# If test leaves resources behind
cd /tmp/livefire-*
yg destroy --force
```

---

## Cost Breakdown

| Resource | Unit Cost | Duration | Cost per Scenario |
|----------|-----------|----------|-------------------|
| t3.small EC2 | $0.0208/hr | 8 min | $0.028 |
| c5.2xlarge EC2 | $0.34/hr | 8 min | $0.045 |
| Data transfer | $0.09/GB | ~100MB | $0.009 |
| EBS storage | $0.10/GB-month | 8GB × 8min | $0.001 |

**Average per scenario**: ~$0.10
**Full suite (17 scenarios)**: ~$1.70
