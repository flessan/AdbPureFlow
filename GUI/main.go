package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type desktopUI struct {
	fyneApp fyne.App
	win     fyne.Window
	core    *App

	deviceRefreshMu sync.Mutex
	devices         []Device
	selectedSerial  string
	deviceSelect    *widget.Select
	status          *widget.Label
	activity        *widget.ProgressBarInfinite
	contentTitle    *widget.Label
	content         *fyne.Container

	monitorCancel  context.CancelFunc
	logCancel      context.CancelFunc
	logMu          sync.Mutex
	logEntries     []LogEntry
	logList        *widget.List
	logFilter      *widget.Entry
	logLevel       *widget.Select
	logPaused      bool
	lastLogRefresh time.Time
}

func main() {
	fyneApp := app.NewWithID("com.flessan.adbpureflow")
	core := NewApp()
	applyTheme(fyneApp, core.State().Theme)
	win := fyneApp.NewWindow("AdbPureFlow " + version)
	win.Resize(fyne.NewSize(1120, 760))

	ui := &desktopUI{fyneApp: fyneApp, win: win, core: core}
	win.SetContent(ui.buildShell())
	ui.setupShortcuts()
	win.SetOnClosed(func() {
		ui.stopDeviceMonitor()
		ui.stopLogs()
		ui.core.scrcpy.StopAll()
		_ = ui.core.SaveState(func(state *AppState) {
			state.SelectedSerial = ui.selectedSerial
		})
	})
	win.SetOnDropped(func(_ fyne.Position, uris []fyne.URI) {
		for _, uri := range uris {
			if strings.EqualFold(filepath.Ext(uri.Path()), ".apk") {
				ui.inspectAPK(uri.Path())
				return
			}
		}
		if len(uris) > 0 {
			ui.showInfo("Unsupported drop", "Drop an .apk file to inspect and install it. Other file types are ignored.")
		}
	})
	go ui.refreshDevicesStartup()
	ui.startDeviceMonitor()
	ui.showOnboardingIfNeeded()
	win.CenterOnScreen()
	win.ShowAndRun()
}

func (ui *desktopUI) buildShell() fyne.CanvasObject {
	ui.status = widget.NewLabel("Ready")
	ui.activity = widget.NewProgressBarInfinite()
	ui.activity.Hide()
	ui.contentTitle = widget.NewLabelWithStyle("Devices", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	ui.content = container.NewMax()
	ui.deviceSelect = widget.NewSelect([]string{"No devices"}, func(label string) {
		for _, d := range ui.devices {
			if label == d.Label() {
				ui.selectedSerial = d.Serial
				_ = ui.core.SaveState(func(state *AppState) { state.SelectedSerial = d.Serial })
				ui.setStatus("Selected "+d.Label(), false)
				ui.showDevices()
				return
			}
		}
	})
	ui.deviceSelect.PlaceHolder = "Select device"

	top := container.NewBorder(nil, nil,
		widget.NewLabelWithStyle(appName, fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		container.NewHBox(
			widget.NewButtonWithIcon("Add Wireless", theme.ContentAddIcon(), ui.showWirelessDialog),
			widget.NewButtonWithIcon("Refresh", theme.ViewRefreshIcon(), func() { go ui.refreshDevices() }),
		),
		container.NewBorder(nil, nil, widget.NewLabel("Device:"), nil, ui.deviceSelect),
	)

	nav := container.NewVBox(
		widget.NewButtonWithIcon("Devices", theme.ComputerIcon(), ui.showDevices),
		widget.NewButtonWithIcon("Screen", theme.MediaPlayIcon(), ui.showScreen),
		widget.NewButtonWithIcon("Apps", theme.FileIcon(), ui.showApps),
		widget.NewButtonWithIcon("Logs", theme.FileIcon(), ui.showLogs),
		widget.NewButtonWithIcon("Deploy", theme.FileIcon(), ui.showDeploy),
		widget.NewButtonWithIcon("Workflows", theme.SettingsIcon(), ui.showWorkflows),
		widget.NewSeparator(),
		widget.NewButtonWithIcon("Settings", theme.SettingsIcon(), ui.showSettings),
		widget.NewButtonWithIcon("Diagnostics", theme.WarningIcon(), ui.showDiagnostics),
		widget.NewButtonWithIcon("Help", theme.HelpIcon(), ui.showHelp),
	)
	center := container.NewBorder(container.NewPadded(ui.contentTitle), nil, nil, nil, ui.content)
	bottom := container.NewBorder(nil, nil, ui.status, nil, ui.activity)
	shell := container.NewBorder(top, bottom, nav, nil, center)
	ui.showDevices()
	return shell
}

func (ui *desktopUI) setupShortcuts() {
	ui.win.Canvas().AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyK, Modifier: fyne.KeyModifierControl}, func(fyne.Shortcut) {
		if _, editing := ui.win.Canvas().Focused().(*widget.Entry); editing {
			return
		}
		ui.showCommandPalette()
	})
	ui.win.Canvas().AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyR, Modifier: fyne.KeyModifierControl}, func(fyne.Shortcut) {
		if _, editing := ui.win.Canvas().Focused().(*widget.Entry); editing {
			return
		}
		go ui.refreshDevices()
	})
}

func (ui *desktopUI) setStatus(text string, busy bool) {
	fyne.Do(func() {
		ui.status.SetText(text)
		if busy {
			ui.activity.Show()
		} else {
			ui.activity.Hide()
		}
	})
}

func (ui *desktopUI) showError(err error) {
	if err == nil {
		return
	}
	fyne.Do(func() { dialog.ShowError(err, ui.win) })
}

func (ui *desktopUI) showInfo(title, message string) {
	fyne.Do(func() { dialog.ShowInformation(title, message, ui.win) })
}

func (ui *desktopUI) notify(title, message string) {
	ui.fyneApp.SendNotification(&fyne.Notification{Title: title, Content: message})
}

func (ui *desktopUI) replaceContent(title string, obj fyne.CanvasObject) {
	fyne.Do(func() {
		ui.contentTitle.SetText(title)
		ui.content.Objects = []fyne.CanvasObject{obj}
		ui.content.Refresh()
	})
}

func (ui *desktopUI) selectedDevice() (Device, bool) {
	for _, d := range ui.devices {
		if d.Serial == ui.selectedSerial {
			return d, true
		}
	}
	return Device{}, false
}

func (ui *desktopUI) requireDevice() (Device, bool) {
	d, ok := ui.selectedDevice()
	if !ok || d.Status != DeviceOnline {
		ui.showInfo("Choose a device", "Connect and select an authorized Android device first. If this is your first time, open Devices or Add Wireless.")
		return Device{}, false
	}
	return d, true
}

func (ui *desktopUI) applyDevices(devices []Device) {
	fyne.Do(func() {
		ui.devices = devices
		labels := []string{}
		for _, d := range devices {
			labels = append(labels, d.Label())
		}
		if len(labels) == 0 {
			labels = []string{"No devices"}
		}
		ui.deviceSelect.Options = labels
		ui.deviceSelect.Refresh()
		ui.selectedSerial = ReconcileSelectedDevice(ui.selectedSerial, ui.core.State().SelectedSerial, devices)
		for _, d := range devices {
			if d.Serial == ui.selectedSerial {
				ui.deviceSelect.SetSelected(d.Label())
				break
			}
		}
	})
}

func (ui *desktopUI) refreshDevicesStartup() {
	if !ui.deviceRefreshMu.TryLock() {
		return
	}
	defer ui.deviceRefreshMu.Unlock()
	ui.setStatus("Checking for connected devices…", true)
	devices, err := ui.core.adb.ProbeDevices(context.Background())
	if err != nil {
		ui.setStatus("ADB not ready. Open Diagnostics or press Refresh when ready.", false)
		return
	}
	ui.applyDevices(devices)
	ui.setStatus(fmt.Sprintf("%d device(s) available", len(devices)), false)
	ui.showDevices()
}

func (ui *desktopUI) refreshDevices() {
	if !ui.deviceRefreshMu.TryLock() {
		return
	}
	defer ui.deviceRefreshMu.Unlock()
	ui.setStatus("Scanning devices…", true)
	devices, err := ui.core.adb.Devices(context.Background())
	if err != nil {
		ui.setStatus("Device scan failed", false)
		ui.showError(err)
		return
	}
	ui.applyDevices(devices)
	ui.setStatus(fmt.Sprintf("%d device(s) available", len(devices)), false)
	ui.showDevices()
}

func (ui *desktopUI) startDeviceMonitor() {
	ctx, cancel := context.WithCancel(context.Background())
	ui.monitorCancel = cancel
	go func() {
		ticker := time.NewTicker(7 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				ui.refreshDevicesPassive(ctx)
			}
		}
	}()
}

func (ui *desktopUI) stopDeviceMonitor() {
	if ui.monitorCancel != nil {
		ui.monitorCancel()
		ui.monitorCancel = nil
	}
}

func (ui *desktopUI) refreshDevicesPassive(ctx context.Context) {
	if !ui.deviceRefreshMu.TryLock() {
		return
	}
	defer ui.deviceRefreshMu.Unlock()
	devices, err := ui.core.adb.ProbeDevices(ctx)
	if err != nil {
		ui.setStatus("Device monitor could not refresh: "+err.Error(), false)
		return
	}
	fyne.Do(func() {
		previous := ui.selectedSerial
		ui.devices = devices
		labels := []string{}
		selectedStillOnline := false
		for _, d := range devices {
			labels = append(labels, d.Label())
			if d.Serial == previous && d.Status == DeviceOnline {
				selectedStillOnline = true
			}
		}
		if len(labels) == 0 {
			labels = []string{"No devices"}
		}
		ui.deviceSelect.Options = labels
		ui.deviceSelect.Refresh()
		ui.selectedSerial = ReconcileSelectedDevice(previous, ui.core.State().SelectedSerial, devices)
		if previous != "" && !selectedStillOnline {
			ui.setStatus("Selected device disconnected or unavailable", false)
		}
	})
}

func (ui *desktopUI) showDevices() {
	rows := container.NewVBox()
	if len(ui.devices) == 0 {
		if ui.selectedSerial != "" {
			rows.Add(widget.NewLabel("Previously selected device is unavailable: " + ui.selectedSerial))
		}
		rows.Add(widget.NewLabel("No devices yet. Connect USB with debugging enabled, or use Add Wireless for Android 11+ Wireless debugging."))
	} else {
		for _, d := range ui.devices {
			dev := d
			status := fmt.Sprintf("%s • %s • Android %s • Battery %s • Storage %s", d.Transport, d.Status, fallback(d.AndroidVersion, "unknown"), fallback(d.BatteryPercent, "unknown"), fallback(d.StorageSummary, "unknown"))
			name := d.Label()
			if alias := ui.core.State().DeviceAliases[d.Serial]; alias != "" {
				name = alias + "  —  " + d.Serial
			}
			selectedMark := ""
			if d.Serial == ui.selectedSerial {
				selectedMark = "Selected • "
			}
			rows.Add(container.NewBorder(nil, nil, nil,
				container.NewHBox(
					widget.NewButton("Select", func() {
						ui.selectedSerial = dev.Serial
						_ = ui.core.SaveState(func(state *AppState) { state.SelectedSerial = dev.Serial })
						ui.deviceSelect.SetSelected(dev.Label())
					}),
					widget.NewButton("Rename", func() { ui.renameDevice(dev.Serial) }),
					widget.NewButton("Disconnect", func() { go ui.disconnectDevice(dev.Serial) }),
				),
				container.NewVBox(widget.NewLabelWithStyle(name, fyne.TextAlignLeading, fyne.TextStyle{Bold: true}), widget.NewLabel(selectedMark+status))))
			rows.Add(widget.NewSeparator())
		}
	}
	if endpoints := ui.core.State().WirelessEndpoints; len(endpoints) > 0 {
		rows.Add(widget.NewRichTextFromMarkdown("### Known wireless endpoints"))
		for _, endpoint := range endpoints {
			addr := endpoint
			connected := false
			for _, d := range ui.devices {
				if d.Serial == addr {
					connected = true
					break
				}
			}
			state := "Previously connected"
			if connected {
				state = "Connected"
			}
			rows.Add(container.NewBorder(nil, nil, widget.NewLabel(addr+" — "+state), container.NewHBox(
				widget.NewButton("Reconnect", func() { go ui.reconnectWireless(addr) }),
				widget.NewButton("Forget", func() { ui.forgetWireless(addr) }),
			), nil))
		}
	}
	intro := widget.NewRichTextFromMarkdown("### Device workstation\nSelect a device once; Screen, Apps, Logs, Deploy, and Workflows use that same target. Unauthorized devices need you to accept Android's debugging prompt on the phone.")
	ui.replaceContent("Devices", container.NewVScroll(container.NewVBox(intro, rows)))
}

func (ui *desktopUI) renameDevice(serial string) {
	entry := widget.NewEntry()
	entry.SetText(ui.core.State().DeviceAliases[serial])
	d := dialog.NewCustomConfirm("Rename Device", "Save", "Cancel", container.NewVBox(widget.NewLabel("Local alias for "+serial), entry), func(ok bool) {
		if !ok {
			return
		}
		alias := strings.TrimSpace(entry.Text)
		_ = ui.core.SaveState(func(state *AppState) {
			if state.DeviceAliases == nil {
				state.DeviceAliases = map[string]string{}
			}
			if alias == "" {
				delete(state.DeviceAliases, serial)
			} else {
				state.DeviceAliases[serial] = alias
			}
		})
		ui.showDevices()
	}, ui.win)
	d.Show()
}

func (ui *desktopUI) reconnectWireless(endpoint string) {
	ui.setStatus("Reconnecting wireless device…", true)
	msg, err := ui.core.adb.ConnectWireless(context.Background(), endpoint)
	if err != nil {
		ui.setStatus("Wireless reconnect failed", false)
		ui.showError(fmt.Errorf("Reconnect failed: %s", fallback(msg, err.Error())))
		return
	}
	ui.setStatus(fallback(msg, "Wireless reconnect requested"), false)
	ui.refreshDevices()
}

func (ui *desktopUI) forgetWireless(endpoint string) {
	_ = ui.core.SaveState(func(state *AppState) {
		filtered := state.WirelessEndpoints[:0]
		for _, e := range state.WirelessEndpoints {
			if e != endpoint {
				filtered = append(filtered, e)
			}
		}
		state.WirelessEndpoints = filtered
	})
	ui.setStatus("Forgot wireless endpoint", false)
	ui.showDevices()
}

func (ui *desktopUI) disconnectDevice(serial string) {
	ui.setStatus("Disconnecting…", true)
	msg, err := ui.core.adb.Disconnect(context.Background(), serial)
	ui.setStatus(fallback(msg, "Disconnect requested"), false)
	if err != nil {
		ui.showError(err)
	}
	ui.refreshDevices()
}

func (ui *desktopUI) showWirelessDialog() {
	pairAddr := widget.NewEntry()
	pairAddr.SetPlaceHolder("Pairing address, e.g. 192.168.1.24:37123")
	code := widget.NewEntry()
	code.SetPlaceHolder("Six-digit pairing code")
	connectAddr := widget.NewEntry()
	connectAddr.SetPlaceHolder("Connect address, e.g. 192.168.1.24:5555")
	if endpoints := ui.core.State().WirelessEndpoints; len(endpoints) > 0 {
		connectAddr.SetText(endpoints[0])
	}
	info := widget.NewRichTextFromMarkdown("Android 11+ Wireless debugging uses two steps: **Pair device with pairing code**, then **Connect** using the device IP/port shown by Android. AdbPureFlow runs the real `adb pair` and `adb connect` commands. QR pairing is intentionally not simulated; use pairing-code mode when Android does not expose a compatible QR payload to desktop tools.")
	content := container.NewVBox(info, widget.NewForm(
		widget.NewFormItem("Pair", pairAddr), widget.NewFormItem("Code", code), widget.NewFormItem("Connect", connectAddr),
	))
	d := dialog.NewCustomConfirm("Add Wireless Device", "Pair + Connect", "Cancel", content, func(ok bool) {
		if !ok {
			return
		}
		go func() {
			ui.setStatus("Pairing wireless device…", true)
			if pairAddr.Text != "" || code.Text != "" {
				msg, err := ui.core.adb.PairWireless(context.Background(), pairAddr.Text, code.Text)
				if err != nil {
					ui.setStatus("Pairing failed", false)
					ui.showError(errors.New(msg))
					return
				}
			}
			msg, err := ui.core.adb.ConnectWireless(context.Background(), connectAddr.Text)
			ui.setStatus(fallback(msg, "Wireless connect requested"), false)
			if err != nil {
				ui.showError(err)
				return
			}
			_ = ui.core.SaveState(func(state *AppState) {
				state.WirelessEndpoints = addUniqueString(state.WirelessEndpoints, connectAddr.Text, 12)
			})
			devices, scanErr := ui.core.adb.ProbeDevices(context.Background())
			verified := false
			for _, d := range devices {
				if d.Serial == strings.TrimSpace(connectAddr.Text) && d.Status == DeviceOnline {
					verified = true
					break
				}
			}
			if scanErr == nil && !verified {
				ui.setStatus("Wireless connect requested; waiting for device to appear", false)
			} else if verified {
				ui.setStatus("Wireless device connected", false)
			}
			ui.refreshDevices()
		}()
	}, ui.win)
	d.Resize(fyne.NewSize(620, 420))
	d.Show()
}

func (ui *desktopUI) showScreen() {
	d, ok := ui.selectedDevice()
	deviceText := "No device selected"
	if ok {
		deviceText = d.Label()
	}
	always := widget.NewCheck("Keep mirror window on top", nil)
	fullscreen := widget.NewCheck("Start fullscreen", nil)
	quality := widget.NewSelect([]string{"Native", "1080p", "720p"}, nil)
	quality.SetSelected("Native")
	bitrate := widget.NewSelect([]string{"Default", "8M", "4M", "2M"}, nil)
	bitrate.SetSelected("Default")
	mirrorState := widget.NewLabel("Mirror is stopped.")

	startWithOptions := func(recordPath string) {
		dev, ok := ui.requireDevice()
		if !ok {
			return
		}
		opts := ScrcpyOptions{AlwaysOnTop: always.Checked, Fullscreen: fullscreen.Checked, RecordPath: recordPath}
		if quality.Selected == "1080p" {
			opts.MaxSize = 1080
		} else if quality.Selected == "720p" {
			opts.MaxSize = 720
		}
		if bitrate.Selected != "Default" {
			opts.VideoBitRate = bitrate.Selected
		}
		go func() {
			ui.setStatus("Starting screen mirror…", true)
			fyne.Do(func() { mirrorState.SetText("Starting scrcpy companion window…") })
			mp, err := ui.core.scrcpy.StartMirror(context.Background(), dev.Serial, opts)
			if err != nil {
				ui.setStatus("Mirror failed", false)
				fyne.Do(func() { mirrorState.SetText("Mirror failed to start.") })
				ui.showError(err)
				return
			}
			if recordPath != "" {
				ui.setStatus("Mirror and recording active", false)
				fyne.Do(func() {
					mirrorState.SetText("Mirror and recording are active. Stop Mirror finishes the recording file.")
				})
			} else {
				ui.setStatus("Mirror active in companion window", false)
				fyne.Do(func() { mirrorState.SetText("Mirror active in scrcpy companion window.") })
			}
			go func() {
				<-mp.Done()
				if err := mp.Err(); err != nil && !strings.Contains(strings.ToLower(err.Error()), "killed") {
					ui.setStatus("Mirror exited unexpectedly", false)
					fyne.Do(func() { mirrorState.SetText("Mirror exited unexpectedly: " + err.Error()) })
					return
				}
				if recordPath != "" {
					if info, statErr := os.Stat(recordPath); statErr == nil && info.Size() > 0 {
						ui.setStatus("Recording saved", false)
						fyne.Do(func() { mirrorState.SetText("Recording saved: " + recordPath) })
					} else {
						ui.setStatus("Recording stopped; output not found", false)
						fyne.Do(func() { mirrorState.SetText("Recording stopped, but AdbPureFlow could not verify the output file.") })
					}
				} else {
					ui.setStatus("Mirror stopped", false)
					fyne.Do(func() { mirrorState.SetText("Mirror is stopped.") })
				}
			}()
		}()
	}

	start := widget.NewButtonWithIcon("Start Mirror", theme.MediaPlayIcon(), func() {
		startWithOptions("")
	})
	record := widget.NewButton("Start Mirror + Record…", func() {
		save := dialog.NewFileSave(func(w fyne.URIWriteCloser, err error) {
			if err != nil || w == nil {
				return
			}
			path := w.URI().Path()
			_ = w.Close()
			startWithOptions(path)
		}, ui.win)
		save.SetFileName("adbpureflow-recording.mp4")
		save.SetFilter(storage.NewExtensionFileFilter([]string{".mp4", ".mkv"}))
		save.Show()
	})
	stop := widget.NewButton("Stop Mirror", func() {
		if d, ok := ui.selectedDevice(); ok {
			ui.core.scrcpy.StopMirror(d.Serial)
			ui.setStatus("Mirror stopped", false)
		}
	})
	screenshot := widget.NewButtonWithIcon("Capture Screenshot…", theme.DocumentSaveIcon(), ui.saveScreenshot)
	body := container.NewVBox(
		widget.NewRichTextFromMarkdown("### Screen\nAdbPureFlow manages scrcpy as an intentional Windows companion window, not an embedded web view. That keeps mouse, keyboard, fullscreen, rotation, and recording behavior aligned with scrcpy itself."),
		widget.NewLabel("Target: "+deviceText),
		mirrorState,
		widget.NewForm(widget.NewFormItem("Size", quality), widget.NewFormItem("Bit rate", bitrate)),
		always,
		fullscreen,
		container.NewHBox(start, record, stop, screenshot),
	)
	ui.replaceContent("Screen", body)
}

func (ui *desktopUI) saveScreenshot() {
	dev, ok := ui.requireDevice()
	if !ok {
		return
	}
	save := dialog.NewFileSave(func(w fyne.URIWriteCloser, err error) {
		if err != nil || w == nil {
			return
		}
		path := w.URI().Path()
		_ = w.Close()
		go func() {
			ui.setStatus("Capturing screenshot…", true)
			err := ui.core.adb.CaptureScreenshot(context.Background(), dev.Serial, path)
			if err != nil {
				ui.setStatus("Screenshot failed", false)
				ui.showError(err)
				return
			}
			ui.setStatus("Screenshot saved", false)
			ui.notify("Screenshot saved", filepath.Base(path))
			fyne.Do(func() {
				dialog.NewConfirm("Screenshot saved", "Saved to "+path+"\n\nOpen its folder in the system file manager?", func(open bool) {
					if open {
						if err := OpenInFileManager(path); err != nil {
							ui.showError(err)
						}
					}
				}, ui.win).Show()
			})
		}()
	}, ui.win)
	save.SetFileName("adbpureflow-screenshot.png")
	save.SetFilter(storage.NewExtensionFileFilter([]string{".png"}))
	save.Show()
}

func (ui *desktopUI) chooseAPK(onChoose func(string)) {
	open := dialog.NewFileOpen(func(r fyne.URIReadCloser, err error) {
		if err != nil {
			ui.showError(err)
			return
		}
		if r == nil {
			return
		}
		path := r.URI().Path()
		_ = r.Close()
		if !strings.EqualFold(filepath.Ext(path), ".apk") {
			ui.showInfo("Choose an APK", "Select a file ending in .apk.")
			return
		}
		onChoose(path)
	}, ui.win)
	open.SetFilter(storage.NewExtensionFileFilter([]string{".apk"}))
	open.Show()
}

func (ui *desktopUI) inspectAPK(path string) {
	ui.setStatus("Inspecting APK…", true)
	go func() {
		meta, err := InspectAPK(context.Background(), path)
		ui.setStatus("APK inspected", false)
		if err != nil {
			ui.showError(err)
			return
		}
		_ = ui.core.SaveState(func(state *AppState) {
			state.RecentAPKs = addRecentPath(state.RecentAPKs, meta.Path, 12)
		})
		ui.showAPKInstall(meta)
	}()
}

func (ui *desktopUI) showAPKInstall(meta APKMetadata) {
	lines := []string{fmt.Sprintf("**File:** %s", meta.FileName), fmt.Sprintf("**Size:** %s", FormatBytes(meta.SizeBytes)), fmt.Sprintf("**SHA-256:** `%s`", meta.SHA256)}
	if meta.PackageID != "" {
		lines = append(lines, fmt.Sprintf("**Package:** `%s`", meta.PackageID))
	}
	if meta.VersionName != "" || meta.VersionCode != "" {
		lines = append(lines, fmt.Sprintf("**Version:** %s (%s)", meta.VersionName, meta.VersionCode))
	}
	if meta.MinSDK != "" || meta.TargetSDK != "" {
		lines = append(lines, fmt.Sprintf("**SDK:** min %s, target %s", meta.MinSDK, meta.TargetSDK))
	}
	if len(meta.Permissions) > 0 {
		lines = append(lines, "**Permissions:** "+strings.Join(meta.Permissions, ", "))
	}
	for _, w := range meta.Warnings {
		lines = append(lines, "⚠ "+w)
	}
	clearData := widget.NewCheck("Clear existing app data before launch", nil)
	launch := widget.NewCheck("Launch after install", nil)
	launch.SetChecked(true)
	install := widget.NewButtonWithIcon("Install to selected device", theme.ConfirmIcon(), func() {
		dev, ok := ui.requireDevice()
		if !ok {
			return
		}
		go func() {
			ui.setStatus("Installing APK…", true)
			msg, err := ui.core.adb.InstallAPK(context.Background(), dev.Serial, meta.Path, true)
			if err != nil {
				ui.setStatus("Install failed", false)
				ui.showError(errors.New(msg))
				return
			}
			if clearData.Checked && meta.PackageID != "" {
				if clearMsg, clearErr := ui.core.adb.ClearData(context.Background(), dev.Serial, meta.PackageID); clearErr != nil {
					ui.setStatus("Install completed; clear data failed", false)
					ui.showError(fmt.Errorf("APK installed, but Android did not clear app data: %s", fallback(clearMsg, clearErr.Error())))
					return
				}
			}
			if launch.Checked && meta.PackageID != "" {
				if launchMsg, launchErr := ui.core.adb.Launch(context.Background(), dev.Serial, meta.PackageID); launchErr != nil {
					ui.setStatus("Install completed; launch failed", false)
					ui.showError(fmt.Errorf("APK installed, but launch failed: %s", fallback(launchMsg, launchErr.Error())))
					return
				}
			}
			ui.setStatus("Install complete", false)
			ui.notify("Install complete", meta.FileName)
			ui.showInfo("Install complete", fallback(msg, "APK installed successfully."))
		}()
	})
	ui.replaceContent("Install APK", container.NewVScroll(container.NewVBox(widget.NewRichTextFromMarkdown("### APK workflow\n"+strings.Join(lines, "\n\n")), clearData, launch, install)))
}

func (ui *desktopUI) showApps() {
	dev, ok := ui.requireDevice()
	if !ok {
		return
	}
	filter := widget.NewEntry()
	filter.SetPlaceHolder("Search packages…")
	showSystem := widget.NewCheck("Include system apps", nil)
	listBox := container.NewVBox(widget.NewLabel("Loading apps…"))
	render := func(pkgs []PackageInfo) {
		q := strings.ToLower(filter.Text)
		listBox.Objects = nil
		for _, p := range pkgs {
			if q != "" && !strings.Contains(strings.ToLower(p.PackageName), q) {
				continue
			}
			pkg := p.PackageName
			kind := "User app"
			if !p.UserApp {
				kind = "System app"
			}
			listBox.Add(container.NewBorder(nil, nil, nil, container.NewHBox(
				widget.NewButton("Launch", func() { go ui.packageAction("Launch", pkg, ui.core.adb.Launch) }),
				widget.NewButton("Stop", func() { go ui.packageAction("Force stop", pkg, ui.core.adb.ForceStop) }),
				widget.NewButton("Clear", func() {
					ui.confirmPackageAction("Clear data", pkg, "This removes local app data on the selected Android device.", ui.core.adb.ClearData)
				}),
				widget.NewButton("Uninstall", func() {
					ui.confirmPackageAction("Uninstall", pkg, "This removes the app from the selected Android device.", ui.core.adb.Uninstall)
				}),
				widget.NewButton("Copy", func() { ui.win.Clipboard().SetContent(pkg); ui.setStatus("Copied package name", false) }),
			), container.NewVBox(widget.NewLabelWithStyle(pkg, fyne.TextAlignLeading, fyne.TextStyle{Monospace: true}), widget.NewLabel(kind+" • "+fallback(p.APKPath, "installed package")))))
			listBox.Add(widget.NewSeparator())
		}
		listBox.Refresh()
	}
	filter.OnChanged = func(string) {}
	load := func(includeSystem bool) {
		listBox.Objects = []fyne.CanvasObject{widget.NewLabel("Loading apps…")}
		listBox.Refresh()
		go func() {
			pkgs, err := ui.core.adb.Packages(context.Background(), dev.Serial, !includeSystem)
			if err != nil {
				ui.showError(err)
				return
			}
			fyne.Do(func() { render(pkgs); filter.OnChanged = func(string) { render(pkgs) } })
		}()
	}
	showSystem.OnChanged = func(include bool) { load(include) }
	ui.replaceContent("Apps", container.NewBorder(container.NewVBox(filter, showSystem), nil, nil, nil, container.NewVScroll(listBox)))
	load(false)
}

type packageActionFunc func(context.Context, string, string) (string, error)

func (ui *desktopUI) confirmPackageAction(name, pkg, warning string, fn packageActionFunc) {
	dialog.NewConfirm(name+"?", warning+"\n\nPackage: "+pkg, func(ok bool) {
		if ok {
			go ui.packageAction(name, pkg, fn)
		}
	}, ui.win).Show()
}

func (ui *desktopUI) packageAction(name, pkg string, fn packageActionFunc) {
	dev, ok := ui.requireDevice()
	if !ok {
		return
	}
	ui.setStatus(name+"…", true)
	msg, err := fn(context.Background(), dev.Serial, pkg)
	if err != nil {
		ui.setStatus(name+" failed", false)
		if msg != "" {
			ui.showError(fmt.Errorf("%s failed: %s", name, msg))
		} else {
			ui.showError(err)
		}
		return
	}
	ui.setStatus(name+" complete", false)
	if strings.TrimSpace(msg) != "" {
		ui.showInfo(name, msg)
	}
	if name == "Uninstall" {
		fyne.Do(ui.showApps)
	}
}

func (ui *desktopUI) showLogs() {
	filter := widget.NewEntry()
	filter.SetPlaceHolder("Filter by text, tag, package, or message…")
	level := widget.NewSelect([]string{"All levels", "Verbose", "Debug", "Info", "Warn", "Error", "Fatal"}, func(string) {
		if ui.logList != nil {
			ui.logList.Refresh()
		}
	})
	level.SetSelected("All levels")
	ui.logFilter = filter
	ui.logLevel = level
	ui.logList = widget.NewList(func() int { return len(ui.filteredLogs()) }, func() fyne.CanvasObject { return widget.NewLabel("template") }, func(id widget.ListItemID, obj fyne.CanvasObject) {
		entries := ui.filteredLogs()
		if id < len(entries) {
			obj.(*widget.Label).SetText(formatLog(entries[id]))
		}
	})
	start := widget.NewButton("Start", func() { ui.startLogs() })
	stop := widget.NewButton("Stop", func() { ui.stopLogs(); ui.setStatus("Log stream stopped", false) })
	pause := widget.NewButton("Pause/Resume", func() {
		ui.logPaused = !ui.logPaused
		ui.setStatus(map[bool]string{true: "Logs paused", false: "Logs resumed"}[ui.logPaused], false)
	})
	clear := widget.NewButton("Clear Device Log", func() {
		if d, ok := ui.requireDevice(); ok {
			go ui.core.adb.ClearLogcat(context.Background(), d.Serial)
		}
		ui.logMu.Lock()
		ui.logEntries = nil
		ui.logMu.Unlock()
		ui.logList.Refresh()
	})
	filter.OnChanged = func(string) { ui.logList.Refresh() }
	ui.replaceContent("Logs", container.NewBorder(container.NewVBox(widget.NewRichTextFromMarkdown("Live logcat uses readable rows while preserving the raw technical message. The visible buffer keeps the latest 2,000 entries to stay responsive."), filter, level, container.NewHBox(start, stop, pause, clear)), nil, nil, nil, ui.logList))
}

func (ui *desktopUI) filteredLogs() []LogEntry {
	ui.logMu.Lock()
	defer ui.logMu.Unlock()
	q := ""
	if ui.logFilter != nil {
		q = strings.ToLower(ui.logFilter.Text)
	}
	level := ""
	if ui.logLevel != nil {
		level = LogLevelCode(ui.logLevel.Selected)
	}
	var out []LogEntry
	for _, e := range ui.logEntries {
		if level != "" && e.Level != level {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(formatLog(e)), q) {
			continue
		}
		out = append(out, e)
	}
	return out
}

func formatLog(e LogEntry) string {
	if e.Level != "" {
		return fmt.Sprintf("%s  %-1s  %-18s  %s", e.Time, e.Level, e.Tag, e.Message)
	}
	return e.Raw
}

func (ui *desktopUI) startLogs() {
	dev, ok := ui.requireDevice()
	if !ok {
		return
	}
	ui.stopLogs()
	ctx, cancel := context.WithCancel(context.Background())
	ui.logCancel = cancel
	ui.setStatus("Streaming logcat…", true)
	go func() {
		_, err := ui.core.adb.StartLogcat(ctx, dev.Serial, func(e LogEntry) {
			if ui.logPaused {
				return
			}
			ui.logMu.Lock()
			ui.logEntries = append(ui.logEntries, e)
			if len(ui.logEntries) > 2000 {
				ui.logEntries = ui.logEntries[len(ui.logEntries)-2000:]
			}
			shouldRefresh := time.Since(ui.lastLogRefresh) > 150*time.Millisecond
			if shouldRefresh {
				ui.lastLogRefresh = time.Now()
			}
			ui.logMu.Unlock()
			if shouldRefresh {
				fyne.Do(func() {
					if ui.logList != nil {
						ui.logList.Refresh()
						entries := ui.filteredLogs()
						if len(entries) > 0 {
							ui.logList.ScrollTo(widget.ListItemID(len(entries) - 1))
						}
					}
				})
			}
		})
		if ctx.Err() == nil && err != nil {
			ui.showError(err)
		}
		ui.setStatus("Log stream stopped", false)
	}()
}

func (ui *desktopUI) stopLogs() {
	if ui.logCancel != nil {
		ui.logCancel()
		ui.logCancel = nil
	}
}

func (ui *desktopUI) showDeploy() {
	var deployCancel context.CancelFunc
	apkPath := widget.NewEntry()
	apkPath.SetPlaceHolder("Choose an APK artifact…")
	pkg := widget.NewEntry()
	pkg.SetPlaceHolder("Package to launch/filter, e.g. com.example.app")
	clear := widget.NewCheck("Clear data before launch", nil)
	mirror := widget.NewCheck("Start screen mirror", nil)
	logs := widget.NewCheck("Open filtered logs after deploy", nil)
	choose := widget.NewButton("Browse…", func() {
		ui.chooseAPK(func(path string) {
			apkPath.SetText(path)
			go func() {
				meta, inspectErr := InspectAPK(context.Background(), path)
				if inspectErr == nil && meta.PackageID != "" {
					fyne.Do(func() { pkg.SetText(meta.PackageID) })
				}
			}()
		})
	})
	run := widget.NewButtonWithIcon("Deploy", theme.FileIcon(), func() {
		dev, ok := ui.requireDevice()
		if !ok {
			return
		}
		if strings.TrimSpace(apkPath.Text) == "" {
			ui.showInfo("Choose an APK", "Select an APK artifact before running Deploy.")
			return
		}
		if deployCancel != nil {
			deployCancel()
		}
		ctx, cancel := context.WithCancel(context.Background())
		deployCancel = cancel
		profile := FlowProfile{Name: "Deploy APK", Actions: []FlowAction{{Type: FlowInstallAPK, APKPath: apkPath.Text}}}
		if clear.Checked {
			profile.Actions = append(profile.Actions, FlowAction{Type: FlowClearData, PackageName: pkg.Text})
		}
		if pkg.Text != "" {
			profile.Actions = append(profile.Actions, FlowAction{Type: FlowLaunchApp, PackageName: pkg.Text})
		}
		if mirror.Checked {
			profile.Actions = append(profile.Actions, FlowAction{Type: FlowStartMirror})
		}
		go func() {
			ui.setStatus("Deploying…", true)
			err := ui.core.RunFlow(ctx, dev.Serial, profile, func(e FlowEvent) {
				ui.setStatus(string(e.Action.Type)+" "+e.State, true)
			})
			deployCancel = nil
			if err != nil {
				ui.setStatus("Deploy failed", false)
				ui.showError(err)
				return
			}
			ui.setStatus("Deploy complete", false)
			ui.notify("Deploy complete", filepath.Base(apkPath.Text))
			_ = ui.core.SaveState(func(state *AppState) {
				state.RecentAPKs = addRecentPath(state.RecentAPKs, apkPath.Text, 12)
			})
			if logs.Checked {
				fyne.Do(ui.showLogs)
				ui.startLogs()
			}
		}()
	})
	cancelButton := widget.NewButton("Cancel Deploy", func() {
		if deployCancel != nil {
			deployCancel()
			ui.setStatus("Deploy cancellation requested", false)
		}
	})
	ui.replaceContent("Deploy", container.NewVBox(
		widget.NewRichTextFromMarkdown("### Repeatable deploy\nChoose an APK artifact, choose behavior, then run a deterministic install → optional clear → launch → mirror/log flow. Failures stop the sequence instead of being reported as success."),
		container.NewBorder(nil, nil, nil, choose, apkPath),
		pkg,
		clear,
		mirror,
		logs,
		container.NewHBox(run, cancelButton),
	))
}

func (ui *desktopUI) showWorkflows() {
	ui.replaceContent("Workflows", widget.NewRichTextFromMarkdown("### Flow automation\nThe workflow engine is implemented underneath Deploy and currently executes Install APK, Clear Data, Launch App, Start Mirror, and Wait with explicit running/complete/failed states. Live logs remain a UI-owned streaming feature, so Deploy opens Logs after a successful flow instead of pretending Start Logs is a completed core action. A persistent visual editor is not exposed yet because the available actions should remain honest and reliable rather than decorative."))
}

func (ui *desktopUI) showDiagnostics() {
	box := container.NewVBox(widget.NewLabel("Running diagnostics…"), widget.NewButton("Restart ADB Server", func() {
		go func() {
			ui.setStatus("Restarting ADB server…", true)
			err := ui.core.adb.RestartServer(context.Background())
			if err != nil {
				ui.setStatus("ADB restart failed", false)
				ui.showError(err)
				return
			}
			ui.setStatus("ADB server restarted", false)
			ui.showDiagnostics()
		}()
	}))
	ui.replaceContent("Diagnostics", container.NewVScroll(box))
	go func() {
		checks := ui.core.Doctor(context.Background())
		fyne.Do(func() {
			box.Objects = nil
			for _, c := range checks {
				box.Add(widget.NewRichTextFromMarkdown(fmt.Sprintf("**%s:** %s\n\n%s\n\n%s", c.Name, c.Status, c.Message, c.Recovery)))
				if c.Technical != "" {
					box.Add(widget.NewAccordion(widget.NewAccordionItem("Technical details", widget.NewLabel(c.Technical))))
				}
				box.Add(widget.NewSeparator())
			}
			box.Refresh()
		})
	}()
}

func applyTheme(fyneApp fyne.App, selected string) {
	switch selected {
	case "light":
		fyneApp.Settings().SetTheme(theme.LightTheme())
	case "dark":
		fyneApp.Settings().SetTheme(theme.DarkTheme())
	default:
		fyneApp.Settings().SetTheme(theme.DefaultTheme())
	}
}

func (ui *desktopUI) showSettings() {
	themeSelect := widget.NewSelect([]string{"system", "light", "dark"}, func(value string) {
		applyTheme(ui.fyneApp, value)
		_ = ui.core.SaveState(func(state *AppState) { state.Theme = value })
	})
	themeSelect.SetSelected(fallback(ui.core.State().Theme, "system"))
	statePath := ui.core.stateStore.path
	body := container.NewVBox(
		widget.NewRichTextFromMarkdown("### Settings\nAdbPureFlow stores non-sensitive preferences in the current user's application data. External tools are kept in the current user's cache/tools location so the app does not need administrator rights."),
		widget.NewForm(widget.NewFormItem("Theme", themeSelect)),
		widget.NewLabel("Preferences: "+statePath),
		widget.NewButton("Open Help & Tutorials", ui.showHelp),
	)
	ui.replaceContent("Settings", body)
}

func (ui *desktopUI) showHelp() {
	text := "### Help & Tutorials\n\n**New here? Baru di sini?** Start with Devices: connect by USB, or Add Wireless for Android 11+ pairing-code setup. Then try Screen, Install APK, Apps, Logs, and Deploy.\n\n**Keyboard:** Ctrl+K opens the command palette. Ctrl+R refreshes devices.\n\n**Troubleshooting:** Diagnostics checks ADB, connected devices, authorization, and scrcpy. Raw details are kept behind expandable technical sections. Help can be reopened from Settings → Open Help & Tutorials.\n\n**Windows-first choices:** APK selection, screenshots, and build artifacts use native file dialogs. File management intentionally stays in Windows Explorer where possible."
	ui.replaceContent("Help & Tutorials", widget.NewRichTextFromMarkdown(text))
}

func (ui *desktopUI) copySelectedSerial() {
	dev, ok := ui.selectedDevice()
	if !ok {
		ui.showInfo("No device selected", "Select a device before copying its serial.")
		return
	}
	ui.win.Clipboard().SetContent(dev.Serial)
	ui.setStatus("Copied device serial to clipboard", false)
}

func (ui *desktopUI) showCommandPalette() {
	type command struct {
		name    string
		enabled bool
		fn      func()
	}
	_, hasDevice := ui.selectedDevice()
	commands := []command{
		{"Connect Device / Add Wireless", true, ui.showWirelessDialog},
		{"Refresh devices", true, func() { go ui.refreshDevices() }},
		{"Install APK", true, func() { ui.chooseAPK(ui.inspectAPK) }},
		{"Open Apps", hasDevice, ui.showApps},
		{"Open Logs", hasDevice, ui.showLogs},
		{"Start Mirror", hasDevice, ui.showScreen},
		{"Stop Mirror", hasDevice, func() {
			if d, ok := ui.selectedDevice(); ok {
				ui.core.scrcpy.StopMirror(d.Serial)
				ui.setStatus("Mirror stopped", false)
			}
		}},
		{"Take Screenshot", hasDevice, ui.saveScreenshot},
		{"Open Screen recording controls", hasDevice, ui.showScreen},
		{"Copy selected device serial", hasDevice, ui.copySelectedSerial},
		{"Deploy", hasDevice, ui.showDeploy},
		{"Open Diagnostics", true, ui.showDiagnostics},
		{"Open Settings", true, ui.showSettings},
		{"Open Help", true, ui.showHelp},
	}
	box := container.NewVBox(widget.NewLabel("Actions for the current device context"))
	for _, c := range commands {
		cmd := c
		button := widget.NewButton(cmd.name, func() { cmd.fn() })
		if !cmd.enabled {
			button.Disable()
		}
		box.Add(button)
	}
	d := dialog.NewCustom("Command Palette", "Close", box, ui.win)
	d.Resize(fyne.NewSize(400, 520))
	d.Show()
}

func (ui *desktopUI) showOnboardingIfNeeded() {
	if ui.core.State().OnboardingComplete || ui.fyneApp.Preferences().BoolWithFallback("onboarding.dismissed", false) {
		return
	}
	intro := widget.NewRichTextFromMarkdown("### Baru di sini? / New here?\nAdbPureFlow is a native Windows-first Android workstation. You can connect a phone, mirror the screen, install APKs, manage apps, read logs, and run deploy flows without living in an ADB terminal. If you already know your way around Android debugging, skip this—Help & Tutorials is always available later.")
	d := dialog.NewCustomConfirm("Welcome to AdbPureFlow", "Show me around", "Skip", intro, func(show bool) {
		ui.fyneApp.Preferences().SetBool("onboarding.dismissed", true)
		_ = ui.core.SaveState(func(state *AppState) { state.OnboardingComplete = true })
		if show {
			ui.showHelp()
		} else {
			ui.setStatus("Wah, anda sudah pro. Kalau suatu saat buntu, tutorial bisa dibuka lewat Settings → Help & Tutorials.", false)
		}
	}, ui.win)
	d.Resize(fyne.NewSize(560, 320))
	d.Show()
}

func fallback(v, alt string) string {
	if strings.TrimSpace(v) == "" {
		return alt
	}
	return v
}
