package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type App struct{}

func NewApp() *App {
	return &App{}
}

// Scrcpy release download mapping for version 3.3.4
var downloadURLs = map[string]string{
	"windows-amd64": "https://github.com/Genymobile/scrcpy/releases/download/v3.3.4/scrcpy-win64-v3.3.4.zip",
	"windows-386":   "https://github.com/Genymobile/scrcpy/releases/download/v3.3.4/scrcpy-win32-v3.3.4.zip",
	"windows-arm64": "https://github.com/Genymobile/scrcpy/releases/download/v3.3.4/scrcpy-win64-v3.3.4.zip",
	"linux-amd64":   "https://github.com/Genymobile/scrcpy/releases/download/v3.3.4/scrcpy-linux-x86_64-v3.3.4.tar.gz",
	"darwin-amd64":  "https://github.com/Genymobile/scrcpy/releases/download/v3.3.4/scrcpy-macos-x86_64-v3.3.4.tar.gz",
	"darwin-arm64":  "https://github.com/Genymobile/scrcpy/releases/download/v3.3.4/scrcpy-macos-aarch64-v3.3.4.tar.gz",
}

const scrcpyFolder = "scrcpy_core"

// --- HELPER FUNCTIONS ---

var adbURLs = map[string]string{
	"windows": "https://dl.google.com/android/repository/platform-tools-latest-windows.zip",
	"darwin":  "https://dl.google.com/android/repository/platform-tools-latest-darwin.zip",
	"linux":   "https://dl.google.com/android/repository/platform-tools-latest-linux.zip",
}

const adbFolder = "adb_engine"

func getADBPath() string {
	adbName := "adb"

	if runtime.GOOS == "windows" {
		adbName = "adb.exe"
	}

	base, err := os.Getwd()
	if err != nil {
		base = "."
	}

	localADB := filepath.Join(
		base,
		adbFolder,
		"platform-tools",
		adbName,
	)

	if _, err := os.Stat(localADB); err == nil {
		return localADB
	}

	// check system adb
	if path, err := exec.LookPath(adbName); err == nil {
		return path
	}

	// auto download
	url, ok := adbURLs[runtime.GOOS]
	if !ok {
		return adbName
	}

	fmt.Println("Downloading ADB engine...")

	resp, err := http.Get(url)
	if err != nil {
		return adbName
	}
	defer resp.Body.Close()

	tmp := filepath.Join(base, "adb.zip")

	f, err := os.Create(tmp)
	if err != nil {
		return adbName
	}

	io.Copy(f, resp.Body)
	f.Close()

	err = unzip(tmp, adbFolder)
	os.Remove(tmp)

	if err != nil {
		return adbName
	}

	if _, err := os.Stat(localADB); err == nil {
		return localADB
	}

	return adbName
}

// Unified Command Runner
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

// --- ADB FEATURES ---

func (a *App) GetDetailedDevices() ([]string, error) {
	adb := getADBPath()
	out, err := runCommand(adb, "devices", "-l")
	if err != nil {
		return nil, fmt.Errorf("ADB Path: %s | Error: %v", adb, err)
	}

	var devices []string
	lines := strings.Split(out, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "List of") {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) >= 1 {
			serial := parts[0]
			model := "Unknown Device"

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

	// Fetch installed packages beforehand
	beforeOut, _ := runCommand(adb, cmdArgs("shell", "pm", "list", "packages", "-3")...)
	beforePackages := parsePackages(beforeOut)

	installArgs := cmdArgs("install", "-r", "-d", apkPath)
	installCmd := exec.Command(adb, installArgs...)
	installOut, err := installCmd.CombinedOutput()
	resultStr := string(installOut)

	if err != nil || !strings.Contains(resultStr, "Success") {
		return fmt.Sprintf("Install Failed: %s", strings.TrimSpace(resultStr))
	}

	// Fetch installed packages after
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
		_, _ = runCommand(adb, cmdArgs("shell", "monkey", "-p", newPackageID, "-c", "android.intent.category.LAUNCHER", "1")...)
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

// --- SCRCPY FEATURES ---

func (a *App) StartScrcpy(serial string, logFunc func(string)) error {
	exe, err := os.Executable()

	baseDir := "."

	if err == nil {
		baseDir = filepath.Dir(exe)
	}
	if err != nil {
		baseDir = "."
	}
	targetDir := filepath.Join(baseDir, scrcpyFolder)

	// Ensure scrcpy Folder Root Exists
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("failed to create target folder: %v", err)
	}

	// 1. Find or Download
	scrcpyPath := findScrcpyFolder(targetDir)
	if scrcpyPath == "" {
		platformKey := fmt.Sprintf("%s-%s", runtime.GOOS, runtime.GOARCH)
		url, ok := downloadURLs[platformKey]
		if !ok {
			return fmt.Errorf("platform %s not supported for auto-download", platformKey)
		}

		logFunc(fmt.Sprintf("Downloading Scrcpy for %s...", platformKey))
		err := downloadAndSetup(url, runtime.GOOS, targetDir, logFunc)
		if err != nil {
			return fmt.Errorf("download failed: %v", err)
		}
		scrcpyPath = findScrcpyFolder(targetDir)
	}

	if scrcpyPath == "" {
		return fmt.Errorf("scrcpy binary folder not found after setup")
	}

	// 2. Execute
	logFunc(fmt.Sprintf("Launching Mirror for %s...", serial))

	execName := "scrcpy"
	if runtime.GOOS == "windows" {
		execName = "scrcpy.exe"
	}

	fullPath := filepath.Join(scrcpyPath, execName)

	// Set executable permissions for Unix-like systems
	if runtime.GOOS != "windows" {
		_ = os.Chmod(fullPath, 0755)
		_ = os.Chmod(filepath.Join(scrcpyPath, "adb"), 0755)
	}

	// Run in background
	cmd := exec.Command(fullPath, "-s", serial, "--always-on-top", "--window-title", "ADBPureFlow-Mirror")
	cmd.Dir = scrcpyPath // Set working directory

	return cmd.Start()
}

func findScrcpyFolder(root string) string {
	files, err := os.ReadDir(root)
	if err != nil {
		return ""
	}

	execName := "scrcpy"
	if runtime.GOOS == "windows" {
		execName = "scrcpy.exe"
	}

	for _, f := range files {
		if f.IsDir() && (strings.Contains(f.Name(), "scrcpy-") || f.Name() == "bin") {
			// Check if executable exists inside
			subPath := filepath.Join(root, f.Name())
			if _, err := os.Stat(filepath.Join(subPath, execName)); err == nil {
				return subPath
			}
		}
	}

	// Fallback check if scrcpy is directly in the folder
	if _, err := os.Stat(filepath.Join(root, execName)); err == nil {
		return root
	}

	return ""
}

func downloadAndSetup(url string, osType string, destRoot string, logFunc func(string)) error {
	ext := ".tar.gz"
	if osType == "windows" {
		ext = ".zip"
	}
	tempFile := filepath.Join(destRoot, "download_temp"+ext)

	logFunc("Connecting to GitHub...")
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("received bad status code: %s", resp.Status)
	}

	f, err := os.Create(tempFile)
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %v", err)
	}

	_, err = io.Copy(f, resp.Body)
	f.Close()
	if err != nil {
		os.Remove(tempFile)
		return fmt.Errorf("failed to save download: %v", err)
	}

	logFunc("Extracting engine components...")
	if osType == "windows" {
		err = unzip(tempFile, destRoot)
	} else {
		err = untar(tempFile, destRoot)
	}
	os.Remove(tempFile)
	return err
}

func unzip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	destAbs, err := filepath.Abs(dest)
	if err != nil {
		return err
	}

	for _, f := range r.File {
		fpath := filepath.Join(dest, f.Name)

		// Prevent Zip Slip vulnerability
		fpathAbs, err := filepath.Abs(fpath)
		if err != nil {
			return err
		}
		if !strings.HasPrefix(fpathAbs, destAbs+string(filepath.Separator)) && fpathAbs != destAbs {
			return fmt.Errorf("illegal file path in zip: %s", f.Name)
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(fpath, 0755); err != nil {
				return err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(fpath), 0755); err != nil {
			return err
		}

		out, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			out.Close()
			return err
		}

		_, err = io.Copy(out, rc)
		out.Close()
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func untar(src, dest string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gzr.Close()
	tr := tar.NewReader(gzr)

	destAbs, err := filepath.Abs(dest)
	if err != nil {
		return err
	}

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		target := filepath.Join(dest, header.Name)

		// Prevent Tar Slip vulnerability
		targetAbs, err := filepath.Abs(target)
		if err != nil {
			return err
		}
		if !strings.HasPrefix(targetAbs, destAbs+string(filepath.Separator)) && targetAbs != destAbs {
			return fmt.Errorf("illegal file path in tar: %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			outFile, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			_, err = io.Copy(outFile, tr)
			outFile.Close()
			if err != nil {
				return err
			}
		}
	}
	return nil
}
