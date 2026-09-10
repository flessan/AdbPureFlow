# Changelog

## Unreleased

### Changed

- Repositioned AdbPureFlow as a Windows-first native Android device management and development workstation.
- Reworked the GUI from a single APK installer window into a device-centric shell with Devices, Screen, Apps, Logs, Deploy, Workflows, Diagnostics, and Help workspaces.
- Introduced typed domain models and a process-runner abstraction for ADB/scrcpy operations.
- Rewrote documentation to match the desktop product direction.

### Added

- Real wireless debugging pairing-code workflow using `adb pair` and `adb connect`.
- Device enrichment for manufacturer, model, Android version, SDK, battery, screen size, and storage where available.
- Managed scrcpy lifecycle, mirror options, and screenshot capture through native Save File dialogs.
- APK inspection with zip validation, SHA-256, size, and optional `aapt` badging metadata.
- Apps workspace with searchable user-app list and actions for launch, force-stop, clear data, and uninstall.
- Logs workspace with live `logcat` streaming, filtering, pause/resume, and clear behavior.
- Deploy workflow backed by an extensible Flow engine.
- Diagnostics/Doctor checks for ADB, ADB server, devices/authorization, and scrcpy.
- First-launch onboarding and Help & Tutorials content.
- Windows build helper script with SHA-256 checksum output.
- Persistent non-sensitive app state in per-user config storage.
- Removed checked-in executable/platform-tools artifacts; runtime tools are now provisioned/discovered and ignored by Git.
- Passive device monitor, strict package/address validation, deterministic selected-device reconciliation, bounded log buffering, log stop/level filtering, deploy cancellation, screen recording entry point, clipboard actions, theme setting, device aliases, wireless reconnect/forget actions, screenshot file-manager handoff, and hardened Windows build script.
- Manual Windows smoke test, real-device test matrix, troubleshooting bundle, release checklist, and Windows release README template.

### Retained

- Legacy CLI helper remains available during migration.
- ADB and scrcpy auto-provisioning retain archive traversal protections.
