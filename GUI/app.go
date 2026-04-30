package main

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

type App struct {
	ctx context.Context
}

func NewApp() *App {
	return &App{}
}

// Startup diatur oleh Wails
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// RunADBPure adalah fungsi yang akan dipanggil dari tombol "Get Started" di HTML
func (a *App) RunADBPure(apkPath string) string {
	// 1. Scan before install
	before, _ := exec.Command("adb", "shell", "pm", "list", "packages").Output()

	// 2. Install APK
	installCmd := exec.Command("adb", "install", apkPath)
	if err := installCmd.Run(); err != nil {
		return "Gagal Install: " + err.Error()
	}

	// 3. Scan after install & find package
	after, _ := exec.Command("adb", "shell", "pm", "list", "packages").Output()
	
	packageID := ""
	linesBefore := strings.Split(string(before), "\n")
	linesAfter := strings.Split(string(after), "\n")

	for _, line := range linesAfter {
		found := false
		for _, bLine := range linesBefore {
			if line == bLine {
				found = true
				break
			}
		}
		if !found && strings.Contains(line, "package:") {
			packageID = strings.TrimSpace(strings.Replace(line, "package:", "", 1))
			break
		}
	}

	if packageID == "" {
		return "APK terinstall tapi ID tidak terdeteksi."
	}

	// 4. Launch App (Monkey)
	exec.Command("adb", "shell", "monkey", "-p", packageID, "1").Run()

	return "Success: " + packageID
}