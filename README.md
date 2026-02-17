# Masterblaster - `mb`

Create, manage, orchestrate sandboxed Linux VMs for AI agents.

<img width="1108" height="540" alt="Screenshot 2026-02-03 at 10 09 29 PM" src="./mb-splash.png" />

## Quick start

### 1. Download the Fedora Cloud base image

```bash
mkdir -p ~/.mb/images
curl -L -o ~/.mb/images/fedora-42-aarch64.qcow2 \
  'https://download.fedoraproject.org/pub/fedora/linux/releases/42/Cloud/aarch64/images/Fedora-Cloud-Base-Generic-42-1.1.aarch64.qcow2'
```

### 2. Sample config file

```toml
schema_version = "0"

[vm]
name = "my-agent"
cpus = 4
memory = "8G"
disk_size = "40G"

[ssh]
user = "agent"
public_key_file = "~/.ssh/id_ed25519.pub"
identity_file = "~/.ssh/id_ed25519"

[environment]
ANTHROPIC_API_KEY = "${ANTHROPIC_API_KEY}"
```

### 3. Launch

```bash
mb init ./my-agent.toml   # Create VM, boot, wait for SSH to be ready
mb ssh my-agent           # SSH into a VM
mb list                   # See VMs
mb stop my-agent          # Graceful shutdown
mb rm my-agent            # Delete VM and all resources
```
