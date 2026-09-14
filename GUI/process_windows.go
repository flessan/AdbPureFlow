//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

const createNoWindow = 0x08000000

// configureCommand prevents console-based helper processes such as adb.exe
// from creating a transient console window while leaving GUI applications
// such as scrcpy free to create and show their own windows.
func configureCommand(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNoWindow}
}
