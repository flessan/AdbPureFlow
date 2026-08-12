// Command adbpureflow-gui is the Fyne-based desktop GUI for ADBPureFlow.
//
// Architecture note: this file is a *presentation layer* only. All ADB
// operations (device discovery, package listing, install/launch/stop/
// uninstall, scrcpy) live in the shared `internal/adb` package, which is
// also used by the CLI.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/flessan/AdbPureFlow/internal/adb"
)

// version is stamped at build time via -ldflags="-X main.version=vX.Y.Z".
var version = "dev"

func main() {
	a := app.NewWithID("com.thio.adbpureflow")
	a.Settings().SetTheme(theme.DarkTheme())
	w := a.NewWindow(fmt.Sprintf("ADBPureFlow %s", version))
	w.Resize(fyne.NewSize(960, 640))

	ui := newUI(a, w)
	w.SetContent(ui.build())
	w.CenterOnScreen()

	go ui.refreshDevices()

	w.ShowAndRun()
}

// ui bundles GUI state.
type ui struct {
	app    fyne.App
	window fyne.Window
	mgr    *adb.Manager
	ctx    context.Context
	cancel context.CancelFunc

	// Header widgets.
	statusLbl     *widget.Label
	deviceSelect  *widget.Select
	refreshDevBtn *widget.Button
	searchEntry   *widget.Entry

	// List view.
	appList   *widget.List
	logArea   *widget.Label
	logScroll *container.Scroll

	// Detail view.
	detailName   *canvas.Text
	detailPkg    *widget.Label
	detailForm   *widget.Form
	detailScroll *container.Scroll
	backBtn      *widget.Button

	// Center stack swaps between list and detail panels.
	center *fyne.Container

	// Footer actions (context-sensitive).
	refreshBtn   *widget.Button
	installBtn   *widget.Button
	launchBtn    *widget.Button
	stopBtn      *widget.Button
	uninstallBtn *widget.Button
	detailsBtn   *widget.Button
	mirrorBtn    *widget.Button
	actionBar    *fyne.Container

	// State.
	devices        []adb.Device
	apps           []adb.Package // filtered list view
	allApps        []adb.Package // unfiltered
	selectedDevice *adb.Device
	selectedApp    *adb.Package
	detailApp      *adb.Package
	detailMode     bool
	logLines       []string
}

func newUI(_ fyne.App, w fyne.Window) *ui {
	ctx, cancel := context.WithCancel(context.Background())
	u := &ui{window: w, ctx: ctx, cancel: cancel}

	dataDir := ""
	if exe, err := os.Executable(); err == nil {
		dataDir = filepath.Dir(exe)
	}
	mgr, err := adb.NewManager(dataDir, true)
	if err != nil {
		dialog.ShowError(fmt.Errorf("failed to initialize ADB: %w", err), w)
	}
	u.mgr = mgr
	return u
}

func (u *ui) build() fyne.CanvasObject {
	u.statusLbl = widget.NewLabel("Status: initializing…")

	// ---- Device selector ---------------------------------------------------
	u.deviceSelect = widget.NewSelect([]string{"(no devices)"}, func(s string) {
		u.onDeviceSelected(s)
	})
	u.deviceSelect.PlaceHolder = "Select a device…"
	u.refreshDevBtn = widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), func() { go u.refreshDevices() })
	deviceRow := container.NewBorder(nil, nil, widget.NewLabel("Target device: "), u.refreshDevBtn, u.deviceSelect)

	// ---- Search ------------------------------------------------------------
	u.searchEntry = widget.NewEntry()
	u.searchEntry.SetPlaceHolder("Search installed applications…")
	u.searchEntry.OnChanged = func(s string) { u.applyFilter(s) }

	// ---- App list ----------------------------------------------------------
	u.appList = widget.NewList(
		func() int { return len(u.apps) },
		func() fyne.CanvasObject { return newAppListItem() },
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			if id < 0 || id >= len(u.apps) {
				return
			}
			it := obj.(*appListItem)
			it.set(u.apps[id])
			isSel := u.selectedApp != nil && u.selectedApp.Name == u.apps[id].Name
			it.setSelected(isSel)
		},
	)
	u.appList.OnSelected = func(id widget.ListItemID) {
		if id < 0 || id >= len(u.apps) {
			return
		}
		p := u.apps[id]
		u.selectedApp = &p
		u.updateActionState()
		u.appList.RefreshItem(id)
	}

	// ---- Log ---------------------------------------------------------------
	u.logArea = widget.NewLabel("")
	u.logArea.Wrapping = fyne.TextWrapWord
	u.logScroll = container.NewVScroll(u.logArea)
	u.logScroll.SetMinSize(fyne.NewSize(0, 120))
	u.log("Ready.")

	// ---- Action buttons ----------------------------------------------------
	u.refreshBtn = widget.NewButtonWithIcon("Refresh", theme.ViewRefreshIcon(), func() { go u.refreshApps() })
	u.installBtn = widget.NewButtonWithIcon("Install APK…", theme.UploadIcon(), func() { u.onInstall() })
	u.launchBtn = widget.NewButtonWithIcon("Launch", theme.MediaPlayIcon(), func() { go u.onLaunch() })
	u.stopBtn = widget.NewButtonWithIcon("Force Stop", theme.MediaStopIcon(), func() { go u.onStop() })
	u.uninstallBtn = widget.NewButtonWithIcon("Uninstall", theme.DeleteIcon(), func() { u.onUninstall() })
	u.detailsBtn = widget.NewButtonWithIcon("Details", theme.InfoIcon(), func() { u.openDetail() })
	u.mirrorBtn = widget.NewButtonWithIcon("Mirror", theme.ComputerIcon(), func() { go u.onMirror() })

	u.actionBar = container.NewHBox(
		u.refreshBtn,
		widget.NewSeparator(),
		u.installBtn,
		u.launchBtn,
		u.stopBtn,
		u.uninstallBtn,
		widget.NewSeparator(),
		u.detailsBtn,
		u.mirrorBtn,
	)

	// ---- Detail view widgets (created once, populated on demand) -----------
	u.detailName = canvas.NewText("", theme.ForegroundColor())
	u.detailName.TextSize = 22
	u.detailName.TextStyle = fyne.TextStyle{Bold: true}
	u.detailPkg = widget.NewLabel("")
	u.detailPkg.TextStyle = fyne.TextStyle{Monospace: true}
	u.detailPkg.Wrapping = fyne.TextWrapWord
	u.detailForm = widget.NewForm()
	u.detailScroll = container.NewVScroll(u.detailForm)
	u.backBtn = widget.NewButtonWithIcon("← Back to applications", theme.NavigateBackIcon(), func() { u.showList() })

	// ---- Layout ------------------------------------------------------------
	header := container.NewVBox(
		heading("ADBPureFlow"),
		subheading("Simple Android device & application management"),
		widget.NewSeparator(),
		deviceRow,
		widget.NewSeparator(),
	)
	footer := container.NewVBox(
		widget.NewSeparator(),
		u.actionBar,
		widget.NewSeparator(),
		container.NewBorder(nil, nil, u.statusLbl, nil, nil),
	)

	u.center = container.NewStack(u.buildListPanel())

	u.updateActionState()
	return container.NewBorder(header, footer, nil, nil, u.center)
}

// ---------------------------------------------------------------------------
// Panels
// ---------------------------------------------------------------------------

func (u *ui) buildListPanel() fyne.CanvasObject {
	header := container.NewBorder(nil, nil, widget.NewLabel("Applications"), nil, u.searchEntry)
	body := container.NewHSplit(u.appList, u.logScroll)
	body.SetOffset(0.7)
	return container.NewBorder(header, nil, nil, nil, body)
}

func (u *ui) buildDetailPanel() fyne.CanvasObject {
	p := u.detailApp
	if p == nil {
		return u.buildListPanel()
	}
	u.detailName.Text = p.DisplayTitle()
	u.detailName.Color = theme.ForegroundColor()
	u.detailName.Refresh()
	u.detailPkg.SetText(p.Name)

	items := u.detailItems(p)
	u.detailForm.Items = items
	u.detailForm.Refresh()

	// Detail-only actions in the panel body (footer still shows global set
	// but these are conveniently placed next to the metadata).
	dLaunch := widget.NewButtonWithIcon("Launch", theme.MediaPlayIcon(), func() { go u.onLaunch() })
	dStop := widget.NewButtonWithIcon("Force Stop", theme.MediaStopIcon(), func() { go u.onStop() })
	dUninstall := widget.NewButtonWithIcon("Uninstall", theme.DeleteIcon(), func() { u.onUninstall() })
	detailActions := container.NewHBox(dLaunch, dStop, dUninstall, layout.NewSpacer())

	head := container.NewVBox(
		u.backBtn,
		widget.NewSeparator(),
		u.detailName,
		u.detailPkg,
		widget.NewSeparator(),
	)
	foot := container.NewVBox(widget.NewSeparator(), detailActions)
	u.detailScroll = container.NewVScroll(u.detailForm)
	return container.NewBorder(head, foot, nil, nil, u.detailScroll)
}

func (u *ui) detailItems(p *adb.Package) []*widget.FormItem {
	ver := p.VersionSummary()
	if ver == "" {
		ver = "—"
	}
	enabled := "Yes"
	if !p.Enabled {
		enabled = "No"
	}
	installer := p.Installer
	if installer == "" {
		installer = "—"
	}
	apkPath := p.Path
	if apkPath == "" {
		apkPath = "—"
	}
	uid := "—"
	if p.UID > 0 {
		uid = fmt.Sprintf("%d", p.UID)
	}
	targetSdk := "—"
	if p.TargetSdk > 0 {
		targetSdk = fmt.Sprintf("%d", p.TargetSdk)
	}
	minSdk := "—"
	if p.MinSdk > 0 {
		minSdk = fmt.Sprintf("%d", p.MinSdk)
	}
	first := adb.FormatMillis(p.FirstInstall)
	last := adb.FormatMillis(p.LastUpdate)
	splits := "—"
	if len(p.SplitCodePaths) > 0 {
		splits = strings.Join(p.SplitCodePaths, "\n")
	}
	return []*widget.FormItem{
		widget.NewFormItem("Type", widget.NewLabel(p.Kind.String())),
		widget.NewFormItem("Version", widget.NewLabel(ver)),
		widget.NewFormItem("Enabled", widget.NewLabel(enabled)),
		widget.NewFormItem("Installer", widget.NewLabelWithStyle(installer, fyne.TextAlignLeading, fyne.TextStyle{Monospace: installer != "—"})),
		widget.NewFormItem("APK path", widget.NewLabelWithStyle(apkPath, fyne.TextAlignLeading, fyne.TextStyle{Monospace: true})),
		widget.NewFormItem("Split APKs", widget.NewLabelWithStyle(splits, fyne.TextAlignLeading, fyne.TextStyle{Monospace: splits != "—"})),
		widget.NewFormItem("UID", widget.NewLabel(uid)),
		widget.NewFormItem("Target SDK", widget.NewLabel(targetSdk)),
		widget.NewFormItem("Min SDK", widget.NewLabel(minSdk)),
		widget.NewFormItem("First installed", widget.NewLabel(first)),
		widget.NewFormItem("Last updated", widget.NewLabel(last)),
	}
}

func (u *ui) showList() {
	u.detailMode = false
	u.detailApp = nil
	u.center.Objects = []fyne.CanvasObject{u.buildListPanel()}
	u.center.Refresh()
	u.updateActionState()
}

func (u *ui) openDetail() {
	if u.selectedApp == nil {
		dialog.ShowInformation("No application selected", "Select an application from the list to view details.", u.window)
		return
	}
	u.detailMode = true
	u.detailApp = u.selectedApp
	u.center.Objects = []fyne.CanvasObject{u.buildDetailPanel()}
	u.center.Refresh()
	u.updateActionState()
}

func (u *ui) refreshDetail() {
	if u.detailMode && u.detailApp != nil {
		u.center.Objects = []fyne.CanvasObject{u.buildDetailPanel()}
		u.center.Refresh()
	}
}

// ---------------------------------------------------------------------------
// Event handlers
// ---------------------------------------------------------------------------

func (u *ui) refreshDevices() {
	if u.mgr == nil {
		u.setStatus("ADB not initialized", false)
		return
	}
	u.setStatus("Scanning for devices…", true)
	u.log("scanning for devices…")

	devs, err := u.mgr.RefreshDevices(u.ctx)
	if err != nil {
		u.doUI(func() {
			u.setStatus("device scan failed", false)
			dialog.ShowError(err, u.window)
		})
		u.logf("error: %v", err)
		return
	}

	u.doUI(func() {
		u.devices = devs
		options := make([]string, 0, len(devs))
		for _, d := range devs {
			options = append(options, d.DisplayName()+"  ["+string(d.State)+"]")
		}
		if len(options) == 0 {
			options = []string{"(no devices)"}
			u.deviceSelect.SetOptions(options)
			u.deviceSelect.SetSelectedIndex(0)
			u.selectedDevice = nil
			u.apps = nil
			u.allApps = nil
			u.selectedApp = nil
			u.appList.Refresh()
			u.setStatus("no devices connected", false)
			u.log("no devices found.")
			u.updateActionState()
			if u.detailMode {
				u.showList()
			}
			return
		}
		u.deviceSelect.SetOptions(options)
		if u.selectedDevice == nil {
			for i, d := range devs {
				if d.State == adb.StateDevice {
					u.deviceSelect.SetSelectedIndex(i)
					u.onDeviceSelected(options[i])
					break
				}
			}
		}
		u.setStatus(fmt.Sprintf("found %d device(s)", len(devs)), false)
		u.logf("found %d device(s).", len(devs))
	})
}

func (u *ui) onDeviceSelected(label string) {
	if label == "" || label == "(no devices)" {
		u.selectedDevice = nil
		u.apps = nil
		u.allApps = nil
		u.selectedApp = nil
		u.appList.Refresh()
		u.updateActionState()
		return
	}
	for i := range u.devices {
		d := &u.devices[i]
		if strings.HasPrefix(label, strings.SplitN(d.DisplayName(), " (", 2)[0]) && strings.Contains(label, d.Serial) {
			if d.State != adb.StateDevice {
				u.setStatus(fmt.Sprintf("device %s is %s", d.Serial, d.State), false)
				u.logf("device %s is in state %s (not usable)", d.Serial, d.State)
				u.selectedDevice = nil
				u.updateActionState()
				dialog.ShowInformation("Device not ready",
					fmt.Sprintf("Device %q is in state %q. Please authorize/connect it and refresh.", d.Serial, d.State),
					u.window)
				return
			}
			u.selectedDevice = d
			_ = u.mgr.Client.InspectDevice(u.ctx, *d)
			u.setStatus("target: "+d.DisplayName(), false)
			u.logf("selected %s", d.DisplayName())
			go u.refreshApps()
			u.updateActionState()
			return
		}
	}
}

func (u *ui) refreshApps() {
	if u.selectedDevice == nil {
		return
	}
	u.setStatus("loading applications…", true)
	u.logf("listing apps on %s…", u.selectedDevice.Serial)

	pkgs, err := u.mgr.ListPackages(u.ctx, u.selectedDevice.Serial, false)
	if err != nil {
		u.doUI(func() {
			u.setStatus("failed to list apps", false)
			dialog.ShowError(err, u.window)
		})
		u.logf("error listing apps: %v", err)
		return
	}

	u.doUI(func() {
		u.allApps = pkgs
		u.applyFilter(u.searchEntry.Text)
		u.setStatus(fmt.Sprintf("%s — %d apps", u.selectedDevice.DisplayName(), len(pkgs)), false)
		u.logf("loaded %d apps.", len(pkgs))
		// If we're in detail mode and the detail app still exists, refresh
		// its entry; otherwise fall back to list.
		if u.detailMode && u.detailApp != nil {
			for i := range pkgs {
				if pkgs[i].Name == u.detailApp.Name {
					u.detailApp = &pkgs[i]
					u.selectedApp = u.detailApp
					u.refreshDetail()
					return
				}
			}
			u.showList()
		}
	})
}

func (u *ui) applyFilter(q string) {
	if u.allApps == nil {
		u.apps = nil
		if u.appList != nil {
			u.appList.Refresh()
		}
		u.updateActionState()
		return
	}
	q = strings.ToLower(strings.TrimSpace(q))
	u.apps = u.apps[:0]
	if q == "" {
		u.apps = append(u.apps, u.allApps...)
	} else {
		for _, p := range u.allApps {
			label := strings.ToLower(p.Label)
			name := strings.ToLower(p.Name)
			if strings.Contains(name, q) || strings.Contains(label, q) {
				u.apps = append(u.apps, p)
			}
		}
	}
	sort.SliceStable(u.apps, func(i, j int) bool {
		if u.apps[i].Kind != u.apps[j].Kind {
			return u.apps[i].Kind < u.apps[j].Kind
		}
		return strings.ToLower(u.apps[i].DisplayTitle()) < strings.ToLower(u.apps[j].DisplayTitle())
	})
	if u.appList != nil {
		u.appList.Refresh()
	}
	u.selectedApp = nil
	u.updateActionState()
}

func (u *ui) onInstall() {
	if u.selectedDevice == nil {
		dialog.ShowInformation("No device", "Select a connected device first.", u.window)
		return
	}
	dialog.ShowFileOpen(func(reader fyne.URIReadCloser, err error) {
		if err != nil || reader == nil {
			return
		}
		defer reader.Close()
		path := reader.URI().Path()
		if len(path) >= 3 && path[0] == '/' && path[2] == ':' {
			path = path[1:]
		}
		path = filepath.FromSlash(path)
		go func() {
			u.setStatus("installing "+filepath.Base(path)+"…", true)
			u.logf("installing %s …", path)
			msg, err := u.mgr.InstallAPK(u.ctx, u.selectedDevice.Serial, path)
			u.doUI(func() {
				if err != nil {
					u.setStatus("install failed", false)
					dialog.ShowError(err, u.window)
					u.logf("install error: %v", err)
					return
				}
				u.setStatus("install OK", false)
				u.log(msg)
				u.refreshApps()
			})
		}()
	}, u.window)
}

func (u *ui) currentApp() *adb.Package {
	if u.detailMode && u.detailApp != nil {
		return u.detailApp
	}
	return u.selectedApp
}

func (u *ui) onLaunch() {
	if u.selectedDevice == nil {
		return
	}
	pkg := u.currentApp()
	if pkg == nil {
		return
	}
	name := pkg.Name
	title := pkg.DisplayTitle()
	u.setStatus("launching "+title+"…", true)
	u.logf("launching %s …", name)
	if err := u.mgr.LaunchApp(u.ctx, u.selectedDevice.Serial, name); err != nil {
		u.doUI(func() {
			u.setStatus("launch failed", false)
			dialog.ShowError(err, u.window)
		})
		u.logf("launch error: %v", err)
		return
	}
	u.doUI(func() { u.setStatus("launched "+title, false) })
	u.logf("launched %s.", name)
}

func (u *ui) onStop() {
	if u.selectedDevice == nil {
		return
	}
	pkg := u.currentApp()
	if pkg == nil {
		return
	}
	target := pkg.Name
	title := pkg.DisplayTitle()
	dialog.ShowConfirm("Force stop", fmt.Sprintf("Force-stop %q?", title), func(ok bool) {
		if !ok {
			return
		}
		go func() {
			u.setStatus("stopping "+title+"…", true)
			if err := u.mgr.ForceStopApp(u.ctx, u.selectedDevice.Serial, target); err != nil {
				u.doUI(func() {
					dialog.ShowError(err, u.window)
					u.setStatus("force-stop failed", false)
				})
				u.logf("stop error: %v", err)
				return
			}
			u.doUI(func() { u.setStatus("stopped "+title, false) })
			u.logf("force-stopped %s.", target)
		}()
	}, u.window)
}

func (u *ui) onUninstall() {
	if u.selectedDevice == nil {
		return
	}
	pkg := u.currentApp()
	if pkg == nil {
		return
	}
	target := pkg.Name
	title := pkg.DisplayTitle()
	dialog.ShowConfirm("Uninstall",
		fmt.Sprintf("Uninstall %q from %s? This cannot be undone.", title, u.selectedDevice.DisplayName()),
		func(ok bool) {
			if !ok {
				return
			}
			go func() {
				u.setStatus("uninstalling "+title+"…", true)
				if err := u.mgr.UninstallApp(u.ctx, u.selectedDevice.Serial, target, false); err != nil {
					u.doUI(func() {
						dialog.ShowError(err, u.window)
						u.setStatus("uninstall failed", false)
					})
					u.logf("uninstall error: %v", err)
					return
				}
				u.doUI(func() {
					wasDetail := u.detailMode
					if wasDetail {
						u.showList()
					}
					u.selectedApp = nil
					u.setStatus("uninstalled "+title, false)
				})
				u.logf("uninstalled %s.", target)
				u.refreshApps()
			}()
		}, u.window)
}

func (u *ui) onMirror() {
	if u.selectedDevice == nil {
		dialog.ShowInformation("No device", "Select a connected device first.", u.window)
		return
	}
	u.setStatus("starting scrcpy…", true)
	u.log("starting scrcpy mirror …")
	cmd, err := u.mgr.Scrcpy.StartMirror(u.ctx, u.selectedDevice.Serial, "ADBPureFlow-Mirror")
	if err != nil {
		u.doUI(func() {
			dialog.ShowError(err, u.window)
			u.setStatus("mirror failed", false)
		})
		u.logf("mirror error: %v", err)
		return
	}
	u.doUI(func() { u.setStatus("mirror active", false) })
	u.logf("scrcpy started (pid %d).", cmd.Process.Pid)
}

// ---------------------------------------------------------------------------
// UI helpers
// ---------------------------------------------------------------------------

func (u *ui) updateActionState() {
	hasDevice := u.selectedDevice != nil
	hasApp := u.currentApp() != nil
	for _, b := range []*widget.Button{u.installBtn, u.mirrorBtn, u.refreshBtn} {
		b.Enable()
		if !hasDevice {
			b.Disable()
		}
	}
	if u.mgr == nil {
		for _, b := range []*widget.Button{u.refreshBtn, u.installBtn, u.launchBtn, u.stopBtn, u.uninstallBtn, u.detailsBtn, u.mirrorBtn} {
			b.Disable()
		}
		return
	}
	if !hasDevice {
		u.refreshBtn.Enable()
	}
	setE := func(b *widget.Button, enabled bool) {
		if enabled {
			b.Enable()
		} else {
			b.Disable()
		}
	}
	setE(u.launchBtn, hasApp)
	setE(u.stopBtn, hasApp)
	setE(u.uninstallBtn, hasApp)
	setE(u.detailsBtn, hasApp && !u.detailMode)
}

func (u *ui) setStatus(text string, loading bool) {
	u.doUI(func() {
		prefix := "Status: "
		if loading {
			prefix = "Status: ⟳ "
		}
		u.statusLbl.SetText(prefix + text)
	})
}

func (u *ui) log(s string) {
	u.doUI(func() {
		ts := time.Now().Format("15:04:05")
		u.logLines = append(u.logLines, "["+ts+"] "+s)
		if len(u.logLines) > 200 {
			u.logLines = u.logLines[len(u.logLines)-200:]
		}
		u.logArea.SetText(strings.Join(u.logLines, "\n"))
		u.logScroll.ScrollToBottom()
	})
}

func (u *ui) logf(format string, args ...any) { u.log(fmt.Sprintf(format, args...)) }

func heading(text string) *canvas.Text {
	t := canvas.NewText(text, theme.ForegroundColor())
	t.TextSize = 20
	t.TextStyle = fyne.TextStyle{Bold: true}
	return t
}

func subheading(text string) *canvas.Text {
	t := canvas.NewText(text, theme.DisabledColor())
	t.TextSize = 12
	t.TextStyle = fyne.TextStyle{Italic: true}
	return t
}

func (u *ui) doUI(f func()) { f() }

// ---------------------------------------------------------------------------
// App list item: a compact two-line row with label (bold primary), package
// name (monospace secondary), and version tag on the right. Implemented as
// a simple container (not a custom widget) to keep the rendering simple.
// ---------------------------------------------------------------------------

type appListItem struct {
	widget.BaseWidget
	primary   *widget.Label
	secondary *widget.Label
	meta      *widget.Label
	box       *fyne.Container
	selected  bool
}

func newAppListItem() *appListItem {
	it := &appListItem{
		primary:   widget.NewLabel(""),
		secondary: widget.NewLabel(""),
		meta:      widget.NewLabel(""),
	}
	it.primary.TextStyle = fyne.TextStyle{Bold: true}
	it.primary.Truncation = fyne.TextTruncateClip
	it.secondary.TextStyle = fyne.TextStyle{Monospace: true}
	it.secondary.Truncation = fyne.TextTruncateClip
	it.meta.Alignment = fyne.TextAlignTrailing
	it.box = container.NewBorder(nil, nil, nil, it.meta,
		container.NewVBox(it.primary, it.secondary),
	)
	it.ExtendBaseWidget(it)
	return it
}

func (it *appListItem) set(p adb.Package) {
	it.primary.SetText(p.DisplayTitle())
	it.secondary.SetText(p.Name)
	it.meta.SetText(p.VersionSummary())
}

func (it *appListItem) setSelected(s bool) {
	if it.selected == s {
		return
	}
	it.selected = s
	if s {
		it.primary.Importance = widget.HighImportance
		it.secondary.Importance = widget.HighImportance
	} else {
		it.primary.Importance = widget.MediumImportance
		it.secondary.Importance = widget.MediumImportance
	}
	it.primary.Refresh()
	it.secondary.Refresh()
}

func (it *appListItem) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(container.NewPadded(it.box))
}
