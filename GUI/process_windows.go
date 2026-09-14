//go:build windows

package main

import "os/exec"

// configureCommand prevents console-based helper processes such as adb.exe
// from flashing a transient console window when launched by the GUI app.
func configureCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}
