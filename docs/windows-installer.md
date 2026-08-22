# Windows 11 Installer

HAI ships as a local Windows installer around the canonical Docker Compose
stack. It installs product files under `Program Files`, stores the local
configuration in `%LOCALAPPDATA%\HAI`, and keeps all services bound to
`127.0.0.1` on port `8088` by default.

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

## First run

Run the installer, then select **Start HAI** from the Start menu. The first run
asks for the local owner email and password, creates independent secrets in
`%LOCALAPPDATA%\HAI\hai.env`, builds the real local containers, waits for
readiness, and opens `http://127.0.0.1:8088`.

Use the Start menu entries to open the dashboard, inspect HAI status, or stop
the stack. Stop HAI preserves the Docker volumes and local settings.

### Optional governed cloud access

After configuring a reserved HTTPS ngrok endpoint and its restricted agent
token in `%LOCALAPPDATA%\HAI\hai.env`, use **Start governed cloud access** in
the HAI Local Start menu group. It passes the installed environment and Compose
file to the governed ngrok preflight; it will refuse placeholder credentials,
an insecure login configuration, a non-loopback gateway, or OAuth redirects
that do not match the configured public origin. Use the same shortcut's
PowerShell script with `-ValidateOnly` to inspect configuration without opening
a tunnel, or `-Stop` to close the tunnel while leaving the local HAI stack
running.

The tunnel can publish only nginx on HAI's private Docker network. It does not
expose the backend, IDP, databases, Redis, Kafka, or ngrok inspector directly.

The default local control plane has explicit Docker ceilings: 1 GB for the
backend and automation database, 512 MB for the IDP and identity database,
256 MB for Redis and the static frontend, and 128 MB for the gateway. These
are ceilings, not reserved memory. Optional models, scanners, document tools,
and event-bus services have separate profiles and do not start unless enabled.

### Verify a configured Trello board

For an owner who has configured a read-only Trello API key, token, and a board
in `%LOCALAPPDATA%\HAI\hai.env`, run the packaged acceptance test from the HAI
program folder:

```powershell
.\scripts\test-live-trello.ps1 -EnvFile "$env:LOCALAPPDATA\HAI\hai.env"
```

It runs only read-only Trello connector checks in a disposable container. A
skipped test is treated as a failure, so a successful run proves board access,
incremental sync, and read-only token permissions without mutating Trello.

The first run writes unique database owner and reserved runtime-role passwords
to `%LOCALAPPDATA%\HAI\hai.env`. They are never shown in the installer output.

### Optional Kafka event bus

The ordinary local installation does **not** start Kafka or ZooKeeper. This
keeps the Windows footprint lower while preserving login, source intake,
approvals, workflows, and audit records. IDP logs remain available through
Docker Desktop, while non-critical account-notification events are skipped.

The obsolete `generic-auto` compatibility server is also disabled by default.
It is not part of HAI's governed execution engine. A legacy installation that
still requires its `/generic-auto/` route may explicitly add the
`legacy-compatibility` Compose profile after reviewing that dependency.

For a deployment that explicitly needs the event bus, run this from the
installed HAI program folder in PowerShell:

```powershell
.\installer\windows\Start-HAI.ps1 -EnableEventBus
```

This starts the `event-bus` Compose profile, including Kafka, ZooKeeper, and
the Kafka-driven nginx configuration manager. It also enables the three
event-bus settings in `%LOCALAPPDATA%\HAI\hai.env`, so the IDP and backend use
the broker rather than merely starting it. The next normal Start HAI run
automatically stops those optional services, restores the smaller local stack,
and clears the local broker settings.

### Enable the local safe worker

The dashboard starts in read-only-safe mode. Before HAI can run its confined
local safe-worker check, sign in as the local owner and open
`http://127.0.0.1:8088/framework-registry`. Review the built-in Constitution,
prepare a draft, and explicitly activate it with the required confirmation and
approval note. Then use **Stop HAI** and **Start HAI** once so the backend
rebuilds its execution authorization boundary from the durable owner-approved
policy.

This does not authorize email, financial, legal, public, destructive, or
account actions. Those remain separately approval-gated. If the Constitution
is not activated, HAI reports the local safe worker as blocked rather than
silently executing it.

## Enable a local EUR 0 model

The installed stack does not download a model until you choose to do so. Select
**Enable local model** from the HAI Local Start menu group to download the
reviewed small `qwen2.5:0.5b` model, configure HAI to use only the private
`ollama-local` service, restart the backend, and wait for `/readyz`.

The model download can take several minutes and consumes local disk space. It
does not enable cloud providers, expose ngrok access, change HAI's paid budget,
or send connected-source data to another service. Use **Stop HAI** afterwards
when the local model is not needed; Docker retains the downloaded model volume
until you explicitly remove it.

## One installation at a time

HAI uses one canonical Compose project and named Docker volumes. If it detects
an existing HAI stack started from another directory, Start HAI stops and names
the existing directory. Stop or migrate that installation before using the
installer. This prevents two HAI instances from competing for the same data.

## Uninstall and data

Uninstall removes the installed program files only. It **does not delete**
`%LOCALAPPDATA%\HAI` or Docker volumes. Use the documented backup and restore
procedure before manually removing data.
