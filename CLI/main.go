// Command adbpureflow-cli is the interactive command-line interface for
// ADBPureFlow. It is a thin presentation layer over the shared `internal/adb`
// core, which also powers the Fyne GUI.
//
// Launch with no arguments to enter the interactive REPL. Pass a subcommand
// to run a single operation scriptably:
//
//	adbpureflow-cli devices
//	adbpureflow-cli apps [-u] [serial]
//	adbpureflow-cli info <package> [serial]
//	adbpureflow-cli install <path-to.apk> [serial]
//	adbpureflow-cli launch  <package>  [serial]
//	adbpureflow-cli stop    <package>  [serial]
//	adbpureflow-cli uninstall <package> [serial]
//	adbpureflow-cli mirror  [serial]
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/flessan/AdbPureFlow/internal/adb"
)

var version = "dev"

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	flag.Usage = usage
	if len(os.Args) > 1 {
		os.Exit(runCLI(ctx, os.Args[1:]))
	}
	runREPL(ctx)
}

func usage() {
	fmt.Fprintf(os.Stderr, "ADBPureFlow CLI %s\n\n", version)
	fmt.Fprintf(os.Stderr, "Usage:\n")
	fmt.Fprintf(os.Stderr, "  %s                  interactive REPL\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "  %s devices\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "  %s apps [-u] [serial]\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "  %s info <package> [serial]\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "  %s install <path-to.apk> [serial]\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "  %s launch <package> [serial]\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "  %s stop <package> [serial]\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "  %s uninstall <package> [serial]\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "  %s mirror [serial]\n", os.Args[0])
}

// ---------------------------------------------------------------------------
// Interactive REPL
// ---------------------------------------------------------------------------

func runREPL(ctx context.Context) {
	m := mustInitManager()
	r := bufio.NewReader(os.Stdin)

	printBanner()
	fmt.Println("  Type `help` for a list of commands, `exit` to quit.\n")

	var selected *adb.Device
	var lastPackages []adb.Package
	prompt := func() string {
		if selected == nil {
			return "adbpureflow> "
		}
		return fmt.Sprintf("adbpureflow@%s> ", shortSerial(selected.Serial))
	}

	for {
		fmt.Print(prompt())
		line, err := r.ReadString('\n')
		if err != nil {
			fmt.Println()
			return
		}
		args := tokenize(strings.TrimSpace(line))
		if len(args) == 0 {
			continue
		}
		cmd := strings.ToLower(args[0])

		switch cmd {
		case "help", "?", "h":
			printHelp()

		case "exit", "quit", "q":
			fmt.Println("Goodbye.")
			return

		case "devices", "ls":
			devs, err := m.RefreshDevices(ctx)
			if err != nil {
				fmt.Fprintln(os.Stderr, "error:", err)
				continue
			}
			if len(devs) == 0 {
				fmt.Println("(no devices connected)")
				continue
			}
			for i, d := range devs {
				marker := " "
				if selected != nil && d.Serial == selected.Serial {
					marker = "*"
				}
				fmt.Printf("  %s [%d] %-13s %s\n", marker, i+1, d.State, d.DisplayName())
			}
			lastPackages = nil

		case "use", "select":
			dev, err := pickDevice(ctx, m, args[1:], r)
			if err != nil {
				fmt.Fprintln(os.Stderr, "error:", err)
				continue
			}
			selected = &dev
			info := m.Client.InspectDevice(ctx, dev)
			fmt.Printf("selected %s", dev.DisplayName())
			if info.Model != "" || info.AndroidVer != "" {
				fmt.Printf("  (%s Android %s, SDK %s)", info.Model, info.AndroidVer, info.SDK)
			}
			fmt.Println()
			lastPackages = nil

		case "apps", "list", "packages":
			dev, err := ensureSelected(ctx, m, selected, r)
			if err != nil {
				fmt.Fprintln(os.Stderr, "error:", err)
				continue
			}
			selected = &dev
			userOnly := false
			var filter string
			for _, a := range args[1:] {
				switch a {
				case "-u", "--user":
					userOnly = true
				default:
					if !strings.HasPrefix(a, "-") {
						filter = a
					}
				}
			}
			pkgs, err := m.ListPackages(ctx, dev.Serial, userOnly)
			if err != nil {
				fmt.Fprintln(os.Stderr, "error:", err)
				continue
			}
			lastPackages = pkgs
			printPackages(pkgs, filter)

		case "search":
			dev, err := ensureSelected(ctx, m, selected, r)
			if err != nil {
				fmt.Fprintln(os.Stderr, "error:", err)
				continue
			}
			selected = &dev
			if len(args) < 2 {
				fmt.Fprintln(os.Stderr, "usage: search <query>")
				continue
			}
			pkgs, err := m.ListPackages(ctx, dev.Serial, false)
			if err != nil {
				fmt.Fprintln(os.Stderr, "error:", err)
				continue
			}
			lastPackages = pkgs
			printPackages(pkgs, strings.Join(args[1:], " "))

		case "info", "show", "details":
			dev, err := ensureSelected(ctx, m, selected, r)
			if err != nil {
				fmt.Fprintln(os.Stderr, "error:", err)
				continue
			}
			selected = &dev
			pkg, err := pickPackageWithIndex(args[1:], lastPackages, r, true)
			if err != nil {
				fmt.Fprintln(os.Stderr, "error:", err)
				continue
			}
			info, err := m.PackageInfo(ctx, dev.Serial, pkg)
			if err != nil {
				fmt.Fprintln(os.Stderr, "error:", err)
				continue
			}
			printPackageInfo(info)

		case "refresh":
			if _, err := m.RefreshDevices(ctx); err != nil {
				fmt.Fprintln(os.Stderr, "error:", err)
				continue
			}
			lastPackages = nil
			fmt.Println("refreshed.")

		case "install":
			dev, err := ensureSelected(ctx, m, selected, r)
			if err != nil {
				fmt.Fprintln(os.Stderr, "error:", err)
				continue
			}
			selected = &dev
			apkPath := ""
			if len(args) >= 2 {
				apkPath = strings.Trim(args[1], "\"'")
			} else {
				fmt.Print("path to APK: ")
				raw, _ := r.ReadString('\n')
				apkPath = strings.Trim(strings.TrimSpace(raw), "\"'")
			}
			fmt.Printf("installing %s ...\n", apkPath)
			if _, err := m.InstallAPK(ctx, dev.Serial, apkPath); err != nil {
				fmt.Fprintln(os.Stderr, "error:", err)
				continue
			}
			fmt.Println("install succeeded.")
			lastPackages = nil

		case "launch":
			dev, err := ensureSelected(ctx, m, selected, r)
			if err != nil {
				fmt.Fprintln(os.Stderr, "error:", err)
				continue
			}
			selected = &dev
			pkg, err := pickPackageWithIndex(args[1:], lastPackages, r, false)
			if err != nil {
				fmt.Fprintln(os.Stderr, "error:", err)
				continue
			}
			fmt.Printf("launching %s ...\n", pkg)
			if err := m.LaunchApp(ctx, dev.Serial, pkg); err != nil {
				fmt.Fprintln(os.Stderr, "error:", err)
				continue
			}
			fmt.Println("launched.")

		case "stop", "force-stop":
			dev, err := ensureSelected(ctx, m, selected, r)
			if err != nil {
				fmt.Fprintln(os.Stderr, "error:", err)
				continue
			}
			selected = &dev
			pkg, err := pickPackageWithIndex(args[1:], lastPackages, r, false)
			if err != nil {
				fmt.Fprintln(os.Stderr, "error:", err)
				continue
			}
			fmt.Printf("force-stopping %s ...\n", pkg)
			if err := m.ForceStopApp(ctx, dev.Serial, pkg); err != nil {
				fmt.Fprintln(os.Stderr, "error:", err)
				continue
			}
			fmt.Println("stopped.")

		case "uninstall", "rm":
			dev, err := ensureSelected(ctx, m, selected, r)
			if err != nil {
				fmt.Fprintln(os.Stderr, "error:", err)
				continue
			}
			selected = &dev
			pkg, err := pickPackageWithIndex(args[1:], lastPackages, r, false)
			if err != nil {
				fmt.Fprintln(os.Stderr, "error:", err)
				continue
			}
			fmt.Printf("uninstall %s? type YES to confirm: ", pkg)
			confirm, _ := r.ReadString('\n')
			if strings.TrimSpace(strings.ToUpper(confirm)) != "YES" {
				fmt.Println("aborted.")
				continue
			}
			if err := m.UninstallApp(ctx, dev.Serial, pkg, false); err != nil {
				fmt.Fprintln(os.Stderr, "error:", err)
				continue
			}
			fmt.Println("uninstalled.")
			lastPackages = nil

		case "mirror", "scrcpy":
			dev, err := ensureSelected(ctx, m, selected, r)
			if err != nil {
				fmt.Fprintln(os.Stderr, "error:", err)
				continue
			}
			selected = &dev
			fmt.Println("launching scrcpy ...")
			cmd, err := m.Scrcpy.StartMirror(ctx, dev.Serial, "ADBPureFlow-Mirror")
			if err != nil {
				fmt.Fprintln(os.Stderr, "error:", err)
				continue
			}
			fmt.Printf("scrcpy started (pid %d).\n", cmd.Process.Pid)

		case "version", "-v", "--version":
			fmt.Println("ADBPureFlow CLI", version)

		default:
			fmt.Fprintf(os.Stderr, "unknown command %q (try `help`)\n", cmd)
		}
	}
}

// ---------------------------------------------------------------------------
// Non-interactive subcommands
// ---------------------------------------------------------------------------

func runCLI(ctx context.Context, args []string) int {
	m := mustInitManager()
	cmd := strings.ToLower(args[0])

	requireSerial := func(positional []string) (string, error) {
		devs, err := m.RefreshDevices(ctx)
		if err != nil {
			return "", err
		}
		var online []adb.Device
		for _, d := range devs {
			if d.State == adb.StateDevice {
				online = append(online, d)
			}
		}
		for _, p := range positional {
			for _, d := range online {
				if d.Serial == p {
					return d.Serial, nil
				}
			}
		}
		if len(online) == 1 {
			return online[0].Serial, nil
		}
		if len(online) == 0 {
			return "", fmt.Errorf("no online devices; connect one or specify a serial")
		}
		return "", fmt.Errorf("multiple devices online; specify a serial: %s",
			strings.Join(serials(online), ", "))
	}

	switch cmd {
	case "devices":
		devs, err := m.RefreshDevices(ctx)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return 1
		}
		for _, d := range devs {
			fmt.Printf("%s\t%s\t%s\n", d.Serial, d.State, d.DisplayName())
		}
		return 0

	case "version":
		fmt.Println(version)
		return 0

	case "apps":
		userOnly := false
		var positional []string
		for _, a := range args[1:] {
			if a == "-u" || a == "--user" {
				userOnly = true
			} else {
				positional = append(positional, a)
			}
		}
		serial, err := requireSerial(positional)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return 1
		}
		pkgs, err := m.ListPackages(ctx, serial, userOnly)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return 1
		}
		for _, p := range pkgs {
			fmt.Printf("%s\t%s\t%s\t%s\n", p.Kind, p.Name, formatVersion(p), p.DisplayTitle())
		}
		return 0

	case "info":
		if len(args) < 2 {
			usage()
			return 2
		}
		pkgName := args[1]
		serial, err := requireSerial(args[2:])
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return 1
		}
		info, err := m.PackageInfo(ctx, serial, pkgName)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return 1
		}
		printPackageInfo(info)
		return 0

	case "install":
		if len(args) < 2 {
			usage()
			return 2
		}
		serial, err := requireSerial(args[2:])
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return 1
		}
		if _, err := m.InstallAPK(ctx, serial, args[1]); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return 1
		}
		fmt.Println("install OK")
		return 0

	case "launch":
		if len(args) < 2 {
			usage()
			return 2
		}
		serial, err := requireSerial(args[2:])
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return 1
		}
		if err := m.LaunchApp(ctx, serial, args[1]); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return 1
		}
		return 0

	case "stop":
		if len(args) < 2 {
			usage()
			return 2
		}
		serial, err := requireSerial(args[2:])
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return 1
		}
		if err := m.ForceStopApp(ctx, serial, args[1]); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return 1
		}
		return 0

	case "uninstall":
		if len(args) < 2 {
			usage()
			return 2
		}
		serial, err := requireSerial(args[2:])
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return 1
		}
		if err := m.UninstallApp(ctx, serial, args[1], false); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return 1
		}
		fmt.Println("uninstalled", args[1])
		return 0

	case "mirror":
		serial, err := requireSerial(args[1:])
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return 1
		}
		cmd, err := m.Scrcpy.StartMirror(ctx, serial, "ADBPureFlow-Mirror")
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return 1
		}
		fmt.Printf("scrcpy started (pid %d)\n", cmd.Process.Pid)
		_ = cmd.Wait()
		return 0

	case "help", "-h", "--help":
		usage()
		return 0

	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", cmd)
		usage()
		return 2
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func mustInitManager() *adb.Manager {
	dataDir := ""
	if exe, err := os.Executable(); err == nil {
		dataDir = filepath.Dir(exe)
	}
	m, err := adb.NewManager(dataDir, true)
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to initialize ADB:", err)
		fmt.Fprintln(os.Stderr, "Install adb or allow this binary to download platform-tools.")
		os.Exit(1)
	}
	return m
}

func printBanner() {
	fmt.Println("=================================================================")
	fmt.Printf("               ADBPureFlow CLI %s                   \n", version)
	fmt.Println("         Automated Android APK Lifecycle Companion               ")
	fmt.Println("=================================================================")
}

func printHelp() {
	fmt.Println(`Commands:
  devices | ls                 list connected devices
  use <n|serial>               select a device for subsequent commands
  apps [-u] [query]            list installed packages (-u = user only)
  search <query>               search apps by name / package
  info [<pkg>|n]               show detailed info for an application
  refresh                      re-scan connected devices
  install <path>               install an APK onto the selected device
  launch [<pkg>|n]             launch an app (number = from last apps/search)
  stop [<pkg>|n]               force-stop an app
  uninstall [<pkg>|n]          uninstall an app (prompts for confirmation)
  mirror                       launch scrcpy screen mirror
  version                      print CLI version
  help                         show this help
  exit                         quit`)
}

func tokenize(line string) []string {
	var (
		out []string
		cur strings.Builder
		inQ bool
	)
	for _, r := range line {
		switch {
		case r == '"':
			inQ = !inQ
		case (r == ' ' || r == '\t') && !inQ:
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

func pickDevice(ctx context.Context, m *adb.Manager, args []string, r *bufio.Reader) (adb.Device, error) {
	devs, err := m.RefreshDevices(ctx)
	if err != nil {
		return adb.Device{}, err
	}
	var online []adb.Device
	for _, d := range devs {
		if d.State == adb.StateDevice {
			online = append(online, d)
		}
	}
	if len(online) == 0 {
		return adb.Device{}, fmt.Errorf("no online devices")
	}
	if len(args) >= 1 {
		s := strings.TrimSpace(args[0])
		if n, err := strconv.Atoi(s); err == nil && n >= 1 && n <= len(online) {
			return online[n-1], nil
		}
		for _, d := range online {
			if d.Serial == s {
				return d, nil
			}
		}
		return adb.Device{}, fmt.Errorf("no such device: %s", s)
	}
	if len(online) == 1 {
		return online[0], nil
	}
	for i, d := range online {
		fmt.Printf("  [%d] %s\n", i+1, d.DisplayName())
	}
	fmt.Print("select device (number): ")
	raw, _ := r.ReadString('\n')
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n < 1 || n > len(online) {
		return adb.Device{}, fmt.Errorf("invalid selection")
	}
	return online[n-1], nil
}

func ensureSelected(ctx context.Context, m *adb.Manager, cur *adb.Device, r *bufio.Reader) (adb.Device, error) {
	if cur != nil {
		devs, err := m.RefreshDevices(ctx)
		if err == nil {
			for _, d := range devs {
				if d.Serial == cur.Serial && d.State == adb.StateDevice {
					return d, nil
				}
			}
		}
	}
	return pickDevice(ctx, m, nil, r)
}

// pickPackageWithIndex resolves a package from CLI args, accepting either an
// explicit package name or a numeric index into the most recent listing
// (when allowIndex is true). When no argument is given and a reader is
// available, it prompts interactively.
func pickPackageWithIndex(args []string, lastPackages []adb.Package, r *bufio.Reader, allowIndex bool) (string, error) {
	if len(args) >= 1 {
		s := strings.TrimSpace(args[0])
		if s == "" {
			return "", fmt.Errorf("no package specified")
		}
		if allowIndex {
			if n, err := strconv.Atoi(s); err == nil && n >= 1 && n <= len(lastPackages) {
				return lastPackages[n-1].Name, nil
			}
		}
		return s, nil
	}
	if allowIndex && r != nil && len(lastPackages) > 0 {
		fmt.Println("  (from last listing)")
		for i, p := range lastPackages {
			fmt.Printf("    [%d] %s  %s\n", i+1, p.DisplayTitle(), p.Name)
		}
		fmt.Print("select app (number or package name): ")
		raw, _ := r.ReadString('\n')
		s := strings.TrimSpace(raw)
		if n, err := strconv.Atoi(s); err == nil && n >= 1 && n <= len(lastPackages) {
			return lastPackages[n-1].Name, nil
		}
		if s != "" {
			return s, nil
		}
		return "", fmt.Errorf("no package specified")
	}
	return "", fmt.Errorf("no package specified")
}

func printPackages(pkgs []adb.Package, filter string) {
	filter = strings.ToLower(filter)
	count := 0
	for _, p := range pkgs {
		label := p.DisplayTitle()
		line := fmt.Sprintf("%-16s %-16s %-40s %s", p.Kind, formatVersion(p), label, p.Name)
		if filter != "" && !strings.Contains(strings.ToLower(line), filter) {
			continue
		}
		fmt.Println(line)
		count++
	}
	if count == 0 {
		fmt.Println("(no packages match)")
	}
}

// printPackageInfo renders a structured human-readable block of metadata.
func printPackageInfo(p *adb.Package) {
	fmt.Println()
	fmt.Printf("  %s\n", p.DisplayTitle())
	fmt.Printf("  %s\n", p.Name)
	fmt.Println(strings.Repeat("-", 60))
	rows := []struct{ k, v string }{
		{"Type", p.Kind.String()},
		{"Version", dashIfEmpty(p.VersionSummary())},
		{"Enabled", yesNo(p.Enabled)},
		{"Installer", dashIfEmpty(p.Installer)},
		{"APK path", dashIfEmpty(p.Path)},
		{"UID", dashIfEmpty(i64toa(int64(p.UID)))},
		{"Target SDK", dashIfEmpty(i64toa(int64(p.TargetSdk)))},
		{"Min SDK", dashIfEmpty(i64toa(int64(p.MinSdk)))},
		{"First installed", adb.FormatMillis(p.FirstInstall)},
		{"Last updated", adb.FormatMillis(p.LastUpdate)},
	}
	for _, r := range rows {
		fmt.Printf("  %-16s %s\n", r.k+":", r.v)
	}
	if len(p.SplitCodePaths) > 0 {
		fmt.Printf("  %-16s\n", "Split APKs:")
		for _, s := range p.SplitCodePaths {
			fmt.Printf("                 %s\n", s)
		}
	}
	fmt.Println()
}

func formatVersion(p adb.Package) string {
	return p.VersionSummary()
}

func shortSerial(s string) string {
	if len(s) <= 12 {
		return s
	}
	return s[:8] + "…"
}

func serials(ds []adb.Device) []string {
	out := make([]string, 0, len(ds))
	for _, d := range ds {
		out = append(out, d.Serial)
	}
	return out
}

func dashIfEmpty(s string) string {
	if s == "" || s == "-1" {
		return "—"
	}
	return s
}

func yesNo(b bool) string {
	if b {
		return "Yes"
	}
	return "No"
}

func i64toa(v int64) string {
	if v <= 0 {
		return ""
	}
	return strconv.FormatInt(v, 10)
}
