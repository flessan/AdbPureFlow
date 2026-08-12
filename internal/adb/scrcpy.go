package adb

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// ScrcpyURL maps GOOS-GOARCH pairs to official scrcpy release binaries
// (v3.3.4, the same release the legacy GUI shipped with).
var ScrcpyURL = map[string]string{
	"windows-amd64": "https://github.com/Genymobile/scrcpy/releases/download/v3.3.4/scrcpy-win64-v3.3.4.zip",
	"windows-386":   "https://github.com/Genymobile/scrcpy/releases/download/v3.3.4/scrcpy-win32-v3.3.4.zip",
	"windows-arm64": "https://github.com/Genymobile/scrcpy/releases/download/v3.3.4/scrcpy-win64-v3.3.4.zip",
	"linux-amd64":   "https://github.com/Genymobile/scrcpy/releases/download/v3.3.4/scrcpy-linux-x86_64-v3.3.4.tar.gz",
	"darwin-amd64":  "https://github.com/Genymobile/scrcpy/releases/download/v3.3.4/scrcpy-macos-x86_64-v3.3.4.tar.gz",
	"darwin-arm64":  "https://github.com/Genymobile/scrcpy/releases/download/v3.3.4/scrcpy-macos-aarch64-v3.3.4.tar.gz",
}

// ScrcpyManager locates (and auto-downloads on first use) the scrcpy binary
// and launches mirror sessions for a specific device.
type ScrcpyManager struct {
	client   *Client
	baseDir  string
	httpc    *http.Client
}

// NewScrcpyManager creates a ScrcpyManager that stores binaries under dir
// (defaults to <cwd>/scrcpy_core if empty).
func NewScrcpyManager(c *Client, dir string) *ScrcpyManager {
	if dir == "" {
		if cwd, err := os.Getwd(); err == nil {
			dir = filepath.Join(cwd, "scrcpy_core")
		} else {
			dir = "scrcpy_core"
		}
	}
	return &ScrcpyManager{client: c, baseDir: dir, httpc: http.DefaultClient}
}

// BinaryPath returns the absolute path to the scrcpy executable, downloading
// it if necessary.
func (m *ScrcpyManager) BinaryPath(ctx context.Context) (string, error) {
	if err := os.MkdirAll(m.baseDir, 0o755); err != nil {
		return "", err
	}
	if p := m.findExisting(); p != "" {
		return p, nil
	}
	key := fmt.Sprintf("%s-%s", runtime.GOOS, runtime.GOARCH)
	url, ok := ScrcpyURL[key]
	if !ok {
		return "", fmt.Errorf("scrcpy: no prebuilt binary for %s", key)
	}
	if err := m.download(ctx, url); err != nil {
		return "", err
	}
	if p := m.findExisting(); p != "" {
		return p, nil
	}
	return "", fmt.Errorf("scrcpy: binary not found after extraction")
}

// StartMirror launches scrcpy for the given serial and returns without
// waiting for scrcpy to exit. The mirror runs in its own process group; the
// caller can cancel ctx to send SIGKILL (platforms vary).
func (m *ScrcpyManager) StartMirror(ctx context.Context, serial string, title string) (*exec.Cmd, error) {
	if serial == "" {
		return nil, fmt.Errorf("scrcpy: mirror requires a serial")
	}
	bin, err := m.BinaryPath(ctx)
	if err != nil {
		return nil, err
	}
	if title == "" {
		title = "ADBPureFlow-Mirror"
	}
	args := []string{"-s", serial, "--always-on-top", "--window-title", title}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = filepath.Dir(bin)
	// On POSIX, chmod the binary just in case the archive extraction didn't
	// preserve the executable bit.
	if runtime.GOOS != "windows" {
		_ = os.Chmod(bin, 0o755)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("scrcpy: %w", err)
	}
	return cmd, nil
}

func (m *ScrcpyManager) exeName() string {
	if runtime.GOOS == "windows" {
		return "scrcpy.exe"
	}
	return "scrcpy"
}

func (m *ScrcpyManager) findExisting() string {
	exe := m.exeName()
	entries, err := os.ReadDir(m.baseDir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, "scrcpy-") || strings.HasPrefix(name, "scrcpy") || name == "bin" {
			candidate := filepath.Join(m.baseDir, name, exe)
			if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
				return candidate
			}
		}
	}
	candidate := filepath.Join(m.baseDir, exe)
	if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
		return candidate
	}
	return ""
}

func (m *ScrcpyManager) download(ctx context.Context, url string) error {
	ext := ".tar.gz"
	if runtime.GOOS == "windows" {
		ext = ".zip"
	}
	tmp, err := os.CreateTemp(m.baseDir, "scrcpy-download-*"+ext)
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := m.httpc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("scrcpy: download returned %s", resp.Status)
	}
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		return err
	}
	tmp.Close()

	if ext == ".zip" {
		return SafeUnzip(tmp.Name(), m.baseDir)
	}
	return SafeUntar(tmp.Name(), m.baseDir)
}
