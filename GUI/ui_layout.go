package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
)

// insetLayout adds predictable page and shell spacing without changing the
// sizing behavior of the wrapped Fyne widget or container.
type insetLayout struct {
	horizontal float32
	vertical   float32
}

func (l insetLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	if len(objects) == 0 || objects[0] == nil {
		return
	}

	width := size.Width - 2*l.horizontal
	height := size.Height - 2*l.vertical
	if width < 0 {
		width = 0
	}
	if height < 0 {
		height = 0
	}

	objects[0].Move(fyne.NewPos(l.horizontal, l.vertical))
	objects[0].Resize(fyne.NewSize(width, height))
}

func (l insetLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	if len(objects) == 0 || objects[0] == nil {
		return fyne.NewSize(2*l.horizontal, 2*l.vertical)
	}

	minimum := objects[0].MinSize()
	return fyne.NewSize(
		minimum.Width+2*l.horizontal,
		minimum.Height+2*l.vertical,
	)
}

func withPadding(object fyne.CanvasObject, horizontal, vertical float32) fyne.CanvasObject {
	return container.New(insetLayout{horizontal: horizontal, vertical: vertical}, object)
}


// fixedWidthLayout reserves a stable width for a child while allowing it to
// fill the available height. This keeps a scrollable sidebar from collapsing
// to the scroll widget's small default minimum width.
type fixedWidthLayout struct {
	width float32
}

func (l fixedWidthLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	if len(objects) == 0 || objects[0] == nil {
		return
	}

	objects[0].Move(fyne.NewPos(0, 0))
	objects[0].Resize(fyne.NewSize(l.width, size.Height))
}

func (l fixedWidthLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	if len(objects) == 0 || objects[0] == nil {
		return fyne.NewSize(l.width, 0)
	}

	minimum := objects[0].MinSize()
	return fyne.NewSize(l.width, minimum.Height)
}
