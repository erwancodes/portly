# Portly Windows shell

This directory contains the native Windows desktop shell for Portly. The Go
backend is embedded in a Wails window and talks to the existing Portly daemon
over its local HTTP API. The React/Vite frontend stays deliberately dense and
developer-focused: projects, temporary jobs, resource usage, logs, and the
daemon settings are available without duplicating supervisor logic.

## Local development

From `windows/`:

```powershell
pnpm install
pnpm --dir frontend typecheck
pnpm --dir frontend build
go test ./...
go run .
```

The shell looks for `portly.exe` next to the desktop executable, in a sibling
`cli/` directory, or on `PATH`. During development, point it at a local CLI
with:

```powershell
$env:PORTLY_CLI_PATH = "C:\Dev\HelionisLabs\portly\cli\portly.exe"
```

The API port defaults to `7737` and can be changed with `PORTLY_API_PORT`.

## Release build

Build the frontend first, then use the Wails CLI from `windows/`:

```powershell
pnpm install --frozen-lockfile
pnpm --dir frontend build
go run github.com/wailsapp/wails/v2/cmd/wails@v2.10.2 build
dotnet publish ../packaging/windows/installer/PortlySetup.csproj -c Release -r win-x64 --self-contained true -p:PublishSingleFile=true -p:IncludeNativeLibrariesForSelfExtract=true -o setup-build
```

The resulting `Portly.exe` is generated in this directory. Closing the window
hides it while the daemon keeps running; the explicit **Quit Portly** action
stops the managed daemon and exits the desktop shell.

Copy `setup-build/PortlySetup.exe` next to the release payload (`Portly.exe`,
`cli/portly.exe`, `skills/portly`, and `packaging/windows`) before distributing
the package. Opening `PortlySetup.exe` without arguments shows the graphical
installer; CI and scripted workflows can still pass the documented skip flags.

The system tray is implemented with the native Windows notification API, and
the package includes a per-user `PortlySetup.exe` installer. The automatic
updater currently hands off to the official GitHub release page so a running
Portly process is never replaced in place.
