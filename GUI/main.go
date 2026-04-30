package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

func main() {
	// 1. App Initialization
	myApp := app.NewWithID("com.thio.adbpureflow")
	myApp.Settings().SetTheme(theme.DarkTheme())
	myWindow := myApp.NewWindow("ADBPureFlow Pro v4.0")
	myWindow.Resize(fyne.NewSize(700, 550))

	// Backend Logic
	adbLogic := NewApp()

	// 2. UI Components
	title := widget.NewLabelWithStyle("ADBPureFlow Pro", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	subtitle := widget.NewLabelWithStyle("Universal ADB Installer & Manager", fyne.TextAlignCenter, fyne.TextStyle{Italic: true})

	// Status Bar
	statusLabel := widget.NewLabel("Status: Ready")
	progress := widget.NewProgressBarInfinite()
	progress.Hide()

	// Log Area
	logArea := widget.NewMultiLineEntry()
	logArea.SetPlaceHolder("System Logs:\n1. Connect your device.\n2. Drag & Drop APK or Click Open.")
	logArea.Wrapping = fyne.TextWrapBreak
	logArea.Disable()

	// --- DEFINE HELPERS FIRST (Fix for undefined error) ---

	// Thread-safe Logger
	appendLog := func(msg string) {
		fyne.Do(func() {
			timestamp := time.Now().Format("15:04:05")
			logArea.SetText(fmt.Sprintf("%s [%s] %s\n", logArea.Text, timestamp, msg))
		})
	}

	// Thread-safe Status Updater
	updateStatus := func(status string, loading bool) {
		fyne.Do(func() {
			statusLabel.SetText("Status: " + status)
			if loading {
				progress.Show()
			} else {
				progress.Hide()
			}
		})
	}

	// Device Selector (Defined after updateStatus)
	deviceSelect := widget.NewSelect([]string{"No Device"}, func(s string) {
		if s == "No Device" {
			updateStatus("Waiting for device...", false)
		} else {
			updateStatus("Target -> "+s, false)
		}
	})
	deviceSelect.PlaceHolder = "Select Device..."

	// Process APK Logic (Defined after deviceSelect and appendLog)
	processAPK := func(path string) {
		// Robust Path Cleaning for Windows
		cleanPath := filepath.FromSlash(path)
		// Fix specific Fyne Windows URI issue (e.g. /C:/Users -> C:/Users)
		if len(cleanPath) > 2 && cleanPath[0] == '\\' && cleanPath[2] == ':' {
			cleanPath = cleanPath[1:]
		}

		// Check if device selected
		selectedDevice := deviceSelect.Selected
		if selectedDevice == "" || selectedDevice == "No Device" {
			dialog.ShowError(fmt.Errorf("no device selected"), myWindow)
			appendLog("Error: No device selected")
			return
		}

		// Extract Serial ID from "Model (Serial)" string
		parts := strings.Split(selectedDevice, "(")
		if len(parts) < 2 {
			return
		}
		serial := strings.TrimSuffix(parts[len(parts)-1], ")")

		fileName := filepath.Base(cleanPath)
		appendLog(fmt.Sprintf("Processing: %s", fileName))
		updateStatus("Installing to "+selectedDevice, true)

		go func() {
			result := adbLogic.RunADBPure(cleanPath, serial)
			appendLog(result)
			updateStatus("Finished", false)
		}()
	}

	// Refresh Device List Logic
	refreshDevices := func() {
		go func() {
			appendLog("Scanning for devices...")
			devices, err := adbLogic.GetDetailedDevices()
			if err != nil {
				appendLog("Error: " + err.Error())
				return
			}

			fyne.Do(func() {
				if len(devices) == 0 {
					deviceSelect.Options = []string{"No Device"}
					deviceSelect.SetSelectedIndex(0)
					appendLog("No devices found.")
				} else {
					deviceSelect.Options = devices
					deviceSelect.SetSelectedIndex(0) // Auto select first
					appendLog(fmt.Sprintf("Found %d device(s).", len(devices)))
				}
			})
		}()
	}

	// Uninstall Logic
	uninstallApp := func() {
		selectedDevice := deviceSelect.Selected
		if selectedDevice == "" || selectedDevice == "No Device" {
			dialog.ShowError(fmt.Errorf("no device selected"), myWindow)
			return
		}

		entry := widget.NewEntry()
		entry.SetPlaceHolder("com.example.app")

		dialog.ShowForm("Uninstall App", "Uninstall", "Cancel",
			[]*widget.FormItem{
				widget.NewFormItem("Package Name", entry),
			},
			func(confirm bool) {
				if !confirm || entry.Text == "" {
					return
				}

				parts := strings.Split(selectedDevice, "(")
				serial := strings.TrimSuffix(parts[len(parts)-1], ")")

				updateStatus("Uninstalling...", true)
				go func() {
					out, err := adbLogic.Uninstall(entry.Text, serial)
					if err != nil {
						appendLog("Uninstall Failed: " + out)
					} else {
						appendLog("Success: " + out)
					}
					updateStatus("Ready", false)
				}()
			}, myWindow)
	}

	// 3. Toolbar
	toolbar := widget.NewToolbar(
		widget.NewToolbarAction(theme.FileIcon(), func() {
			dialog.ShowFileOpen(func(reader fyne.URIReadCloser, err error) {
				if err != nil || reader == nil {
					return
				}
				reader.Close()
				processAPK(reader.URI().Path())
			}, myWindow)
		}),
		widget.NewToolbarSeparator(),
		widget.NewToolbarAction(theme.ComputerIcon(), refreshDevices),
		widget.NewToolbarAction(theme.DeleteIcon(), uninstallApp),
		widget.NewToolbarSpacer(),
		widget.NewToolbarAction(theme.ViewRefreshIcon(), func() { logArea.SetText("") }),
	)

	// 4. Layout
	topSection := container.NewVBox(
		title,
		subtitle,
		widget.NewSeparator(),
		container.NewBorder(nil, nil, widget.NewLabel("Target Device:"), nil, deviceSelect),
		toolbar,
		widget.NewSeparator(),
	)

	bottomSection := container.NewVBox(
		widget.NewSeparator(),
		container.NewBorder(nil, nil, statusLabel, nil, progress),
	)

	mainContent := container.NewBorder(
		topSection,
		bottomSection,
		nil,
		nil,
		logArea,
	)

	// 5. Event Bindings
	myWindow.SetContent(mainContent)

	// Use Window SetOnDropped (Fix for container OnDropped error)
	myWindow.SetOnDropped(func(pos fyne.Position, uris []fyne.URI) {
		for _, uri := range uris {
			processAPK(uri.Path())
		}
	})

	myWindow.CenterOnScreen()

	// Auto-refresh devices on start
	go refreshDevices()

	myWindow.ShowAndRun()
}
