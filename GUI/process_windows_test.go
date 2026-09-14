//go:build windows

package main

import (
	"os/exec"
	"testing"
)

func TestConfigureCommandHidesWindowsWindow(t *testing.T) {
	cmd := exec.Command("cmd.exe", "/c", "exit", "0")
	configureCommand(cmd)
	if cmd.SysProcAttr == nil {
		t.Fatal("expected Windows process attributes")
	}
	if !cmd.SysProcAttr.HideWindow {
		t.Fatal("expected child console window to be hidden")
	}
}
