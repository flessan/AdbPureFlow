package adb

import (
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestParseDevicesList(t *testing.T) {
	out := `List of devices attached
emulator-5554          device product:sdk_gphone64_arm64 model:sdk_gphone64_arm64 device:emu64a transport_id:14
ABCDEF123456           unauthorized usb:1-2 transport_id:11
OFFLINE001             offline usb:1-3

`
	devs := parseDevicesList(out)
	if len(devs) != 3 {
		t.Fatalf("want 3 devices, got %d: %+v", len(devs), devs)
	}
	want := []Device{
		{Serial: "emulator-5554", State: StateDevice, Product: "sdk_gphone64_arm64", Model: "sdk_gphone64_arm64", Device: "emu64a", TransportID: "14"},
		{Serial: "ABCDEF123456", State: StateUnauthorized, Device: "", TransportID: "11"},
		{Serial: "OFFLINE001", State: StateOffline, Device: "", TransportID: ""},
	}
	for i := range want {
		if devs[i].Serial != want[i].Serial {
			t.Errorf("dev[%d] serial = %q, want %q", i, devs[i].Serial, want[i].Serial)
		}
		if devs[i].State != want[i].State {
			t.Errorf("dev[%d] state = %q, want %q", i, devs[i].State, want[i].State)
		}
		if want[i].Model != "" && devs[i].Model != want[i].Model {
			t.Errorf("dev[%d] model = %q, want %q", i, devs[i].Model, want[i].Model)
		}
	}
	if name := devs[0].DisplayName(); name != "sdk gphone64 arm64 (emulator-5554)" {
		t.Errorf("DisplayName() = %q, want underscores replaced with spaces", name)
	}
}

func TestParseDevicesListEmpty(t *testing.T) {
	got := parseDevicesList("List of devices attached\n\n")
	if len(got) != 0 {
		t.Errorf("want 0 devices, got %d", len(got))
	}
}

func TestParsePackagesListing(t *testing.T) {
	out := `package:com.android.settings uid:1000
package:com.example.app uid:10123 versionCode:42
package:com.google.android.youtube uid:10086 versionCode:123
`
	pkgs := parsePackageListing(out)
	if len(pkgs) != 3 {
		t.Fatalf("want 3, got %d", len(pkgs))
	}
	byName := map[string]Package{}
	for _, p := range pkgs {
		byName[p.Name] = p
	}
	if p, ok := byName["com.example.app"]; !ok || p.VersionCode != 42 {
		t.Errorf("com.example.app versionCode got %+v, want 42", p)
	}
	if p, ok := byName["com.android.settings"]; !ok || p.VersionCode != -1 {
		t.Errorf("com.android.settings versionCode default = %d, want -1", p.VersionCode)
	}
	if p, ok := byName["com.example.app"]; !ok || p.UID != 10123 {
		t.Errorf("com.example.app UID got %+v, want 10123", p)
	}
}

func TestEnrichFromDumpsys(t *testing.T) {
	dump := `
Packages:
  Package [com.example.app] (1234):
    userId=10123
    pkg=Package{... com.example.app}
    codePath=/data/app/~~xxx/com.example.app-abc==
    splitCodePaths=[/data/app/~~xxx/com.example.app-abc==/split_config.arm64_v8a.apk, /data/app/~~xxx/com.example.app-abc==/split_config.en.apk]
    versionName=1.2.3
    versionCode=42 minSdk=29 targetSdk=34
    firstInstallTime=2024-03-10 12:34:56
    lastUpdateTime=2024-04-01 09:00:00
    installerPackageName=com.android.vending
    applicationLabel="Example App"
    pkgFlags=[ SYSTEM HAS_CODE ALLOW_CLEAR_USER_DATA UPDATED_SYSTEM_APP ]
    User 0: installed=true hidden=false suspended=false stopped=true notLaunched=false enabled=0 instant=false virtual=false
  Package [com.example.user] (5678):
    userId=10456
    codePath=/data/app/~~yyy/com.example.user-xyz==
    versionName=2.0
    versionCode=10
    applicationLabel=0x7f010001
    pkgFlags=[ HAS_CODE ]
    User 0: installed=true hidden=false suspended=false stopped=false notLaunched=false enabled=0 instant=false virtual=false
  Package [com.example.disabled] (9999):
    userId=12000
    codePath=/system/priv-app/Disabled
    versionName=1.0
    versionCode=1
    pkgFlags=[ SYSTEM ]
    User 0: installed=false hidden=false suspended=false stopped=true notLaunched=false enabled=3 instant=false virtual=false
`
	enrichment := map[string]*Package{
		"com.example.app":      {Name: "com.example.app", VersionCode: -1, UID: -1, Enabled: true, MinSdk: -1, TargetSdk: -1},
		"com.example.user":     {Name: "com.example.user", VersionCode: -1, UID: -1, Enabled: true, MinSdk: -1, TargetSdk: -1},
		"com.example.disabled": {Name: "com.example.disabled", VersionCode: -1, UID: -1, Enabled: true, MinSdk: -1, TargetSdk: -1},
	}
	enrichFromDumpsys(dump, enrichment)

	p := enrichment["com.example.app"]
	if p.VersionName != "1.2.3" {
		t.Errorf("com.example.app versionName = %q, want 1.2.3", p.VersionName)
	}
	if p.VersionCode != 42 {
		t.Errorf("com.example.app versionCode = %d, want 42", p.VersionCode)
	}
	if p.Installer != "com.android.vending" {
		t.Errorf("com.example.app installer = %q", p.Installer)
	}
	if p.Kind != KindSystemUpdated {
		t.Errorf("com.example.app kind = %v, want system (updated)", p.Kind)
	}
	if p.Label != "Example App" {
		t.Errorf("com.example.app label = %q, want Example App", p.Label)
	}
	if p.MinSdk != 29 || p.TargetSdk != 34 {
		t.Errorf("com.example.app min/target = %d/%d, want 29/34", p.MinSdk, p.TargetSdk)
	}
	if len(p.SplitCodePaths) != 2 {
		t.Errorf("com.example.app splits = %d (%v), want 2", len(p.SplitCodePaths), p.SplitCodePaths)
	}
	if p.UID != 10123 {
		t.Errorf("com.example.app UID = %d, want 10123", p.UID)
	}
	u := enrichment["com.example.user"]
	if u.Kind != KindUnknown {
		t.Errorf("com.example.user kind = %v, want unknown", u.Kind)
	}
	if u.VersionCode != 10 {
		t.Errorf("com.example.user versionCode = %d, want 10", u.VersionCode)
	}
	// Resource ID must NOT be used as a label.
	if u.Label != "" {
		t.Errorf("com.example.user label should be empty for resource id, got %q", u.Label)
	}
	d := enrichment["com.example.disabled"]
	if d.Enabled {
		t.Errorf("com.example.disabled Enabled should be false")
	}
	if p.FirstInstall == 0 {
		t.Errorf("com.example.app firstInstall was 0, expected non-zero")
	}
}

func TestPackageSorting(t *testing.T) {
	pkgs := []Package{
		{Name: "z.example", Kind: KindUser, Label: "Zeta"},
		{Name: "a.example", Kind: KindSystem, Label: "Alpha"},
		{Name: "m.example", Kind: KindUser, Label: "Mu"},
	}
	sort.SliceStable(pkgs, func(i, j int) bool {
		if pkgs[i].Kind != pkgs[j].Kind {
			return pkgs[i].Kind < pkgs[j].Kind
		}
		return packageSortKey(&pkgs[i]) < packageSortKey(&pkgs[j])
	})
	want := []string{"m.example", "z.example", "a.example"}
	got := []string{pkgs[0].Name, pkgs[1].Name, pkgs[2].Name}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("sorted order = %v, want %v", got, want)
	}
}

func TestPackageSortKeyFallsBackToName(t *testing.T) {
	p := &Package{Name: "com.example.no_label"}
	if k := packageSortKey(p); k != "com.example.no_label" {
		t.Errorf("sort key = %q, want package name", k)
	}
}

func TestParseOnePackageLine(t *testing.T) {
	if got := parseOnePackageLine("package:com.example.app"); got != "com.example.app" {
		t.Errorf("got %q", got)
	}
	if got := parseOnePackageLine("  junk line  "); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestDumpsysKV(t *testing.T) {
	kv := dumpsysKV("    versionName=1.2.3")
	if kv.key != "versionName" || kv.val != "1.2.3" {
		t.Errorf("unexpected kv: %+v", kv)
	}
	kv = dumpsysKV("    pkgFlags=[ SYSTEM HAS_CODE ]")
	if kv.key != "pkgFlags" || !strings.Contains(kv.val, "SYSTEM") {
		t.Errorf("unexpected flag kv: %+v", kv)
	}
}

func TestParsePerPackageLabels(t *testing.T) {
	out := `---ADBPURE_PKG:com.example.one---
applicationLabel=One App
---ADBPURE_PKG:com.example.two---
applicationLabel=0x7f010002
label=Two App
---ADBPURE_PKG:com.example.three---
application-label:'Three App'
---ADBPURE_PKG:com.example.four---
nonLocalizedLabel=Four the App
---ADBPURE_PKG:com.example.five---
applicationLabel=12345
someOtherLine=true
---ADBPURE_PKG:com.example.six---
(no label lines here)
`
	enrich := map[string]*Package{
		"com.example.one":   {Name: "com.example.one"},
		"com.example.two":   {Name: "com.example.two"},
		"com.example.three": {Name: "com.example.three"},
		"com.example.four":  {Name: "com.example.four"},
		"com.example.five":  {Name: "com.example.five"},
		"com.example.six":   {Name: "com.example.six"},
	}
	parsePerPackageLabels(out, enrich)
	cases := map[string]string{
		"com.example.one":   "One App",
		"com.example.two":   "Two App", // resource id skipped, next wins
		"com.example.three": "Three App",
		"com.example.four":  "Four the App",
		"com.example.five":  "", // 12345 is numeric -> skipped
		"com.example.six":   "",
	}
	for pkg, want := range cases {
		if got := enrich[pkg].Label; got != want {
			t.Errorf("%s label = %q, want %q", pkg, got, want)
		}
	}
}

func TestLooksLikeResourceID(t *testing.T) {
	cases := map[string]bool{
		"0x7f010001":   true,
		"@0x7f010001":  true,
		"Settings":     false,
		"":             false,
		"0xdeadbeef":   true,
		"0x":           false,
		"Hello World":  false,
	}
	for in, want := range cases {
		if got := looksLikeResourceID(in); got != want {
			t.Errorf("looksLikeResourceID(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestLooksNumeric(t *testing.T) {
	cases := map[string]bool{
		"12345":    true,
		"0x1a2b":   true,
		"1.5":      true,
		"-42":      true,
		"Settings": false,
		"":         false,
	}
	for in, want := range cases {
		if got := looksNumeric(in); got != want {
			t.Errorf("looksNumeric(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestParseBracketedList(t *testing.T) {
	got := parseBracketedList("[/a/b.apk, /c/d.apk]")
	want := []string{"/a/b.apk", "/c/d.apk"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseBracketedList = %v, want %v", got, want)
	}
	got = parseBracketedList("[]")
	if len(got) != 0 {
		t.Errorf("empty brackets should yield empty slice, got %v", got)
	}
	got = parseBracketedList("singleton")
	if !reflect.DeepEqual(got, []string{"singleton"}) {
		t.Errorf("non-bracketed should wrap as single elem, got %v", got)
	}
}

func TestVersionSummary(t *testing.T) {
	p := Package{VersionName: "1.2.3", VersionCode: 42}
	if got := p.VersionSummary(); got != "1.2.3 (42)" {
		t.Errorf("VersionSummary() = %q", got)
	}
	p = Package{VersionCode: 7}
	if got := p.VersionSummary(); got != "(7)" {
		t.Errorf("VersionSummary() = %q", got)
	}
	p = Package{VersionName: "2.0"}
	if got := p.VersionSummary(); got != "2.0" {
		t.Errorf("VersionSummary() = %q", got)
	}
	p = Package{}
	if got := p.VersionSummary(); got != "" {
		t.Errorf("VersionSummary() = %q, want empty", got)
	}
}

func TestDisplayTitle(t *testing.T) {
	p := Package{Name: "com.example.app", Label: "Example"}
	if p.DisplayTitle() != "Example" {
		t.Errorf("expected label, got %q", p.DisplayTitle())
	}
	p = Package{Name: "com.example.app"}
	if p.DisplayTitle() != "com.example.app" {
		t.Errorf("expected name fallback, got %q", p.DisplayTitle())
	}
}

func TestFormatMillis(t *testing.T) {
	if got := FormatMillis(0); got != "—" {
		t.Errorf("zero = %q", got)
	}
	if got := FormatMillis(1704067200000); !strings.Contains(got, "2024") {
		t.Errorf("expected 2024 date, got %q", got)
	}
}
