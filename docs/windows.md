# Portly on Windows

The Windows port starts with the Go supervisor in `cli/`. The macOS SwiftUI
application is not part of the Windows build.

## Build

Use Go 1.22 or newer:

```powershell
cd cli
go test ./...
go build -trimpath -o portly.exe .
```

The resulting `portly.exe` exposes the same agent-facing CLI and loopback API
as the Linux supervisor. The daemon listens on `127.0.0.1` only.

The native desktop shell lives in `windows/` and embeds the React/Vite UI in a
Wails window:

```powershell
cd windows
pnpm install --frozen-lockfile
pnpm --dir frontend build
go run github.com/wailsapp/wails/v2/cmd/wails@v2.10.2 build
```

Set `PORTLY_CLI_PATH` when the desktop executable cannot find the CLI next to
it or on `PATH`. Closing the window hides it and leaves the daemon running;
**Quit Portly** explicitly stops the managed daemon and exits the shell.
Portly also registers a native Windows notification-area icon with actions to
open the window, start or stop a project, open the dashboard, and quit.

The CI package includes a graphical `PortlySetup.exe` for a per-user install.
Run it from the unpacked release root; it installs the app and CLI without
administrator privileges:

```powershell
.\PortlySetup.exe
```

The same package includes `packaging/windows/PortlySetup.ps1` for scripted
installations. Run it from the unpacked release root in PowerShell:

```powershell
Set-ExecutionPolicy -Scope Process Bypass
.\packaging\windows\PortlySetup.ps1
```

It installs the desktop app and CLI under `%LOCALAPPDATA%\Portly`, adds the
CLI to the user PATH, creates a Start Menu shortcut, prepares the Portly config
and log directories, and installs the agent skill only when the target skill
directory does not already exist. `Uninstall-Portly.ps1` removes only the
installer-owned files and leaves the config and skill in place.

## Run a project

```powershell
portly.exe add-project --name demo --path 'C:\Users\John Doe\My Project'
portly.exe add-server --project demo --name web --command 'pnpm dev' --port 5173 --start
portly.exe status --details
portly.exe logs demo/web --tail 100
portly.exe restart demo/web
portly.exe stop demo/web
```

Commands are launched through PowerShell when `pwsh.exe` or
`powershell.exe` is available, and through `cmd.exe` otherwise. Set
`PORTLY_SHELL` to select a compatible shell explicitly.

## Automatic launch

```powershell
portly.exe forever enable
portly.exe forever status
portly.exe forever disable
```

This registers an idempotent `Portly` task at user logon. Disabling the task
does not silently stop the servers that were already running.

## Platform behavior

- Configuration stays in `%USERPROFILE%\.config\portly` for format parity.
- Port discovery uses `netstat.exe -ano`; Docker-published ports are resolved
  through the Docker CLI before an external process is stopped.
- Graceful and forced process-tree shutdown use `taskkill.exe`.
- Process startup attempts a ConPTY-backed terminal for interactive-compatible
  output and falls back to inherited pipes when the pseudo-console API is not
  available.
- Resource samples use the Windows formatted process performance provider;
  the memory value is private working set, not macOS physical footprint.
- `npm`, `pnpm`, `bun`, `python`, Docker CLI, and other console commands work
  through the selected shell as ordinary user commands.

## Known limitations

The desktop surface exposes stdin, Ctrl+C, and automatic terminal resize over
ConPTY. When Windows falls back to inherited pipes, stdin remains available but
resize is not supported by the operating system path. The updater currently
opens the official release page rather than replacing a running binary
automatically.
