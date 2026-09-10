# Wireless Debugging support

AdbPureFlow currently supports Android's real pairing-code workflow:

1. On Android 11+, open **Developer options → Wireless debugging**.
2. Choose **Pair device with pairing code**.
3. Enter the pairing address and six-digit code in AdbPureFlow.
4. Enter the Wireless debugging connect address shown by Android.
5. AdbPureFlow runs `adb pair <pairing-host:port> <code>`, then `adb connect <connect-host:port>`, then verifies that the device appears in `adb devices -l`.

## QR pairing status

QR pairing is **not implemented** in this release. This is deliberate.

Android's Wireless Debugging QR flow is not equivalent to a QR code containing an IP address. Shipping such a QR would be misleading and would not perform real ADB pairing. AdbPureFlow should only add QR pairing after the genuine ADB/Android pairing payload and service behavior are implemented and tested against real Android devices.

Until then, pairing-code mode is the supported, truthful fallback.

## Stored data

AdbPureFlow may remember non-sensitive connect endpoints such as `192.168.1.24:5555` so users can retry reconnecting later. It does not store pairing codes or credentials.

## Common failures

- **Pairing expired:** reopen Android Wireless debugging and request a fresh pairing code.
- **Network unreachable / connection refused:** confirm the PC and phone are on the same network, Wireless debugging is still enabled, and firewall/VPN rules are not blocking the connection.
- **Device does not appear after connect:** Android may have rotated the debugging port; check the current Wireless debugging screen and reconnect with the current address.
