package adb

import "testing"

// Device parser / display tests are in adb_test.go; this file is reserved
// for tests that require mocking the adb client (future integration tests)
// or heavier device logic. For now, keep a tiny build-warmup test so `go
// test ./...` exercises this file as code is added.
func TestDeviceStateConstants(t *testing.T) {
	states := []DeviceState{StateDevice, StateOffline, StateUnauthorized, StateBootloader, StateRecovery}
	for _, s := range states {
		if s == "" {
			t.Error("empty DeviceState const")
		}
	}
}
