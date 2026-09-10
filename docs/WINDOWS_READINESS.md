# Windows readiness notes

This project is Windows-first, but this Linux sandbox cannot perform a real Windows runtime smoke test. The following readiness work has been implemented in code and must still be verified on Windows hardware/VM with an Android device.

## Implemented readiness work

- Normal Windows GUI build path is `scripts/build-windows.ps1 -Version <version>`. The script verifies `go` exists, runs GUI tests unless `-SkipTests` is supplied, builds `AdbPureFlow-<version>-windows-amd64.exe`, verifies the executable exists, and writes a SHA-256 checksum file.
- Generated executables, DLLs, provisioned platform-tools, scrcpy archives, and runtime caches are ignored by Git.
- Downloaded ADB/scrcpy tools default to per-user cache/config storage instead of the executable directory, avoiding administrator requirements for installations under `Program Files`.
- Startup and passive monitoring use non-provisioning ADB probes; missing tools are surfaced through status/Diagnostics instead of being silently downloaded on launch.
- Persistent app state uses per-user config storage and contains only non-sensitive preferences/endpoints.
- Open File and Save File dialogs are used for APKs, screenshots, recordings, and deploy artifacts.
- Screenshot success offers a handoff to the system file manager; on Windows this calls `explorer.exe /select,<path>` without shell interpolation.
- Fyne desktop notifications are sent for completed screenshot, install, and deploy operations where the OS permits notifications.
- Application shutdown stops log streaming, passive device monitoring, and all managed scrcpy child processes.
- Paths are passed to subprocesses as individual arguments, so spaces and non-ASCII characters are not shell-split by AdbPureFlow.
- Keyboard shortcuts avoid firing while a text entry has focus where Fyne exposes the focused widget.

## Automated Windows verification

A dedicated Windows GitHub Actions workflow is still required, but it is not included in this branch because the current automation credentials cannot update files under `.github/workflows/`. Until that permission is available, use `scripts/build-windows.ps1` on a Windows machine to verify dependency resolution, GUI tests, Windows executable creation, and checksum output.

## Windows runtime verification still required

Run these checks before marking a release stable:

1. Build with `scripts/build-windows.ps1 -Version <version>`.
2. Start the executable from a path containing spaces and non-ASCII characters.
3. Confirm the main window opens, resizes, maximizes, and closes cleanly.
4. Verify preferences are written under the current user's config directory and tool downloads under the current user's cache/config tools directory.
5. Verify native file dialogs for APK selection, screenshots, recordings, and deploy artifacts.
6. Verify Explorer handoff after screenshots.
7. Verify clipboard actions for selected device serial and package names.
8. Verify USB device connect/authorize/disconnect/reconnect.
9. Verify Wireless Debugging pairing-code flow, reconnect of remembered endpoints, and forget behavior.
10. Verify scrcpy start/stop, unexpected process exit reporting, recording file verification, and app shutdown cleanup.
11. Verify logcat streaming under high-volume logs remains responsive.

No QR pairing support is claimed until the real Android/ADB QR pairing payload and service behavior are implemented and tested.
