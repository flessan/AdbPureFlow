# AdbPureFlow repository audit

Date: 2026-09-09

## Starting point

The repository contained three separate surfaces:

- `GUI/`: a Fyne application with a single-window APK install/uninstall and scrcpy launcher.
- `CLI/`: a console APK install/launch/uninstall helper.
- `website/`: a static promotional page.

The Go code was useful but tightly coupled to UI callbacks and direct `exec.Command` calls. ADB discovery, archive extraction, package parsing, APK install flow, and scrcpy provisioning were duplicated or embedded in top-level `main` packages. Existing tests covered package-diff detection, package parsing, and scrcpy folder discovery. CI builds CLI and GUI artifacts, including Windows GUI builds with `-H windowsgui`.

## Retained functionality

- Automatic ADB/platform-tools provisioning with zip-slip protection.
- Automatic scrcpy discovery/provisioning with archive traversal protection.
- APK drag/drop and install-to-device flow.
- Package launch, uninstall, and clean-up actions.
- scrcpy-powered screen mirroring.
- Existing CLI remains intact for users who still want the console helper.

## Reworked direction

The GUI is now the product center: a Windows-first native desktop workstation named **AdbPureFlow**. The shell is device-centric, with the selected Android device visible across workspaces:

- Devices
- Screen
- Apps
- Logs
- Deploy
- Workflows
- Diagnostics
- Help & Tutorials

The implementation remains Fyne/Go rather than an Electron or web wrapper. Fyne provides native windows, native file dialogs, DPI-aware rendering, clipboard/desktop integration paths, keyboard shortcuts, and a normal Windows executable build.

## Architectural changes

`GUI/app.go` now provides a typed native core:

- `CommandRunner`, `OSRunner`, `CommandResult`, `CommandError`, and `ManagedProcess` for safe direct process execution, cancellation, timeouts, output capture, streaming, and process lifecycle.
- `ADB` service for device discovery, device enrichment, wireless pairing/connect, install, launch, force-stop, clear data, uninstall, package list, logcat streaming, log clearing, and screenshot capture.
- Domain models: `Device`, `APKMetadata`, `PackageInfo`, `LogEntry`, `FlowProfile`, `FlowAction`, and `DiagnosticCheck`.
- `ScrcpyManager` for scrcpy discovery/provisioning and managed mirror process lifecycle.
- APK metadata inspection that validates APK zip structure, computes size/SHA-256, and reads package/version/SDK/permission metadata when Android build-tools `aapt` is available.
- Parser functions for ADB device output, battery output, screen size, storage, package lists, aapt badging, and logcat threadtime rows.
- A small workflow engine used by Deploy and designed for future persistence/editor work.

The GUI no longer treats raw ADB output as the main interface. Operations expose human-oriented labels and dialogs while preserving technical details in diagnostics and errors.

## Known deliberate limitations

- Wireless debugging uses the real supported `adb pair host:port code` and `adb connect host:port` flow. QR pairing is not faked. A future enhancement can add Android Studio compatible QR payload generation/discovery if implemented against documented/verified pairing service behavior.
- Detailed APK manifest metadata requires `aapt` on PATH. Without it, AdbPureFlow still validates the APK container and shows size/SHA-256 with a clear warning.
- File management intentionally avoids an in-app fake Explorer. Current file interactions use native open/save dialogs; future Android file transfer should continue to prefer Windows Explorer/native shell flows where practical.
- Persistent visual workflow editing is not exposed yet. The reliable engine foundation exists and Deploy uses it.

## Testing and verification strategy

Normal tests should remain hardware-free. Physical-device integration tests should be gated behind an explicit environment variable or manual matrix so normal CI does not depend on connected devices. Current unit coverage focuses on parser/domain behavior, archive discovery, process targeting, validation, state persistence, workflow failure propagation, and selected-device reconciliation. External process execution is behind `CommandRunner`, so tests can inject fakes.

The repository now separates three evidence levels:

1. **Unit/core verification**: deterministic Go tests that do not require Fyne, Windows, ADB, scrcpy, or hardware.
2. **Windows CI verification**: GitHub Actions on `windows-latest` that resolves Fyne, runs full GUI/CLI tests and vet, builds the executable, and validates release artifacts/checksums.
3. **Manual Windows + Android verification**: `docs/WINDOWS_SMOKE_TEST.md` and `docs/DEVICE_TEST_MATRIX.md`, required before claiming real-device readiness.
