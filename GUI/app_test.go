package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParsePackages(t *testing.T) {
	output := `package:com.android.settings
package:com.google.android.youtube
package:com.thio.adbpureflow
`
	res := parsePackages(output)
	if len(res) != 3 {
		t.Errorf("Expected 3 packages, got %d", len(res))
	}
	if !res["com.android.settings"] {
		t.Errorf("Expected com.android.settings to be found")
	}
	if !res["com.thio.adbpureflow"] {
		t.Errorf("Expected com.thio.adbpureflow to be found")
	}
}

func TestParsePackagesEmpty(t *testing.T) {
	output := ""
	res := parsePackages(output)
	if len(res) != 0 {
		t.Errorf("Expected 0 packages, got %d", len(res))
	}
}

func TestFindScrcpyFolder(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "scrcpy_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create simulated scrcpy subdirectory
	scrcpySubDir := filepath.Join(tempDir, "scrcpy-win64-v3.3.4")
	if err := os.MkdirAll(scrcpySubDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create simulated scrcpy binary inside
	execName := "scrcpy"
	if os.PathSeparator == '\\' {
		// Windows
		execName = "scrcpy.exe"
	}
	
	simulatedBinary := filepath.Join(scrcpySubDir, execName)
	if err := os.WriteFile(simulatedBinary, []byte("dummy binary contents"), 0755); err != nil {
		t.Fatal(err)
	}

	foundDir := findScrcpyFolder(tempDir)
	if foundDir != scrcpySubDir {
		t.Errorf("Expected to find %s, got %s", scrcpySubDir, foundDir)
	}
}
