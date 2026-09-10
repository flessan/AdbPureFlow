# Real-device test matrix

This matrix is intentionally practical. It does not claim compatibility for devices or Android versions that have not been tested. Fill it in when running `docs/WINDOWS_SMOKE_TEST.md`.

## Status labels

- **Required:** must pass before calling a Windows device release production-ready.
- **Optional:** useful coverage but not required for the first release candidate.
- **Unsupported:** not expected to work in the current release.
- **Not tested:** no claim is made.
- **Pass / Fail:** result from a real Windows + Android run.

## Minimum required coverage

| Area | Required coverage | Result | Notes |
| --- | --- | --- | --- |
| Windows | Windows 10 or 11, non-admin user | Not tested | Record exact Windows build. |
| Build | `scripts/build-windows.ps1 -Version <version>` | Not tested | Must produce exe, checksum, README-WINDOWS.txt. |
| USB device | One Android 11+ physical device | Not tested | Must authorize and appear in Devices. |
| Wireless Debugging | One Android 11+ physical device on same network | Not tested | Pairing-code mode only. |
| scrcpy | USB mirror with selected device | Not tested | Companion window, start/stop, no orphan process. |
| APK workflow | Install/reinstall/launch one safe debug APK | Not tested | Must distinguish install vs launch/clear failure. |
| Apps | List/search/force-stop/clear/uninstall safe test app | Not tested | Destructive actions require confirmation. |
| Logs | Start/stop/filter logcat | Not tested | UI must remain responsive. |
| Disconnect/reconnect | USB disconnect/reconnect while app is open | Not tested | Selected serial restored only when visible. |
| Shutdown cleanup | Close app while mirror/logs are active | Not tested | Managed processes must exit. |

## Device rows

Add one row per real device tested.

| Date | Tester | Windows version | App version/commit | Manufacturer/model | Android version/API | Connection | USB devices | Wireless pair/connect | Mirror | APK install | Apps actions | Logs | Disconnect/reconnect | Result | Notes |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| TBD | TBD | TBD | TBD | TBD | TBD | USB | Not tested | N/A | Not tested | Not tested | Not tested | Not tested | Not tested | Not tested |  |
| TBD | TBD | TBD | TBD | TBD | Android 11+ | Wireless Debugging | N/A | Not tested | Not tested | Not tested | Optional | Not tested | Not tested | Not tested | Pairing-code workflow only. |

## Recommended device spread

Required before a production device release:

1. One Google/Pixel or AOSP-like Android 13+ device over USB.
2. One Samsung/Xiaomi/Oppo/Vivo/other OEM Android 11+ device over USB.
3. One Android 11+ device using Wireless Debugging pairing-code mode.

Optional before broader release:

- Android 10 USB-only device. Wireless Debugging is unsupported by Android itself before Android 11.
- Android 15+ current device.
- Device with non-English locale.
- Device connected through a USB hub.
- Two simultaneous devices to verify explicit selection.

Unsupported/currently not claimed:

- Wireless Debugging QR pairing.
- Devices without ADB debugging enabled.
- MTP/Explorer filesystem mounting as an in-app file manager.
- iOS or non-Android devices.

## Per-device required assertions

For every required device row, verify:

- Device appears in `adb devices -l` and in AdbPureFlow Devices.
- Unauthorized/offline states are shown correctly when forced.
- Selected serial remains stable across refresh.
- Device-specific operations include explicit serial targeting.
- scrcpy opens for the selected device and stops without orphan processes.
- APK install success requires Android/ADB success confirmation.
- Logcat stream can be stopped and does not freeze the GUI.
- App shutdown cleans up managed child processes.
