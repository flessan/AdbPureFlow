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

// Mapping download Scrcpy (v3.3.4)
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

func getADBPath() string {
	ex, err := os.Executable()
	if err == nil {
		// Cek di folder scrcpy dulu (jika sudah ada)
		scrcpyPath := findScrcpyFolder(filepath.Join(filepath.Dir(ex), scrcpyFolder))
		if scrcpyPath != "" {
			adbPath := filepath.Join(scrcpyPath, "adb.exe") // Windows centric, could add logic for others
			if _, err := os.Stat(adbPath); err == nil {
				return adbPath
			}
		}

		// Cek di folder lokal standar
		localPath := filepath.Join(filepath.Dir(ex), "./adb/adb.exe")
		if _, err := os.Stat(localPath); err == nil {
			return localPath
		}
		localPathAbd := filepath.Join(filepath.Dir(ex), "./abd/adb.exe") // Legacy check typo
		if _, err := os.Stat(localPathAbd); err == nil {
			return localPathAbd
		}
	}
	return "adb" // Fallback to system PATH
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

// --- SCRCPY FEATURES ---

func (a *App) StartScrcpy(serial string, logFunc func(string)) error {
	baseDir, _ := os.Getwd()
	targetDir := filepath.Join(baseDir, scrcpyFolder)

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

	// Set executable permission for Linux/Mac
	if runtime.GOOS != "windows" {
		os.Chmod(fullPath, 0755)
		// Also chmod adb inside if needed
		os.Chmod(filepath.Join(scrcpyPath, "adb"), 0755)
	}

	// Run in background
	cmd := exec.Command(fullPath, "-s", serial, "--always-on-top", "--window-title", "ADBPureFlow-Mirror")
	cmd.Dir = scrcpyPath // Set working directory

	// We don't wait for scrcpy to finish, just start it
	return cmd.Start()
}

func findScrcpyFolder(root string) string {
	files, err := os.ReadDir(root)
	if err != nil {
		return ""
	}
	for _, f := range files {
		if f.IsDir() && (strings.Contains(f.Name(), "scrcpy-") || f.Name() == "bin") {
			// Check if executable exists inside
			subPath := filepath.Join(root, f.Name())
			execName := "scrcpy"
			if runtime.GOOS == "windows" {
				execName = "scrcpy.exe"
			}
			if _, err := os.Stat(filepath.Join(subPath, execName)); err == nil {
				return subPath
			}
		}
	}
	return ""
}

func downloadAndSetup(url string, osType string, destRoot string, logFunc func(string)) error {
	ext := ".tar.gz"
	if osType == "windows" {
		ext = ".zip"
	}
	tempFile := "download_temp" + ext

	logFunc("Connecting to GitHub...")
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	f, _ := os.Create(tempFile)
	io.Copy(f, resp.Body)
	f.Close()

	logFunc("Extracting engine...")
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

	for _, f := range r.File {
		fpath := filepath.Join(dest, f.Name)
		if f.FileInfo().IsDir() {
			os.MkdirAll(fpath, os.ModePerm)
			continue
		}
		os.MkdirAll(filepath.Dir(fpath), os.ModePerm)
		out, _ := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		rc, _ := f.Open()
		io.Copy(out, rc)
		out.Close()
		rc.Close()
	}
	return nil
}

func untar(src, dest string) error {
	f, _ := os.Open(src)
	defer f.Close()
	gzr, _ := gzip.NewReader(f)
	defer gzr.Close()
	tr := tar.NewReader(gzr)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		target := filepath.Join(dest, header.Name)
		switch header.Typeflag {
		case tar.TypeDir:
			os.MkdirAll(target, 0755)
		case tar.TypeReg:
			f, _ := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			io.Copy(f, tr)
			f.Close()
		}
	}
	return nil
}
