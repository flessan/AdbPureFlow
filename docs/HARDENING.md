# Hardening audit notes

This document records the follow-up audit after the initial native GUI migration.

## Findings from the first migration

- The GUI direction was correct, but several callbacks still claimed completion after optional follow-up actions failed. APK install now stops/report failures when optional clear-data or launch fails.
- Diagnostics previously called normal provisioning paths for ADB/scrcpy. That could download tools during a diagnostic check and report availability too optimistically. Diagnostics now probe existing tools first and run actual `adb version` / `scrcpy --version` capability checks when binaries are found.
- The log streaming runner buffered unlimited output. Streaming output is now capped at 256 KiB inside the process runner, and the GUI log view keeps a bounded 2,000-entry buffer with throttled refreshes.
- Managed process completion used a single error channel that could be consumed by either `Stop` or the cleanup goroutine, risking leaked goroutines. Managed processes now close a done channel that can be observed by multiple waiters.
- Device refresh could overlap manual refreshes and passive monitoring. A refresh mutex now prevents overlapping scans.
- The app selected the first connected device after refresh. It now restores a remembered selected serial, auto-selects only when exactly one device is connected, and otherwise requires explicit selection.
- User preferences were split across Fyne preferences only. A JSON state model now stores non-sensitive settings in per-user config storage with a schema version.
- Tool provisioning paths were relative to the executable directory. They now default to per-user cache/config locations so a Windows install under Program Files does not need administrator rights.
- Wireless address validation accepted any string containing a colon. It now validates host:port shape and numeric port before calling ADB.
- Package actions accepted arbitrary strings. Package names now pass a conservative Android package-name validator before ADB receives them.
- Flow contained a Start Logs action that did nothing but could appear complete. The core runner now rejects that action until a real streaming-flow contract exists; Deploy opens logs after successful execution instead.

## Implemented hardening

- Added application state model and state store.
- Added passive device monitoring every seven seconds with refresh reconciliation.
- Added explicit restart ADB server action in Diagnostics.
- Added screen recording through scrcpy using a native Save File dialog.
- Added clipboard integration for selected device serial and package names.
- Added theme setting with persistence.
- Added explicit Deploy cancellation.
- Added device alias editing, known wireless endpoint reconnect/forget actions, and context-aware command palette behavior.
- Added screenshot post-save system file-manager handoff and scrcpy recording lifecycle feedback that verifies output existence after the companion window exits.
- Added log stop control and level filtering, while keeping the bounded latest-2,000-entry visible buffer explicit in the UI.
- Added deterministic selected-device reconciliation so multiple devices do not cause arbitrary retargeting.
- Added tests for process targeting, APK install success validation, package/address validation, error mapping, app state persistence/corruption recovery, bounded buffers, workflow failure propagation, selected-device reconciliation, log level mapping, and recent-path de-duplication.
- Removed tracked executable/platform-tools artifacts from the source tree and added ignore rules for generated/runtime-provisioned binaries.

## Verification performed in this sandbox

- `/tmp/gotool/node_modules/hxjiang-go/go/bin/hxjiang-gofmt -w GUI/app.go GUI/main.go GUI/app_test.go CLI/main.go CLI/main_test.go` completed in this sandbox.
- `cd GUI && /tmp/gotool/node_modules/hxjiang-go/go/bin/hxjiang-go test app.go app_test.go` passed. This compiles and tests the hardware-free application core without importing Fyne.
- `cd GUI && /tmp/gotool/node_modules/hxjiang-go/go/bin/hxjiang-go test -race app.go app_test.go` passed for the hardware-free core.
- `cd GUI && /tmp/gotool/node_modules/hxjiang-go/go/bin/hxjiang-go vet app.go` passed for the hardware-free core.
- `cd CLI && /tmp/gotool/node_modules/hxjiang-go/go/bin/hxjiang-go test ./...` passed.
- `cd CLI && /tmp/gotool/node_modules/hxjiang-go/go/bin/hxjiang-go vet ./...` passed.
- `git diff --check` passed.
- Full `cd GUI && go test ./...` could not complete in this sandbox because Fyne dependencies could not be fetched from `proxy.golang.org`, `goproxy.io`, `goproxy.cn`, or `fyne.io` due repeated EOF/network failures. A Windows machine running `scripts/build-windows.ps1` is currently the authoritative check for full Fyne GUI dependency resolution and Windows compilation until workflow-file permissions are available.

## Remaining high-priority work

- Run a real Windows build and smoke test with the actual Fyne dependency graph.
- Add Windows icon/version resources and installer/signing support.
- Add configurable ADB/scrcpy paths in Settings.
- Add real device integration tests gated by `ADBPUREFLOW_DEVICE_SERIAL`.
- Add a robust log filter by selected package/PID instead of only text filtering.
- Persist workflow profiles once the flow action contract is broader than Deploy.
