# Windows 11 Installer

HAI ships as a local Windows installer around the canonical Docker Compose
stack. It installs product files under `Program Files`, stores the local
configuration in `%LOCALAPPDATA%\HAI`, and keeps all services bound to
`127.0.0.1` on port `8088` by default. The optional local A2A planning
connector is separately bound to `127.0.0.1:8091`; it cannot be reached from
the cloud tunnel and only serves the Agent Card plus its bearer-authenticated
planning endpoint.

## Prerequisite

Install and start current **Docker Desktop** with its Linux engine enabled.
The installer does not silently install Docker Desktop, enable public access,
or turn on the local-login bypass.

## Build the installer

From a Git checkout on Windows 11:

```powershell
winget install --id JRSoftware.InnoSetup -e
.\scripts\build-windows-installer.ps1
```

The generated executable is `installer\release\HAI-Setup-<commit>.exe`. The
build copies tracked product source files plus the installer source files being
built. It excludes arbitrary local files, personal source exports, credentials,
Docker data, diagnostics, and build output.

Release builds require a clean Git worktree. This prevents an installer from
claiming a commit while silently omitting newer local source changes. Commit or
stash work before building a distributable installer. `-AllowDirtyWorktree` is
available only for deliberate non-release developer payload experiments and
must not be used for a client or production installer.

## First run

Run the installer, then select **Start HAI** from the Start menu. The first run
asks for the local owner email and password, creates independent secrets in
`%LOCALAPPDATA%\HAI\hai.env`, builds the real local containers, waits for
readiness, and opens `http://127.0.0.1:8088`.

Use the Start menu entries to open the dashboard, inspect HAI status, or stop
the stack. **Test local agent connector** fetches and validates the local A2A
Agent Card, then sends one bearer-authenticated, non-executable planning probe.
It verifies the bounded planning response without exposing the connector
publicly or creating work. Stop HAI preserves the Docker volumes and local
settings.

## One installation at a time

HAI uses one canonical Compose project and named Docker volumes. If it detects
an existing HAI stack started from another directory, Start HAI stops and names
the existing directory. Stop or migrate that installation before using the
installer. This prevents two HAI instances from competing for the same data.

## Optional DeepSeek Harness worker

DeepSeek Harness is not part of the normal installer start-up. HAI keeps it
disabled until an operator explicitly enables the host bridge in
`%LOCALAPPDATA%\HAI\hai.env`, assigns a random 32+ character
`HAI_HOST_RUNTIME_BRIDGE_TOKEN`, pins `DEEPSEEK_HARNESS_VERSION`, and sets a
dedicated Windows `DEEPSEEK_HARNESS_WORKSPACE` and state directory beneath it.
The same file must set `DEEPSEEK_HARNESS_WORKSPACE_KEY` to the stable identifier
used by the HAI backend.

Set `HAI_HOST_RUNTIME_LEASE_SECONDS` to cover the selected DSH timeout plus
terminal-result submission. The default is 1200 seconds. HAI raises an unsafe
shorter value to at least `DEEPSEEK_HARNESS_TIMEOUT_SECONDS + 60`, preventing a
still-running approved task from being leased again.

`Start-HAI.ps1 -EnableHostRuntime` performs a strict local preflight. It refuses
to start the bridge unless the backend and bridge are both explicitly enabled,
the bridge URL is loopback-only, the configured `dsh` executable reports the
pinned version, and the state directory stays within the dedicated workspace.
This prevents an apparently running bridge that HAI would still block, or a
worker pointed at the wrong local executable.

Start HAI with `Start HAI`'s optional host-runtime switch when the worker is
enabled:

```powershell
.\Start-HAI.ps1 -EnableHostRuntime
```

The gateway is bound to `127.0.0.1:8092`; do not expose that port, route it
through ngrok, or place it behind a reverse proxy. The worker runs only
already-approved tasks, validates the local pinned `dsh --version` before it
leases work, obtains a final server confirmation immediately before starting
DSH, and does not listen for inbound connections. While DSH runs, the bridge
reconfirms that lease every two seconds and cancels the local DSH process if an
emergency stop activates, the lease expires, or confirmation fails. An emergency
stop or expired lease between polling and launch also blocks the process rather
than relying on the worker to notice later. The released installer
packages the reviewed `hai-dsh-bridge.exe`; no separate Go installation is
required on the operator device. It is not an unattended installation step or a
substitute for reviewing DeepSeek's preview release.

When the bridge reports a terminal result, HAI's existing durable-job worker
adds an immutable completion event to the linked automation ledger and updates
its last-success or last-failure record. This projection is retry-safe and does
not invoke DSH again. A host job without a matching queued launch is retained
for investigation rather than being silently marked complete.

## Uninstall and data

Uninstall removes the installed program files only. It **does not delete**
`%LOCALAPPDATA%\HAI` or Docker volumes. Use the documented backup and restore
procedure before manually removing data.
