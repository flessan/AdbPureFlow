# Release checklist

A release candidate is not ready until every applicable gate below is complete.

## CI/build gates

Required for every Windows release candidate:

- [ ] Run `scripts/build-windows.ps1 -Version <version>` successfully on Windows, or run an equivalent Windows CI workflow once workflow-file permissions are available.
- [ ] `go mod download` succeeds for `GUI/` and `CLI/`.
- [ ] Go formatting check passes (`gofmt -l` reports no files).
- [ ] `go test ./...` passes for `GUI/`.
- [ ] `go test ./...` passes for `CLI/`.
- [ ] `go vet ./...` passes for `GUI/`.
- [ ] `go vet ./...` passes for `CLI/`.
- [ ] `scripts/build-windows.ps1 -Version <version>` produces `dist/AdbPureFlow-<version>-windows-amd64.exe`.
- [ ] `dist/checksums-<version>.txt` exists and matches the executable SHA-256.
- [ ] `dist/README-WINDOWS.txt` exists.
- [ ] No generated binaries, DLLs, ADB platform-tools, scrcpy archives, or cache directories are tracked by Git.

## Manual Windows gates

Required before claiming Windows runtime verification:

- [ ] Run `docs/WINDOWS_SMOKE_TEST.md` on a real Windows 10/11 machine.
- [ ] Verify startup/shutdown without administrator privileges.
- [ ] Verify native open/save dialogs.
- [ ] Verify Explorer/file-manager handoff after screenshot.
- [ ] Verify notifications and clipboard actions.
- [ ] Verify paths containing spaces and non-ASCII characters.
- [ ] Verify persisted settings after app restart.

## Real Android device gates

Required before claiming real-device verification:

- [ ] Fill at least the required rows in `docs/DEVICE_TEST_MATRIX.md`.
- [ ] Verify USB device detection/authorization/enrichment.
- [ ] Verify Wireless Debugging pairing-code flow on Android 11+.
- [ ] Verify remembered wireless endpoint reconnect and forget.
- [ ] Verify scrcpy mirror start/stop and shutdown cleanup.
- [ ] Verify screenshot and recording outputs.
- [ ] Verify APK install/reinstall/launch/failure handling.
- [ ] Verify Apps actions with destructive confirmation.
- [ ] Verify Logs start/stop/filter under realistic output.
- [ ] Verify disconnect/reconnect behavior during active operations.

## Documentation gates

- [ ] README status labels match evidence.
- [ ] QR pairing is not claimed unless implemented and tested.
- [ ] scrcpy is described as a managed companion window unless true embedding exists.
- [ ] Troubleshooting reflects real Diagnostics messages.
- [ ] Release artifact layout is documented.
- [ ] Known limitations remain visible.

## Release artifact layout

A Windows release candidate should contain exactly:

```text
AdbPureFlow-<version>-windows-amd64.exe
checksums-<version>.txt
README-WINDOWS.txt
```

ADB and scrcpy are not bundled unless a future release explicitly documents licensing, provenance, and update policy.
