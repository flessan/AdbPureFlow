//go:build windows

package main

import (
	"os/exec"
	"testing"
)

func TestConfigureCommandUsesNoConsoleCreationFlag(t *testing.T) {
	cmd := exec.Command("cmd.exe", "/c", "exit", "0")
	configureCommand(cmd)
	if cmd.SysProcAttr == nil {
		t.Fatal("expected Windows process attributes")
	}
	if cmd.SysProcAttr.CreationFlags != createNoWindow {
		t.Fatalf("expected CREATE_NO_WINDOW flag, got 0x%x", cmd.SysProcAttr.CreationFlags)
	}
	if cmd.SysProcAttr.HideWindow {
		t.Fatal("scrcpy GUI must not receive SW_HIDE")
	}
}
