# AdbPureFlow Roadmap

AdbPureFlow is moving from a lightweight APK installer into a Windows-first native Android device management and development workstation.

## Implemented foundation

- Native Go/Fyne desktop shell with device-oriented navigation.
- Typed ADB device, APK, app/package, log, diagnostics, and workflow models.
- Safe process runner abstraction with context cancellation, timeouts, captured output, streaming output, and managed child processes.
- USB/wireless device listing and device enrichment.
- Real `adb pair` / `adb connect` wireless debugging workflow using pairing codes.
- scrcpy-managed screen mirroring, screenshots, and recording-to-file start flow.
- APK inspect/install workflow with optional `aapt` metadata.
- Apps workspace with search and package actions.
- Logs workspace with live logcat streaming and filtering.
- Deploy workflow backed by the workflow engine.
- Diagnostics and onboarding/help surfaces.
- Hardening pass for process cleanup, bounded log buffering, passive device monitoring, persistent non-sensitive state, theme setting, clipboard actions, and stricter ADB result validation.
- Manual Windows smoke test, real-device matrix, troubleshooting bundle, release checklist, hardened Windows build script, and Windows release README template.

## Next high-priority work

1. Add a dedicated Windows Verification GitHub Actions workflow once workflow-file permissions are available, then fix any full Fyne/Windows compile failures it exposes.
2. Execute the Windows smoke test and fill the real-device matrix with at least the required device coverage.
3. Add configurable tool paths, known device aliases UI refinements, tutorial step progress, and workflow profiles to the existing per-user state model.
4. Improve Windows packaging with version resources, icons, code signing support, and an installer.
5. Add richer Android app metadata by bundling or discovering Android build-tools reliably.
6. Improve mirror/recording visible state by surfacing scrcpy process exit reasons in the GUI.
7. Implement package-focused log filters by mapping selected app package to PID/process state.
8. Add integration-test harness gated behind `ADBPUREFLOW_DEVICE_SERIAL`.
9. Add real QR pairing only after the payload format/service-discovery behavior is verified; do not ship simulated QR pairing.
10. Add Explorer-based file-transfer helpers where Android/Windows integration allows it without recreating a file manager.

## Non-goals

- Electron/web wrapper UI.
- Fake terminal output presented as successful operations.
- Decorative node editor before workflow persistence/execution semantics are mature.
- Custom in-app file manager that duplicates Windows Explorer for ordinary file management.
