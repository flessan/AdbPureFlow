// Package adb provides a thin, ergonomic wrapper around the Android Debug
// Bridge (`adb`) binary. It is the only place in ADBPureFlow that shells out
// to adb; every higher-level feature (device discovery, package management,
// install, launch, …) is built on top of Client so that GUI and CLI share one
// implementation.
//
// The package intentionally avoids global state: a Client carries the resolved
// adb path and a few configuration knobs, and every device-specific command
// accepts an explicit serial via `-s <serial>` so commands can never
// accidentally run against the wrong device.
package adb

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Client is a handle to a resolved adb binary. Construct with New or
// MustNew. A Client is safe for concurrent use from multiple goroutines
// as long as callers don't mutate its exported fields after construction.
type Client struct {
	// Path is the absolute (or PATH-resolved) location of the adb binary.
	Path string

	// Timeout is the default per-command timeout. Zero means no timeout (the
	// caller is expected to supply a context).
	Timeout time.Duration

	// PlatformToolsDir is the directory where platform-tools live; it is used
	// as the download destination by EnsureServer.
	PlatformToolsDir string

	// HTTPClient is used for auto-download; defaults to http.DefaultClient.
	HTTPClient *http.Client
}

// CommandError is returned for adb invocations that exit non-zero. It
// preserves stderr/stdout snippets and the exit code so callers can surface
// precise errors in the GUI/CLI.
type CommandError struct {
	Args   []string
	Exit   int
	Stdout string
	Stderr string
	Err    error // underlying error, e.g. executable not found
}

func (e *CommandError) Error() string {
	msg := strings.TrimSpace(e.Stderr)
	if msg == "" {
		msg = strings.TrimSpace(e.Stdout)
	}
	if msg == "" && e.Err != nil {
		msg = e.Err.Error()
	}
	return fmt.Sprintf("adb %s: %s", strings.Join(e.Args, " "), msg)
}

func (e *CommandError) Unwrap() error { return e.Err }

// ErrNoADB is returned by New if the adb binary cannot be located and
// auto-download is not possible / disabled.
var ErrNoADB = errors.New("adb: executable not found on PATH or bundled platform-tools")

// platformADBURL maps runtime.GOOS to the official Google platform-tools
// download URL. (These URLs redirect to the latest available build.)
var platformADBURL = map[string]string{
	"windows": "https://dl.google.com/android/repository/platform-tools-latest-windows.zip",
	"darwin":  "https://dl.google.com/android/repository/platform-tools-latest-darwin.zip",
	"linux":   "https://dl.google.com/android/repository/platform-tools-latest-linux.zip",
}

// exeSuffix is ".exe" on Windows, "" elsewhere.
func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

// New returns a Client pointing at a usable adb binary. Resolution order:
//
//  1. The bundled `<dir>/platform-tools/adb[.exe]` (relative to the running
//     executable or cwd),
//  2. An `adb` binary on the system PATH,
//  3. If `downloadIfMissing` is true, downloads the official platform-tools
//     package into `dir` and uses that.
//
// dir is the directory under which platform-tools should be placed; it
// defaults to `<cwd>/adb_engine` if empty.
func New(dir string, downloadIfMissing bool) (*Client, error) {
	if dir == "" {
		if cwd, err := os.Getwd(); err == nil {
			dir = filepath.Join(cwd, "adb_engine")
		} else {
			dir = "adb_engine"
		}
	}

	adbName := "adb" + exeSuffix()

	// 1. bundled copy
	local := filepath.Join(dir, "platform-tools", adbName)
	if st, err := os.Stat(local); err == nil && !st.IsDir() {
		return &Client{Path: local, PlatformToolsDir: dir, Timeout: 30 * time.Second, HTTPClient: http.DefaultClient}, nil
	}

	// 2. system PATH
	if p, err := exec.LookPath("adb"); err == nil {
		return &Client{Path: p, PlatformToolsDir: dir, Timeout: 30 * time.Second, HTTPClient: http.DefaultClient}, nil
	}

	// 3. download
	if !downloadIfMissing {
		return nil, ErrNoADB
	}
	url, ok := platformADBURL[runtime.GOOS]
	if !ok {
		return nil, fmt.Errorf("%w: unsupported OS %s", ErrNoADB, runtime.GOOS)
	}
	if err := downloadAndExtract(url, dir, adbName); err != nil {
		return nil, fmt.Errorf("adb: download failed: %w", err)
	}
	if runtime.GOOS != "windows" {
		_ = os.Chmod(local, 0o755)
	}
	if _, err := os.Stat(local); err != nil {
		return nil, fmt.Errorf("adb: download completed but binary missing at %s: %w", local, err)
	}
	return &Client{Path: local, PlatformToolsDir: dir, Timeout: 30 * time.Second, HTTPClient: http.DefaultClient}, nil
}

// Command runs an adb command with the given arguments and returns its
// trimmed stdout. If the serial argument is non-empty it is prepended as
// `-s <serial>` so the command targets a specific device.
//
// Stderr is captured and returned via *CommandError on non-zero exit.
func (c *Client) Command(ctx context.Context, serial string, args ...string) (string, error) {
	full := c.deviceArgs(serial, args)
	return c.run(ctx, full)
}

// CommandWithInput runs an adb command and pipes stdin to it. This is useful
// for commands like `exec-out` or interactive shell commands (though we
// generally prefer non-interactive shell invocations).
func (c *Client) CommandWithInput(ctx context.Context, serial string, stdin io.Reader, args ...string) (string, error) {
	full := c.deviceArgs(serial, args)
	cmd := exec.CommandContext(ctx, c.Path, full...)
	cmd.Stdin = stdin
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		return "", c.mkErr(full, stdout.String(), stderr.String(), err)
	}
	return strings.TrimSpace(stdout.String()), nil
}

// ServerCmd runs an `adb` host-side command that does NOT take a serial
// (e.g. `start-server`, `devices`, `kill-server`).
func (c *Client) ServerCmd(ctx context.Context, args ...string) (string, error) {
	return c.run(ctx, args)
}

func (c *Client) deviceArgs(serial string, args []string) []string {
	if serial == "" {
		return args
	}
	out := make([]string, 0, 2+len(args))
	out = append(out, "-s", serial)
	out = append(out, args...)
	return out
}

func (c *Client) run(ctx context.Context, args []string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if c.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.Timeout)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, c.Path, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	outStr := strings.TrimSpace(stdout.String())
	if err != nil {
		return outStr, c.mkErr(args, stdout.String(), stderr.String(), err)
	}
	return outStr, nil
}

func (c *Client) mkErr(args []string, stdout, stderr string, err error) error {
	ce := &CommandError{
		Args:   append([]string{}, args...),
		Stdout: stdout,
		Stderr: stderr,
		Err:    err,
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		ce.Exit = ee.ExitCode()
	}
	return ce
}

// StartServer runs `adb start-server`. It is a no-op if the server is
// already running.
func (c *Client) StartServer(ctx context.Context) error {
	_, err := c.ServerCmd(ctx, "start-server")
	return err
}

// Version returns the first line of `adb version` output.
func (c *Client) Version(ctx context.Context) (string, error) {
	out, err := c.ServerCmd(ctx, "version")
	if err != nil {
		return "", err
	}
	if i := strings.IndexByte(out, '\n'); i > 0 {
		return out[:i], nil
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Auto-download of platform-tools (shared with the legacy CLI/GUI
// implementation, but moved into the shared core so both frontends benefit).
// ---------------------------------------------------------------------------

func downloadAndExtract(url, destDir, adbName string) error {
	httpc := http.DefaultClient
	resp, err := httpc.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned %s", resp.Status)
	}
	tmp, err := os.CreateTemp("", "adb-platform-tools-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		return err
	}
	tmp.Close()

	destAbs, err := filepath.Abs(destDir)
	if err != nil {
		return err
	}
	lowerURL := strings.ToLower(url)
	switch {
	case strings.HasSuffix(lowerURL, ".zip"):
		return safeUnzip(tmp.Name(), destAbs)
	case strings.HasSuffix(lowerURL, ".tar.gz"), strings.HasSuffix(lowerURL, ".tgz"):
		return safeUntar(tmp.Name(), destAbs)
	default:
		return fmt.Errorf("unknown archive format: %s", url)
	}
}

// SafeUnzip extracts a zip archive into dest with Zip Slip protection.
// Exported for use by the scrcpy downloader.
func SafeUnzip(src, dest string) error { return safeUnzip(src, dest) }

func safeUnzip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()
	destAbs, err := filepath.Abs(dest)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(destAbs, 0o755); err != nil {
		return err
	}
	for _, f := range r.File {
		fp := filepath.Join(destAbs, f.Name)
		fpAbs, err := filepath.Abs(fp)
		if err != nil {
			return err
		}
		if !strings.HasPrefix(fpAbs, destAbs+string(os.PathSeparator)) && fpAbs != destAbs {
			return fmt.Errorf("zip slip: illegal path %q", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(fpAbs, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(fpAbs), 0o755); err != nil {
			return err
		}
		out, err := os.OpenFile(fpAbs, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode().Perm()|0o600)
		if err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			out.Close()
			return err
		}
		_, err = io.Copy(out, rc)
		rc.Close()
		out.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// SafeUntar extracts a .tar.gz archive into dest with Tar Slip protection.
func SafeUntar(src, dest string) error { return safeUntar(src, dest) }

func safeUntar(src, dest string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	destAbs, err := filepath.Abs(dest)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(destAbs, 0o755); err != nil {
		return err
	}
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		fp := filepath.Join(destAbs, hdr.Name)
		fpAbs, err := filepath.Abs(fp)
		if err != nil {
			return err
		}
		if !strings.HasPrefix(fpAbs, destAbs+string(os.PathSeparator)) && fpAbs != destAbs {
			return fmt.Errorf("tar slip: illegal path %q", hdr.Name)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(fpAbs, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(fpAbs), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(fpAbs, os.O_CREATE|os.O_RDWR|os.O_TRUNC, os.FileMode(hdr.Mode)&0o755)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			out.Close()
		}
	}
}
