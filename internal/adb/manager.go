package adb

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Manager is the high-level façade that the GUI and CLI both consume. It
// owns a Client, handles device selection, caches package lists, and
// exposes a simple API for install / launch / stop / uninstall.
type Manager struct {
	Client *Client
	Scrcpy *ScrcpyManager
}

// NewManager returns a Manager that stores bundled binaries (platform-tools
// and scrcpy) under dataDir. If download is true, missing adb binaries are
// auto-fetched on construction.
func NewManager(dataDir string, download bool) (*Manager, error) {
	if dataDir == "" {
		if cwd, err := os.Getwd(); err == nil {
			dataDir = cwd
		} else {
			dataDir = "."
		}
	}
	adbDir := filepath.Join(dataDir, "adb_engine")
	scrcpyDir := filepath.Join(dataDir, "scrcpy_core")

	client, err := New(adbDir, download)
	if err != nil {
		return nil, err
	}
	// Best-effort: start the adb server once so `devices` isn't slow on
	// first call. We don't hard-fail if this errors (e.g. adb is present
	// but broken; callers will see the error on the next command).
	_ = client.StartServer(context.Background())

	return &Manager{
		Client: client,
		Scrcpy: NewScrcpyManager(client, scrcpyDir),
	}, nil
}

// RefreshDevices lists connected devices and returns them sorted (online
// first, then by display name).
func (m *Manager) RefreshDevices(ctx context.Context) ([]Device, error) {
	devs, err := m.Client.ListDevices(ctx)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(devs, func(i, j int) bool {
		if devs[i].State == StateDevice && devs[j].State != StateDevice {
			return true
		}
		if devs[j].State == StateDevice && devs[i].State != StateDevice {
			return false
		}
		return strings.ToLower(devs[i].DisplayName()) < strings.ToLower(devs[j].DisplayName())
	})
	return devs, nil
}

// FindDevice returns the Device whose serial matches, or an error. It is a
// tiny helper for validating user selection before running an operation.
func (m *Manager) FindDevice(ctx context.Context, serial string) (Device, error) {
	devs, err := m.RefreshDevices(ctx)
	if err != nil {
		return Device{}, err
	}
	for _, d := range devs {
		if d.Serial == serial {
			if d.State != StateDevice {
				return d, fmt.Errorf("device %s is in state %q (not ready)", serial, d.State)
			}
			return d, nil
		}
	}
	return Device{}, fmt.Errorf("device %q not connected", serial)
}

// ListPackages returns installed packages for the given serial. It delegates
// to Client.ListPackages with a userOnly flag (true = third-party apps
// only; false = all packages including system).
func (m *Manager) ListPackages(ctx context.Context, serial string, userOnly bool) ([]Package, error) {
	if _, err := m.FindDevice(ctx, serial); err != nil {
		return nil, err
	}
	return m.Client.ListPackages(ctx, serial, userOnly)
}

// InstallAPK installs a local APK to the device. The localPath is resolved
// to an absolute path and stat'd before invoking adb.
func (m *Manager) InstallAPK(ctx context.Context, serial, localPath string) (string, error) {
	if _, err := m.FindDevice(ctx, serial); err != nil {
		return "", err
	}
	if err := m.Client.Install(ctx, serial, localPath); err != nil {
		return "", err
	}
	abs, _ := filepath.Abs(localPath)
	return fmt.Sprintf("Installed %s to %s", filepath.Base(abs), serial), nil
}

// LaunchApp launches the package. The device must be online.
func (m *Manager) LaunchApp(ctx context.Context, serial, pkg string) error {
	if _, err := m.FindDevice(ctx, serial); err != nil {
		return err
	}
	return m.Client.Launch(ctx, serial, pkg)
}

// ForceStopApp force-stops the package.
func (m *Manager) ForceStopApp(ctx context.Context, serial, pkg string) error {
	if _, err := m.FindDevice(ctx, serial); err != nil {
		return err
	}
	return m.Client.ForceStop(ctx, serial, pkg)
}

// UninstallApp removes a package. If keepData is true, the -k flag is passed.
func (m *Manager) UninstallApp(ctx context.Context, serial, pkg string, keepData bool) error {
	if _, err := m.FindDevice(ctx, serial); err != nil {
		return err
	}
	return m.Client.Uninstall(ctx, serial, pkg, keepData)
}

// ErrNotFound is returned when a package lookup fails.
var ErrNotFound = errors.New("package not found on device")

// FindPackage returns metadata for a single package, or ErrNotFound.
func (m *Manager) FindPackage(ctx context.Context, serial, pkg string, userOnly bool) (*Package, error) {
	pkgs, err := m.ListPackages(ctx, serial, userOnly)
	if err != nil {
		return nil, err
	}
	for i := range pkgs {
		if strings.EqualFold(pkgs[i].Name, pkg) {
			return &pkgs[i], nil
		}
	}
	return nil, fmt.Errorf("%w: %s", ErrNotFound, pkg)
}

// PackageInfo refreshes package metadata and returns a single package's
// record. It is a thin wrapper around ListPackages + linear lookup kept
// as a convenience for GUI/CLI detail views.
func (m *Manager) PackageInfo(ctx context.Context, serial, pkg string) (*Package, error) {
	if _, err := m.FindDevice(ctx, serial); err != nil {
		return nil, err
	}
	pkgs, err := m.Client.ListPackages(ctx, serial, false)
	if err != nil {
		return nil, err
	}
	lower := strings.ToLower(strings.TrimSpace(pkg))
	for i := range pkgs {
		if strings.ToLower(pkgs[i].Name) == lower {
			return &pkgs[i], nil
		}
	}
	return nil, fmt.Errorf("%w: %s", ErrNotFound, pkg)
}
