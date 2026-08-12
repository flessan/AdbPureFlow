package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/flessan/AdbPureFlow/internal/adb"
)

// Core ADB behavior (device parsing, package parsing, etc.) lives in
// internal/adb and is tested there. These tests cover CLI-specific helpers
// that don't require a real adb binary or Android device.

func TestTokenize(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"launch com.example.app", []string{"launch", "com.example.app"}},
		{`install "C:\My Apps\foo.apk"`, []string{"install", `C:\My Apps\foo.apk`}},
		{"", nil},
		{"   apps   -u   ", []string{"apps", "-u"}},
	}
	for _, c := range cases {
		got := tokenize(c.in)
		if len(got) != len(c.want) {
			t.Errorf("tokenize(%q) = %v, want %v", c.in, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("tokenize(%q)[%d] = %q, want %q", c.in, i, got[i], c.want[i])
			}
		}
	}
}

func TestShortSerial(t *testing.T) {
	if s := shortSerial("ABC123"); s != "ABC123" {
		t.Errorf("shortSerial short = %q", s)
	}
	got := shortSerial("ABCDEFGHIJKLMNOP")
	if !strings.HasPrefix(got, "ABCDEFGH") || !strings.Contains(got, "…") {
		t.Errorf("shortSerial long = %q", got)
	}
}

// formatVersion is a small presenter in CLI/main.go. It must not panic and
// should produce a stable string for both empty and populated inputs.
func TestFormatVersion(t *testing.T) {
	cases := []struct {
		p    adb.Package
		want string
	}{
		{adb.Package{}, ""},
		{adb.Package{VersionCode: 12}, "(12)"},
		{adb.Package{VersionName: "1.2.3", VersionCode: 42}, "1.2.3 (42)"},
	}
	for _, c := range cases {
		got := formatVersion(c.p)
		if got != c.want {
			t.Errorf("formatVersion(%+v) = %q, want %q", c.p, got, c.want)
		}
	}
}

func TestDashIfEmpty(t *testing.T) {
	cases := map[string]string{
		"":    "—",
		"-1":  "—",
		"0":   "0",
		"foo": "foo",
	}
	for in, want := range cases {
		if got := dashIfEmpty(in); got != want {
			t.Errorf("dashIfEmpty(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestYesNo(t *testing.T) {
	if yesNo(true) != "Yes" {
		t.Errorf("yesNo(true) = %q", yesNo(true))
	}
	if yesNo(false) != "No" {
		t.Errorf("yesNo(false) = %q", yesNo(false))
	}
}

func TestI64toa(t *testing.T) {
	if i64toa(0) != "" {
		t.Errorf("i64toa(0) = %q", i64toa(0))
	}
	if i64toa(-5) != "" {
		t.Errorf("i64toa(-5) = %q", i64toa(-5))
	}
	if i64toa(42) != "42" {
		t.Errorf("i64toa(42) = %q", i64toa(42))
	}
}

// captureStdout temporarily redirects os.Stdout while calling fn and
// returns the captured output as a string.
func captureStdout(fn func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	fn()
	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

func TestPrintPackageInfo(t *testing.T) {
	p := &adb.Package{
		Name:         "com.example.app",
		Label:        "Example",
		VersionName:  "1.2.3",
		VersionCode:  42,
		Installer:    "com.android.vending",
		Kind:         adb.KindUser,
		Path:         "/data/app/~~x/com.example.app-y==",
		FirstInstall: 1704067200000, // 2024-01-01 UTC
		LastUpdate:   1704067200000,
		UID:          10123,
		MinSdk:       24,
		TargetSdk:    34,
		Enabled:      true,
	}
	out := captureStdout(func() { printPackageInfo(p) })
	for _, want := range []string{
		"Example",
		"com.example.app",
		"1.2.3 (42)",
		"com.android.vending",
		"/data/app/",
		"10123",
		"34",
		"24",
		"user",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("printPackageInfo output missing %q; got:\n%s", want, out)
		}
	}

	// Empty label falls back to package name in the title.
	p2 := &adb.Package{Name: "com.example.nolabel"}
	out2 := captureStdout(func() { printPackageInfo(p2) })
	if !strings.Contains(out2, "com.example.nolabel") {
		t.Errorf("fallback title should include package name, got:\n%s", out2)
	}
	// Missing metadata is rendered as "—".
	if !strings.Contains(out2, "—") {
		t.Errorf("expected em-dash for missing fields, got:\n%s", out2)
	}
}

// TestUsageDoesNotPanic ensures the usage function renders without
// erroring when called directly.
func TestUsageDoesNotPanic(t *testing.T) {
	captureStdout(func() { usage() })
}
