# AdbPureFlow

AdbPureFlow is being reworked into a **Windows-first native Android device management and development workstation**. It helps you connect a phone, understand its state, mirror and control the screen, install APKs, manage apps, read logs, and run repeatable deploy flows without living inside an ADB terminal.

The desktop app is written in Go with a Fyne native GUI. It is not an Electron/web dashboard wrapper.

## Current implementation status

This repository has started the migration from the earlier APK installer/mirror utility into the new desktop product. The current GUI provides working foundations for:

- Device-centric shell with workspaces: Devices, Screen, Apps, Logs, Deploy, Workflows, Diagnostics, and Help.
- Safe direct process execution with context cancellation, timeouts, exit codes, stdout/stderr capture, streaming output, and structured errors.
- ADB discovery/provisioning and typed ADB operations.
- USB and wireless device listing with model/status/transport details and enrichment where available.
- Real Wireless Debugging pairing-code flow via `adb pair host:port code` and connect flow via `adb connect host:port`.
- scrcpy discovery/provisioning and managed screen mirror lifecycle, with companion-window status feedback and cleanup on shutdown.
- Screenshot capture through native Save File dialog with optional Explorer handoff after saving.
- Recording start flow through native Save File dialog; output is verified when the scrcpy companion process exits.
- APK drag/drop or file selection, APK zip validation, size/SHA-256, and `aapt`-powered package/version/SDK/permission metadata when Android build-tools are available.
- Install/reinstall, launch, force stop, clear app data, uninstall, and searchable app list with user/system distinction when requested.
- Live logcat streaming with readable rows, pause/resume/stop, text and level filtering, clear/reset, bounded visible buffer, and raw technical detail preserved.
- Deploy workflow: install APK → optional clear data → launch → optional mirror/logs.
- Diagnostics/Doctor checks for ADB executable capability, device visibility/authorization, and scrcpy capability, plus an ADB server restart action.
- Passive device monitoring with explicit selection when multiple devices are present.
- Known wireless endpoint reconnect/forget actions and local device aliases.
- First-launch onboarding with “Baru di sini? / New here?” and reopenable Help & Tutorials.
- Settings for theme selection and persisted non-sensitive user state in per-user application data.
- Clipboard shortcuts/actions for device serials and package names, plus desktop notifications for completed long-running actions where supported by the OS.
- Command palette via `Ctrl+K`; refresh devices via `Ctrl+R`.

See [`docs/AUDIT.md`](docs/AUDIT.md) for the repository audit, [`docs/HARDENING.md`](docs/HARDENING.md) for reliability findings, [`docs/WINDOWS_READINESS.md`](docs/WINDOWS_READINESS.md) for Windows verification notes, [`docs/WINDOWS_SMOKE_TEST.md`](docs/WINDOWS_SMOKE_TEST.md) for manual runtime validation, [`docs/DEVICE_TEST_MATRIX.md`](docs/DEVICE_TEST_MATRIX.md) for real-device coverage, [`docs/TROUBLESHOOTING.md`](docs/TROUBLESHOOTING.md) for failure recovery, [`docs/RELEASE_CHECKLIST.md`](docs/RELEASE_CHECKLIST.md) for release gates, and [`docs/WIRELESS_DEBUGGING.md`](docs/WIRELESS_DEBUGGING.md) for the Wireless Debugging support boundary.

## Capability status

- **Implemented and unit-tested in this repository:** core ADB command construction, parser logic, APK validation/hash inspection, workflow failure propagation, app-state fallback/persistence, selected-device reconciliation, and bounded log buffering.
- **Implemented but still requires Windows/device runtime verification:** Fyne GUI startup/shutdown, native Windows dialogs/notifications/Explorer handoff, USB device operations, Wireless Debugging pairing-code flow, scrcpy mirror/recording, real APK install/manage actions, and live logcat behavior.
- **Platform-dependent:** automatic ADB/scrcpy provisioning and native file-manager handoff.
- **Not implemented:** Wireless Debugging QR pairing. The app intentionally does not ship a fake QR code.

## Product principles

- Users think in actions: **connect my phone**, **show my screen**, **install this APK**, **manage apps**, **see why my app crashed**, **deploy this build**.
- ADB, Wireless Debugging, scrcpy, package management, and logcat remain behind clear native UI flows.
- Windows conventions matter: normal executable, native windows, native open/save dialogs, keyboard shortcuts, DPI-aware UI, and familiar filesystem behavior.
- AdbPureFlow does not duplicate Windows Explorer unless Android-specific capabilities require it.
- Visible features should either perform real device operations or clearly explain why they are unavailable.

## Prerequisites

### For users

- Windows 10/11 is the primary target.
- Android device with Developer Options and USB Debugging enabled.
- For wireless devices: Android 11+ Wireless debugging is recommended.
- Internet access on first run if ADB/platform-tools or scrcpy need to be automatically downloaded.
- Optional: Android build-tools `aapt` on PATH for richer APK metadata inspection.

### For developers

- Go 1.21 or newer.
- For Fyne GUI builds:
  - Windows: a working C toolchain such as MinGW-w64.
  - Linux CI/dev: OpenGL/X11 development packages.
  - macOS: Xcode command line tools.

## Build and run

### Native desktop GUI

```bash
cd GUI
go run .
```

### Windows executable

```powershell
.\scripts\build-windows.ps1 -Version dev
```

The script runs GUI tests, builds `dist\AdbPureFlow-dev-windows-amd64.exe`, and writes `dist\checksums-dev.txt`. Use `-SkipTests` only when dependency/network issues are already understood and tests have been run elsewhere.

Manual equivalent:

```bash
cd GUI
go build -trimpath -ldflags="-s -w -H windowsgui -X main.version=dev" -o ..\dist\AdbPureFlow-dev-windows-amd64.exe .
```

The local build script is the documented Windows build path. A dedicated Windows CI workflow should be added once repository workflow-file permissions are available.

### Legacy CLI helper

The CLI remains available while the desktop migration continues:

```bash
cd CLI
go run .
```

## Main workflows

### Connect a device over USB

1. Enable Developer Options and USB Debugging on Android.
2. Connect the phone with a data-capable USB cable.
3. Unlock the phone and accept the RSA debugging prompt.
4. Open AdbPureFlow and press **Refresh**.
5. Select the device from the device selector.

### Add a wireless device

1. On Android 11+, open **Developer options → Wireless debugging**.
2. Choose **Pair device with pairing code**.
3. In AdbPureFlow, choose **Add Wireless**.
4. Enter the pairing address and pairing code exactly as Android shows them.
5. Enter the normal connect address/port shown by Android Wireless debugging.
6. AdbPureFlow runs the real `adb pair` and `adb connect` flow, then refreshes devices.

QR pairing is not faked. If a future implementation supports Android’s QR payload format reliably, it can be added without changing the device model or ADB service.

### Mirror and control the screen

1. Select an authorized device.
2. Open **Screen**.
3. Choose quality/fullscreen/always-on-top options.
4. Press **Start Mirror**.

AdbPureFlow manages the scrcpy process and opens a normal native scrcpy companion window. Use **Stop Mirror** to end the managed process. Screenshots and recordings use native Save File dialogs; after a screenshot is saved, AdbPureFlow can hand off to Explorer.

### Install an APK

- Drag a `.apk` file onto the window, or use the command palette (`Ctrl+K`) → **Install APK**.
- Review APK information.
- Choose install options and press **Install to selected device**.
- AdbPureFlow installs with ADB, optionally clears app data, and optionally launches the app when the package ID is known.

### Manage apps

1. Select a device.
2. Open **Apps**.
3. Search installed user applications.
4. Use actions: **Launch**, **Stop**, **Clear**, **Uninstall**.

### Read logs

1. Select a device.
2. Open **Logs**.
3. Press **Start**.
4. Filter by text/tag/package/message and by log level.
5. Pause/resume, stop, or clear device logs as needed. The visible list keeps the latest 2,000 entries for responsiveness.

### Deploy

The Deploy workspace runs a deterministic developer flow:

1. Choose an APK artifact.
2. Optionally set package name if metadata extraction cannot infer it.
3. Choose options: clear data, start mirror, open logs.
4. Press **Deploy**.

The underlying workflow engine executes steps with cancellation/error state and is designed to support persistent user workflows later.

## Application data and tools

AdbPureFlow stores non-sensitive state such as onboarding completion, selected theme, recently used APK paths, remembered wireless endpoints, and the last selected device serial in the current user's config directory. Downloaded external tools are stored under the current user's cache/config tools directory, not beside the executable, so a normal Windows installation should not require administrator rights.

## Diagnostics

Open **Diagnostics** to check:

- ADB availability and version.
- ADB server responsiveness.
- Connected/authorized devices.
- scrcpy availability.

Failures include human recovery suggestions and expandable technical details.

## Tests

Hardware-free tests are the default:

```bash
cd GUI
go test ./...

cd ../CLI
go test ./...
```

Tests cover parsers, APK metadata handling, archive discovery, and legacy package detection. Future integration tests that require real Android hardware should be gated behind an explicit environment variable so normal CI does not depend on connected devices.

## Release notes

The local Windows release-candidate layout is produced by `scripts/build-windows.ps1`. A dedicated Windows GitHub Actions workflow is intentionally not included in this branch because the current automation credentials cannot update `.github/workflows/` files.

The expected Windows release layout is:

```text
AdbPureFlow-<version>-windows-amd64.exe
checksums-<version>.txt
README-WINDOWS.txt
```

ADB and scrcpy are not bundled in that layout unless a future release explicitly documents licensing, provenance, and update policy. Future release work should add Windows version resources, icons, signing, and an installer/package without requiring administrator privileges unless a concrete feature needs it.

## Repository layout

```text
GUI/        Native desktop application and Android workstation core
CLI/        Legacy command-line APK helper retained during migration
docs/       Architecture/audit documentation
website/    Existing static project page
.github/    CI/CD workflow
```

## Security

AdbPureFlow executes external tools directly with argument arrays rather than shell command strings where practical. Archive extraction validates paths to prevent Zip Slip/Tar Slip traversal. No credentials or telemetry are hardcoded.
