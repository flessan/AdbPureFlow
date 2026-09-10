# Troubleshooting bundle

Start in **Diagnostics**. It is designed to show human-readable status first and keep command output in expandable technical details.

Do not paste private app logs publicly without reviewing them first.

## ADB missing

Expected Diagnostics result: **ADB availability: Fail**.

Meaning: AdbPureFlow did not find `adb` in its per-user tools cache or on PATH.

Recovery:

1. Install Android platform-tools from Google, or allow AdbPureFlow to provision tools when a real ADB operation needs them.
2. Restart AdbPureFlow.
3. Reopen Diagnostics.

Useful technical details: configured/discovered ADB path, if any.

## ADB found but unusable / ADB server unavailable

Expected Diagnostics result: **ADB capability: Fail/Warn** or device scan failure.

Meaning: an `adb` executable exists but `adb version` or `adb devices -l` failed.

Recovery:

1. Press **Restart ADB Server** in Diagnostics.
2. Reconnect USB devices.
3. Check antivirus/quarantine if `adb.exe` cannot run.
4. Replace old platform-tools with the current release.

Useful technical details: `adb version` output, command error, stderr.

## No device

Expected Diagnostics result: **Connected devices: Warn**.

Recovery:

1. Enable Developer Options and USB Debugging.
2. Use a data-capable USB cable.
3. Unlock the Android device.
4. Try another USB port/cable.
5. For wireless, use Add Wireless and verify the phone and PC are on the same network.

Useful technical details: `adb devices -l` output.

## Unauthorized device

Expected Diagnostics result: **Authorization for <serial>: Warn**.

Recovery:

1. Unlock the phone.
2. Accept the RSA debugging prompt.
3. If no prompt appears, revoke USB debugging authorizations on Android and reconnect.
4. Refresh Devices.

Useful technical details: device line from `adb devices -l`.

## Offline device

Expected Diagnostics result: **Connection for <serial>: Warn**.

Recovery:

1. Reconnect USB or toggle Wireless debugging.
2. Restart ADB Server from Diagnostics.
3. Refresh Devices.

Useful technical details: device state and serial.

## Multiple devices

Expected behavior: AdbPureFlow should not silently target an arbitrary device. Select the intended device explicitly.

Recovery:

1. Open Devices.
2. Select the intended device.
3. Confirm the selected marker and top device selector before running APK, Apps, Logs, Screen, or Deploy actions.

Useful technical details: all visible device serials.

## Wireless Debugging unreachable

Expected user error: message mentions same network, Wireless debugging enabled, current port, firewall/VPN.

Recovery:

1. Confirm PC and Android are on the same Wi-Fi/network segment.
2. Disable VPN/firewall rules temporarily if safe.
3. Reopen Android Wireless debugging and copy the current connect address.
4. Use known endpoint Reconnect only if Android is still advertising that endpoint.

Useful technical details: `adb connect` output.

## Pairing failure or pairing expired

Expected user error: message asks for a fresh pairing code.

Recovery:

1. On Android, open Wireless debugging → Pair device with pairing code.
2. Enter the fresh pairing address and code.
3. Enter the connect address after pairing succeeds.
4. Do not reuse expired pairing codes.

Useful technical details: `adb pair` output. Do not share still-valid pairing codes.

## scrcpy unavailable

Expected Diagnostics result: **scrcpy: Warn**.

Recovery:

1. Install scrcpy and put it on PATH, or allow AdbPureFlow to provision it when starting Mirror.
2. Reopen Diagnostics.
3. Verify `scrcpy --version` from PowerShell if needed.

Useful technical details: scrcpy path and `scrcpy --version` output.

## scrcpy launch failure

Expected behavior: Screen reports Mirror failed; it must not claim Mirror active.

Recovery:

1. Verify the selected device is online and authorized.
2. Verify scrcpy is usable in Diagnostics.
3. Try lower size/bitrate options.
4. Restart ADB Server if scrcpy reports device not found.

Useful technical details: scrcpy error text if exposed, selected serial, options used.

## APK installation failure

Expected behavior: APK workflow reports Install failed and does not continue to clear/launch.

Recovery:

- `INSTALL_FAILED_VERSION_DOWNGRADE`: install a newer build, enable downgrade only for debug flows, or uninstall first.
- `INSTALL_FAILED_UPDATE_INCOMPATIBLE`: uninstall the existing app if signing keys changed.
- Device disconnected: reconnect and rerun install.
- Permission/storage problem: check Android screen for prompts and available storage.

Useful technical details: ADB install output, APK SHA-256, package ID if known.

## Device disconnect during operation

Expected behavior: passive monitoring reports the selected device unavailable; operations fail clearly or stop.

Recovery:

1. Reconnect USB or use Wireless Reconnect.
2. Refresh Devices.
3. Re-run the failed operation.

Useful technical details: operation name, status text, device serial.

## logcat failure

Expected behavior: Logs stream stops and shows an error instead of freezing.

Recovery:

1. Confirm selected device is online.
2. Restart ADB Server.
3. Start Logs again.
4. Use filters to reduce visible output.

Useful technical details: error dialog and selected serial.
