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
	contentScroll   *container.Scroll
	navButtons      map[string]*widget.Button
	sidebarPane     fyne.CanvasObject
	sidebarSlot     *fyne.Container
	sidebarToggle   *widget.Button
	fullscreenButton *widget.Button
	windowSizeSelect *widget.Select
	shell           *fyne.Container
	windowedSize    fyne.Size

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
	win.Resize(fyne.NewSize(1280, 820))

	ui := &desktopUI{fyneApp: fyneApp, win: win, core: core, windowedSize: fyne.NewSize(1280, 820)}
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
	ui.contentScroll = container.NewVScroll(widget.NewLabel("Select a workspace"))
	ui.contentScroll.Direction = container.ScrollVerticalOnly

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

	brand := container.NewVBox(
		widget.NewLabelWithStyle(appName, fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewLabel("Android device workstation"),
	)
	deviceTarget := container.NewBorder(nil, nil, widget.NewLabel("DEVICE"), nil, ui.deviceSelect)
	actions := container.NewHBox(
		widget.NewButtonWithIcon("Add Wireless", theme.ContentAddIcon(), ui.showWirelessDialog),
		widget.NewButtonWithIcon("Refresh", theme.ViewRefreshIcon(), func() { go ui.refreshDevices() }),
	)

	ui.sidebarToggle = widget.NewButtonWithIcon("Hide sidebar", theme.MenuIcon(), ui.toggleSidebar)
	ui.windowSizeSelect = widget.NewSelect([]string{
		"Compact · 1100 × 720",
		"Default · 1280 × 820",
		"Wide · 1480 × 920",
	}, ui.setWindowSize)
	ui.windowSizeSelect.SetSelected("Default · 1280 × 820")
	ui.fullscreenButton = widget.NewButtonWithIcon("Full screen", theme.ViewFullScreenIcon(), ui.toggleFullscreen)

	windowControls := container.NewHBox(
		ui.sidebarToggle,
		deviceTarget,
		actions,
		ui.windowSizeSelect,
		ui.fullscreenButton,
	)
	top := withPadding(container.NewBorder(nil, nil, brand, windowControls, nil), 18, 10)

	ui.navButtons = make(map[string]*widget.Button)
	makeNavButton := func(name string, icon fyne.Resource, action func()) *widget.Button {
		button := widget.NewButtonWithIcon(name, icon, action)
		button.Importance = widget.MediumImportance
		ui.navButtons[name] = button
		return button
	}
	nav := container.NewVBox(
		widget.NewLabelWithStyle("WORKSPACE", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		makeNavButton("Devices", theme.ComputerIcon(), ui.showDevices),
		makeNavButton("Screen", theme.MediaPlayIcon(), ui.showScreen),
		makeNavButton("Apps", theme.FileIcon(), ui.showApps),
		makeNavButton("Logs", theme.DocumentIcon(), ui.showLogs),
		makeNavButton("Deploy", theme.UploadIcon(), ui.showDeploy),
		makeNavButton("Workflows", theme.ListIcon(), ui.showWorkflows),
		widget.NewSeparator(),
		widget.NewLabelWithStyle("TOOLS", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		makeNavButton("Settings", theme.SettingsIcon(), ui.showSettings),
		makeNavButton("Diagnostics", theme.WarningIcon(), ui.showDiagnostics),
		makeNavButton("Help", theme.HelpIcon(), ui.showHelp),
	)
	sidebarScroll := container.NewVScroll(withPadding(nav, 12, 16))
	sidebarScroll.Direction = container.ScrollVerticalOnly
	ui.sidebarPane = container.New(fixedWidthLayout{width: 244}, sidebarScroll)
	ui.sidebarSlot = container.NewMax(ui.sidebarPane)

	center := container.NewBorder(
		withPadding(ui.contentTitle, 22, 12),
		nil, nil, nil,
		ui.contentScroll,
	)
	bottom := withPadding(container.NewBorder(nil, nil, ui.status, nil, ui.activity), 20, 8)
	ui.shell = container.NewBorder(top, bottom, ui.sidebarSlot, nil, center)

	ui.showDevices()
	return ui.shell
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
	ui.win.Canvas().AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyB, Modifier: fyne.KeyModifierControl}, func(fyne.Shortcut) {
		if _, editing := ui.win.Canvas().Focused().(*widget.Entry); editing {
			return
		}
		ui.toggleSidebar()
	})
	ui.win.Canvas().AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyF11}, func(fyne.Shortcut) {
		ui.toggleFullscreen()
	})
	ui.win.Canvas().AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyEscape}, func(fyne.Shortcut) {
		if ui.win.FullScreen() {
			ui.toggleFullscreen()
		}
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

		active := title
		if active == "Help & Tutorials" {
			active = "Help"
		}
		for name, button := range ui.navButtons {
			button.Importance = widget.MediumImportance
			if name == active {
				button.Importance = widget.HighImportance
			}
			button.Refresh()
		}

		// A single page-level scroll host keeps tall workspaces (including
		// Screen, Deploy, Diagnostics, and Help) reachable on short windows.
		ui.contentScroll.Content = withPadding(obj, 22, 18)
		ui.contentScroll.ScrollToTop()
		ui.contentScroll.Refresh()
	})
}

func (ui *desktopUI) toggleSidebar() {
	if ui.sidebarSlot == nil || ui.sidebarPane == nil {
		return
	}
	if len(ui.sidebarSlot.Objects) > 0 {
		ui.sidebarSlot.Objects = nil
		ui.sidebarToggle.SetText("Show sidebar")
	} else {
		ui.sidebarSlot.Objects = []fyne.CanvasObject{ui.sidebarPane}
		ui.sidebarToggle.SetText("Hide sidebar")
	}
	ui.sidebarSlot.Refresh()
	if ui.shell != nil {
		ui.shell.Refresh()
	}
}

func (ui *desktopUI) setWindowSize(value string) {
	var size fyne.Size
	switch value {
	case "Compact · 1100 × 720":
		size = fyne.NewSize(1100, 720)
	case "Wide · 1480 × 920":
		size = fyne.NewSize(1480, 920)
	default:
		value = "Default · 1280 × 820"
		size = fyne.NewSize(1280, 820)
	}

	if ui.win.FullScreen() {
		ui.win.SetFullScreen(false)
		ui.fullscreenButton.SetText("Full screen")
		ui.windowSizeSelect.Enable()
	}
	ui.windowedSize = size
	ui.win.Resize(size)
	ui.win.CenterOnScreen()
}

func (ui *desktopUI) toggleFullscreen() {
	if ui.win.FullScreen() {
		ui.win.SetFullScreen(false)
		ui.win.Resize(ui.windowedSize)
		ui.win.CenterOnScreen()
		ui.fullscreenButton.SetText("Full screen")
		ui.windowSizeSelect.Enable()
		return
	}

	current := ui.win.Canvas().Size()
	if current.Width >= 640 && current.Height >= 480 {
		ui.windowedSize = current
	}
	ui.win.SetFullScreen(true)
	ui.fullscreenButton.SetText("Windowed")
	ui.windowSizeSelect.Disable()
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
	body := container.NewVBox(
		widget.NewRichTextFromMarkdown("### Your Android devices\nConnect over USB or Wireless debugging. Select one device to use it across Screen, Apps, Logs, and Deploy."),
	)

	if len(ui.devices) == 0 {
		detail := widget.NewLabel("Connect your phone with a data-capable USB cable and accept the debugging prompt, or pair a device over Android 11+ Wireless debugging.")
		actions := container.NewHBox(
			widget.NewButtonWithIcon("Add Wireless Device", theme.ContentAddIcon(), ui.showWirelessDialog),
			widget.NewButtonWithIcon("Refresh Devices", theme.ViewRefreshIcon(), func() { go ui.refreshDevices() }),
		)
		body.Add(widget.NewCard("No Android device connected", "Your workspace is ready when a device appears here.", container.NewVBox(detail, actions)))
	} else {
		for _, device := range ui.devices {
			dev := device
			name := dev.Label()
			if alias := ui.core.State().DeviceAliases[dev.Serial]; alias != "" {
				name = alias
			}
			status := fmt.Sprintf("%s · %s · Android %s · Battery %s · Storage %s",
				dev.Transport, dev.Status,
				fallback(dev.AndroidVersion, "unknown"),
				fallback(dev.BatteryPercent, "unknown"),
				fallback(dev.StorageSummary, "unknown"),
			)
			serial := dev.Serial
			selectText := "Select device"
			if dev.Serial == ui.selectedSerial {
				selectText = "Currently selected"
			}
			actions := container.NewHBox(
				widget.NewButton(selectText, func() {
					ui.selectedSerial = dev.Serial
					_ = ui.core.SaveState(func(state *AppState) { state.SelectedSerial = dev.Serial })
					ui.deviceSelect.SetSelected(dev.Label())
					ui.setStatus("Selected "+dev.Label(), false)
					ui.showDevices()
				}),
				widget.NewButton("Rename", func() { ui.renameDevice(dev.Serial) }),
				widget.NewButton("Disconnect", func() { go ui.disconnectDevice(dev.Serial) }),
				widget.NewButton("Copy serial", func() {
					ui.win.Clipboard().SetContent(serial)
					ui.setStatus("Copied device serial", false)
				}),
			)
			subtitle := status
			if dev.Serial == ui.selectedSerial {
				subtitle = "ACTIVE DEVICE · " + subtitle
			}
			body.Add(widget.NewCard(name, subtitle, container.NewVBox(
				widget.NewLabelWithStyle("Serial: "+serial, fyne.TextAlignLeading, fyne.TextStyle{Monospace: true}),
				actions,
			)))
		}
	}

	if endpoints := ui.core.State().WirelessEndpoints; len(endpoints) > 0 {
		body.Add(widget.NewRichTextFromMarkdown("### Saved wireless devices\nReconnect a previously paired endpoint or remove it from this list."))
		for _, endpoint := range endpoints {
			addr := endpoint
			connected := false
			for _, device := range ui.devices {
				if device.Serial == addr {
					connected = true
					break
				}
			}
			state := "Previously connected"
			if connected {
				state = "Connected"
			}
			actions := container.NewHBox(
				widget.NewButton("Reconnect", func() { go ui.reconnectWireless(addr) }),
				widget.NewButton("Forget", func() { ui.forgetWireless(addr) }),
			)
			body.Add(widget.NewCard(addr, state, actions))
		}
	}

	ui.replaceContent("Devices", body)
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

	start := widget.NewButtonWithIcon("Start Mirror", theme.MediaPlayIcon(), func() { startWithOptions("") })
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
			mirrorState.SetText("Mirror stop requested.")
		}
	})
	screenshot := widget.NewButtonWithIcon("Capture Screenshot…", theme.DocumentSaveIcon(), ui.saveScreenshot)

	intro := widget.NewCard("Screen mirroring", "Powered by scrcpy in a native companion window.", widget.NewLabel("Mouse, keyboard, fullscreen, rotation, and recording stay in scrcpy itself. AdbPureFlow manages the session and its lifecycle."))
	target := widget.NewCard("Selected device", deviceText, widget.NewLabel("The selected device is shared with the other workspaces."))
	options := widget.NewCard("Mirror options", "Tune the session before starting.", container.NewVBox(
		widget.NewForm(widget.NewFormItem("Resolution", quality), widget.NewFormItem("Video bit rate", bitrate)),
		always,
		fullscreen,
	))
	controls := widget.NewCard("Session controls", "Start, stop, record, or capture the current device screen.", container.NewVBox(
		mirrorState,
		container.NewHBox(start, record, stop, screenshot),
	))
	ui.replaceContent("Screen", container.NewVBox(intro, target, options, controls))
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
	ui.replaceContent("Install APK", container.NewVBox(widget.NewRichTextFromMarkdown("### APK workflow\n"+strings.Join(lines, "\n\n")), clearData, launch, install))
}

func (ui *desktopUI) showApps() {
	dev, ok := ui.requireDevice()
	if !ok {
		return
	}

	filter := widget.NewEntry()
	filter.SetPlaceHolder("Search by package name…")
	showSystem := widget.NewCheck("Include system apps", nil)
	countLabel := widget.NewLabel("Loading apps…")
	listBox := container.NewVBox(widget.NewLabel("Loading apps…"))
	var loaded []PackageInfo

	render := func(pkgs []PackageInfo) {
		query := strings.ToLower(strings.TrimSpace(filter.Text))
		listBox.Objects = nil
		visible := 0
		for _, app := range pkgs {
			if query != "" && !strings.Contains(strings.ToLower(app.PackageName), query) {
				continue
			}
			pkg := app.PackageName
			kind := "User app"
			if !app.UserApp {
				kind = "System app"
			}
			actions := container.NewHBox(
				widget.NewButton("Launch", func() { go ui.packageAction("Launch", pkg, ui.core.adb.Launch) }),
				widget.NewButton("Force stop", func() { go ui.packageAction("Force stop", pkg, ui.core.adb.ForceStop) }),
				widget.NewButton("Clear data", func() {
					ui.confirmPackageAction("Clear data", pkg, "This removes local app data on the selected Android device.", ui.core.adb.ClearData)
				}),
				widget.NewButton("Uninstall", func() {
					ui.confirmPackageAction("Uninstall", pkg, "This removes the app from the selected Android device.", ui.core.adb.Uninstall)
				}),
				widget.NewButton("Copy package", func() {
					ui.win.Clipboard().SetContent(pkg)
					ui.setStatus("Copied package name", false)
				}),
			)
			listBox.Add(widget.NewCard(pkg, kind, container.NewVBox(
				widget.NewLabel(fallback(app.APKPath, "Installed package")),
				actions,
			)))
			visible++
		}

		countLabel.SetText(fmt.Sprintf("%d app(s)", visible))
		if visible == 0 {
			message := "No apps are installed on this device."
			if len(pkgs) > 0 {
				message = "No packages match your search. Try another package name or clear the filter."
			}
			listBox.Add(widget.NewCard("No matching apps", message, widget.NewLabel("Change the search or refresh the app list.")))
		}
		listBox.Refresh()
	}

	filter.OnChanged = func(string) { render(loaded) }
	load := func(includeSystem bool) {
		loaded = nil
		countLabel.SetText("Loading apps…")
		listBox.Objects = []fyne.CanvasObject{widget.NewLabel("Loading installed apps…")}
		listBox.Refresh()
		go func() {
			packages, err := ui.core.adb.Packages(context.Background(), dev.Serial, !includeSystem)
			if err != nil {
				ui.showError(err)
				return
			}
			fyne.Do(func() {
				loaded = packages
				render(loaded)
			})
		}()
	}
	showSystem.OnChanged = func(include bool) { load(include) }

	install := widget.NewButtonWithIcon("Install APK…", theme.ContentAddIcon(), func() { ui.chooseAPK(ui.inspectAPK) })
	refresh := widget.NewButtonWithIcon("Refresh", theme.ViewRefreshIcon(), func() { load(showSystem.Checked) })
	toolbar := container.NewHBox(showSystem, install, refresh, countLabel)
	header := container.NewVBox(
		widget.NewRichTextFromMarkdown("### Installed applications\nSearch packages and run actions on the selected Android device. Destructive actions ask for confirmation."),
		filter,
		toolbar,
	)
	ui.replaceContent("Apps", container.NewVBox(header, listBox))
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
	ui.logList = widget.NewList(func() int { return len(ui.filteredLogs()) }, func() fyne.CanvasObject {
		label := widget.NewLabel("Log entry")
		label.Wrapping = fyne.TextWrapWord
		return label
	}, func(id widget.ListItemID, obj fyne.CanvasObject) {
		entries := ui.filteredLogs()
		if id < len(entries) {
			obj.(*widget.Label).SetText(formatLog(entries[id]))
		}
	})

	start := widget.NewButtonWithIcon("Start", theme.MediaPlayIcon(), func() { ui.startLogs() })
	stop := widget.NewButtonWithIcon("Stop", theme.MediaStopIcon(), func() {
		ui.stopLogs()
		ui.setStatus("Log stream stopped", false)
	})
	var pause *widget.Button
	pause = widget.NewButton("Pause", func() {
		ui.logPaused = !ui.logPaused
		if ui.logPaused {
			pause.SetText("Resume")
			ui.setStatus("Logs paused", false)
		} else {
			pause.SetText("Pause")
			ui.setStatus("Logs resumed", false)
		}
	})
	if ui.logPaused {
		pause.SetText("Resume")
	}
	clear := widget.NewButton("Clear device log", func() {
		d, ok := ui.requireDevice()
		if !ok {
			return
		}
		go func() {
			if err := ui.core.adb.ClearLogcat(context.Background(), d.Serial); err != nil {
				ui.showError(err)
				return
			}
			ui.logMu.Lock()
			ui.logEntries = nil
			ui.logMu.Unlock()
			fyne.Do(func() { ui.logList.Refresh() })
			ui.setStatus("Device log cleared", false)
		}()
	})
	export := widget.NewButtonWithIcon("Export visible logs…", theme.DocumentSaveIcon(), func() {
		entries := ui.filteredLogs()
		if len(entries) == 0 {
			ui.showInfo("Nothing to export", "There are no visible log entries. Start logcat or adjust the filters first.")
			return
		}
		var output strings.Builder
		for _, entry := range entries {
			output.WriteString(formatLog(entry))
			output.WriteByte('\n')
		}
		save := dialog.NewFileSave(func(file fyne.URIWriteCloser, err error) {
			if err != nil {
				ui.showError(err)
				return
			}
			if file == nil {
				return
			}
			if _, err := file.Write([]byte(output.String())); err != nil {
				_ = file.Close()
				ui.showError(err)
				return
			}
			if err := file.Close(); err != nil {
				ui.showError(err)
				return
			}
			ui.setStatus(fmt.Sprintf("Exported %d log entries", len(entries)), false)
		}, ui.win)
		save.SetFileName("adbpureflow-logcat.txt")
		save.SetFilter(storage.NewExtensionFileFilter([]string{".txt", ".log"}))
		save.Show()
	})

	filter.OnChanged = func(string) { ui.logList.Refresh() }
	controls := container.NewHBox(start, stop, pause, clear, export)
	header := container.NewVBox(
		widget.NewRichTextFromMarkdown("### Live device logs\nStream Android logcat, narrow the visible entries, then export only the entries that match your filters."),
		filter,
		container.NewHBox(level, controls),
	)
	ui.replaceContent("Logs", container.NewBorder(header, nil, nil, nil, ui.logList))
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
	pkg.SetPlaceHolder("Package to launch, e.g. com.example.app")
	clear := widget.NewCheck("Clear app data before launch", nil)
	mirror := widget.NewCheck("Start screen mirror after install", nil)
	logs := widget.NewCheck("Open device logs after deploy", nil)
	progress := widget.NewLabel("Ready. Choose an APK and review the options.")

	setAPK := func(path string) {
		apkPath.SetText(path)
		go func() {
			meta, inspectErr := InspectAPK(context.Background(), path)
			if inspectErr == nil && meta.PackageID != "" {
				fyne.Do(func() { pkg.SetText(meta.PackageID) })
			}
		}()
	}
	choose := widget.NewButton("Browse…", func() {
		ui.chooseAPK(setAPK)
	})

	run := widget.NewButtonWithIcon("Run deploy", theme.MediaPlayIcon(), func() {
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
		artifact := apkPath.Text
		openLogs := logs.Checked
		go func() {
			ui.setStatus("Deploying…", true)
			fyne.Do(func() { progress.SetText("Deployment started…") })
			err := ui.core.RunFlow(ctx, dev.Serial, profile, func(event FlowEvent) {
				ui.setStatus(string(event.Action.Type)+" "+event.State, true)
				fyne.Do(func() { progress.SetText(string(event.Action.Type) + " · " + event.State) })
			})
			deployCancel = nil
			if err != nil {
				ui.setStatus("Deploy failed", false)
				fyne.Do(func() { progress.SetText("Deployment failed: " + err.Error()) })
				ui.showError(err)
				return
			}
			ui.setStatus("Deploy complete", false)
			fyne.Do(func() { progress.SetText("Deployment completed successfully.") })
			ui.notify("Deploy complete", filepath.Base(artifact))
			_ = ui.core.SaveState(func(state *AppState) {
				state.RecentAPKs = addRecentPath(state.RecentAPKs, artifact, 12)
			})
			if openLogs {
				fyne.Do(ui.showLogs)
				ui.startLogs()
			}
		}()
	})
	cancelButton := widget.NewButton("Cancel deploy", func() {
		if deployCancel != nil {
			deployCancel()
			ui.setStatus("Deploy cancellation requested", false)
			progress.SetText("Cancellation requested…")
		}
	})

	recentContent := container.NewVBox()
	recentPaths := ui.core.State().RecentAPKs
	if len(recentPaths) == 0 {
		recentContent.Add(widget.NewLabel("APKs appear here after a successful deployment."))
	} else {
		for _, recentPath := range recentPaths {
			path := recentPath
			recentContent.Add(widget.NewCard(filepath.Base(path), path, widget.NewButton("Use this APK", func() { setAPK(path) })))
		}
	}

	artifactCard := widget.NewCard("APK artifact", "Choose a local APK or reuse a recent artifact.", container.NewBorder(nil, nil, nil, choose, apkPath))
	optionsCard := widget.NewCard("Deployment options", "Choose what should happen after installation.", container.NewVBox(
		pkg,
		clear,
		mirror,
		logs,
	))
	runCard := widget.NewCard("Deployment progress", "The flow stops when a step fails; it does not report partial success as complete.", container.NewVBox(
		progress,
		container.NewHBox(run, cancelButton),
	))
	recentCard := widget.NewCard("Recent APKs", "Quickly reuse a previously deployed artifact.", recentContent)

	ui.replaceContent("Deploy", container.NewVBox(
		widget.NewRichTextFromMarkdown("### Repeatable deployment\nInstall an APK and optionally clear data, launch the app, start screen mirroring, or open logcat."),
		artifactCard,
		optionsCard,
		runCard,
		recentCard,
	))
}

func (ui *desktopUI) showWorkflows() {
	description := widget.NewCard(
		"Repeatable device actions",
		"Run a predictable sequence instead of repeating the same manual steps.",
		widget.NewLabel("The workflow engine currently supports Install APK, Clear Data, Launch App, Start Mirror, and Wait. Deploy uses this same engine and reports each step as it runs."),
	)
	steps := container.NewVBox(
		widget.NewCard("1 · Install APK", "Install an APK file on the selected device.", widget.NewLabel("The artifact is validated and installed through ADB.")),
		widget.NewCard("2 · Optional cleanup", "Clear application data before launching.", widget.NewLabel("This step is opt-in because it removes local app data.")),
		widget.NewCard("3 · Launch and inspect", "Launch the package, then optionally open mirror and logs.", widget.NewLabel("The flow stops on a failed step and preserves the error for troubleshooting.")),
	)
	actions := widget.NewCard("Create a deployment run", "Choose the APK and configure the available actions.", widget.NewButtonWithIcon("Open Deploy", theme.UploadIcon(), ui.showDeploy))
	ui.replaceContent("Workflows", container.NewVBox(description, steps, actions))
}

func (ui *desktopUI) showDiagnostics() {
	box := container.NewVBox(widget.NewLabel("Running diagnostics…"))
	restart := widget.NewButtonWithIcon("Restart ADB server", theme.ViewRefreshIcon(), func() {
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
	})
	ui.replaceContent("Diagnostics", container.NewVBox(
		widget.NewRichTextFromMarkdown("### Device and tool health\nChecks ADB availability, device authorization, and scrcpy capability. Expand technical details only when you need to investigate a failure."),
		restart,
		box,
	))
	go func() {
		checks := ui.core.Doctor(context.Background())
		fyne.Do(func() {
			box.Objects = nil
			for _, check := range checks {
				content := container.NewVBox(
					widget.NewLabel(check.Message),
					widget.NewLabel("Suggested action: "+check.Recovery),
				)
				if check.Technical != "" {
					content.Add(widget.NewAccordion(widget.NewAccordionItem("Technical details", widget.NewLabel(check.Technical))))
				}
				box.Add(widget.NewCard(check.Name, check.Status, content))
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
	appearance := widget.NewCard("Appearance", "Choose how AdbPureFlow follows your desktop.", container.NewVBox(
		widget.NewForm(widget.NewFormItem("Theme", themeSelect)),
	))
	storage := widget.NewCard("Local preferences", "Only non-sensitive application preferences are stored here.", container.NewVBox(
		widget.NewLabel("Preferences file:"),
		widget.NewLabelWithStyle(statePath, fyne.TextAlignLeading, fyne.TextStyle{Monospace: true}),
		widget.NewLabel("Downloaded tools are stored in the current user's cache/config directory so a normal installation does not need administrator rights."),
	))
	help := widget.NewCard("Need a hand?", "Setup steps and troubleshooting are always available.", widget.NewButtonWithIcon("Open Help & Tutorials", theme.HelpIcon(), ui.showHelp))
	ui.replaceContent("Settings", container.NewVBox(appearance, storage, help))
}

func (ui *desktopUI) showHelp() {
	guide := widget.NewCard("Start here", "A short route through the main workspaces.", widget.NewRichTextFromMarkdown("1. **Devices:** connect by USB or Add Wireless, then select your phone.\n2. **Screen:** start scrcpy mirroring, capture a screenshot, or record a session.\n3. **Apps:** search installed packages, launch, stop, clear data, or uninstall.\n4. **Logs:** stream logcat, filter entries, and export the visible results.\n5. **Deploy:** install an APK and optionally clear, launch, mirror, or open logs."))
	shortcuts := widget.NewCard("Keyboard shortcuts", "Use the app without leaving the keyboard.", widget.NewRichTextFromMarkdown("- **Ctrl+K** opens the command palette.\n- **Ctrl+R** refreshes connected devices.\n- **Ctrl+B** shows or hides the sidebar.\n- **F11** toggles full screen; **Esc** returns to windowed mode."))
	recovery := widget.NewCard("When something does not work", "Start with Diagnostics before changing device settings.", widget.NewLabel("Check that USB debugging is enabled, accept the RSA prompt on the phone, and verify ADB/scrcpy in Diagnostics. Wireless pairing uses Android's pairing-code flow; QR pairing is not simulated."))
	ui.replaceContent("Help & Tutorials", container.NewVBox(guide, shortcuts, recovery))
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
