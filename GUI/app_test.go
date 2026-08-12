package main

import (
	"testing"
)

// The previous GUI tests exercised helpers (parsePackages, findScrcpyFolder)
// that have been moved into the shared `internal/adb` package along with
// their test coverage. This file now acts as a placeholder so `go test ./GUI`
// continues to pass without requiring a real device or an adb binary at
// test time — see internal/adb/*_test.go for the behavioral tests.
func TestGUIAppStub(t *testing.T) {
	// Nothing to assert: the GUI is a presentation layer that can only be
	// exercised interactively or with a display; unit tests for core ADB
	// behavior live in internal/adb.
}
