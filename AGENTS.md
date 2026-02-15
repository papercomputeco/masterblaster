# Agents

Guidelines for AI coding agents working on the Masterblaster codebase.

## Project overview

Masterblaster (`mb`) is a Go CLI and daemon for managing StereOS-based
AI agent sandbox VMs. It targets aarch64 guests using HVF acceleration on
Apple Silicon Macs (QEMU today, Apple Virtualization.framework in the future).

The tool communicates with `stereosd` inside StereOS guests over vsock for
secret injection, shared directory mounting, health monitoring, and graceful
shutdown. Agent harnesses (Claude Code, OpenCode, Gemini CLI) are managed by
`agentd` inside the guest.

See SPEC.md for the full RFC specification.

## Build and test

```bash
make build        # Produces ./build/mb binary
make test         # Runs go test ./...
make clean        # Removes the build/ directory
make fmt          # Formats code
make vet          # Runs go vet
```

Go 1.25+ is required. The Nix flake (`flake.nix`) provides a reproducible dev
environment with QEMU, Go, and build tools. Use `nix develop` or direnv.

## Architecture

The codebase follows the daemon + CLI client pattern:

```
mb CLI  --[JSON-RPC over $config-dir/mb.sock]--> mb daemon (mb serve)
  daemon --[QMP unix socket]-------------------> QEMU process
  daemon --[vsock CID:3:1024]------------------> stereosd (guest)
    stereosd --[tmpfs/unix socket]-------------> agentd (guest)
```

The default config directory is `$XDG_CONFIG_HOME/mb` (falls back to
`~/.config/mb`). It can be overridden via `--config-dir` flag, `MB_CONFIG_DIR`
env var, or `config-dir` in `config.toml`. Precedence: flag > env > config
file > default.

### Key packages

- **`main.go`** -- Thin shim entrypoint. Creates the root command (`NewMbCmd()`),
  registers persistent flags (`--verbose`, `--config-dir`), and wires in all
  subcommands. A `PersistentPreRunE` hook calls `mbconfig.Init()` to
  initialize Viper before any subcommand runs.

- **`pkg/mbconfig/`** -- Centralizes config directory resolution using Viper.
  `Init(cmd)` binds CLI flags, sets the `MB_` env prefix (so `MB_CONFIG_DIR`
  and `MB_VERBOSE` work), applies XDG defaults, and reads an optional
  `config.toml` from the resolved config dir. Exposes `ConfigDir()` and
  `Verbose()` for use by subcommands.

- **`cmd/<name>/`** -- Each subcommand gets its own package directory following
  the `New<Name>Cmd()` factory pattern. Packages: `serve`, `init`, `up`,
  `down`, `status`, `destroy`, `ssh`, `list`, `mixtapes`. Commands are thin
  wrappers that delegate to the daemon via `pkg/client/`.

- **`pkg/config/`** -- jcard.toml config parsing, validation, and defaults.
  `config.go` defines `JcardConfig` and `Load()`. `expand.go` handles
  `${ENV_VAR}` expansion. `marshal.go` provides TOML serialization and the
  default jcard.toml template. Always use `Load()` to get a validated config.

- **`pkg/daemon/`** -- The long-lived Masterblaster daemon. `daemon.go` defines
  the `Daemon` struct with a `sync.RWMutex`-protected VM map, backend, and
  unix socket listener. `rpc.go` defines the JSON-RPC wire format (`Request`,
  `Response`, `SandboxInfo`). The daemon manages `$config-dir/mb.sock` for CLI
  communication and `$config-dir/daemon.pid` for liveness.

- **`pkg/client/`** -- Thin JSON-RPC client for CLI commands to talk to the
  daemon. Provides `Up()`, `Down()`, `Status()`, `Destroy()`, `List()`.

- **`pkg/vm/`** -- VM lifecycle management.
  - `backend.go` defines the `Backend` interface (`Up`, `Down`, `ForceDown`,
    `Destroy`, `Status`, `List`, `LoadInstance`).
  - `qemu.go` is the QEMU backend. It builds `qemu-system-aarch64` args with
    vsock device, virtio-9p shares, and configurable port forwards. Post-boot
    it connects to stereosd via vsock to inject secrets and mount directories.
  - `qmp.go` is a minimal QMP client for QEMU process control.
  - `image.go` handles mixtape resolution, qcow2 overlay creation, and raw
    image copying/resizing.
  - `state.go` persists VM metadata as `state.json`.
  - `types.go` defines `State`, `Instance`, and path helpers.
  - `backend_darwin_arm64.go`, `backend_linux.go`, `backend_other.go` provide
    platform-specific `NewPlatformBackend()` dispatch via build tags.

- **`pkg/vsock/`** -- Host-side vsock client matching stereosd's ndjson protocol.
  `protocol.go` defines message types and payload structs. `client.go` provides
  `Dial()`, `Ping()`, `InjectSecret()`, `Mount()`, `Shutdown()`, `GetHealth()`,
  and `WaitForReady()`.

- **`pkg/mixtapes/`** -- Manages local mixtape images in `$config-dir/mixtapes/`.
  `List()` scans available mixtapes, `Pull()` is the OCI pull placeholder
  (to be implemented with `oras.land/oras-go/v2`).

- **`pkg/ssh/`** -- SSH connectivity. `connect.go` uses `syscall.Exec` to
  replace the Go process with OpenSSH. `wait.go` polls TCP until sshd responds.

- **`pkg/ui/`** -- Terminal output helpers. Colored status/success/warn/error
  messages and an animated progress spinner. All output goes to stderr.

## Design principles

1. **Daemon architecture.** The daemon (`mb serve`) owns all VM lifecycle. CLI
   commands are thin RPC clients. The daemon can auto-start via `mb up` if not
   running (fork + setsid).

2. **Backend interface is the key abstraction.** All VM operations go through
   `vm.Backend`. QEMU is the only implementation today. Platform-specific
   `NewPlatformBackend()` with build tags enables future backends (Apple Virt
   framework, KVM/libvirt).

3. **Vsock for guest control plane.** stereosd inside the guest is the bridge.
   Secrets are injected over vsock (never baked into images). Shared directories
   are mounted via vsock mount commands. Shutdown is coordinated through vsock.

4. **jcard.toml is the config format.** Defines mixtape, resources, network
   (with port forwards and egress allowlists), shared directories, secrets,
   and agent configuration. The `[agent]` section is passed through to agentd.

5. **SSH uses process replacement.** `mb ssh` calls `syscall.Exec` for correct
   terminal handling. Do not change to a Go SSH library.

6. **Config defaults are generous.** Most fields in jcard.toml are optional.
   `applyDefaults()` fills in sensible values (2 CPUs, 4GiB RAM, 20GiB disk,
   NAT networking, claude-code harness).

7. **No cloud-init.** StereOS images are pre-built with stereosd and agentd.
   Runtime provisioning (secrets, mounts, agent config) happens over vsock,
   not via cloud-init ISOs.

## Conventions

- **Error handling:** Wrap errors with `fmt.Errorf("context: %w", err)`. Include
  actionable guidance in user-facing errors (e.g., "install QEMU: brew install
  qemu").

- **Output:** Use `ui.Status()`, `ui.Success()`, `ui.Warn()`, `ui.Error()`, and
  `ui.Info()` for user-facing messages. Write to stderr so stdout stays clean.

- **Cleanup on failure:** If `Up()` fails partway through creating VM resources,
  remove the VM directory (`os.RemoveAll`). Follow this pattern for any
  operation that creates resources.

- **Process management:** Use `syscall.Signal(0)` to check if a PID is alive.
  Use SIGTERM before SIGKILL. Read PIDs from the QEMU pidfile.

- **Testing:** Config parsing is tested in `pkg/config/config_test.go`. VM and
  SSH packages require QEMU/network and use manual smoke testing.

## File layout on disk

```
~/.config/mb/
├── config.toml                       # Optional persistent config (read by Viper)
├── mb.sock                           # Daemon unix socket (runtime)
├── daemon.pid                        # Daemon PID file (runtime)
├── mixtapes/
│   └── <name>/
│       └── nixos.img                 # StereOS raw image (or nixos.qcow2)
└── vms/
    └── <name>/
        ├── state.json                # Metadata (name, ports, cpus, etc.)
        ├── jcard.toml                # Copy of the sandbox configuration
        ├── disk.raw                  # VM disk (copied from mixtape)
        ├── disk.qcow2               # Or qcow2 overlay (if base is qcow2)
        ├── efi-vars.fd              # Writable EFI variable store (64MB)
        ├── qmp.sock                 # QMP unix socket (exists while VM runs)
        ├── serial.log               # Serial console output
        └── qemu.pid                 # QEMU process ID
```

## Common tasks

### Adding a new command

1. Create `cmd/<name>/<name>.go` with package `<name>cmder`.
2. Implement `New<Name>Cmd(configDirFn func() string) *cobra.Command`.
3. Register it in `main.go` with `cmd.AddCommand(...)`, passing `mbconfig.ConfigDir`.
4. Use `client.New(configDirFn())` to talk to the daemon.
5. For commands that need the daemon running, use the `ensureDaemon()` pattern
   from `cmd/up/up.go`.

### Adding a new config field

1. Add the field to the appropriate struct in `pkg/config/config.go`.
2. Add a default in `applyDefaults()` if needed.
3. Add validation in `validate()` if needed.
4. If it's a path, expand it in `expandPaths()`.
5. Add a test case in `pkg/config/config_test.go`.

### Adding a new daemon RPC method

1. Add the method constant in `pkg/daemon/rpc.go`.
2. Add request/response fields as needed.
3. Implement the handler in `pkg/daemon/daemon.go`.
4. Add a client method in `pkg/client/client.go`.

### Changing the QEMU command line

Edit `buildArgs()` in `pkg/vm/qemu.go`. The full QEMU invocation is assembled
there from the `Instance` and `JcardConfig`.

### Debugging boot issues

```bash
cat ~/.config/mb/vms/<name>/serial.log        # Serial console output

socat - UNIX-CONNECT:~/.config/mb/vms/<name>/qmp.sock
{"execute": "qmp_capabilities"}
{"execute": "query-status"}
```
