package main

import (
	"image/color"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
)

func TestWithPaddingUsesFyneLayoutSizing(t *testing.T) {
	object := canvas.NewRectangle(color.Black)
	padded := withPadding(object, 20, 12)

	padded.Resize(fyne.NewSize(400, 300))

	if got, want := object.Position(), fyne.NewPos(20, 12); got != want {
		t.Fatalf("position = %v, want %v", got, want)
	}
	if got, want := object.Size(), fyne.NewSize(360, 276); got != want {
		t.Fatalf("size = %v, want %v", got, want)
	}
}

func TestWithPaddingMinSizeIncludesInsets(t *testing.T) {
	object := canvas.NewRectangle(color.Black)
	padded := withPadding(object, 18, 10)

	got := padded.MinSize()
	if got.Width < 36 || got.Height < 20 {
		t.Fatalf("minimum size %v does not include the configured padding", got)
	}
}

func TestCompactDeviceLabelStaysBounded(t *testing.T) {
	device := Device{
		DisplayName: strings.Repeat("A very long Android model name ", 4),
		Serial:      strings.Repeat("SERIAL", 8),
	}

	got := compactDeviceLabel(device)
	if length := len([]rune(got)); length > 28 {
		t.Fatalf("compact device label has %d runes (%q), want at most 28", length, got)
	}
	if !strings.Contains(got, "…") {
		t.Fatalf("compact device label %q should indicate truncated values", got)
	}
}

func TestShortenTextBoundsStatusMessages(t *testing.T) {
	got := shortenText(strings.Repeat("status ", 20), 52)
	if length := len([]rune(got)); length > 52 {
		t.Fatalf("shortened status has %d runes, want at most 52", length)
	}
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("shortened status %q should end with an ellipsis", got)
	}
}
