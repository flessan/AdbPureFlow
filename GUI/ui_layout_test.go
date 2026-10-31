package main

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
)

func TestInsetLayoutAppliesPagePadding(t *testing.T) {
	object := canvas.NewRectangle(color.Black)
	layout := insetLayout{horizontal: 20, vertical: 12}

	layout.Layout([]fyne.CanvasObject{object}, fyne.NewSize(400, 300))

	if got, want := object.Position(), fyne.NewPos(20, 12); got != want {
		t.Fatalf("position = %v, want %v", got, want)
	}
	if got, want := object.Size(), fyne.NewSize(360, 276); got != want {
		t.Fatalf("size = %v, want %v", got, want)
	}
}

func TestInsetLayoutMinSizeIncludesPadding(t *testing.T) {
	object := canvas.NewRectangle(color.Black)
	layout := insetLayout{horizontal: 18, vertical: 10}

	got := layout.MinSize([]fyne.CanvasObject{object})
	if got.Width < 36 || got.Height < 20 {
		t.Fatalf("minimum size %v does not include the configured padding", got)
	}
}

func TestFixedWidthLayoutKeepsSidebarWidth(t *testing.T) {
	object := canvas.NewRectangle(color.Black)
	layout := fixedWidthLayout{width: 244}

	layout.Layout([]fyne.CanvasObject{object}, fyne.NewSize(900, 640))

	if got, want := object.Size(), fyne.NewSize(244, 640); got != want {
		t.Fatalf("size = %v, want %v", got, want)
	}
	if got := layout.MinSize([]fyne.CanvasObject{object}); got.Width != 244 {
		t.Fatalf("minimum width = %v, want 244", got.Width)
	}
}
