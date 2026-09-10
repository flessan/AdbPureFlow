AdbPureFlow for Windows
=======================

This release layout contains:

- AdbPureFlow-<version>-windows-amd64.exe
- checksums-<version>.txt
- README-WINDOWS.txt

Status
------

AdbPureFlow is a Windows-first native Android device workstation. The executable is built from the Go/Fyne GUI. ADB and scrcpy are not bundled in this release artifact unless a future release explicitly says otherwise.

Runtime tools
-------------

AdbPureFlow looks for adb and scrcpy on PATH first. Startup, passive device monitoring, and Diagnostics do not download missing tools merely to make checks pass. If a supported platform download is available and a user-initiated operation requires the tool, AdbPureFlow may provision Android platform-tools or scrcpy into the current user's cache/config tools directory.

Normal use should not require administrator privileges.

Before first use
----------------

1. Enable Developer Options on your Android device.
2. Enable USB Debugging.
3. For Wireless Debugging, use Android 11+ and open Developer options -> Wireless debugging.
4. Start AdbPureFlow.
5. Open Diagnostics if the device does not appear.

Verification
------------

Before trusting a release for daily use, run the manual checklist in docs/WINDOWS_SMOKE_TEST.md against a real Windows machine and at least one Android device.

QR pairing
----------

Wireless Debugging QR pairing is not implemented in this release. Use Pairing Code mode. AdbPureFlow intentionally does not generate fake IP-only QR codes.

Troubleshooting
---------------

Use the Diagnostics workspace first. Keep the technical-details output when reporting bugs, but do not post private device data or app logs publicly unless you have reviewed them.
