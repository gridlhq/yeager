# 🛩️ Yeager

[![CI](https://github.com/gridlhq/yeager/actions/workflows/test.yml/badge.svg)](https://github.com/gridlhq/yeager/actions/workflows/test.yml)
[![Release](https://github.com/gridlhq/yeager/actions/workflows/release.yml/badge.svg)](https://github.com/gridlhq/yeager/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Status: Beta](https://img.shields.io/badge/Status-Beta-orange)](https://github.com/gridlhq/yeager)

Prefix any command with `yg`. It runs on a cloud VM instead of your laptop.

## Quickstart

```bash
curl -fsSL https://yeager.sh/install | sh
yg pytest
```

That's it. First run takes ~2 min (VM creation). After that, instant.

---

## How it works

Detects language from manifest files (Cargo.toml, package.json, go.mod, etc.), launches an ARM64 EC2 instance with the right toolchain, syncs via rsync, runs the command, streams output back.

VM persists across runs — caches and build artifacts carry over. Auto-stops after 10 min idle, auto-starts on next `yg`.

**Ctrl+C detaches, doesn't kill.** Use `yg logs` to re-attach or `yg kill` to cancel.

## Prerequisites

- macOS or Linux
- rsync (pre-installed on macOS, `apt install rsync` on Linux)
- AWS credentials (`aws configure`)

Creates EC2 instances, an S3 bucket, and a security group in your account.

<details>
<summary>Minimum IAM permissions</summary>

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "ec2:RunInstances", "ec2:DescribeInstances", "ec2:StartInstances",
        "ec2:StopInstances", "ec2:TerminateInstances", "ec2:CreateSecurityGroup",
        "ec2:DescribeSecurityGroups", "ec2:AuthorizeSecurityGroupIngress",
        "ec2:CreateTags", "ec2:DescribeImages"
      ],
      "Resource": "*"
    },
    {
      "Effect": "Allow",
      "Action": [
        "s3:CreateBucket", "s3:PutBucketLifecycleConfiguration",
        "s3:HeadBucket", "s3:PutObject", "s3:GetObject"
      ],
      "Resource": ["arn:aws:s3:::yeager-*", "arn:aws:s3:::yeager-*/*"]
    },
    {
      "Effect": "Allow",
      "Action": ["ec2-instance-connect:SendSSHPublicKey"],
      "Resource": "*"
    },
    {
      "Effect": "Allow",
      "Action": ["sts:GetCallerIdentity"],
      "Resource": "*"
    }
  ]
}
```

IAM → Users → Create user → Programmatic access → attach the above policy → `aws configure` with new creds.

</details>

## Install

```bash
curl -fsSL https://yeager.sh/install | sh
```

## Usage

```bash
yg <any command>         # run on the VM
yg status                # what's running
yg logs                  # replay + stream last run
yg logs --tail 50        # last 50 lines, then stream
yg kill                  # cancel a running command
yg stop                  # stop VM (no cost when stopped)
yg destroy               # tear it down
yg up                    # boot VM without running anything
yg init                  # generate .yeager.toml
```

Multiple commands run concurrently from different terminals.

## VM sizes

| Size | vCPU | RAM | $/hr |
|---|---|---|---|
| `small` | 2 | 4 GB | ~$0.02 |
| `medium` | 4 | 8 GB | ~$0.03 |
| `large` | 8 | 16 GB | ~$0.07 |
| `xlarge` | 16 | 32 GB | ~$0.13 |

ARM64 Graviton. Default: `medium`. Typical 2-hour session: ~$0.07.

## Config

Zero config by default. Optional `.yeager.toml`:

```toml
[compute]
size = "large"

[setup]
packages = ["libpq-dev"]

[sync]
exclude = ["data/"]

[artifacts]
paths = ["coverage/"]
```

`yg init` generates a commented config with every option.

## Troubleshooting

**AWS creds:** `aws configure` or set `AWS_ACCESS_KEY_ID` + `AWS_SECRET_ACCESS_KEY`.

**rsync:** `apt install rsync` (Linux) or `brew install rsync` (macOS).

**Missing deps:** Add to `.yeager.toml` under `[setup] packages`, then `yg destroy && yg up`.

**Debug:** `yg --verbose <command>`. First boot takes 2-3 min (cloud-init installing toolchains).

## Limitations

Beta.

- Setup changes require `yg destroy`
- AWS only (GCP/Azure planned)
- macOS/Linux only (Windows planned)
- No team features yet

## Under the hood

Single Go binary, ~15 MB. Direct AWS SDK — no Terraform, no CloudFormation. rsync over SSH. EC2 Instance Connect with ephemeral Ed25519 keys (never on disk). One instance per project dir. tmux for disconnect resilience.

## License

MIT
