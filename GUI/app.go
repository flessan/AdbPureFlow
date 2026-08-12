package main

// This file was originally the GUI's ad-hoc "App" type that bundled ADB
// helpers (getADBPath, runCommand, parsePackages, etc). All of that logic
// has moved to the shared `internal/adb` package, which is used by both the
// GUI and CLI. Only presentation-layer code (widget wiring, event handlers,
// layout) lives in this directory now.
//
// The package is kept as `package main` so `go build ./GUI` still produces
// a working executable. See internal/adb/{adb,devices,packages,scrcpy,manager}.go
// for the device/app management implementation.

