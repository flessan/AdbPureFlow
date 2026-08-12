package adb

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// PackageKind indicates whether a package is user-installed, a system app,
// or updated from a system app.
type PackageKind int

const (
	KindUnknown PackageKind = iota
	KindUser
	KindSystem
	KindSystemUpdated
)

func (k PackageKind) String() string {
	switch k {
	case KindUser:
		return "user"
	case KindSystem:
		return "system"
	case KindSystemUpdated:
		return "system (updated)"
	default:
		return "unknown"
	}
}

// Package is a lightweight description of an installed Android package. All
// string fields are best-effort: some devices / adb versions don't surface
// versionName/versionCode/installer/labels for system packages, so callers
// should gracefully handle empty values.
type Package struct {
	Name        string      // package id, e.g. com.example.app
	Label       string      // human-readable name when resolvable; "" otherwise
	VersionName string      // e.g. 1.2.3
	VersionCode int64       // -1 when unknown
	Installer   string      // package that installed this (e.g. com.android.vending); "" when unknown
	Kind        PackageKind
	// Path is the on-device APK base path (codePath).
	Path string
	// SplitCodePaths lists split APK paths when present (e.g. for App Bundles).
	SplitCodePaths []string
	// FirstInstall / LastUpdate are epoch-millis when available; 0 when unknown.
	FirstInstall int64
	LastUpdate   int64
	// TargetSdk / MinSdk are parsed from dumpsys when available; -1 when unknown.
	TargetSdk int
	MinSdk    int
	// Enabled reflects whether the package is enabled for the default user.
	// True when unknown (we default up).
	Enabled bool
	// UID is the package's linux UID; -1 when unknown.
	UID int
}

// DisplayTitle returns a human-readable title preferring Label and falling
// back to Name.
func (p *Package) DisplayTitle() string {
	if p.Label != "" {
		return p.Label
	}
	return p.Name
}

// VersionSummary returns a compact "versionName (versionCode)" string when
// available, or an empty string if no version metadata is known.
func (p *Package) VersionSummary() string {
	switch {
	case p.VersionName != "" && p.VersionCode > 0:
		return fmt.Sprintf("%s (%d)", p.VersionName, p.VersionCode)
	case p.VersionName != "":
		return p.VersionName
	case p.VersionCode > 0:
		return fmt.Sprintf("(%d)", p.VersionCode)
	default:
		return ""
	}
}

// ListPackages retrieves the set of installed packages on the given device.
// When `userOnly` is true, only third-party (non-system) packages are
// returned. Best-effort label resolution is attempted so users see
// human-readable names.
func (c *Client) ListPackages(ctx context.Context, serial string, userOnly bool) ([]Package, error) {
	var out string
	var err error
	if userOnly {
		out, err = c.Command(ctx, serial, "shell", "pm", "list", "packages", "-3", "-U", "--show-versioncode")
	} else {
		out, err = c.Command(ctx, serial, "shell", "pm", "list", "packages", "-U", "--show-versioncode")
	}
	if err != nil {
		// Some older devices don't support --show-versioncode / -U; fall back
		// to plain `pm list packages`.
		if userOnly {
			out, err = c.Command(ctx, serial, "shell", "pm", "list", "packages", "-3")
		} else {
			out, err = c.Command(ctx, serial, "shell", "pm", "list", "packages")
		}
		if err != nil {
			return nil, fmt.Errorf("list packages: %w", err)
		}
	}

	pkgs := parsePackageListing(out)
	if len(pkgs) == 0 {
		return pkgs, nil
	}

	// Determine system vs user via a dedicated call.
	systemSet := map[string]bool{}
	if sysOut, err := c.Command(ctx, serial, "shell", "pm", "list", "packages", "-s"); err == nil {
		for _, line := range strings.Split(sysOut, "\n") {
			if name := parseOnePackageLine(line); name != "" {
				systemSet[name] = true
			}
		}
	}

	// Bulk dump of every package — much faster than one-per-package calls.
	enrichment := make(map[string]*Package, len(pkgs))
	for i := range pkgs {
		pkgs[i].Enabled = true
		enrichment[pkgs[i].Name] = &pkgs[i]
	}
	if dumpsys, err := c.Command(ctx, serial, "shell", "dumpsys", "package", "packages"); err == nil {
		enrichFromDumpsys(dumpsys, enrichment)
	}

	// Resolve human-readable labels. We try two strategies:
	//   1) Look for labels present directly in dumpsys (`applicationLabel=`
	//      values on older builds are sometimes pre-resolved strings).
	//   2) Fall back to a batched `cmd package resolve-activity --brief` loop
	//      running in a single adb shell session. This yields launcher-
	//      activity labels for apps that declare a MAIN/LAUNCHER activity
	//      (the common case for apps users actually see).
	c.resolveLabels(ctx, serial, enrichment)

	for i := range pkgs {
		p := &pkgs[i]
		switch {
		case p.Kind == KindSystemUpdated:
			// dumpsys wins.
		case systemSet[p.Name]:
			if p.Kind == KindUnknown {
				p.Kind = KindSystem
			}
		default:
			if p.Kind == KindUnknown {
				p.Kind = KindUser
			}
		}
	}

	// Sort: user packages first, then system, then by display title.
	sort.SliceStable(pkgs, func(i, j int) bool {
		if pkgs[i].Kind != pkgs[j].Kind {
			return pkgs[i].Kind < pkgs[j].Kind
		}
		return packageSortKey(&pkgs[i]) < packageSortKey(&pkgs[j])
	})
	return pkgs, nil
}

func packageSortKey(p *Package) string {
	return strings.ToLower(p.DisplayTitle())
}

// parsePackageListing parses `pm list packages [-U] [--show-versioncode]`
// output and returns a slice of Package with Name/VersionCode/UID set.
func parsePackageListing(out string) []Package {
	var pkgs []Package
	// Output examples:
	// package:com.example.app uid:10123 versionCode:42
	// package:com.example.app
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "package:") {
			continue
		}
		rest := strings.TrimPrefix(line, "package:")
		fields := strings.Fields(rest)
		if len(fields) == 0 {
			continue
		}
		pkg := Package{Name: fields[0], VersionCode: -1, UID: -1, Enabled: true, TargetSdk: -1, MinSdk: -1}
		for _, f := range fields[1:] {
			switch {
			case strings.HasPrefix(f, "versionCode:"):
				if n, err := strconv.ParseInt(strings.TrimPrefix(f, "versionCode:"), 10, 64); err == nil {
					pkg.VersionCode = n
				}
			case strings.HasPrefix(f, "uid:"):
				if n, err := strconv.Atoi(strings.TrimPrefix(f, "uid:")); err == nil {
					pkg.UID = n
				}
			}
		}
		pkgs = append(pkgs, pkg)
	}
	return pkgs
}

func parseOnePackageLine(line string) string {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "package:") {
		return ""
	}
	rest := strings.TrimPrefix(line, "package:")
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

// enrichFromDumpsys parses the `dumpsys package packages` output and fills
// metadata fields for packages present in enrichment. It is deliberately
// forgiving about minor format differences across Android versions 8–15.
func enrichFromDumpsys(out string, enrichment map[string]*Package) {
	var cur *Package
	// Track whether we're inside a sub-block we don't care about (like
	// "User 0:" installed lists).
	for _, raw := range strings.Split(out, "\n") {
		line := strings.TrimRight(raw, "\r")
		trim := strings.TrimSpace(line)

		if m := pkgBlockRe.FindStringSubmatch(trim); m != nil {
			name := m[1]
			cur = enrichment[name] // may be nil if we didn't list this pkg
			continue
		}
		if cur == nil {
			continue
		}

		if kv := dumpsysKV(trim); kv.key != "" {
			switch kv.key {
			case "versionName":
				cur.VersionName = unquoteAndTrim(kv.val)
			case "versionCode":
				// "42 minSdk=29 targetSdk=34" or "42 (34) minSdk=29 targetSdk=34"
				verPart, restVal := splitFirstSpace(kv.val)
				if n, err := strconv.ParseInt(verPart, 10, 64); err == nil {
					cur.VersionCode = n
				}
				// Parse minSdk=/targetSdk= from remainder.
				for _, token := range strings.Fields(restVal) {
					token = strings.Trim(token, "(),")
					if strings.HasPrefix(token, "minSdk=") {
						if n, err := strconv.Atoi(strings.TrimPrefix(token, "minSdk=")); err == nil {
							cur.MinSdk = n
						}
					} else if strings.HasPrefix(token, "targetSdk=") {
						if n, err := strconv.Atoi(strings.TrimPrefix(token, "targetSdk=")); err == nil {
							cur.TargetSdk = n
						}
					}
				}
			case "codePath":
				cur.Path = unquoteAndTrim(kv.val)
			case "splitCodePaths":
				// Sometimes form: "[split0.apk, split1.apk]"
				cur.SplitCodePaths = parseBracketedList(kv.val)
			case "installerPackageName":
				cur.Installer = unquoteAndTrim(kv.val)
			case "firstInstallTime":
				cur.FirstInstall = parseEpochMillis(kv.val)
			case "lastUpdateTime":
				cur.LastUpdate = parseEpochMillis(kv.val)
			case "pkgFlags", "privateFlags", "hiddenApiPolicy":
				flags := kv.val
				if strings.Contains(flags, "SYSTEM") && strings.Contains(flags, "UPDATED_SYSTEM_APP") {
					cur.Kind = KindSystemUpdated
				} else if strings.Contains(flags, "SYSTEM") && cur.Kind == KindUnknown {
					cur.Kind = KindSystem
				}
			case "applicationLabel":
				// On many Android builds this is a resource id (0x7f...),
				// but on some (especially older / OEM builds, or when dumpsys
				// resolved the label for us) it is a quoted string. Sniff it.
				v := strings.TrimSpace(kv.val)
				if s := unquoteAndTrim(v); s != "" && !looksLikeResourceID(s) {
					cur.Label = s
				}
			case "enabledComponents", "disabledComponents":
				// Not used yet; reserved.
			case "uid":
				// uid line appears inside the block as well (e.g. on older builds).
				if n, err := strconv.Atoi(strings.TrimSpace(kv.val)); err == nil {
					cur.UID = n
				}
			case "userid":
				if n, err := strconv.Atoi(strings.TrimSpace(kv.val)); err == nil && cur.UID == -1 {
					cur.UID = n
				}
			}
			continue
		}

		// "enabled=X" / "userId=1234" style lines appear without brackets.
		// Some builds print "User 0: installed=true hidden=false suspended=false ..."
		// within a package block — we use that to set Enabled=false when we
		// see installed=false or stopped-but-enabled variants.
		if strings.HasPrefix(trim, "User ") && strings.Contains(trim, "installed=") {
			// Look at the default user entry only. Format varies; we just
			// look for installed=false / enabled=false.
			if strings.Contains(trim, "installed=false") || strings.Contains(trim, "enabled=false") {
				if strings.HasPrefix(trim, "User 0:") || strings.Contains(trim, "installed=") {
					// Don't flip off for secondary profiles; just track
					// default user 0.
					if strings.HasPrefix(trim, "User 0:") {
						cur.Enabled = false
					}
				}
			}
		}
	}
}

// resolveLabels attempts to fill Label for packages where dumpsys did not
// provide a human-readable string. It uses a single batched `adb shell`
// invocation issuing `cmd package resolve-activity --brief -c LAUNCHER <pkg>`
// calls, then for any packages still missing a label it extracts the
// launcher activity's label via a secondary dumpsys lookup of
// "Activity Resolver Table" blocks.
//
// When everything fails, Label is left empty; callers fall back to Name.
func (c *Client) resolveLabels(ctx context.Context, serial string, enrichment map[string]*Package) {
	// Build list of packages that still need a label.
	var need []string
	for name, p := range enrichment {
		if p.Label == "" {
			need = append(need, name)
		}
	}
	if len(need) == 0 {
		return
	}

	// Batch resolve-activity calls. We use a shell "for" loop over arguments
	// passed on stdin via `shell -x -s` so all lookups happen in one adb
	// round-trip. cmd package resolve-activity --brief outputs:
	//   priority=0 preferredOrder=0 match=0x108000 specificIndex=-1 isDefault=true
	//   com.example/.MainActivity
	// which only gives us the component name. To get the label we add `-d`
	// to get a description table; instead we prefer querying the launcher
	// activity via `dumpsys package <component>` but that's expensive.
	// A simpler, widely-supported trick: `pm resolve-activity --brief`
	// doesn't print labels, so we fall back to parsing "Activity Resolver
	// Table" in our already-fetched dumpsys output — but we don't retain it.
	//
	// Instead, use this strategy:
	//   For each package without a label, shell out to `cmd package dump
	//   <pkg>` (or `dumpsys package <pkg>`) and pull any `android:labelRes=`
	//   reference AND the literal "application label=..." lines that some
	//   OEM builds emit. We batch these as a single shell command printing
	//   markers per package so we can associate the output cheaply.
	//
	// To avoid huge output (thousands of packages on real devices), we only
	// attempt label resolution for the *first* 300 packages (user apps come
	// first in our sort but this function runs before sorting, so we do our
	// best — a cap prevents pathological cases). Batch size is small.
	const cap = 300
	if len(need) > cap {
		need = need[:cap]
	}

	// Build a single shell script that prints separators and the dumpsys
	// snippet for each package. Use `dumpsys package <pkg>` per-package —
	// still expensive but bounded by cap, and avoids needing aapt.
	var sb strings.Builder
	sb.WriteString("for p in \"$@\"; do\n")
	sb.WriteString("  echo \"---ADBPURE_PKG:$p---\"\n")
	// `dumpsys package <pkg>` outputs the single-package block; we grep for
	// just the label-ish lines to keep traffic small. We use `toybox grep`
	// / `grep` where available; fall back to just piping the full output
	// through `sed -n` for key lines.
	sb.WriteString("  dumpsys package \"$p\" 2>/dev/null | ")
	sb.WriteString("grep -E -m 5 '(applicationLabel=|application-label|labelRes=|android:label=)' || true\n")
	sb.WriteString("done\n")
	script := sb.String()

	// Build args list: sh -c <script> -- <pkg1> <pkg2> ...
	// Use `shell sh -c <script> - <pkgs...>` to avoid argument quoting
	// pitfalls.
	args := []string{"shell", "sh", "-c", script, "-"}
	args = append(args, need...)

	out, err := c.Command(ctx, serial, args...)
	if err != nil {
		// Give up silently — labels are best-effort.
		return
	}
	parsePerPackageLabels(out, enrichment)

	// Second strategy for still-missing labels: attempt `cmd package
	// resolve-activity` batched to discover launcher components, then for
	// each component parse the label out of the global activity resolver
	// table from a second call. Since we don't keep the full dumpsys
	// output, we instead issue a tiny `dumpsys package <pkg>` looking
	// for "Application" label lines like "labelRes=0x7f..." — those are
	// still resource ids. The most portable adb-only trick that yields
	// real strings for some apps is checking the activity-resolver output
	// of `cmd package resolve-activity -c android.intent.category.LAUNCHER
	// <pkg>` which on newer Android (13+) prints a `label=` line.
	stillNeed := make([]string, 0, len(need))
	for _, n := range need {
		if p, ok := enrichment[n]; ok && p.Label == "" {
			stillNeed = append(stillNeed, n)
		}
	}
	if len(stillNeed) == 0 {
		return
	}
	var sb2 strings.Builder
	sb2.WriteString("for p in \"$@\"; do\n")
	sb2.WriteString("  echo \"---ADBPURE_PKG:$p---\"\n")
	sb2.WriteString("  cmd package resolve-activity -c android.intent.category.LAUNCHER \"$p\" 2>/dev/null | ")
	sb2.WriteString("grep -E -m 2 '(^label=|labelRes=|nonLocalizedLabel=)' || true\n")
	sb2.WriteString("done\n")
	args2 := []string{"shell", "sh", "-c", sb2.String(), "-"}
	args2 = append(args2, stillNeed...)
	out2, err := c.Command(ctx, serial, args2...)
	if err == nil {
		parsePerPackageLabels(out2, enrichment)
	}
}

// parsePerPackageLabels parses the output from the label-resolution shell
// script, which is delimited by `---ADBPURE_PKG:<name>---` markers.
func parsePerPackageLabels(out string, enrichment map[string]*Package) {
	var cur *Package
	for _, raw := range strings.Split(out, "\n") {
		line := strings.TrimRight(raw, "\r")
		if m := adbPkgMarkerRe.FindStringSubmatch(line); m != nil {
			cur = enrichment[m[1]]
			continue
		}
		if cur == nil || cur.Label != "" {
			continue
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Match lines like:
		//   applicationLabel=Settings
		//   applicationLabel="My App"
		//   label=My App
		//   nonLocalizedLabel=My App
		//   application-label:'My App'
		for _, re := range labelRes {
			if m := re.FindStringSubmatch(line); m != nil {
				v := unquoteAndTrim(m[1])
				if v != "" && !looksLikeResourceID(v) && !looksNumeric(v) {
					cur.Label = v
					break
				}
			}
		}
	}
}

var (
	pkgBlockRe      = regexp.MustCompile(`^Package \[([^\]]+)\]`)
	adbPkgMarkerRe  = regexp.MustCompile(`^---ADBPURE_PKG:(.+?)---\s*$`)
	hexResourceRe   = regexp.MustCompile(`^0x[0-9a-fA-F]+$`)
	bracketedListRe = regexp.MustCompile(`^\[(.*)\]$`)
)

// labelRes contains regex patterns for extracting label strings. Each must
// have exactly one capture group producing the candidate value. Leading
// and trailing quotes are stripped later by unquoteAndTrim, so the regexes
// are intentionally permissive.
var labelRes = []*regexp.Regexp{
	regexp.MustCompile(`^\s*applicationLabel=(.+)$`),
	regexp.MustCompile(`^\s*nonLocalizedLabel=(.+)$`),
	regexp.MustCompile(`^\s*label=(.+)$`),
	regexp.MustCompile(`^\s*application-label:\s*(.+?)\s*$`),
	regexp.MustCompile(`^\s*android:label=(.+)$`),
}

// kv holds a parsed key=value pair.
type kvPair struct {
	key string
	val string
}

// dumpsysKV parses a "key=value" style line from dumpsys.
func dumpsysKV(line string) kvPair {
	i := strings.IndexByte(line, '=')
	if i <= 0 {
		return kvPair{}
	}
	key := strings.TrimSpace(line[:i])
	val := strings.TrimSpace(line[i+1:])
	return kvPair{key: key, val: val}
}

// parseEpochMillis tries to convert adb's "2024-01-02 15:04:05" timestamps
// to milliseconds since epoch. ADB timestamps are in the device's local
// time, which we can't recover precisely; we parse as wall-clock UTC-ish.
func parseEpochMillis(s string) int64 {
	s = strings.TrimSpace(s)
	for _, layout := range []string{
		"2006-01-02 15:04:05",
		"2006-01-02 15:04:05.000",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UnixMilli()
		}
	}
	return 0
}

// FormatMillis formats an epoch-millis timestamp for display, or returns
// placeholder when zero.
func FormatMillis(ms int64) string {
	if ms <= 0 {
		return "—"
	}
	return time.UnixMilli(ms).Local().Format("2006-01-02 15:04:05")
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func splitFirstSpace(s string) (string, string) {
	s = strings.TrimSpace(s)
	i := strings.IndexByte(s, ' ')
	if i < 0 {
		return s, ""
	}
	return strings.TrimSpace(s[:i]), strings.TrimSpace(s[i+1:])
}

func unquoteAndTrim(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			s = s[1 : len(s)-1]
		}
	}
	return strings.TrimSpace(s)
}

func looksLikeResourceID(s string) bool {
	if len(s) == 0 || len(s) > 32 {
		return false
	}
	if s[0] == '@' {
		// e.g. @0x7f010001
		return hexResourceRe.MatchString(s[1:])
	}
	return hexResourceRe.MatchString(s)
}

func looksNumeric(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	for _, r := range s {
		if !((r >= '0' && r <= '9') || r == '-' || r == '.' || r == 'x' || r == 'X' || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return false
		}
	}
	// Treat pure integers/hex as "not a label" even if they parse.
	_, err1 := strconv.ParseInt(s, 0, 64)
	_, err2 := strconv.ParseFloat(s, 64)
	return err1 == nil || err2 == nil
}

func parseBracketedList(s string) []string {
	s = strings.TrimSpace(s)
	m := bracketedListRe.FindStringSubmatch(s)
	if m == nil {
		if s == "" {
			return nil
		}
		return []string{unquoteAndTrim(s)}
	}
	inner := strings.TrimSpace(m[1])
	if inner == "" {
		return nil
	}
	parts := strings.Split(inner, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, unquoteAndTrim(p))
	}
	return out
}

// ---------------------------------------------------------------------------
// App-level operations
// ---------------------------------------------------------------------------

// Install installs an APK located at localPath to the device.
func (c *Client) Install(ctx context.Context, serial, localPath string) error {
	if serial == "" {
		return errors.New("adb: install requires a device serial")
	}
	abs, err := filepath.Abs(localPath)
	if err != nil {
		return fmt.Errorf("install: %w", err)
	}
	if _, err := os.Stat(abs); err != nil {
		return fmt.Errorf("install: %w", err)
	}
	out, err := c.Command(ctx, serial, "install", "-r", "-d", "--streaming", abs)
	if err != nil {
		// `--streaming` was added in newer platform-tools; fall back without it.
		out, err = c.Command(ctx, serial, "install", "-r", "-d", abs)
	}
	if err != nil {
		return fmt.Errorf("install: %w", err)
	}
	if !strings.Contains(out, "Success") {
		return fmt.Errorf("install: adb returned %q", out)
	}
	return nil
}

// Uninstall removes a package from the device. When `keepData` is true,
// passes `-k` to keep the app's data/cache.
func (c *Client) Uninstall(ctx context.Context, serial, pkg string, keepData bool) error {
	if serial == "" {
		return errors.New("adb: uninstall requires a device serial")
	}
	if pkg == "" {
		return errors.New("adb: uninstall: empty package name")
	}
	args := []string{"uninstall"}
	if keepData {
		args = append(args, "-k")
	}
	args = append(args, pkg)
	out, err := c.Command(ctx, serial, args...)
	if err != nil {
		return fmt.Errorf("uninstall %s: %w", pkg, err)
	}
	if !strings.Contains(out, "Success") {
		return fmt.Errorf("uninstall %s: %s", pkg, out)
	}
	return nil
}

// Launch launches the default launcher activity for the given package.
func (c *Client) Launch(ctx context.Context, serial, pkg string) error {
	if serial == "" {
		return errors.New("adb: launch requires a device serial")
	}
	if pkg == "" {
		return errors.New("adb: launch: empty package name")
	}
	out, err := c.Command(ctx, serial, "shell", "monkey", "-p", pkg, "-c", "android.intent.category.LAUNCHER", "1")
	if err != nil {
		return fmt.Errorf("launch %s: %w", pkg, err)
	}
	lower := strings.ToLower(out)
	if strings.Contains(lower, "no activities found") || strings.Contains(lower, "aborted") {
		return fmt.Errorf("launch %s: %s", pkg, strings.TrimSpace(out))
	}
	return nil
}

// ForceStop sends `am force-stop <pkg>`.
func (c *Client) ForceStop(ctx context.Context, serial, pkg string) error {
	if serial == "" {
		return errors.New("adb: force-stop requires a device serial")
	}
	if pkg == "" {
		return errors.New("adb: force-stop: empty package name")
	}
	_, err := c.Command(ctx, serial, "shell", "am", "force-stop", pkg)
	if err != nil {
		return fmt.Errorf("force-stop %s: %w", pkg, err)
	}
	return nil
}

// IsInstalled reports whether `pm list packages <pkg>` reports the package.
func (c *Client) IsInstalled(ctx context.Context, serial, pkg string) (bool, error) {
	out, err := c.Command(ctx, serial, "shell", "pm", "list", "packages", pkg)
	if err != nil {
		return false, err
	}
	return strings.Contains(out, "package:"+pkg), nil
}

// PackageExists is an alias for IsInstalled.
func (c *Client) PackageExists(ctx context.Context, serial, pkg string) (bool, error) {
	return c.IsInstalled(ctx, serial, pkg)
}
