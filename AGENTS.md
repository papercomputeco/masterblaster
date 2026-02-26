# Agents

Guidelines for AI coding agents working on the Masterblaster codebase.

## Project overview

Masterblaster (`mb`) is a Go CLI and daemon for managing StereOS-based
AI agent sandbox VMs. It targets aarch64 guests using HVF acceleration on
Apple Silicon Macs with two backends: QEMU and Apple Virtualization.framework.

The tool communicates with `stereosd` inside StereOS guests over vsock for
secret injection, shared directory mounting, health monitoring, and graceful
shutdown. Agent harnesses (Claude Code, Claude Code, Gemini CLI) are managed by
`agentd` inside the guest.

See SPEC.md for the full RFC specification.

## Build and test

```bash
make build        # Produces ./build/mb binary
make check        # Runs go tests in Dagger
make clean        # Removes the build/ directory
make fmt          # Formats code
make vet          # Runs go vet
```

Go 1.25+ is required. The Nix flake (`flake.nix`) provides a reproducible dev
environment with QEMU, Go, and build tools. Use `nix develop` or direnv.

## Architecture

The codebase follows the daemon + CLI client + vmhost child process pattern:

```
mb CLI  --[JSON-RPC over $config-dir/mb.sock]--> mb daemon (mb serve)
  daemon --[JSON-RPC over vmhost.sock]---------> mb vmhost (one per VM)
    vmhost --[QMP unix socket]----------------> QEMU process (child of vmhost)
    vmhost --[in-process]---------------------> Apple Virt VM (Vz framework)
    vmhost --[vsock/tcp]----------------------> stereosd (guest)
      stereosd --[tmpfs/unix socket]----------> agentd (guest)
```

Each VM gets a dedicated `vmhost` child process (`mb vmhost --name <name>
--backend <qemu|applevirt>`) that holds the hypervisor handle and exposes a
control socket (`vmhost.sock`). The daemon spawns vmhost processes, monitors
their health via PID liveness checks, and routes CLI requests to them. This
architecture enables crash isolation (one VM crashing doesn't affect others),
daemon restart survival (vmhost processes outlive the daemon), and mixed
backends (QEMU and Apple Virt VMs running simultaneously).

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
  `down`, `status`, `destroy`, `ssh`, `list`, `mixtapes`, `vmhost`. Commands
  are thin wrappers that delegate to the daemon via `pkg/client/`.

- **`cmd/vmhost/`** -- Hidden `mb vmhost` subcommand. The daemon spawns one
  vmhost process per VM; each holds the hypervisor handle and exposes a
  control socket. `vmhost.go` defines `NewVMHostCmd()`, the `runVMHost()`
  entrypoint, the `qemuController` adapter (implements `vmhost.VMController`),
  and the `bootVM()` dispatcher. Platform-specific files provide
  `bootAppleVirt()` and `getPlatformConfig()`:
  - `platform_darwin_arm64.go` -- Apple Virt controller + QEMU HVF config.
  - `platform_linux.go` -- QEMU KVM config.
  - `platform_darwin_amd64.go` -- Stub (unsupported).
  - `platform_other.go` -- Stub (unsupported).

- **`pkg/config/`** -- jcard.toml config parsing, validation, and defaults.
  `config.go` defines `JcardConfig` and `Load()`. `expand.go` handles
  `${ENV_VAR}` expansion. `marshal.go` provides TOML serialization and the
  default jcard.toml template. Always use `Load()` to get a validated config.

- **`pkg/daemon/`** -- The long-lived Masterblaster daemon. `daemon.go` defines
  the `Daemon` struct with a `sync.RWMutex`-protected VM map and unix socket
  listener. Each VM is tracked as a `managedVM` struct containing the vmhost
  client connection. The daemon no longer holds a `Backend` directly; it
  spawns vmhost child processes and delegates all VM operations to them via
  `pkg/vmhost.Client`. `rpc.go` defines the JSON-RPC wire format (`Request`,
  `Response`, `SandboxInfo`). The daemon manages `$config-dir/mb.sock` for CLI
  communication and `$config-dir/daemon.pid` for liveness.

- **`pkg/daemon/client/`** -- Thin JSON-RPC client for CLI commands to talk to
  the daemon. Provides `Up()`, `Down()`, `Status()`, `Destroy()`, `List()`.
  Also provides `EnsureDaemon(baseDir)` which auto-starts the daemon if not
  running (fork + setsid + poll). All CLI commands (`up`, `status`, `list`,
  `ssh`, `down`, `destroy`) call `EnsureDaemon` automatically so the daemon
  can rediscover running vmhost processes that may have survived a daemon
  restart.

- **`pkg/vm/`** -- VM lifecycle management.
  - `backend.go` defines the `Backend` interface (`Up`, `Start`, `Down`,
    `ForceDown`, `Destroy`, `Status`, `List`, `LoadInstance`).
  - `platform.go` defines `QEMUPlatformConfig` -- platform-specific settings
    (accelerator, binary, EFI paths, control plane mode, vsock device,
    direct kernel boot, disk AIO/cache) injected into the QEMU backend at
    construction time.
  - `qemu.go` is the QEMU backend. It uses `QEMUPlatformConfig` to build
    QEMU args with the correct accelerator (`hvf`/`kvm`), binary, and EFI
    firmware paths. QEMU runs as a child process of vmhost (no `-daemonize`).
    Post-boot it connects to stereosd via the platform's control plane
    transport to inject secrets and mount directories. Exposes `Boot()` and
    `WaitQEMU()` methods for vmhost use.
  - `applevirt.go` is the Apple Virtualization.framework backend
    (darwin/arm64 only, uses `github.com/Code-Hex/vz/v3`). VMs run in-process
    with an in-memory `live` map. Exposes `Boot()` and `WaitVM()` methods
    for vmhost use.
  - `qmp.go` is a minimal QMP client for QEMU process control.
  - `image.go` handles mixtape resolution, qcow2 overlay creation, and raw
    image copying/resizing.
  - `state.go` persists VM metadata as `state.json`.
  - `types.go` defines `State`, `Instance`, and path helpers (including
    `VMHostPIDPath()`, `VMHostSocketPath()`, `VMHostLogPath()`).
  - `prepare.go` contains daemon-side disk preparation functions run before
    spawning vmhost: `PrepareQEMUDisk()`, `LoadInstanceFromDisk()`,
    `LoadStateFromDisk()`, `CleanupVMDir()`, `ResolveBackend()`.
  - `prepare_darwin_arm64.go` -- `PrepareAppleVirtDisk()` implementation.
  - `prepare_other.go` -- `PrepareAppleVirtDisk()` stub (unsupported).
  - `backend_darwin_arm64.go` -- `NewPlatformBackend()` for macOS/Apple Silicon.
    Configures QEMU with `accel=hvf` and `ControlPlaneMode: "tcp"` (no native
    vsock on macOS/HVF).
  - `backend_linux.go` -- `NewPlatformBackend()` for Linux. Configures QEMU
    with `accel=kvm` and `ControlPlaneMode: "vsock"` (native `vhost-vsock-pci`).
  - `backend_other.go` -- Returns an error on unsupported platforms.

- **`pkg/vmhost/`** -- Control protocol between the daemon and vmhost processes.
  - `protocol.go` defines `Request`/`Response` wire types and method constants
    (`status`, `stop`, `force_stop`, `info`).
  - `server.go` defines `VMController` interface (implemented by backend
    adapters in `cmd/vmhost/`), and `Server` which listens on `vmhost.sock`,
    dispatches JSON-RPC requests, and monitors VM exit via `controller.Wait()`.
  - `client.go` provides `Client` used by the daemon to talk to vmhost
    processes: `Status()`, `Stop()`, `ForceStop()`, `Info()`, `IsAlive()`.

- **`pkg/vsock/`** -- Host-side client for communicating with stereosd.
  - `transport.go` defines the `Transport` interface, abstracting the
    connection mechanism. `TCPTransport` is the current implementation
    (used on macOS/HVF). `VsockTransport` (AF_VSOCK, for Linux/KVM) is
    planned. Used by vmhost internally.
  - `client.go` provides `Connect(transport, timeout)` and message methods:
    `Ping()`, `InjectSecret()`, `Mount()`, `Shutdown()`, `GetHealth()`,
    `WaitForReady()`.
  - `protocol.go` defines the ndjson wire format and message types.

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
   running (fork + setsid). `mb up` is idempotent: if the sandbox is already
   running it's a no-op, if stopped it re-boots the existing disk.

2. **Backend interface is the key abstraction.** All VM operations go through
   `vm.Backend`. QEMU is the only implementation today. Platform-specific
   `NewPlatformBackend()` with build tags enables future backends (Apple Virt
   framework, KVM/libvirt).

3. **Vmhost child process pattern.** Each VM gets a dedicated `mb vmhost`
   child process that holds the hypervisor handle and exposes a control
   socket (`vmhost.sock`). The daemon spawns vmhost processes and routes
   CLI requests to them via `pkg/vmhost.Client`. This provides crash
   isolation, daemon restart survival, and mixed backend support.

4. **Vsock for guest control plane.** stereosd inside the guest is the bridge.
   Secrets are injected over vsock (never baked into images). Shared directories
   are mounted via vsock mount commands. Shutdown is coordinated through vsock.

5. **jcard.toml is the config format.** Defines mixtape, resources, network
   (with port forwards and egress allowlists), shared directories, secrets,
   and agent configuration. The `[agent]` section is passed through to agentd.

6. **SSH uses process replacement.** `mb ssh` calls `syscall.Exec` for correct
   terminal handling. Do not change to a Go SSH library.

7. **Config defaults are generous.** Most fields in jcard.toml are optional.
   `applyDefaults()` fills in sensible values (2 CPUs, 4GiB RAM, 20GiB disk,
   NAT networking, claude-code harness).

8. **No cloud-init.** StereOS images are pre-built with stereosd and agentd.
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
        ├── qemu.pid                 # QEMU process ID
        ├── vmhost.sock              # Vmhost control socket (runtime)
        ├── vmhost.pid               # Vmhost process ID (runtime)
        └── vmhost.log               # Vmhost process log output
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
there from the `Instance` and `JcardConfig`. Platform-specific settings
(accelerator, machine type, EFI paths, vsock device) come from
`QEMUPlatformConfig` -- edit the platform backend files
(`backend_darwin_arm64.go`, `backend_linux.go`) to change those.

Note: QEMU platform config for vmhost is also defined in
`cmd/vmhost/platform_darwin_arm64.go` and `cmd/vmhost/platform_linux.go`
via the `getPlatformConfig()` function.

### Debugging boot issues

```bash
cat ~/.config/mb/vms/<name>/vmhost.log        # Vmhost process log
cat ~/.config/mb/vms/<name>/serial.log        # Serial console output

socat - UNIX-CONNECT:~/.config/mb/vms/<name>/qmp.sock
{"execute": "qmp_capabilities"}
{"execute": "query-status"}
```
