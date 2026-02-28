# Masterblaster - `mb`

Create, manage, and orchestrate AI agents sandboxes.

<img width="1108" height="540" alt="Screenshot 2026-02-03 at 10 09 29 PM" src="./mb-splash.png" />

Masterblaster boots [stereOS](https://stereos.ai) machines - a
NixOS-based Linux distribution purpose-built for AI agents - and manages their
full lifecycle: image pulling, machine creation, secret injection, shared directory
mounting, and agent orchestration.

Sandboxes are ephemeral, isolated, reproducible, and focused on security.

## Install

```bash
curl -fsSL https://mb.stereos.ai/latest/install.sh | bash
```

## Quick start

### 1. Pull a mixtape

Mixtapes are pre-built stereOS images bundled with AI agent harnesses. Pull one
from the registry:

```bash
mb pull opencode-mixtape
```

### 2. Create a jcard

A `jcard.toml` file defines your sandbox configuration. Run `mb init` to
generate one, or create it by hand:

```toml
# jcard.toml

mixtape = "opencode-mixtape:latest"

[resources]
cpus   = 1
memory = "4GiB"
disk   = "40GiB"

[network]
mode = "nat"

[[shared]]
host  = "./"
guest = "/home/agent/workspace"

[secrets]
ANTHROPIC_API_KEY = "${ANTHROPIC_API_KEY}"

[agent]
harness = "opencode"
prompt  = "review the code in this directory and fix any failing tests"
workdir = "/home/agent/workspace"
restart = "on-failure"
```

### 3. Launch

```bash
mb up            # Create sandbox from jcard.tomlmb ssh my-sandbox
mb ssh           # SSH into a running sandbox
mb list          # List all sandboxes
mb status        # Check sandbox status
mb down          # Graceful shutdown
mb destroy       # Remove sandbox and all resources
```

## jcard.toml reference

The jcard is the single configuration file for a sandbox.
Most fields are optional and defaults are applied automatically.


| Section | Field | Default | Description |
|---------|-------|---------|-------------|
| *(top)* | `mixtape` | `"base:latest"` | Mixtape image in `name:tag` format |
| *(top)* | `mixtape_digest` | | Pin to an exact OCI digest |
| *(top)* | `name` | parent directory name | Unique sandbox name |
| `[resources]` | `cpus` | `2` | Virtual CPUs |
| `[resources]` | `memory` | `"4GiB"` | RAM (KiB, MiB, GiB) |
| `[resources]` | `disk` | `"20GiB"` | Root disk size |
| `[network]` | `mode` | `"nat"` | `"nat"`, `"bridged"`, or `"none"` |
| `[network]` | `forwards` | | Port forwards: `[{ host, guest, proto }]` |
| `[network]` | `egress_allow` | | Domain/CIDR allowlist for outbound traffic |
| `[[shared]]` | `host` | | Host directory path |
| `[[shared]]` | `guest` | | Guest mount point |
| `[[shared]]` | `readonly` | `false` | Prevent agent from modifying host files |
| `[secrets]` | *key = value* | | Injected to tmpfs via stereosd (never on disk) |
| `[agent]` | `type` | `"sandboxed"` | `"sandboxed"` (gVisor container) or `"native"` (tmux session) |
| `[agent]` | `harness` | `"claude-code"` | `"claude-code"`, `"opencode"`, `"gemini-cli"`, `"custom"` |
| `[agent]` | `prompt` | | Prompt given to the agent on boot |
| `[agent]` | `prompt_file` | | Path to a prompt file (relative to jcard.toml) |
| `[agent]` | `workdir` | first `[[shared]]` guest path or `"/workspace"` | Agent working directory |
| `[agent]` | `restart` | `"no"` | `"no"`, `"on-failure"`, `"always"` |
| `[agent]` | `max_restarts` | `0` (unlimited) | Max restart attempts |
| `[agent]` | `timeout` | | Agent timeout (e.g. `"2h"`) |
| `[agent]` | `grace_period` | `"30s"` | SIGTERM grace period before SIGKILL |
| `[agent]` | `extra_packages` | | Nix packages to install (sandboxed agents only) |
| `[agent]` | `env` | | Environment variables for the agent process |

Use `${ENV_VAR}` syntax in `[secrets]`, `[agent.env]`, and path fields to
reference host environment variables. Paths support `~` expansion.

## CLI commands

```
mb pull <name[:tag]>       Pull a mixtape from the registry
mb init                    Generate a jcard.toml
mb up [--config <path>]    Create and start a sandbox
mb down [name] [--force]   Stop a running sandbox
mb status [name]           Show sandbox status
mb destroy [name] [--yes]  Remove sandbox and all resources
mb ssh [name] [-u user]    SSH into a sandbox (default user: admin)
mb list                    List all sandboxes
mb mixtapes ls             List locally available mixtapes
mb version                 Display version info
mb serve                   Start the daemon (auto-started by mb up)
```

Global flags: `--config-dir <path>`, `--verbose`

## Backends


| Backend | Platform | Accelerator | Control plane | Notes |
|---------|----------|-------------|---------------|-------|
| QEMU | macOS/Apple Silicon | HVF | TCP (user-mode net) | Default backend |
| QEMU | Linux/aarch64 | KVM | vsock (`vhost-vsock-pci`) | Native vsock, `io_uring` disk I/O |
| Apple Virt | macOS/Apple Silicon | Vz.framework | virtio-socket | `MB_BACKEND=applevirt`, requires codesigning |

