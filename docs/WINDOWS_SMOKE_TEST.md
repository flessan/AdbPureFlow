# Windows smoke test checklist

Use this checklist on a real Windows 10/11 machine with at least one physical Android device. Record the app version, Windows version, Android model/version, connection type, and whether ADB/scrcpy came from PATH or AdbPureFlow provisioning.

Do not mark a release as Windows/device verified until this checklist has been run. A CI build proves compilation only; it does not prove real Android behavior.

## Test setup

- Build or download `AdbPureFlow-<version>-windows-amd64.exe`.
- Keep `checksums-<version>.txt` and `README-WINDOWS.txt` next to the executable for release validation.
- Use a Windows user account without administrator elevation unless a test explicitly says otherwise.
- Prepare one small debug APK that is safe to install/uninstall.
- If possible, prepare a second Android device for the multiple-device test.

For each step capture:

- Expected behavior: what should happen.
- Failure symptoms: what indicates a bug.
- Useful diagnostics: what to copy from Diagnostics, status text, dialogs, or logs.

## 1. Installation/startup

1. Place the executable in a folder whose path contains spaces and non-ASCII characters, for example `C:\Users\<you>\Desktop\AdbPureFlow Test æ`.
2. Start the executable normally, not as administrator.

Expected behavior: main AdbPureFlow window opens, title includes the version, app starts on Devices, no console window is required.

Failure symptoms: app does not start, asks for administrator permission, crashes, or immediately downloads tools without user action.

Useful diagnostics: Windows Event Viewer entry, screenshot of startup error, executable path.

## 2. First-launch onboarding

Expected behavior: a first-run dialog appears with “Baru di sini? / New here?” and choices to show help/tutorial or skip.

Failure symptoms: no first-launch choice on a clean profile, dialog cannot be closed, wording references old Pro/CLI-only product.

Useful diagnostics: screenshot and `%APPDATA%`/config location shown in Settings.

## 3. Skip tutorial

Choose Skip.

Expected behavior: status says “Wah, anda sudah pro...” and points to Settings → Help & Tutorials.

Failure symptoms: tutorial still blocks the app, skip is not persisted, message points to a missing area.

Useful diagnostics: status text and Settings screenshot.

## 4. Reopen Help & Tutorials

Open Settings, then Help & Tutorials.

Expected behavior: help opens and describes actual workspaces: Devices, Wireless Debugging, Screen, APK, Apps, Logs, Deploy.

Failure symptoms: help references controls that do not exist or claims QR pairing is available.

Useful diagnostics: screenshot of inaccurate text.

## 5. ADB diagnostics

Open Diagnostics before connecting a device.

Expected behavior: ADB availability/capability is reported accurately. If ADB is missing, the message explains installation/provisioning without claiming device health.

Failure symptoms: Diagnostics reports healthy ADB when `adb version` cannot run, or downloads tools silently just to run Diagnostics.

Useful diagnostics: all Diagnostics technical-details sections.

## 6. USB device connection

Enable USB Debugging on Android, connect via USB, unlock the device, and accept the RSA prompt.

Expected behavior: device appears in Devices after passive refresh or manual Refresh, with online/authorized status.

Failure symptoms: device never appears, appears under wrong state, or AdbPureFlow chooses an arbitrary target when more than one device is present.

Useful diagnostics: Diagnostics output and `adb devices -l` from a terminal for comparison.

## 7. Device metadata

Select the USB device.

Expected behavior: manufacturer/model/Android version/battery/storage/screen details appear when Android exposes them.

Failure symptoms: metadata belongs to another device, stale metadata remains after disconnect, or missing metadata is shown as success.

Useful diagnostics: Devices screenshot and Diagnostics technical details.

## 8. Multiple-device selection

Connect a second Android device if available.

Expected behavior: both devices are listed; device-specific workspaces require the explicitly selected device; target serial is stable.

Failure symptoms: operations target the wrong device, selection changes unexpectedly after refresh, or no clear selected-device marker exists.

Useful diagnostics: Devices screenshot before/after refresh.

## 9. Wireless Debugging pairing-code workflow

On Android 11+, open Developer options → Wireless debugging → Pair device with pairing code. In AdbPureFlow choose Add Wireless and enter pairing address, code, and connect address.

Expected behavior: states progress through pairing and connecting. After connect, the wireless serial appears in Devices and endpoint is remembered.

Failure symptoms: fake QR behavior, arbitrary IP QR code, pairing result shown as success when ADB output says failure, endpoint not remembered.

Useful diagnostics: AdbPureFlow error text and Android Wireless debugging screen values. Do not publish pairing codes after use.

## 10. Wireless reconnect

Disconnect the wireless device, then use the known endpoint Reconnect action.

Expected behavior: AdbPureFlow runs connect, refreshes Devices, and clearly reports success or unreachable state.

Failure symptoms: reconnect claims success but device never appears, wrong endpoint is used, or stale connected state remains.

Useful diagnostics: status text and Diagnostics output.

## 11. Wireless forget

Use Forget on the known wireless endpoint.

Expected behavior: endpoint is removed from Devices and does not return after app restart unless connected again.

Failure symptoms: endpoint persists after forget or removes unrelated device state.

Useful diagnostics: Devices screenshot before/after restart.

## 12. Screen mirror

Select an online device and open Screen. Start Mirror.

Expected behavior: scrcpy companion window opens for the selected serial; status says mirror is active in a companion window.

Failure symptoms: wrong device mirrored, raw terminal window required, status says embedded when it is not, or failure is reported as success.

Useful diagnostics: scrcpy version/source from Diagnostics and status text.

## 13. Mirror stop

Press Stop Mirror.

Expected behavior: scrcpy window exits, status changes to stopped, no scrcpy process remains in Task Manager for that managed session.

Failure symptoms: orphaned process, permanent active status, app hangs.

Useful diagnostics: Task Manager process list and AdbPureFlow status.

## 14. Screenshot

Use Capture Screenshot, save to a path with spaces/non-ASCII characters, then choose to open the folder.

Expected behavior: PNG is written, dialog confirms save, Explorer opens/selects the file.

Failure symptoms: empty file, wrong file path, Explorer does not open, or success appears after failure.

Useful diagnostics: save path, file size, error dialog.

## 15. Recording

Use Start Mirror + Record, choose an MP4/MKV path, interact briefly, then Stop Mirror.

Expected behavior: recording state is visible, stopping mirror finalizes the file, AdbPureFlow verifies non-empty output.

Failure symptoms: empty/missing recording reported as success, cannot stop recording, orphaned scrcpy process.

Useful diagnostics: recording path, file size, status messages.

## 16. APK drag/drop

Drag a valid APK onto the app.

Expected behavior: APK inspection opens with file name, size, SHA-256, warnings if metadata tooling is missing, and package/version when `aapt` is available.

Failure symptoms: non-APK accepted as APK, drag path mangled, app freezes on large APK.

Useful diagnostics: file path and inspection output.

## 17. APK installation

Install the inspected APK to the selected device.

Expected behavior: install runs against the selected serial and only reports success after ADB reports success.

Failure symptoms: ambiguous multiple-device install, false success on ADB failure, unhelpful raw error only.

Useful diagnostics: install dialog/status and package name.

## 18. APK launch

Enable Launch after install when package metadata is available.

Expected behavior: install success is distinct from launch success. If launch fails, message says install succeeded but launch failed.

Failure symptoms: launch failure shown as complete success or install failure without distinction.

Useful diagnostics: package name and error dialog.

## 19. Apps list

Open Apps.

Expected behavior: installed user apps load; search filters by package; optional system-app toggle adds system apps and labels user/system rows.

Failure symptoms: wrong device list, permanent loading state, no distinction when system apps are included.

Useful diagnostics: selected device and Apps screenshot.

## 20. Force stop

Force stop the test app.

Expected behavior: action targets selected device and reports completion/failure accurately.

Failure symptoms: silent failure or wrong target.

Useful diagnostics: status and package name.

## 21. Clear data

Clear data for the test app after confirmation.

Expected behavior: confirmation appears; Android `pm clear` success is required before completion is shown.

Failure symptoms: destructive action without confirmation, false success, wrong package.

Useful diagnostics: confirmation and result text.

## 22. Uninstall

Uninstall the test app after confirmation.

Expected behavior: confirmation appears; Apps refreshes after uninstall; Android success is required.

Failure symptoms: app remains installed but UI says success, no confirmation, Apps list stale.

Useful diagnostics: package name, result text, Apps screenshot.

## 23. Logcat

Open Logs and Start.

Expected behavior: log rows stream for selected device without blocking the UI.

Failure symptoms: UI freezes, wrong device logs, cannot stop stream.

Useful diagnostics: status text and sample rows.

## 24. Log filtering

Use text filter and level filter.

Expected behavior: visible rows update; latest-2,000-entry buffer behavior is understood from UI text.

Failure symptoms: filters ignored, filtering crashes, memory grows unbounded.

Useful diagnostics: filter values and screenshots.

## 25. Deploy

Choose the test APK, set package if needed, choose launch and optionally mirror/logs, then Deploy.

Expected behavior: steps report progress; failure stops the sequence; Cancel requests cancellation.

Failure symptoms: deploy complete after failed step, cancel ignored, wrong device target.

Useful diagnostics: status sequence and error text.

## 26. Device disconnect during operation

While mirroring or streaming logs, disconnect the device.

Expected behavior: Devices eventually reports unavailable/disconnected; mirror/log state stops or reports failure without hanging.

Failure symptoms: permanent active status, goroutine/process leak, app crash.

Useful diagnostics: status text, Task Manager, Diagnostics.

## 27. Device reconnect

Reconnect USB or wireless endpoint.

Expected behavior: passive monitor or Refresh sees the device again; previous selected serial is restored only when visible.

Failure symptoms: wrong device selected, stale unavailable state remains.

Useful diagnostics: Devices screenshot before/after.

## 28. Application restart

Close and reopen AdbPureFlow.

Expected behavior: app starts cleanly, no old scrcpy process remains, non-sensitive preferences are restored.

Failure symptoms: crash on startup, duplicate background processes, preferences lost unexpectedly.

Useful diagnostics: Task Manager and Settings path.

## 29. Persisted settings

Change theme, rename a device, add/forget wireless endpoint, then restart.

Expected behavior: theme and aliases persist; forgotten endpoint remains forgotten; no pairing codes are stored.

Failure symptoms: corrupted settings prevent startup, secrets stored in state file, settings written beside executable under Program Files.

Useful diagnostics: state JSON path from Settings, after reviewing for private data.

## 30. Application shutdown

Start mirror/logs, then close the main window.

Expected behavior: log stream stops, managed scrcpy processes exit, app process exits cleanly.

Failure symptoms: AdbPureFlow or scrcpy remains running unintentionally, recording file is corrupt without explanation.

Useful diagnostics: Task Manager and final status/error before close.
