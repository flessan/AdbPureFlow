package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
)

// withPadding keeps spacing inside Fyne's standard layout system so minimum
// sizes and resize calculations remain consistent with the surrounding UI.
func withPadding(object fyne.CanvasObject, horizontal, vertical float32) fyne.CanvasObject {
	return container.New(
		layout.NewCustomPaddedLayout(vertical, vertical, horizontal, horizontal),
		object,
	)
}
