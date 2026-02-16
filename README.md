# 🛩️ Yeager

[![Beta](https://img.shields.io/badge/beta-orange?style=for-the-badge)](#limitations)
[![CI](https://img.shields.io/github/actions/workflow/status/gridlhq/yeager/test.yml?branch=main&label=CI&style=flat-square)](https://github.com/gridlhq/yeager/actions/workflows/test.yml)
[![Release](https://img.shields.io/github/v/release/gridlhq/yeager?include_prereleases&style=flat-square&label=release)](https://github.com/gridlhq/yeager/releases)

Prefix any command with `yg`. It runs on a cloud VM instead of your laptop.

```
$ yg cargo test
yeager | syncing 3 files...
yeager | running: cargo test
yeager | ─────────────────────────────────────────────

test result: ok. 42 passed; 0 failed; 0 ignored

yeager | ─────────────────────────────────────────────
yeager | exit 0 — 14s
```

No setup. No config files. No Docker. No git.

## Install

```bash
curl -fsSL https://yeager.sh/install | sh
```

<details>
<summary>npm or Homebrew</summary>

```bash
npm install -g yeager
```

```bash
brew install yeager
```

</details>

Needs AWS credentials — the same ones your `aws` CLI uses.

## First run

```bash
yg cargo test
```

That's it. Yeager:

1. Detects your language from manifest files (Cargo.toml, package.json, go.mod, etc.)
2. Launches an ARM64 EC2 instance with the right toolchain
3. Syncs your project via rsync
4. Runs the command, streams output back in real time

The VM persists. Caches, tools, build artifacts accumulate across runs. Auto-stops after 10 min idle, auto-starts on next `yg`.

## Usage

```bash
yg <any command>         # run on the VM
yg status                # what's running
yg logs                  # replay last completed run
yg stop                  # stop VM (restarts instantly)
yg destroy               # tear it down
```

Multiple commands run concurrently from different terminals.

## VM sizes

| Size | vCPU | RAM | $/hr |
|---|---|---|---|
| `small` | 2 | 4 GB | ~$0.02 |
| `medium` | 4 | 8 GB | ~$0.03 |
| `large` | 8 | 16 GB | ~$0.07 |
| `xlarge` | 16 | 32 GB | ~$0.13 |

ARM64 Graviton. Default: `medium`.

## Config

Zero config works for most projects. Optional `.yeager.toml` for the rest:

```toml
[compute]
size = "large"           # small | medium | large | xlarge

[setup]
packages = ["libpq-dev"]

[sync]
exclude = ["data/"]

[artifacts]
paths = ["coverage/"]
```

`yg init` generates a commented config with every option.

## All commands

| Command | |
|---|---|
| `yg <cmd>` | Run on VM (creates it if needed) |
| `yg status` | VM state + active commands |
| `yg logs [run-id]` | Replay completed run output |
| `yg logs --tail N` | Last N lines |
| `yg kill [run-id]` | Cancel a command |
| `yg stop` | Stop VM (keeps disk, no cost) |
| `yg destroy` | Terminate + clean up |
| `yg up` | Boot VM without running anything |
| `yg init` | Generate `.yeager.toml` |

## Under the hood

Single Go binary, ~15 MB. Direct AWS SDK — no Terraform, no CloudFormation. rsync over SSH for file sync. EC2 Instance Connect with ephemeral Ed25519 keys generated in memory, never written to disk. One EC2 instance per project, shared S3 bucket for output.

## Limitations

Beta. It works, but:

- **Ctrl+C kills the remote command.** No disconnect resilience yet — network drops or closing your laptop kill it too. tmux wrapping is next.
- **Can't re-attach to running commands.** `yg logs` only replays completed runs from S3.
- **Setup changes require `yg destroy`.** Changing `[setup]` means losing cached build artifacts.
- **AWS only.** GCP and Azure planned.

## License

MIT
