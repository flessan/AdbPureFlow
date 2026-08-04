package main

import (
	"testing"
)

func TestFindNewPackage(t *testing.T) {
	before := map[string]bool{
		"com.android.settings": true,
		"com.google.android":   true,
	}

	after := map[string]bool{
		"com.android.settings": true,
		"com.google.android":   true,
		"com.example.newapp":   true,
	}

	pkg := findNewPackage(before, after)
	if pkg != "com.example.newapp" {
		t.Errorf("Expected com.example.newapp, got %s", pkg)
	}
}

func TestFindNewPackageNoChange(t *testing.T) {
	before := map[string]bool{
		"com.android.settings": true,
	}
	after := map[string]bool{
		"com.android.settings": true,
	}

	pkg := findNewPackage(before, after)
	if pkg != "" {
		t.Errorf("Expected empty package, got %s", pkg)
	}
}
