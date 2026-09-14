package main

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var version = "dev"

const (
	appName      = "AdbPureFlow"
	adbFolder    = "adb_engine"
	scrcpyFolder = "scrcpy_core"
)

// App is the native application core used by the Fyne desktop shell.  UI code calls
// typed methods here instead of building shell command strings.
type App struct {
	adb        *ADB
	scrcpy     *ScrcpyManager
	stateStore *AppStateStore
	stateMu    sync.Mutex
	state      AppState
}

func NewApp() *App {
	runner := OSRunner{}
	adb := NewADB(runner, RuntimePaths{})
	store := NewAppStateStore()
	return &App{adb: adb, scrcpy: NewScrcpyManager(runner, RuntimePaths{}), stateStore: store, state: store.Load()}
}

func (a *App) State() AppState {
	a.stateMu.Lock()
	defer a.stateMu.Unlock()
	return a.state
}

func (a *App) SaveState(update func(*AppState)) error {
	a.stateMu.Lock()
	defer a.stateMu.Unlock()
	if update != nil {
		update(&a.state)
	}
	return a.stateStore.Save(a.state)
}

// RuntimePaths keeps external tools discoverable/configurable without hardcoding
// one installation layout forever. Empty fields mean auto-discover.
type RuntimePaths struct {
	ADBPath    string
	ScrcpyPath string
	BaseDir    string
}

type AppState struct {
	SchemaVersion      int               `json:"schemaVersion"`
	Theme              string            `json:"theme"`
	OnboardingComplete bool              `json:"onboardingComplete"`
	SelectedSerial     string            `json:"selectedSerial,omitempty"`
	DeviceAliases      map[string]string `json:"deviceAliases,omitempty"`
	WirelessEndpoints  []string          `json:"wirelessEndpoints,omitempty"`
	RecentAPKs         []string          `json:"recentApks,omitempty"`
}

type AppStateStore struct {
	path string
	mu   sync.Mutex
}

func NewAppStateStore() *AppStateStore {
	base, err := os.UserConfigDir()
	if err != nil || base == "" {
		base = "."
	}
	return &AppStateStore{path: filepath.Join(base, appName, "state.json")}
}

func (s *AppStateStore) Load() AppState {
	s.mu.Lock()
	defer s.mu.Unlock()
	state := AppState{SchemaVersion: 1, Theme: "system", DeviceAliases: map[string]string{}}
	data, err := os.ReadFile(s.path)
	if err != nil {
		return state
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return AppState{SchemaVersion: 1, Theme: "system", DeviceAliases: map[string]string{}}
	}
	if state.SchemaVersion == 0 {
		state.SchemaVersion = 1
	}
	if state.DeviceAliases == nil {
		state.DeviceAliases = map[string]string{}
	}
	return state
}

func (s *AppStateStore) Save(state AppState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if state.SchemaVersion == 0 {
		state.SchemaVersion = 1
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0600)
}

func addRecentPath(paths []string, path string, limit int) []string {
	path = strings.TrimSpace(path)
	if path == "" {
		return paths
	}
	out := []string{path}
	for _, p := range paths {
		if p != path {
			out = append(out, p)
		}
		if len(out) >= limit {
			break
		}
	}
	return out
}

func OpenInFileManager(path string) error {
	path = filepath.Clean(path)
	switch runtime.GOOS {
	case "windows":
		return exec.Command("explorer.exe", "/select,"+path).Start()
	case "darwin":
		return exec.Command("open", "-R", path).Start()
	default:
		return exec.Command("xdg-open", filepath.Dir(path)).Start()
	}
}

func ReconcileSelectedDevice(current, persisted string, devices []Device) string {
	current = strings.TrimSpace(current)
	persisted = strings.TrimSpace(persisted)
	if current != "" {
		for _, d := range devices {
			if d.Serial == current {
				return current
			}
		}
	}
	if persisted != "" {
		for _, d := range devices {
			if d.Serial == persisted {
				return persisted
			}
		}
	}
	if len(devices) == 1 {
		return devices[0].Serial
	}
	return current
}

func addUniqueString(values []string, value string, limit int) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	out := []string{value}
	for _, v := range values {
		if v != value {
			out = append(out, v)
		}
		if len(out) >= limit {
			break
		}
	}
	return out
}

func (p RuntimePaths) baseDir() string {
	if p.BaseDir != "" {
		return p.BaseDir
	}
	if cache, err := os.UserCacheDir(); err == nil && cache != "" {
		return filepath.Join(cache, appName, "tools")
	}
	if config, err := os.UserConfigDir(); err == nil && config != "" {
		return filepath.Join(config, appName, "tools")
	}
	if wd, err := os.Getwd(); err == nil {
		return filepath.Join(wd, ".adbpureflow", "tools")
	}
	return filepath.Join(".", ".adbpureflow", "tools")
}

// CommandResult contains enough structured detail for friendly errors and an
// advanced technical-details view.
type CommandResult struct {
	Executable string
	Args       []string
	Stdout     string
	Stderr     string
	ExitCode   int
	Duration   time.Duration
}

func (r CommandResult) CombinedOutput() string {
	out := strings.TrimSpace(r.Stdout)
	err := strings.TrimSpace(r.Stderr)
	switch {
	case out == "":
		return err
	case err == "":
		return out
	default:
		return out + "\n" + err
	}
}

// CommandError wraps a failed process with user-friendly categorisation.
type CommandError struct {
	Result CommandResult
	Err    error
}

func (e *CommandError) Error() string {
	if e == nil {
		return ""
	}
	combined := e.Result.CombinedOutput()
	if combined == "" {
		combined = e.Err.Error()
	}
	return combined
}

func (e *CommandError) Unwrap() error { return e.Err }

type CommandRunner interface {
	Run(ctx context.Context, executable string, args ...string) (CommandResult, error)
	Start(ctx context.Context, executable string, args ...string) (*ManagedProcess, error)
	Stream(ctx context.Context, executable string, args []string, onLine func(string)) (CommandResult, error)
}

type cappedBuffer struct {
	limit int
	buf   []byte
}

func newCappedBuffer(limit int) *cappedBuffer {
	return &cappedBuffer{limit: limit}
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	b.buf = append(b.buf, p...)
	if b.limit > 0 && len(b.buf) > b.limit {
		b.buf = b.buf[len(b.buf)-b.limit:]
	}
	return len(p), nil
}

func (b *cappedBuffer) String() string {
	return string(b.buf)
}

type OSRunner struct{}

func (OSRunner) Run(ctx context.Context, executable string, args ...string) (CommandResult, error) {
	started := time.Now()
	cmd := exec.CommandContext(ctx, executable, args...)
	configureCommand(cmd)
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	err := cmd.Run()
	result := CommandResult{Executable: executable, Args: append([]string(nil), args...), Stdout: out.String(), Stderr: stderr.String(), Duration: time.Since(started)}
	if cmd.ProcessState != nil {
		result.ExitCode = cmd.ProcessState.ExitCode()
	}
	if err != nil {
		return result, &CommandError{Result: result, Err: err}
	}
	return result, nil
}

func (OSRunner) Start(ctx context.Context, executable string, args ...string) (*ManagedProcess, error) {
	cmd := exec.CommandContext(ctx, executable, args...)
	configureCommand(cmd)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	mp := &ManagedProcess{cmd: cmd, done: make(chan struct{}), cancel: func() { _ = cmd.Process.Kill() }}
	go func() {
		err := cmd.Wait()
		mp.mu.Lock()
		mp.err = err
		mp.mu.Unlock()
		close(mp.done)
	}()
	return mp, nil
}

func (OSRunner) Stream(ctx context.Context, executable string, args []string, onLine func(string)) (CommandResult, error) {
	started := time.Now()
	cmd := exec.CommandContext(ctx, executable, args...)
	configureCommand(cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return CommandResult{}, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return CommandResult{}, err
	}
	outBuf := newCappedBuffer(256 * 1024)
	errBuf := newCappedBuffer(256 * 1024)
	if err := cmd.Start(); err != nil {
		return CommandResult{}, err
	}
	var wg sync.WaitGroup
	read := func(r io.Reader, b io.Writer) {
		defer wg.Done()
		s := bufio.NewScanner(r)
		s.Buffer(make([]byte, 4096), 1024*1024)
		for s.Scan() {
			line := s.Text()
			_, _ = io.WriteString(b, line+"\n")
			if onLine != nil {
				onLine(line)
			}
		}
	}
	wg.Add(2)
	go read(stdout, outBuf)
	go read(stderr, errBuf)
	err = cmd.Wait()
	wg.Wait()
	result := CommandResult{Executable: executable, Args: append([]string(nil), args...), Stdout: outBuf.String(), Stderr: errBuf.String(), Duration: time.Since(started)}
	if cmd.ProcessState != nil {
		result.ExitCode = cmd.ProcessState.ExitCode()
	}
	if err != nil {
		return result, &CommandError{Result: result, Err: err}
	}
	return result, nil
}

type ManagedProcess struct {
	cmd    *exec.Cmd
	done   chan struct{}
	cancel func()
	mu     sync.Mutex
	err    error
}

func (p *ManagedProcess) Stop() {
	if p == nil || p.cancel == nil {
		return
	}
	p.cancel()
	select {
	case <-p.done:
	case <-time.After(2 * time.Second):
	}
}

func (p *ManagedProcess) Done() <-chan struct{} { return p.done }

func (p *ManagedProcess) Err() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.err
}

// Domain models.
type DeviceStatus string

const (
	DeviceOnline       DeviceStatus = "online"
	DeviceOffline      DeviceStatus = "offline"
	DeviceUnauthorized DeviceStatus = "unauthorized"
	DeviceRecovery     DeviceStatus = "recovery"
	DeviceUnknown      DeviceStatus = "unknown"
)

type TransportType string

const (
	TransportUSB      TransportType = "USB"
	TransportWireless TransportType = "Wireless"
	TransportUnknown  TransportType = "Unknown"
)

type Device struct {
	Serial          string
	DisplayName     string
	Manufacturer    string
	Model           string
	AndroidVersion  string
	SDK             string
	Transport       TransportType
	Status          DeviceStatus
	BatteryPercent  string
	StorageSummary  string
	ScreenSize      string
	Product         string
	Authorized      bool
	LastSeen        time.Time
	RawDeviceLine   string
	ConnectionError string
}

func (d Device) Label() string {
	name := d.DisplayName
	if name == "" {
		name = strings.TrimSpace(d.Manufacturer + " " + d.Model)
	}
	if name == "" {
		name = "Android device"
	}
	return fmt.Sprintf("%s  —  %s", name, d.Serial)
}

type APKMetadata struct {
	Path        string
	FileName    string
	SizeBytes   int64
	SHA256      string
	PackageID   string
	VersionName string
	VersionCode string
	MinSDK      string
	TargetSDK   string
	AppLabel    string
	Permissions []string
	Warnings    []string
}

type PackageInfo struct {
	PackageName string
	APKPath     string
	UserApp     bool
	Enabled     bool
	VersionName string
	VersionCode string
}

type LogEntry struct {
	Time    string
	PID     string
	TID     string
	Level   string
	Tag     string
	Message string
	Raw     string
}

// ADB service.
type ADB struct {
	runner CommandRunner
	paths  RuntimePaths
}

func NewADB(runner CommandRunner, paths RuntimePaths) *ADB { return &ADB{runner: runner, paths: paths} }

var adbURLs = map[string]string{
	"windows": "https://dl.google.com/android/repository/platform-tools-latest-windows.zip",
	"darwin":  "https://dl.google.com/android/repository/platform-tools-latest-darwin.zip",
	"linux":   "https://dl.google.com/android/repository/platform-tools-latest-linux.zip",
}

func (a *ADB) Path(ctx context.Context) (string, error) {
	return a.locate(ctx, true)
}

func (a *ADB) ProbePath(ctx context.Context) (string, error) {
	return a.locate(ctx, false)
}

func (a *ADB) locate(ctx context.Context, provision bool) (string, error) {
	if a.paths.ADBPath != "" {
		return a.paths.ADBPath, nil
	}
	adbName := "adb"
	if runtime.GOOS == "windows" {
		adbName = "adb.exe"
	}
	base := a.paths.baseDir()
	localADB := filepath.Join(base, adbFolder, "platform-tools", adbName)
	if _, err := os.Stat(localADB); err == nil {
		return localADB, nil
	}
	if path, err := exec.LookPath(adbName); err == nil {
		return path, nil
	}
	if !provision {
		return adbName, errors.New("ADB was not found in AdbPureFlow tools or on PATH")
	}
	url, ok := adbURLs[runtime.GOOS]
	if !ok {
		return adbName, fmt.Errorf("ADB was not found on PATH and automatic platform-tools download is not configured for %s", runtime.GOOS)
	}
	if err := downloadToZipAndExtract(ctx, url, filepath.Join(base, adbFolder)); err != nil {
		return adbName, fmt.Errorf("ADB was not found and platform-tools download failed: %w", err)
	}
	if runtime.GOOS != "windows" {
		_ = os.Chmod(localADB, 0755)
	}
	if _, err := os.Stat(localADB); err == nil {
		return localADB, nil
	}
	return adbName, errors.New("ADB setup completed but adb executable was not found")
}

func (a *ADB) run(ctx context.Context, timeout time.Duration, args ...string) (CommandResult, error) {
	adb, err := a.Path(ctx)
	if err != nil {
		return CommandResult{}, err
	}
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	return a.runner.Run(ctx, adb, args...)
}

func (a *ADB) runDevice(ctx context.Context, serial string, timeout time.Duration, args ...string) (CommandResult, error) {
	adb, err := a.Path(ctx)
	if err != nil {
		return CommandResult{}, err
	}
	return a.runDeviceWithADB(ctx, adb, serial, timeout, args...)
}

func (a *ADB) runDeviceWithADB(ctx context.Context, adb, serial string, timeout time.Duration, args ...string) (CommandResult, error) {
	serial = strings.TrimSpace(serial)
	if serial == "" {
		return CommandResult{}, errors.New("no target device serial was provided; select a specific authorized device first")
	}
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	all := append([]string{"-s", serial}, args...)
	return a.runner.Run(ctx, adb, all...)
}

func (a *ADB) Devices(ctx context.Context) ([]Device, error) {
	return a.scanDevices(ctx, true)
}

func (a *ADB) ProbeDevices(ctx context.Context) ([]Device, error) {
	return a.scanDevices(ctx, false)
}

func (a *ADB) scanDevices(ctx context.Context, provision bool) ([]Device, error) {
	adb, err := a.locate(ctx, provision)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	res, err := a.runner.Run(ctx, adb, "devices", "-l")
	if err != nil {
		return nil, err
	}
	devices := ParseADBDevices(res.Stdout)
	for i := range devices {
		if devices[i].Status == DeviceOnline {
			_ = a.enrichDeviceWithADB(ctx, adb, &devices[i])
		}
	}
	return devices, nil
}

func ParseADBDevices(output string) []Device {
	var devices []Device
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "List of") || strings.HasPrefix(line, "*") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		d := Device{Serial: fields[0], Status: parseDeviceStatus(fields[1]), LastSeen: time.Now(), RawDeviceLine: line, Transport: TransportUnknown}
		d.Authorized = d.Status == DeviceOnline
		if strings.Contains(d.Serial, ":") {
			d.Transport = TransportWireless
		} else if strings.HasPrefix(d.Serial, "usb:") {
			d.Transport = TransportUSB
		} else {
			d.Transport = TransportUSB
		}
		for _, f := range fields[2:] {
			if strings.HasPrefix(f, "model:") {
				d.Model = cleanProp(strings.TrimPrefix(f, "model:"))
			}
			if strings.HasPrefix(f, "product:") {
				d.Product = cleanProp(strings.TrimPrefix(f, "product:"))
			}
		}
		if d.Model != "" {
			d.DisplayName = d.Model
		}
		devices = append(devices, d)
	}
	sort.Slice(devices, func(i, j int) bool { return devices[i].Label() < devices[j].Label() })
	return devices
}

func parseDeviceStatus(s string) DeviceStatus {
	switch s {
	case "device":
		return DeviceOnline
	case "offline":
		return DeviceOffline
	case "unauthorized":
		return DeviceUnauthorized
	case "recovery":
		return DeviceRecovery
	default:
		return DeviceUnknown
	}
}

func cleanProp(v string) string { return strings.ReplaceAll(strings.TrimSpace(v), "_", " ") }

var packageNamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*(\.[A-Za-z][A-Za-z0-9_]*)+$`)

func validatePackageName(pkg string) error {
	pkg = strings.TrimSpace(pkg)
	if pkg == "" {
		return errors.New("package name is required")
	}
	if !packageNamePattern.MatchString(pkg) {
		return fmt.Errorf("%q does not look like a valid Android package name", pkg)
	}
	return nil
}

func (a *ADB) EnrichDevice(ctx context.Context, d *Device) error {
	adb, err := a.Path(ctx)
	if err != nil {
		return err
	}
	return a.enrichDeviceWithADB(ctx, adb, d)
}

func (a *ADB) enrichDeviceWithADB(ctx context.Context, adb string, d *Device) error {
	props := map[string]*string{
		"ro.product.manufacturer":  &d.Manufacturer,
		"ro.product.model":         &d.Model,
		"ro.build.version.release": &d.AndroidVersion,
		"ro.build.version.sdk":     &d.SDK,
	}
	for prop, target := range props {
		if res, err := a.runDeviceWithADB(ctx, adb, d.Serial, 5*time.Second, "shell", "getprop", prop); err == nil {
			*target = cleanProp(res.Stdout)
		}
	}
	if d.DisplayName == "" {
		d.DisplayName = strings.TrimSpace(d.Manufacturer + " " + d.Model)
	}
	if res, err := a.runDeviceWithADB(ctx, adb, d.Serial, 5*time.Second, "shell", "dumpsys", "battery"); err == nil {
		d.BatteryPercent = ParseBatteryLevel(res.Stdout)
	}
	if res, err := a.runDeviceWithADB(ctx, adb, d.Serial, 5*time.Second, "shell", "wm", "size"); err == nil {
		d.ScreenSize = ParseScreenSize(res.Stdout)
	}
	if res, err := a.runDeviceWithADB(ctx, adb, d.Serial, 8*time.Second, "shell", "df", "-h", "/data"); err == nil {
		d.StorageSummary = ParseStorageSummary(res.Stdout)
	}
	return nil
}

func ParseBatteryLevel(output string) string {
	re := regexp.MustCompile(`(?m)^\s*level:\s*(\d+)`)
	m := re.FindStringSubmatch(output)
	if len(m) == 2 {
		return m[1] + "%"
	}
	return ""
}

func ParseScreenSize(output string) string {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Physical size:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "Physical size:"))
		}
	}
	return ""
}

func ParseStorageSummary(output string) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) < 2 {
		return ""
	}
	fields := strings.Fields(lines[len(lines)-1])
	if len(fields) >= 5 {
		return fmt.Sprintf("%s used of %s (%s free)", fields[2], fields[1], fields[3])
	}
	return ""
}

func (a *ADB) PairWireless(ctx context.Context, hostPort, code string) (string, error) {
	hostPort = strings.TrimSpace(hostPort)
	code = strings.TrimSpace(code)
	if err := validateHostPort(hostPort); err != nil || code == "" {
		return "", errors.New("enter the pairing address exactly as Android shows it, for example 192.168.1.24:37123, plus the six-digit pairing code")
	}
	res, err := a.run(ctx, 45*time.Second, "pair", hostPort, code)
	out := strings.TrimSpace(res.CombinedOutput())
	if err != nil {
		return humanizeADBError(out, err), err
	}
	if !strings.Contains(strings.ToLower(out), "success") && !strings.Contains(strings.ToLower(out), "paired") {
		return out, errors.New("ADB did not confirm that wireless pairing succeeded")
	}
	return out, nil
}

func (a *ADB) ConnectWireless(ctx context.Context, hostPort string) (string, error) {
	hostPort = strings.TrimSpace(hostPort)
	if err := validateHostPort(hostPort); err != nil {
		return "", errors.New("enter the connect address shown by Android Wireless debugging, for example 192.168.1.24:5555")
	}
	res, err := a.run(ctx, 30*time.Second, "connect", hostPort)
	out := strings.TrimSpace(res.CombinedOutput())
	if err != nil {
		return humanizeADBError(out, err), err
	}
	lower := strings.ToLower(out)
	if !strings.Contains(lower, "connected") && !strings.Contains(lower, "already connected") {
		return out, errors.New("ADB did not confirm that the wireless device connected")
	}
	return out, nil
}

func validateHostPort(value string) error {
	host, port, err := net.SplitHostPort(value)
	if err != nil || strings.TrimSpace(host) == "" || strings.TrimSpace(port) == "" {
		return errors.New("address must include host and port")
	}
	if _, err := strconv.Atoi(port); err != nil {
		return errors.New("port must be numeric")
	}
	return nil
}

func (a *ADB) RestartServer(ctx context.Context) error {
	if _, err := a.run(ctx, 10*time.Second, "kill-server"); err != nil {
		return err
	}
	_, err := a.run(ctx, 15*time.Second, "start-server")
	return err
}

func (a *ADB) Disconnect(ctx context.Context, serial string) (string, error) {
	if serial == "" {
		return "", errors.New("no device selected")
	}
	res, err := a.run(ctx, 15*time.Second, "disconnect", serial)
	return strings.TrimSpace(res.CombinedOutput()), err
}

func (a *ADB) InstallAPK(ctx context.Context, serial, apk string, allowDowngrade bool) (string, error) {
	if _, err := os.Stat(apk); err != nil {
		return "", fmt.Errorf("APK file cannot be opened: %w", err)
	}
	args := []string{"install", "-r"}
	if allowDowngrade {
		args = append(args, "-d")
	}
	args = append(args, apk)
	res, err := a.runDevice(ctx, serial, 2*time.Minute, args...)
	out := strings.TrimSpace(res.CombinedOutput())
	if err != nil {
		return humanizeADBError(out, err), err
	}
	if !strings.Contains(strings.ToLower(out), "success") {
		return humanizeADBError(out, errors.New("adb install did not report success")), errors.New("adb install did not report success")
	}
	return out, nil
}

func (a *ADB) Launch(ctx context.Context, serial, pkg string) (string, error) {
	if err := validatePackageName(pkg); err != nil {
		return "", err
	}
	res, err := a.runDevice(ctx, serial, 20*time.Second, "shell", "monkey", "-p", pkg, "-c", "android.intent.category.LAUNCHER", "1")
	out := strings.TrimSpace(res.CombinedOutput())
	if err != nil {
		return humanizeADBError(out, err), err
	}
	if strings.Contains(strings.ToLower(out), "no activities found") {
		return out, errors.New("Android could not find a launchable activity for this package")
	}
	return out, nil
}

func (a *ADB) ForceStop(ctx context.Context, serial, pkg string) (string, error) {
	if err := validatePackageName(pkg); err != nil {
		return "", err
	}
	res, err := a.runDevice(ctx, serial, 20*time.Second, "shell", "am", "force-stop", pkg)
	return strings.TrimSpace(res.CombinedOutput()), err
}

func (a *ADB) ClearData(ctx context.Context, serial, pkg string) (string, error) {
	if err := validatePackageName(pkg); err != nil {
		return "", err
	}
	res, err := a.runDevice(ctx, serial, 30*time.Second, "shell", "pm", "clear", pkg)
	out := strings.TrimSpace(res.CombinedOutput())
	if err != nil {
		return humanizeADBError(out, err), err
	}
	if !strings.Contains(strings.ToLower(out), "success") {
		return out, errors.New("Android did not confirm that app data was cleared")
	}
	return out, nil
}

func (a *ADB) Uninstall(ctx context.Context, serial, pkg string) (string, error) {
	if err := validatePackageName(pkg); err != nil {
		return "", err
	}
	res, err := a.runDevice(ctx, serial, 45*time.Second, "uninstall", pkg)
	out := strings.TrimSpace(res.CombinedOutput())
	if err != nil {
		return humanizeADBError(out, err), err
	}
	if !strings.Contains(strings.ToLower(out), "success") {
		return out, errors.New("Android did not confirm that the package was uninstalled")
	}
	return out, nil
}

func (a *ADB) Packages(ctx context.Context, serial string, userOnly bool) ([]PackageInfo, error) {
	if userOnly {
		res, err := a.runDevice(ctx, serial, 30*time.Second, "shell", "pm", "list", "packages", "-f", "-3")
		if err != nil {
			return nil, err
		}
		pkgs := ParsePackages(res.Stdout, true)
		sort.Slice(pkgs, func(i, j int) bool { return pkgs[i].PackageName < pkgs[j].PackageName })
		return pkgs, nil
	}
	userRes, userErr := a.runDevice(ctx, serial, 30*time.Second, "shell", "pm", "list", "packages", "-f", "-3")
	systemRes, systemErr := a.runDevice(ctx, serial, 30*time.Second, "shell", "pm", "list", "packages", "-f", "-s")
	if userErr != nil && systemErr != nil {
		return nil, userErr
	}
	pkgsByName := map[string]PackageInfo{}
	if userErr == nil {
		for _, p := range ParsePackages(userRes.Stdout, true) {
			pkgsByName[p.PackageName] = p
		}
	}
	if systemErr == nil {
		for _, p := range ParsePackages(systemRes.Stdout, false) {
			pkgsByName[p.PackageName] = p
		}
	}
	pkgs := make([]PackageInfo, 0, len(pkgsByName))
	for _, p := range pkgsByName {
		pkgs = append(pkgs, p)
	}
	sort.Slice(pkgs, func(i, j int) bool { return pkgs[i].PackageName < pkgs[j].PackageName })
	return pkgs, nil
}

func ParsePackages(output string, userOnly bool) []PackageInfo {
	var pkgs []PackageInfo
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "package:") {
			continue
		}
		v := strings.TrimPrefix(line, "package:")
		parts := strings.Split(v, "=")
		if len(parts) == 2 {
			pkgs = append(pkgs, PackageInfo{APKPath: parts[0], PackageName: parts[1], UserApp: userOnly, Enabled: true})
		} else if v != "" {
			pkgs = append(pkgs, PackageInfo{PackageName: v, UserApp: userOnly, Enabled: true})
		}
	}
	return pkgs
}

func parsePackages(output string) map[string]bool { // retained for CLI-era tests/helpers
	m := map[string]bool{}
	for _, p := range ParsePackages(output, true) {
		m[p.PackageName] = true
	}
	return m
}

func (a *ADB) StartLogcat(ctx context.Context, serial string, onLine func(LogEntry)) (CommandResult, error) {
	serial = strings.TrimSpace(serial)
	if serial == "" {
		return CommandResult{}, errors.New("no target device serial was provided; select a specific authorized device first")
	}
	adb, err := a.Path(ctx)
	if err != nil {
		return CommandResult{}, err
	}
	args := []string{"-s", serial, "logcat", "-v", "threadtime"}
	return a.runner.Stream(ctx, adb, args, func(line string) {
		if onLine != nil {
			onLine(ParseLogcatLine(line))
		}
	})
}

func (a *ADB) ClearLogcat(ctx context.Context, serial string) error {
	_, err := a.runDevice(ctx, serial, 10*time.Second, "logcat", "-c")
	return err
}

var threadtimeLogcat = regexp.MustCompile(`^\s*(\d\d-\d\d\s+\d\d:\d\d:\d\d\.\d+)\s+(\d+)\s+(\d+)\s+([VDIWEF])\s+([^:]+):\s?(.*)$`)

func LogLevelCode(label string) string {
	switch label {
	case "Verbose":
		return "V"
	case "Debug":
		return "D"
	case "Info":
		return "I"
	case "Warn":
		return "W"
	case "Error":
		return "E"
	case "Fatal":
		return "F"
	default:
		return ""
	}
}

func ParseLogcatLine(line string) LogEntry {
	m := threadtimeLogcat.FindStringSubmatch(line)
	if len(m) == 7 {
		return LogEntry{Time: m[1], PID: m[2], TID: m[3], Level: m[4], Tag: strings.TrimSpace(m[5]), Message: m[6], Raw: line}
	}
	return LogEntry{Raw: line, Message: line}
}

func (a *ADB) CaptureScreenshot(ctx context.Context, serial, outputPath string) error {
	serial = strings.TrimSpace(serial)
	if serial == "" {
		return errors.New("no target device serial was provided; select a specific authorized device first")
	}
	adb, err := a.Path(ctx)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, adb, "-s", serial, "exec-out", "screencap", "-p")
	configureCommand(cmd)
	data, err := cmd.Output()
	if err != nil {
		return err
	}
	return os.WriteFile(outputPath, data, 0644)
}

// APK metadata.
func InspectAPK(ctx context.Context, path string) (APKMetadata, error) {
	path = filepath.Clean(path)
	st, err := os.Stat(path)
	if err != nil {
		return APKMetadata{}, err
	}
	if st.IsDir() || !strings.EqualFold(filepath.Ext(path), ".apk") {
		return APKMetadata{}, errors.New("choose a valid .apk file")
	}
	f, err := os.Open(path)
	if err != nil {
		return APKMetadata{}, err
	}
	h := sha256.New()
	_, _ = io.Copy(h, f)
	_ = f.Close()
	meta := APKMetadata{Path: path, FileName: filepath.Base(path), SizeBytes: st.Size(), SHA256: hex.EncodeToString(h.Sum(nil))}
	zr, err := zip.OpenReader(path)
	if err != nil {
		return meta, fmt.Errorf("invalid APK zip: %w", err)
	}
	defer zr.Close()
	foundManifest := false
	for _, f := range zr.File {
		if f.Name == "AndroidManifest.xml" {
			foundManifest = true
			break
		}
	}
	if !foundManifest {
		meta.Warnings = append(meta.Warnings, "AndroidManifest.xml was not found; this does not look like an installable APK.")
	}
	if aapt, err := exec.LookPath("aapt"); err == nil {
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, aapt, "dump", "badging", path)
		out, err := cmd.Output()
		if err == nil {
			applyAAPTBadging(&meta, string(out))
		} else {
			meta.Warnings = append(meta.Warnings, "aapt was found but could not read APK metadata; showing file-level metadata only.")
		}
	} else {
		meta.Warnings = append(meta.Warnings, "Installable APK verified as a zip. Detailed package/version/permission metadata requires Android build tools (aapt) on PATH.")
	}
	return meta, nil
}

func applyAAPTBadging(meta *APKMetadata, output string) {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "package:") {
			meta.PackageID = firstQuoted(line, "name=")
			meta.VersionCode = firstQuoted(line, "versionCode=")
			meta.VersionName = firstQuoted(line, "versionName=")
		}
		if strings.HasPrefix(line, "sdkVersion:") {
			meta.MinSDK = strings.Trim(line[len("sdkVersion:"):], "'")
		}
		if strings.HasPrefix(line, "targetSdkVersion:") {
			meta.TargetSDK = strings.Trim(line[len("targetSdkVersion:"):], "'")
		}
		if strings.HasPrefix(line, "application-label:") {
			meta.AppLabel = strings.Trim(line[len("application-label:"):], "'")
		}
		if strings.HasPrefix(line, "uses-permission:") {
			p := firstQuoted(line, "name=")
			if p != "" {
				meta.Permissions = append(meta.Permissions, p)
			}
		}
	}
}

func firstQuoted(line, key string) string {
	idx := strings.Index(line, key+"'")
	if idx < 0 {
		return ""
	}
	rest := line[idx+len(key)+1:]
	end := strings.Index(rest, "'")
	if end < 0 {
		return ""
	}
	return rest[:end]
}

func FormatBytes(n int64) string {
	units := []string{"B", "KB", "MB", "GB"}
	v := float64(n)
	i := 0
	for v >= 1024 && i < len(units)-1 {
		v /= 1024
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%d %s", n, units[i])
	}
	return fmt.Sprintf("%.1f %s", v, units[i])
}

// Scrcpy.
type ScrcpyManager struct {
	runner CommandRunner
	paths  RuntimePaths
	mu     sync.Mutex
	byDev  map[string]*ManagedProcess
}

func NewScrcpyManager(runner CommandRunner, paths RuntimePaths) *ScrcpyManager {
	return &ScrcpyManager{runner: runner, paths: paths, byDev: map[string]*ManagedProcess{}}
}

var downloadURLs = map[string]string{
	"windows-amd64": "https://github.com/Genymobile/scrcpy/releases/download/v3.3.4/scrcpy-win64-v3.3.4.zip",
	"windows-386":   "https://github.com/Genymobile/scrcpy/releases/download/v3.3.4/scrcpy-win32-v3.3.4.zip",
	"windows-arm64": "https://github.com/Genymobile/scrcpy/releases/download/v3.3.4/scrcpy-win64-v3.3.4.zip",
	"linux-amd64":   "https://github.com/Genymobile/scrcpy/releases/download/v3.3.4/scrcpy-linux-x86_64-v3.3.4.tar.gz",
	"darwin-amd64":  "https://github.com/Genymobile/scrcpy/releases/download/v3.3.4/scrcpy-macos-x86_64-v3.3.4.tar.gz",
	"darwin-arm64":  "https://github.com/Genymobile/scrcpy/releases/download/v3.3.4/scrcpy-macos-aarch64-v3.3.4.tar.gz",
}

func (s *ScrcpyManager) Path(ctx context.Context) (string, error) {
	return s.locate(ctx, true)
}

func (s *ScrcpyManager) ProbePath(ctx context.Context) (string, error) {
	return s.locate(ctx, false)
}

func (s *ScrcpyManager) locate(ctx context.Context, provision bool) (string, error) {
	if s.paths.ScrcpyPath != "" {
		return s.paths.ScrcpyPath, nil
	}
	execName := "scrcpy"
	if runtime.GOOS == "windows" {
		execName = "scrcpy.exe"
	}
	if p, err := exec.LookPath(execName); err == nil {
		return p, nil
	}
	root := filepath.Join(s.paths.baseDir(), scrcpyFolder)
	if p := findScrcpyBinary(root); p != "" {
		return p, nil
	}
	if !provision {
		return execName, errors.New("scrcpy was not found in AdbPureFlow tools or on PATH")
	}
	key := runtime.GOOS + "-" + runtime.GOARCH
	url, ok := downloadURLs[key]
	if !ok {
		return execName, fmt.Errorf("scrcpy is not installed and automatic download is not configured for %s", key)
	}
	if err := os.MkdirAll(root, 0755); err != nil {
		return execName, err
	}
	if strings.HasSuffix(url, ".zip") {
		if err := downloadToZipAndExtract(ctx, url, root); err != nil {
			return execName, err
		}
	} else {
		if err := downloadToTarGzAndExtract(ctx, url, root); err != nil {
			return execName, err
		}
	}
	if p := findScrcpyBinary(root); p != "" {
		if runtime.GOOS != "windows" {
			_ = os.Chmod(p, 0755)
		}
		return p, nil
	}
	return execName, errors.New("scrcpy setup completed but scrcpy executable was not found")
}

func (s *ScrcpyManager) StartMirror(ctx context.Context, serial string, opts ScrcpyOptions) (*ManagedProcess, error) {
	if serial == "" {
		return nil, errors.New("no device selected")
	}
	path, err := s.Path(ctx)
	if err != nil {
		return nil, err
	}
	args := []string{"-s", serial, "--window-title", "AdbPureFlow Screen - " + serial}
	if opts.AlwaysOnTop {
		args = append(args, "--always-on-top")
	}
	if opts.Fullscreen {
		args = append(args, "--fullscreen")
	}
	if opts.MaxSize > 0 {
		args = append(args, "--max-size", strconv.Itoa(opts.MaxSize))
	}
	if opts.VideoBitRate != "" {
		args = append(args, "--video-bit-rate", opts.VideoBitRate)
	}
	if opts.RecordPath != "" {
		args = append(args, "--record", opts.RecordPath)
	}
	mp, err := s.runner.Start(context.Background(), path, args...)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	old := s.byDev[serial]
	s.byDev[serial] = mp
	s.mu.Unlock()
	if old != nil {
		old.Stop()
	}
	go func() {
		<-mp.Done()
		s.mu.Lock()
		if s.byDev[serial] == mp {
			delete(s.byDev, serial)
		}
		s.mu.Unlock()
	}()
	return mp, nil
}

func (s *ScrcpyManager) StopMirror(serial string) {
	s.mu.Lock()
	mp := s.byDev[serial]
	delete(s.byDev, serial)
	s.mu.Unlock()
	if mp != nil {
		mp.Stop()
	}
}

func (s *ScrcpyManager) StopAll() {
	s.mu.Lock()
	processes := make([]*ManagedProcess, 0, len(s.byDev))
	for serial, mp := range s.byDev {
		processes = append(processes, mp)
		delete(s.byDev, serial)
	}
	s.mu.Unlock()
	for _, mp := range processes {
		mp.Stop()
	}
}

func (s *ScrcpyManager) IsMirroring(serial string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.byDev[serial] != nil
}

type ScrcpyOptions struct {
	AlwaysOnTop  bool
	Fullscreen   bool
	MaxSize      int
	VideoBitRate string
	RecordPath   string
}

func findScrcpyFolder(root string) string { // retained for older tests
	p := findScrcpyBinary(root)
	if p == "" {
		return ""
	}
	return filepath.Dir(p)
}

func findScrcpyBinary(root string) string {
	execName := "scrcpy"
	if runtime.GOOS == "windows" {
		execName = "scrcpy.exe"
	}
	var found string
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || found != "" || d.IsDir() {
			return nil
		}
		if d.Name() == execName {
			found = path
		}
		return nil
	})
	return found
}

// Workflow engine foundation.
type FlowActionType string

const (
	FlowInstallAPK  FlowActionType = "Install APK"
	FlowClearData   FlowActionType = "Clear Data"
	FlowLaunchApp   FlowActionType = "Launch App"
	FlowStartMirror FlowActionType = "Start Mirror"
	FlowStartLogs   FlowActionType = "Start Logs"
	FlowWait        FlowActionType = "Wait"
)

type FlowAction struct {
	Type        FlowActionType
	APKPath     string
	PackageName string
	Wait        time.Duration
}

type FlowProfile struct {
	Name    string
	Actions []FlowAction
}

type FlowEvent struct {
	Action FlowAction
	State  string
	Err    error
}

func (a *App) RunFlow(ctx context.Context, serial string, profile FlowProfile, emit func(FlowEvent)) error {
	for _, action := range profile.Actions {
		if emit != nil {
			emit(FlowEvent{Action: action, State: "running"})
		}
		var err error
		switch action.Type {
		case FlowInstallAPK:
			_, err = a.adb.InstallAPK(ctx, serial, action.APKPath, true)
		case FlowClearData:
			_, err = a.adb.ClearData(ctx, serial, action.PackageName)
		case FlowLaunchApp:
			_, err = a.adb.Launch(ctx, serial, action.PackageName)
		case FlowStartMirror:
			_, err = a.scrcpy.StartMirror(ctx, serial, ScrcpyOptions{AlwaysOnTop: false})
		case FlowWait:
			select {
			case <-ctx.Done():
				err = ctx.Err()
			case <-time.After(action.Wait):
			}
		case FlowStartLogs:
			err = errors.New("Start Logs is a UI-owned streaming action and is not executable by the core flow runner yet")
		default:
			err = fmt.Errorf("unsupported flow action %q", action.Type)
		}
		if err != nil {
			if emit != nil {
				emit(FlowEvent{Action: action, State: "failed", Err: err})
			}
			return err
		}
		if emit != nil {
			emit(FlowEvent{Action: action, State: "complete"})
		}
	}
	return nil
}

// Diagnostics.
type DiagnosticCheck struct {
	Name      string
	Status    string
	Message   string
	Recovery  string
	Technical string
}

func (a *App) Doctor(ctx context.Context) []DiagnosticCheck {
	var checks []DiagnosticCheck
	adbPath, err := a.adb.ProbePath(ctx)
	if err != nil {
		checks = append(checks, DiagnosticCheck{"ADB availability", "Fail", "ADB could not be found.", "Install Android platform-tools, reconnect to the internet so AdbPureFlow can provision it during an operation, or configure the adb path in a future Settings release.", err.Error()})
	} else {
		checks = append(checks, DiagnosticCheck{"ADB availability", "OK", "ADB executable was found.", "", adbPath})
		versionCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		res, err := a.adb.runner.Run(versionCtx, adbPath, "version")
		cancel()
		if err == nil && strings.Contains(strings.ToLower(res.Stdout), "android debug bridge") {
			checks = append(checks, DiagnosticCheck{"ADB capability", "OK", "ADB responds normally.", "", strings.TrimSpace(res.Stdout)})
		} else if err != nil {
			checks = append(checks, DiagnosticCheck{"ADB capability", "Fail", "ADB exists but did not run successfully.", "Replace the ADB executable, check antivirus quarantine, or install current Android platform-tools.", err.Error()})
		} else {
			checks = append(checks, DiagnosticCheck{"ADB capability", "Warn", "ADB ran, but the output did not look like Android Debug Bridge.", "Check that the configured adb path points to Android platform-tools.", strings.TrimSpace(res.CombinedOutput())})
		}
	}
	if adbPath != "" && err == nil {
		deviceCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
		res, scanErr := a.adb.runner.Run(deviceCtx, adbPath, "devices", "-l")
		cancel()
		if scanErr != nil {
			checks = append(checks, DiagnosticCheck{"Connected devices", "Fail", "Device scan failed.", "Restart ADB server or check USB drivers.", scanErr.Error()})
		} else {
			devices := ParseADBDevices(res.Stdout)
			if len(devices) == 0 {
				checks = append(checks, DiagnosticCheck{"Connected devices", "Warn", "No Android devices are visible.", "Connect over USB, enable USB debugging, or use Add Device → Wireless.", "adb devices returned no devices"})
			} else {
				checks = append(checks, DiagnosticCheck{"Connected devices", "OK", fmt.Sprintf("%d device(s) visible.", len(devices)), "", ""})
				for _, d := range devices {
					switch d.Status {
					case DeviceUnauthorized:
						checks = append(checks, DiagnosticCheck{"Authorization for " + d.Serial, "Warn", "The device has not authorized this computer.", "Unlock the phone and accept the RSA debugging prompt, then refresh.", d.RawDeviceLine})
					case DeviceOffline:
						checks = append(checks, DiagnosticCheck{"Connection for " + d.Serial, "Warn", "ADB sees the device, but it is offline.", "Reconnect USB, toggle Wireless debugging, or restart the ADB server.", d.RawDeviceLine})
					case DeviceRecovery, DeviceUnknown:
						checks = append(checks, DiagnosticCheck{"State for " + d.Serial, "Warn", "The device is visible but not in normal authorized debug mode.", "Boot Android normally, unlock the device, and refresh.", d.RawDeviceLine})
					}
				}
			}
		}
	}
	if scrcpyPath, err := a.scrcpy.ProbePath(ctx); err == nil {
		versionCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		res, runErr := a.scrcpy.runner.Run(versionCtx, scrcpyPath, "--version")
		cancel()
		if runErr == nil && strings.Contains(strings.ToLower(res.CombinedOutput()), "scrcpy") {
			checks = append(checks, DiagnosticCheck{"scrcpy", "OK", "Screen mirroring engine responds normally.", "", strings.TrimSpace(res.CombinedOutput())})
		} else if runErr != nil {
			checks = append(checks, DiagnosticCheck{"scrcpy", "Warn", "scrcpy was found but did not run successfully.", "Install a current scrcpy release or let AdbPureFlow provision it when mirroring starts.", runErr.Error()})
		} else {
			checks = append(checks, DiagnosticCheck{"scrcpy", "Warn", "scrcpy ran, but the output was unexpected.", "Check that the configured scrcpy path points to Genymobile scrcpy.", strings.TrimSpace(res.CombinedOutput())})
		}
	} else {
		checks = append(checks, DiagnosticCheck{"scrcpy", "Warn", "Screen mirroring is not ready yet.", "Install scrcpy or let AdbPureFlow download it when you start mirroring.", err.Error()})
	}
	return checks
}

func humanizeADBError(output string, err error) string {
	text := strings.TrimSpace(output)
	lower := strings.ToLower(text)
	switch {
	case strings.Contains(lower, "unauthorized"):
		return "The device has not authorized this computer. Unlock the phone, accept the USB debugging prompt, then try again.\n\nTechnical details:\n" + text
	case strings.Contains(lower, "no devices") || strings.Contains(lower, "device not found"):
		return "No matching Android device is connected. Reconnect the device or select a different target.\n\nTechnical details:\n" + text
	case strings.Contains(lower, "failed to connect") || strings.Contains(lower, "cannot connect") || strings.Contains(lower, "connection refused") || strings.Contains(lower, "no route") || strings.Contains(lower, "timed out"):
		return "AdbPureFlow could not reach the wireless debugging endpoint. Make sure the PC and phone are on the same network, Wireless debugging is still enabled, the pairing/connect port has not expired, and no firewall is blocking the connection.\n\nTechnical details:\n" + text
	case strings.Contains(lower, "failed to authenticate") || strings.Contains(lower, "pairing"):
		return "Wireless pairing was rejected or expired. On Android, reopen Wireless debugging → Pair device with pairing code and enter the fresh address and code.\n\nTechnical details:\n" + text
	case strings.Contains(lower, "install_failed_version_downgrade"):
		return "Android blocked the install because the APK version is older than the installed app. Enable downgrade for debug builds or uninstall the existing app first.\n\nTechnical details:\n" + text
	case strings.Contains(lower, "install_failed_update_incompatible"):
		return "Android says this APK cannot update the installed app, usually because the signing certificate changed. Uninstall the existing app if you want to replace it.\n\nTechnical details:\n" + text
	case text != "":
		return text
	default:
		return err.Error()
	}
}

// Archive helpers with traversal protection.
func downloadToZipAndExtract(ctx context.Context, url, dest string) error {
	if err := os.MkdirAll(dest, 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dest, "download-*.zip")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	if err := downloadFile(ctx, url, tmpPath); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	defer os.Remove(tmpPath)
	return unzip(tmpPath, dest)
}

func downloadToTarGzAndExtract(ctx context.Context, url, dest string) error {
	if err := os.MkdirAll(dest, 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dest, "download-*.tar.gz")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	if err := downloadFile(ctx, url, tmpPath); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	defer os.Remove(tmpPath)
	return untar(tmpPath, dest)
}

func downloadFile(ctx context.Context, url, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned %s", resp.Status)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	_, err = io.Copy(f, resp.Body)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
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
