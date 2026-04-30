package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type App struct{}

func NewApp() *App {
	return &App{}
}

func getADBPath() string {
	ex, err := os.Executable()
	if err == nil {
		localPath := filepath.Join(filepath.Dir(ex), "./abd/adb.exe")
		if _, err := os.Stat(localPath); err == nil {
			return localPath
		}
		localPathRoot := filepath.Join(filepath.Dir(ex), "./adb/adb.exe")
		if _, err := os.Stat(localPathRoot); err == nil {
			return localPathRoot
		}
	}
	return "adb"
}

func runCommand(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return strings.TrimSpace(stderr.String()), err
	}
	return strings.TrimSpace(out.String()), nil
}

func (a *App) GetDetailedDevices() ([]string, error) {
	adb := getADBPath()
	// Gunakan output mentah dulu untuk memastikan ADB terpanggil
	out, err := runCommand(adb, "devices", "-l")
	if err != nil {
		return nil, fmt.Errorf("ADB Path: %s | Error: %v", adb, err)
	}

	var devices []string
	lines := strings.Split(out, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Lewati baris pertama "List of devices attached" dan baris kosong
		if line == "" || strings.HasPrefix(line, "List of") {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) >= 1 {
			serial := parts[0]
			model := "Unknown Device"

			// Cari tag model:XXXX di output -l
			for _, p := range parts {
				if strings.HasPrefix(p, "model:") {
					model = strings.TrimPrefix(p, "model:")
					model = strings.ReplaceAll(model, "_", " ")
					break
				}
			}
			devices = append(devices, fmt.Sprintf("%s (%s)", model, serial))
		}
	}
	return devices, nil
}

func (a *App) Uninstall(pkg, serial string) (string, error) {
	adb := getADBPath()
	args := []string{"-s", serial, "uninstall", pkg}
	return runCommand(adb, args...)
}

func (a *App) RunADBPure(apkPath, serial string) string {
	adb := getADBPath()

	cmdArgs := func(baseArgs ...string) []string {
		if serial != "" {
			return append([]string{"-s", serial}, baseArgs...)
		}
		return baseArgs
	}

	beforeOut, _ := runCommand(adb, cmdArgs("shell", "pm", "list", "packages", "-3")...)
	beforePackages := parsePackages(beforeOut)

	installArgs := cmdArgs("install", "-r", "-d", apkPath)
	installCmd := exec.Command(adb, installArgs...)
	installOut, err := installCmd.CombinedOutput()
	resultStr := string(installOut)

	if err != nil || !strings.Contains(resultStr, "Success") {
		return fmt.Sprintf("Install Failed: %s", strings.TrimSpace(resultStr))
	}

	afterOut, _ := runCommand(adb, cmdArgs("shell", "pm", "list", "packages", "-3")...)
	afterPackages := parsePackages(afterOut)

	newPackageID := ""
	for pkg := range afterPackages {
		if !beforePackages[pkg] {
			newPackageID = pkg
			break
		}
	}

	if newPackageID != "" {
		runCommand(adb, cmdArgs("shell", "monkey", "-p", newPackageID, "-c", "android.intent.category.LAUNCHER", "1")...)
		return fmt.Sprintf("Success! Installed & Launched: %s", newPackageID)
	}

	return "Install Success, but Package ID detection failed."
}

func parsePackages(output string) map[string]bool {
	pkgs := make(map[string]bool)
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "package:") {
			id := strings.TrimPrefix(line, "package:")
			pkgs[id] = true
		}
	}
	return pkgs
}
