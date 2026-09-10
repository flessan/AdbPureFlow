package main

import (
	"archive/zip"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestParseADBDevices(t *testing.T) {
	out := `List of devices attached
R58N123ABC device usb:1-1 product:a52 model:SM_A525F device:a52 transport_id:3
192.168.1.12:5555 device product:oriole model:Pixel_6 device:oriole transport_id:9
ZY22 unauthorized usb:1-2
`
	devices := ParseADBDevices(out)
	if len(devices) != 3 {
		t.Fatalf("expected 3 devices, got %d", len(devices))
	}
	if devices[0].Serial == "" {
		t.Fatal("expected serials to be populated")
	}
	var wireless, unauthorized bool
	for _, d := range devices {
		if d.Serial == "192.168.1.12:5555" && d.Transport == TransportWireless && d.Model == "Pixel 6" {
			wireless = true
		}
		if d.Serial == "ZY22" && d.Status == DeviceUnauthorized && !d.Authorized {
			unauthorized = true
		}
	}
	if !wireless {
		t.Fatal("expected wireless Pixel device to be parsed")
	}
	if !unauthorized {
		t.Fatal("expected unauthorized device state to be parsed")
	}
}

func TestParsePackagesCompatibilityHelper(t *testing.T) {
	output := `package:/data/app/~~abc/base.apk=com.example.alpha
package:com.example.beta
`
	res := parsePackages(output)
	if len(res) != 2 {
		t.Errorf("expected 2 packages, got %d", len(res))
	}
	if !res["com.example.alpha"] || !res["com.example.beta"] {
		t.Errorf("expected package names to be found: %#v", res)
	}
}

func TestParseLogcatLine(t *testing.T) {
	entry := ParseLogcatLine("09-09 12:34:56.789  1234  5678 E AndroidRuntime: FATAL EXCEPTION: main")
	if entry.Level != "E" || entry.Tag != "AndroidRuntime" || !strings.Contains(entry.Message, "FATAL") {
		t.Fatalf("unexpected log entry: %#v", entry)
	}
}

func TestParseDeviceDetails(t *testing.T) {
	if got := ParseBatteryLevel("  AC powered: false\n  level: 87\n"); got != "87%" {
		t.Fatalf("battery parse = %q", got)
	}
	if got := ParseScreenSize("Physical size: 1080x2400\n"); got != "1080x2400" {
		t.Fatalf("screen parse = %q", got)
	}
	if got := ParseStorageSummary("Filesystem Size Used Avail Use% Mounted on\n/dev/block/dm-8 110G 58G 51G 54% /data\n"); got != "58G used of 110G (51G free)" {
		t.Fatalf("storage parse = %q", got)
	}
}

func TestAAPTBadgingParser(t *testing.T) {
	meta := APKMetadata{}
	applyAAPTBadging(&meta, `package: name='com.example.app' versionCode='42' versionName='1.2.3'
sdkVersion:'23'
targetSdkVersion:'35'
application-label:'Example'
uses-permission: name='android.permission.INTERNET'
uses-permission: name='android.permission.CAMERA'
`)
	if meta.PackageID != "com.example.app" || meta.VersionCode != "42" || meta.VersionName != "1.2.3" {
		t.Fatalf("bad package metadata: %#v", meta)
	}
	if meta.MinSDK != "23" || meta.TargetSDK != "35" || meta.AppLabel != "Example" {
		t.Fatalf("bad sdk/label metadata: %#v", meta)
	}
	if len(meta.Permissions) != 2 {
		t.Fatalf("expected two permissions: %#v", meta.Permissions)
	}
}

func TestInspectAPKValidatesZipAndHashes(t *testing.T) {
	tmp := t.TempDir()
	apk := filepath.Join(tmp, "sample.apk")
	f, err := os.Create(apk)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	w, err := zw.Create("AndroidManifest.xml")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write([]byte("binary manifest bytes for container validation"))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	meta, err := InspectAPK(context.Background(), apk)
	if err != nil {
		t.Fatal(err)
	}
	if meta.FileName != "sample.apk" || meta.SizeBytes <= 0 || meta.SHA256 == "" {
		t.Fatalf("unexpected metadata: %#v", meta)
	}
}

func TestFindScrcpyFolder(t *testing.T) {
	tempDir := t.TempDir()
	scrcpySubDir := filepath.Join(tempDir, "scrcpy-win64-v3.3.4")
	if err := os.MkdirAll(scrcpySubDir, 0755); err != nil {
		t.Fatal(err)
	}
	execName := "scrcpy"
	if runtime.GOOS == "windows" {
		execName = "scrcpy.exe"
	}
	simulatedBinary := filepath.Join(scrcpySubDir, execName)
	if err := os.WriteFile(simulatedBinary, []byte("dummy binary contents"), 0755); err != nil {
		t.Fatal(err)
	}
	foundDir := findScrcpyFolder(tempDir)
	if foundDir != scrcpySubDir {
		t.Errorf("expected to find %s, got %s", scrcpySubDir, foundDir)
	}
}

type fakeRunner struct {
	result CommandResult
	err    error
	calls  [][]string
}

func (f *fakeRunner) Run(ctx context.Context, executable string, args ...string) (CommandResult, error) {
	f.calls = append(f.calls, append([]string{executable}, args...))
	return f.result, f.err
}

func (f *fakeRunner) Start(ctx context.Context, executable string, args ...string) (*ManagedProcess, error) {
	f.calls = append(f.calls, append([]string{executable}, args...))
	done := make(chan struct{})
	close(done)
	return &ManagedProcess{done: done}, f.err
}

func (f *fakeRunner) Stream(ctx context.Context, executable string, args []string, onLine func(string)) (CommandResult, error) {
	f.calls = append(f.calls, append([]string{executable}, args...))
	return f.result, f.err
}

func TestOSRunnerRunHonorsContextCancellation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses POSIX sleep for the lightweight sandbox core test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err := OSRunner{}.Run(ctx, "sleep", "2")
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
	if time.Since(started) > time.Second {
		t.Fatal("runner did not terminate promptly after context timeout")
	}
}

func TestScrcpyStartMirrorBuildsTargetedArguments(t *testing.T) {
	runner := &fakeRunner{}
	manager := NewScrcpyManager(runner, RuntimePaths{ScrcpyPath: "scrcpy"})
	_, err := manager.StartMirror(context.Background(), "device-1", ScrcpyOptions{AlwaysOnTop: true, Fullscreen: true, MaxSize: 1080, VideoBitRate: "4M", RecordPath: "C:/tmp/recording.mp4"})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(runner.calls[0], " ")
	for _, want := range []string{"scrcpy", "-s device-1", "--always-on-top", "--fullscreen", "--max-size 1080", "--video-bit-rate 4M", "--record C:/tmp/recording.mp4"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("scrcpy args missing %q: %v", want, runner.calls[0])
		}
	}
}

func TestInstallAPKRequiresExplicitSuccess(t *testing.T) {
	apk := filepath.Join(t.TempDir(), "demo.apk")
	if err := os.WriteFile(apk, []byte("not used by fake runner"), 0600); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{result: CommandResult{Stdout: "Performing Streamed Install\nSuccess\n"}}
	adb := NewADB(runner, RuntimePaths{ADBPath: "adb"})
	msg, err := adb.InstallAPK(context.Background(), "device-1", apk, true)
	if err != nil || !strings.Contains(msg, "Success") {
		t.Fatalf("expected install success, msg=%q err=%v", msg, err)
	}
	joined := strings.Join(runner.calls[0], " ")
	if !strings.Contains(joined, "-s device-1 install -r -d") {
		t.Fatalf("install did not target explicit serial: %v", runner.calls[0])
	}

	runner = &fakeRunner{result: CommandResult{Stdout: "Failure [INSTALL_FAILED_TEST_ONLY]"}}
	adb = NewADB(runner, RuntimePaths{ADBPath: "adb"})
	if _, err := adb.InstallAPK(context.Background(), "device-1", apk, true); err == nil {
		t.Fatal("expected install without Success to fail")
	}
}

func TestRunDeviceRejectsEmptySerial(t *testing.T) {
	adb := NewADB(&fakeRunner{}, RuntimePaths{ADBPath: "adb"})
	if _, err := adb.Launch(context.Background(), "", "com.example.app"); err == nil {
		t.Fatal("expected empty serial to fail")
	}
}

func TestHumanizeADBErrorWireless(t *testing.T) {
	msg := humanizeADBError("failed to connect to 192.168.1.2:5555", errors.New("exit status 1"))
	if !strings.Contains(msg, "same network") {
		t.Fatalf("expected wireless recovery guidance, got %q", msg)
	}
}

func TestValidateWirelessHostPort(t *testing.T) {
	if err := validateHostPort("192.168.1.24:5555"); err != nil {
		t.Fatalf("expected IPv4 host:port to validate: %v", err)
	}
	if err := validateHostPort("192.168.1.24"); err == nil {
		t.Fatal("expected missing port to fail")
	}
	if err := validateHostPort("192.168.1.24:not-a-port"); err == nil {
		t.Fatal("expected non-numeric port to fail")
	}
}

func TestValidatePackageName(t *testing.T) {
	if err := validatePackageName("com.example.app"); err != nil {
		t.Fatalf("valid package rejected: %v", err)
	}
	if err := validatePackageName("not a package; rm -rf"); err == nil {
		t.Fatal("invalid package should be rejected")
	}
}

func TestCappedBufferKeepsTail(t *testing.T) {
	buf := newCappedBuffer(5)
	_, _ = buf.Write([]byte("hello"))
	_, _ = buf.Write([]byte(" world"))
	if buf.String() != "world" {
		t.Fatalf("expected capped tail, got %q", buf.String())
	}
}

func TestAppStateStoreRoundTrip(t *testing.T) {
	store := AppStateStore{path: filepath.Join(t.TempDir(), "state.json")}
	state := AppState{SchemaVersion: 1, Theme: "dark", OnboardingComplete: true, SelectedSerial: "abc", WirelessEndpoints: []string{"192.168.1.2:5555"}}
	if err := store.Save(state); err != nil {
		t.Fatal(err)
	}
	loaded := store.Load()
	if loaded.Theme != "dark" || !loaded.OnboardingComplete || loaded.SelectedSerial != "abc" || len(loaded.WirelessEndpoints) != 1 {
		t.Fatalf("bad loaded state: %#v", loaded)
	}
}

func TestAppStateStoreCorruptedFileFallsBack(t *testing.T) {
	store := AppStateStore{path: filepath.Join(t.TempDir(), "state.json")}
	if err := os.MkdirAll(filepath.Dir(store.path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.path, []byte("{not-json"), 0600); err != nil {
		t.Fatal(err)
	}
	loaded := store.Load()
	if loaded.SchemaVersion != 1 || loaded.Theme != "system" {
		t.Fatalf("expected default state after corruption, got %#v", loaded)
	}
}

func TestFlowStopsOnFailure(t *testing.T) {
	apk := filepath.Join(t.TempDir(), "demo.apk")
	if err := os.WriteFile(apk, []byte("not used by fake runner"), 0600); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{result: CommandResult{Stdout: "Failure [INSTALL_FAILED]"}}
	app := &App{adb: NewADB(runner, RuntimePaths{ADBPath: "adb"}), scrcpy: NewScrcpyManager(runner, RuntimePaths{}), stateStore: &AppStateStore{path: filepath.Join(t.TempDir(), "state.json")}, state: AppState{SchemaVersion: 1}}
	var events []FlowEvent
	err := app.RunFlow(context.Background(), "device-1", FlowProfile{Name: "fail", Actions: []FlowAction{{Type: FlowInstallAPK, APKPath: apk}, {Type: FlowLaunchApp, PackageName: "com.example.app"}}}, func(e FlowEvent) {
		events = append(events, e)
	})
	if err == nil {
		t.Fatal("expected flow to fail")
	}
	if len(events) != 2 || events[1].State != "failed" {
		t.Fatalf("expected running then failed events, got %#v", events)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("flow should stop after failed install, calls=%#v", runner.calls)
	}
}

func TestReconcileSelectedDevice(t *testing.T) {
	devices := []Device{{Serial: "one"}, {Serial: "two"}}
	if got := ReconcileSelectedDevice("two", "one", devices); got != "two" {
		t.Fatalf("expected current selection, got %q", got)
	}
	if got := ReconcileSelectedDevice("missing", "one", devices); got != "one" {
		t.Fatalf("expected persisted selection, got %q", got)
	}
	if got := ReconcileSelectedDevice("", "", devices); got != "" {
		t.Fatalf("expected no arbitrary selection with multiple devices, got %q", got)
	}
	if got := ReconcileSelectedDevice("", "", []Device{{Serial: "only"}}); got != "only" {
		t.Fatalf("expected single visible device selection, got %q", got)
	}
}

func TestLogLevelCode(t *testing.T) {
	if LogLevelCode("Error") != "E" || LogLevelCode("All levels") != "" {
		t.Fatal("unexpected log level mapping")
	}
}

func TestAddRecentPathDeduplicates(t *testing.T) {
	got := addRecentPath([]string{"a.apk", "b.apk"}, "b.apk", 3)
	if len(got) != 2 || got[0] != "b.apk" || got[1] != "a.apk" {
		t.Fatalf("unexpected recent paths: %#v", got)
	}
}

func TestFormatBytes(t *testing.T) {
	if FormatBytes(1536) != "1.5 KB" {
		t.Fatalf("unexpected byte formatting")
	}
}
